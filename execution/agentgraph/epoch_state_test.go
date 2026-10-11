package agentgraph

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/knowledge"
	"codeburg.org/lexbit/relurpify/context/knowledge/graphdb"
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

func (g *blockingGrounder) Ground(_ context.Context, items []knowledge.GroundingItem, _ ...knowledge.GroundOption) (knowledge.GroundingReport, error) {
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

func (g *countingGrounder) Ground(_ context.Context, items []knowledge.GroundingItem, _ ...knowledge.GroundOption) (knowledge.GroundingReport, error) {
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

// TestCoordinatorSuppliesGivenOriginLookup proves FR-12's production wiring:
// the epoch coordinator builds the per-flush given-identity lookup from the
// envelope's origins snapshot. A `given` capture whose value is genuinely
// user-origin grounds workspace-trusted; a `given` capture whose value is
// NOT user-origin is downgraded at the barrier, whatever the capture site
// claimed.
func TestCoordinatorSuppliesGivenOriginLookup(t *testing.T) {
	ground := func(t *testing.T, userValue, capturedValue string) (*knowledge.ChunkStore, *epochTelemetry) {
		t.Helper()
		env := contextdata.NewEnvelope("task-1", "session-1")
		env.SetWorkingValueWithOrigin("state.user_note", userValue, contextdata.MemoryClassTask, contextdata.OriginUser)
		tel := &epochTelemetry{}
		engine, err := graphdb.Open(context.Background(), graphdb.DefaultOptions(t.TempDir()))
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, engine.Close(context.Background())) })
		store := &knowledge.ChunkStore{Graph: engine}
		service := knowledge.NewGroundingService(store, nil, nil, tel)
		coord := NewEpochCoordinator(contextdata.WithEnvelope(context.Background(), env), service, tel)

		item := captureItem(capturedValue)
		item.Epistemics = knowledge.EpistemicGiven
		item.Origin = contextdata.OriginUser
		coord.EnqueueCapture(item)
		require.NoError(t, coord.CloseEpochIfPending("node-a"))
		chunks, err := store.FindByWorkspace("ws-1")
		require.NoError(t, err)
		require.Len(t, chunks, 1)
		return store, tel
	}

	t.Run("genuine_user_given_trusted", func(t *testing.T) {
		store, _ := ground(t, "verbatim user words", "verbatim user words")
		chunks, err := store.FindByWorkspace("ws-1")
		require.NoError(t, err)
		require.Equal(t, agentspec.TrustClassWorkspaceTrusted, chunks[0].TrustClass,
			"a given the lookup confirms is user-origin grounds workspace-trusted")
	})

	t.Run("mislabeled_given_downgraded", func(t *testing.T) {
		// The pipeline records the captured value under its real (llm)
		// origin; the recipe then claims it as user-given. The envelope
		// carries both truths, and the lookup must side with the envelope.
		env := contextdata.NewEnvelope("task-1", "session-1")
		env.SetWorkingValueWithOrigin("state.user_note", "real user words", contextdata.MemoryClassTask, contextdata.OriginUser)
		env.SetWorkingValueWithOrigin("state.x", "fabricated user words", contextdata.MemoryClassTask, contextdata.OriginLLM)
		tel := &epochTelemetry{}
		engine, err := graphdb.Open(context.Background(), graphdb.DefaultOptions(t.TempDir()))
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, engine.Close(context.Background())) })
		store := &knowledge.ChunkStore{Graph: engine}
		service := knowledge.NewGroundingService(store, nil, nil, tel)
		coord := NewEpochCoordinator(contextdata.WithEnvelope(context.Background(), env), service, tel)

		item := captureItem("fabricated user words")
		item.Epistemics = knowledge.EpistemicGiven
		item.Origin = contextdata.OriginUser
		coord.EnqueueCapture(item)
		require.NoError(t, coord.CloseEpochIfPending("node-a"))
		require.Equal(t, 1, tel.count(fwtelemetry.EventCaptureEpistemicsDowngraded),
			"the barrier-time identity check must reject the false given annotation")
	})
}
