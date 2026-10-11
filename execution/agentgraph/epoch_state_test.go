package agentgraph

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/knowledge"
	execution "codeburg.org/lexbit/relurpify/execution"
	fwtelemetry "codeburg.org/lexbit/relurpify/telemetry"
)

// telSnapshot copies the recorded events of an *epochTelemetry.
func telSnapshot(e *epochTelemetry) []fwtelemetry.Event {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]fwtelemetry.Event(nil), e.events...)
}

// blockingGrounder holds Ground open until released, so a test can enqueue
// captures while a flush is in flight.
type blockingGrounder struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (g *blockingGrounder) Ground(_ context.Context, items []knowledge.GroundingItem) (knowledge.GroundingReport, error) {
	g.once.Do(func() { close(g.entered) })
	<-g.release
	report := knowledge.GroundingReport{Grounded: make([]knowledge.GroundingEntry, len(items))}
	for i := range items {
		report.Grounded[i] = knowledge.GroundingEntry{Index: i, Action: knowledge.GroundingGrounded}
	}
	return report, nil
}

// TestEnqueueDuringFlushStampsNextEpoch is the inFlush table (D2): an item
// enqueued with no flush in flight stamps the opening epoch; an item
// enqueued while a flush is in flight stamps the FOLLOWING epoch — it belongs
// to the next barrier, never the one mid-flight.
func TestEnqueueDuringFlushStampsNextEpoch(t *testing.T) {
	t.Run("no_flush_in_flight", func(t *testing.T) {
		coord := NewEpochCoordinator(context.Background(), &recordingGrounder{}, nil)
		coord.EnqueueCapture(captureItem("a"))
		require.Equal(t, uint64(1), coord.pendingItemEpoch(t), "idle enqueue stamps the opening epoch")
	})

	t.Run("flush_in_flight", func(t *testing.T) {
		env := contextdata.NewEnvelope("task-1", "session-1")
		grounder := &blockingGrounder{entered: make(chan struct{}), release: make(chan struct{})}
		coord := NewEpochCoordinator(contextdata.WithEnvelope(context.Background(), env), grounder, nil)
		coord.EnqueueCapture(captureItem("first"))

		barrierDone := make(chan error, 1)
		go func() { barrierDone <- coord.CloseEpochIfPending("node-a") }()
		select {
		case <-grounder.entered:
		case <-time.After(2 * time.Second):
			t.Fatal("grounder never entered")
		}

		// The flush of epoch 1 is in flight: this capture must be stamped 2.
		coord.EnqueueCapture(captureItem("second"))
		require.Equal(t, uint64(2), coord.pendingItemEpoch(t), "mid-flush enqueue stamps the next epoch")

		close(grounder.release)
		require.NoError(t, <-barrierDone)
		require.Equal(t, uint64(2), coord.EpochID(), "epoch 1 closed, epoch 2 opens")

		// The deferred item flushes at the next barrier, into epoch 2.
		require.NoError(t, coord.CloseEpochIfPending("node-b"))
		require.Equal(t, uint64(3), coord.EpochID(), "epoch 2 closed")
		require.Equal(t, uint64(3), env.AssemblyMetadataSnapshot().EpochID, "envelope mirrors the opening epoch")
	})
}

func (c *EpochCoordinator) pendingItemEpoch(t *testing.T) uint64 {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	require.Len(t, c.items, 1)
	return c.items[0].Epoch
}

// TestConcurrentBarriersSerialize proves D2's structural double-close
// prevention: two barriers racing on a shared coordinator take disjoint
// work, the epoch advances exactly once per flush, and no item is lost or
// grounded twice.
func TestConcurrentBarriersSerialize(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "session-1")
	tel := &epochTelemetry{}
	grounder := &countingGrounder{}
	coord := NewEpochCoordinator(contextdata.WithEnvelope(context.Background(), env), grounder, tel)
	for i := 0; i < 8; i++ {
		coord.EnqueueCapture(captureItem("item"))
	}

	var wg sync.WaitGroup
	barriers := 4
	for i := 0; i < barriers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = coord.CloseEpochIfPending("node-a")
		}()
	}
	wg.Wait()

	require.Equal(t, 8, int(grounder.total.Load()), "every capture grounded exactly once, none duplicated")
	require.Equal(t, uint64(2), coord.EpochID(), "one flush of the pending work: 1→2")
	require.Equal(t, 1, tel.count(fwtelemetry.EventEpochClosed), "exactly one epoch.closed")
	captures, _ := coord.Pending()
	require.Zero(t, captures)
}

// countingGrounder counts grounded items across batches.
type countingGrounder struct {
	total atomic.Int64
}

func (g *countingGrounder) Ground(_ context.Context, items []knowledge.GroundingItem) (knowledge.GroundingReport, error) {
	g.total.Add(int64(len(items)))
	report := knowledge.GroundingReport{Grounded: make([]knowledge.GroundingEntry, len(items))}
	for i := range items {
		report.Grounded[i] = knowledge.GroundingEntry{Index: i, Action: knowledge.GroundingGrounded}
	}
	return report, nil
}

// failingCaptureNode enqueues a capture then fails: the node error propagates
// while the deferred FinalEpoch grounding failure must still be surfaced.
type failingCaptureNode struct {
	id string
}

func (n *failingCaptureNode) ID() string     { return n.id }
func (n *failingCaptureNode) Type() NodeType { return NodeTypeSystem }
func (n *failingCaptureNode) Execute(ctx context.Context, _ *contextdata.Envelope) (*execution.Result, error) {
	if coord := EpochCoordinatorFromContext(ctx); coord != nil {
		coord.EnqueueCapture(captureItem("doomed"))
	}
	return nil, errors.New("node failed after capture")
}

// TestFinalEpochErrorSurfaces proves the swallow fix: when a node error is
// already propagating, a grounding failure at the deferred FinalEpoch is
// emitted on the graph's telemetry sink (capture.ground_failed, deferred)
// instead of being silently discarded.
func TestFinalEpochErrorSurfaces(t *testing.T) {
	tel := &epochTelemetry{}
	g := NewGraph()
	require.NoError(t, g.SetTelemetry(tel))
	require.NoError(t, g.SetGrounder(&recordingGrounder{err: knowledge.ErrGroundingFailed}))
	require.NoError(t, g.AddNode(&failingCaptureNode{id: "failer"}))
	require.NoError(t, g.SetStart("failer"))

	env := contextdata.NewEnvelope("task-1", "session-1")
	_, err := g.Execute(context.Background(), env)
	require.Error(t, err, "the node error propagates")

	deferred := 0
	for _, ev := range telSnapshot(tel) {
		if ev.Type == fwtelemetry.EventCaptureGroundFailed {
			if flag, _ := ev.Metadata["deferred"].(bool); flag {
				deferred++
			}
		}
	}
	require.Equal(t, 1, deferred, "the deferred flush failure is emitted on the graph's sink, never silent")
}

// TestSetGrounderSealedAndSetDrainSealed proves FR-32: the two unsealed
// mutators gain the ErrGraphSealed guard every other mutator has.
func TestSetGrounderSealedAndSetDrainSealed(t *testing.T) {
	g := NewGraph()
	require.NoError(t, g.AddNode(&failingCaptureNode{id: "n"}))
	require.NoError(t, g.SetStart("n"))
	env := contextdata.NewEnvelope("task-1", "session-1")
	_, err := g.Execute(context.Background(), env)
	// The node fails (no coordinator, no grounder) — but the graph is sealed.
	require.Error(t, err)

	require.ErrorIs(t, g.SetGrounder(&recordingGrounder{}), ErrGraphSealed)
	require.ErrorIs(t, g.SetDrain(func(time.Duration) {}), ErrGraphSealed)
}
