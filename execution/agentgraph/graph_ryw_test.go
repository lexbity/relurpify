package agentgraph

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/knowledge"
	"codeburg.org/lexbit/relurpify/context/knowledge/graphdb"
	execution "codeburg.org/lexbit/relurpify/execution"
	fwtelemetry "codeburg.org/lexbit/relurpify/telemetry"
	"codeburg.org/lexbit/relurpify/testsuite/testhelper"
)

type captureNode struct {
	id string
}

func (n *captureNode) ID() string     { return n.id }
func (n *captureNode) Type() NodeType { return NodeTypeSystem }

func (n *captureNode) Execute(ctx context.Context, env *contextdata.Envelope) (*execution.Result, error) {
	sink := CaptureSinkFromContext(ctx)
	if sink == nil {
		return nil, fmt.Errorf("%s: no capture sink in context", n.id)
	}
	sink.EnqueueCapture(knowledge.GroundingItem{
		Value:       map[string]any{"text": "ryw"},
		Epistemics:  knowledge.EpistemicClaimed,
		Origin:      contextdata.OriginLLM,
		StateKey:    "state.findings",
		NodeID:      n.id,
		TaskID:      env.TaskID,
		SessionID:   env.SessionID,
		WorkspaceID: "ws-1",
		Kind:        knowledge.ChunkKindCapture,
	})
	return &execution.Result{NodeID: n.id, Success: true}, nil
}

type checkNode struct {
	id    string
	store *knowledge.ChunkStore
}

func (n *checkNode) ID() string     { return n.id }
func (n *checkNode) Type() NodeType { return NodeTypeSystem }

func (n *checkNode) Execute(_ context.Context, _ *contextdata.Envelope) (*execution.Result, error) {
	all, err := n.store.FindAll()
	if err != nil {
		return nil, err
	}
	for _, chunk := range all {
		if key, _ := chunk.Body.Fields["state_key"].(string); key == "state.findings" {
			return &execution.Result{NodeID: n.id, Success: true}, nil
		}
	}
	return nil, fmt.Errorf("%s: capture not visible in store (read-your-writes violation)", n.id)
}

func newRYWGroundingStore(t *testing.T) (*knowledge.ChunkStore, *knowledge.GroundingService) {
	t.Helper()
	engine, err := graphdb.Open(context.Background(), graphdb.DefaultOptions(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, engine.Close(context.Background())) })
	store := &knowledge.ChunkStore{Graph: engine}
	grounder := knowledge.NewGroundingService(store, nil, nil, nil)
	return store, grounder
}

// TestGraphReadYourWritesAcrossEpochs proves a capture grounded at node A's
// barrier is visible to node B before the run's final epoch closes.
func TestGraphReadYourWritesAcrossEpochs(t *testing.T) {
	store, grounder := newRYWGroundingStore(t)
	graph := NewGraph()
	graph.SetGrounder(grounder)
	require.NoError(t, graph.AddNode(&captureNode{id: "capture"}))
	require.NoError(t, graph.AddNode(&checkNode{id: "check", store: store}))
	require.NoError(t, graph.SetStart("capture"))
	require.NoError(t, graph.AddEdge("capture", "check", nil, false))

	env := contextdata.NewEnvelope("task-1", "session-1")
	result, err := graph.Execute(context.Background(), env)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Success)

	all, err := store.FindAll()
	require.NoError(t, err)
	require.Len(t, all, 1, "grounded capture must persist after the run")
	require.Equal(t, "state.findings", all[0].Body.Fields["state_key"])
	require.Greater(t, env.AssemblyMetadataSnapshot().EpochID, uint64(0), "envelope must carry a closed epoch")
}

// TestGraphSequentialRunsDoNotLeakGoroutines proves the epoch lifecycle leaves
// no goroutines behind over one hundred sequential runs (NFR-4 / AC-7).
func TestGraphSequentialRunsDoNotLeakGoroutines(t *testing.T) {
	_, grounder := newRYWGroundingStore(t)
	graph := NewGraph()
	graph.SetGrounder(grounder)
	require.NoError(t, graph.AddNode(&captureNode{id: "capture"}))
	require.NoError(t, graph.SetStart("capture"))

	testhelper.NoGoroutineLeak(t, func() {
		for i := 0; i < 100; i++ {
			env := contextdata.NewEnvelope(fmt.Sprintf("task-%d", i), "session-leak")
			if _, err := graph.Execute(context.Background(), env); err != nil {
				t.Fatalf("run %d failed: %v", i, err)
			}
		}
	})
}

// recordingGraphTelemetry asserts epoch and capture events flowed.
type recordingGraphTelemetry struct {
	events []fwtelemetry.Event
}

func (r *recordingGraphTelemetry) Emit(ev fwtelemetry.Event) {
	r.events = append(r.events, ev)
}

func (r *recordingGraphTelemetry) count(kind fwtelemetry.EventType) int {
	n := 0
	for _, ev := range r.events {
		if ev.Type == kind {
			n++
		}
	}
	return n
}

// TestGraphGroundingFailureFailsTheRun proves barrier grounding failure surfaces
// as an errors.Is(ErrGroundingFailed) run error, never a silent drop.
func TestGraphGroundingFailureFailsTheRun(t *testing.T) {
	graph := NewGraph()
	graph.SetGrounder(&failingGrounder{})
	require.NoError(t, graph.AddNode(&captureNode{id: "capture"}))
	require.NoError(t, graph.SetStart("capture"))

	env := contextdata.NewEnvelope("task-1", "session-1")
	_, err := graph.Execute(context.Background(), env)
	require.Error(t, err)
	require.ErrorIs(t, err, knowledge.ErrGroundingFailed)
	require.Contains(t, err.Error(), "agentgraph")
}

type failingGrounder struct{}

func (failingGrounder) Ground(_ context.Context, _ []knowledge.GroundingItem) (knowledge.GroundingReport, error) {
	return knowledge.GroundingReport{}, errors.New("store unavailable")
}
