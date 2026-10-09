package agentgraph

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

// Fan-out node IDs used by the Phase 3 parallel tests. Distinct from the
// literals other tests use so the ids are self-documenting here.
const (
	fanOutRootID = "fanout-root"
	fanOutDoneID = "fanout-done"
)

// branchActionNode applies a side effect to the branch envelope. A nil apply
// makes it a pure control-flow (system) node, usable as a fan-out root.
type branchActionNode struct {
	id    string
	apply func(env *contextdata.Envelope)
}

func (n *branchActionNode) ID() string     { return n.id }
func (n *branchActionNode) Type() NodeType { return NodeTypeSystem }

func (n *branchActionNode) Execute(_ context.Context, env *contextdata.Envelope) (*execution.Result, error) {
	if n.apply != nil {
		n.apply(env)
	}
	return &execution.Result{NodeID: n.id, Success: true}, nil
}

// recordingSink is a mutex-protected telemetry sink safe for parallel fan-out,
// where branch subgraphs emit concurrently with the parent.
type recordingSink struct {
	mu     sync.Mutex
	events []telemetry.Event
}

func (s *recordingSink) Emit(event telemetry.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
}

func (s *recordingSink) find(eventType telemetry.EventType) (telemetry.Event, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, event := range s.events {
		if event.Type == eventType {
			return event, true
		}
	}
	return telemetry.Event{}, false
}

// buildFanOut builds fork -> (branch_i in parallel) -> done and returns the
// graph with a fresh envelope.
func buildFanOut(t *testing.T, sink telemetry.Telemetry, branches ...*branchActionNode) (*Graph, *contextdata.Envelope) {
	t.Helper()
	g := NewGraph()
	if sink != nil {
		require.NoError(t, g.SetTelemetry(sink))
	}
	require.NoError(t, g.AddNode(&branchActionNode{id: fanOutRootID}))
	require.NoError(t, g.AddNode(NewTerminalNode(fanOutDoneID)))
	for _, branch := range branches {
		require.NoError(t, g.AddNode(branch))
		require.NoError(t, g.AddEdge(fanOutRootID, branch.ID(), nil, true))
		require.NoError(t, g.AddEdge(branch.ID(), fanOutDoneID, nil, false))
	}
	require.NoError(t, g.SetStart(fanOutRootID))
	return g, contextdata.NewEnvelope("task-merge", "session")
}

func writeKey(key string, value any) func(*contextdata.Envelope) {
	return func(env *contextdata.Envelope) {
		env.SetWorkingValueWithClass(key, value, contextdata.MemoryClassTask)
	}
}

// TestParallelBranchOrder runs the fan-out 1000 times and asserts the highest
// declared edge always wins a shared key, independent of goroutine completion
// order.
func TestParallelBranchOrder(t *testing.T) {
	ctx := context.Background()
	for iteration := 0; iteration < 1000; iteration++ {
		g, env := buildFanOut(t, nil,
			&branchActionNode{id: "b0", apply: writeKey("winner", "b0")},
			&branchActionNode{id: "b1", apply: writeKey("winner", "b1")},
			&branchActionNode{id: "b2", apply: writeKey("winner", "b2")},
		)
		_, err := g.Execute(ctx, env)
		require.NoError(t, err)
		require.Equal(t, "b2", env.WorkingDataSnapshot()["winner"], "iteration %d", iteration)
	}
}

// TestParallelBranchMergeTelemetry asserts graph.branch_merged carries the merge
// stats and the declaration-order winner for each conflict.
func TestParallelBranchMergeTelemetry(t *testing.T) {
	sink := &recordingSink{}
	g, env := buildFanOut(t, sink,
		&branchActionNode{id: "b0", apply: writeKey("winner", "b0")},
		&branchActionNode{id: "b1", apply: writeKey("winner", "b1")},
	)
	_, err := g.Execute(context.Background(), env)
	require.NoError(t, err)

	event, ok := sink.find(telemetry.EventGraphBranchMerged)
	require.True(t, ok, "expected a graph.branch_merged event")
	require.Equal(t, 2, event.Metadata["units_applied"])
	require.Equal(t, 1, event.Metadata["keys_written"])
	require.Equal(t, []string{"winner"}, event.Metadata["conflicted_keys"])

	winners, ok := event.Metadata["winner_index"].(map[string]int)
	require.True(t, ok, "winner_index must be a map[string]int")
	require.Equal(t, 1, winners["winner"])

	// The parent envelope carries the winner's value too.
	require.Equal(t, "b1", env.WorkingDataSnapshot()["winner"])
}

// TestParallelBranchDeletionPropagation covers the delete/write ordering cases
// across lanes.
func TestParallelBranchDeletionPropagation(t *testing.T) {
	ctx := context.Background()

	t.Run("later write overrides earlier delete", func(t *testing.T) {
		g, env := buildFanOut(t, nil,
			&branchActionNode{id: "deleter", apply: func(e *contextdata.Envelope) { e.DeleteWorkingValue("k") }},
			&branchActionNode{id: "writer", apply: writeKey("k", "from-writer")},
		)
		env.SetWorkingValueWithClass("k", "base", contextdata.MemoryClassTask)
		_, err := g.Execute(ctx, env)
		require.NoError(t, err)
		require.Equal(t, "from-writer", env.WorkingDataSnapshot()["k"])
	})

	t.Run("later delete overrides earlier write", func(t *testing.T) {
		g, env := buildFanOut(t, nil,
			&branchActionNode{id: "writer", apply: writeKey("k", "from-writer")},
			&branchActionNode{id: "deleter", apply: func(e *contextdata.Envelope) { e.DeleteWorkingValue("k") }},
		)
		env.SetWorkingValueWithClass("k", "base", contextdata.MemoryClassTask)
		_, err := g.Execute(ctx, env)
		require.NoError(t, err)
		require.NotContains(t, env.WorkingDataSnapshot(), "k")
	})
}
