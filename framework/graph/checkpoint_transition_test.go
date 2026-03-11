package graph_test

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	"github.com/stretchr/testify/require"
)

type checkpointToolStub struct {
	calls int
}

func (t *checkpointToolStub) Name() string        { return "checkpoint_tool" }
func (t *checkpointToolStub) Description() string { return "checkpoint test tool" }
func (t *checkpointToolStub) Category() string    { return "test" }
func (t *checkpointToolStub) Parameters() []core.ToolParameter {
	return nil
}
func (t *checkpointToolStub) Execute(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.ToolResult, error) {
	t.calls++
	state.Set("tool.calls", t.calls)
	return &core.ToolResult{Success: true, Data: map[string]interface{}{"calls": t.calls}}, nil
}
func (t *checkpointToolStub) IsAvailable(context.Context, *core.Context) bool { return true }
func (t *checkpointToolStub) Permissions() core.ToolPermissions               { return core.ToolPermissions{} }
func (t *checkpointToolStub) Tags() []string                                  { return []string{core.TagExecute} }

type countingNode struct {
	id    string
	kind  graph.NodeType
	calls *int
	run   func(*core.Context)
}

func (n *countingNode) ID() string           { return n.id }
func (n *countingNode) Type() graph.NodeType { return n.kind }
func (n *countingNode) Execute(ctx context.Context, state *graph.Context) (*graph.Result, error) {
	if n.calls != nil {
		*n.calls = *n.calls + 1
	}
	if n.run != nil {
		n.run(state)
	}
	return &graph.Result{NodeID: n.id, Success: true, Data: map[string]interface{}{}}, nil
}

func TestResumeFromCheckpointDoesNotReplayCompletedToolNode(t *testing.T) {
	g := graph.NewGraph()
	registry := capability.NewRegistry()
	tool := &checkpointToolStub{}
	require.NoError(t, registry.Register(tool))
	toolNode := graph.NewToolNode("tool", tool, nil, registry)
	done := graph.NewTerminalNode("done")

	require.NoError(t, g.AddNode(toolNode))
	require.NoError(t, g.AddNode(done))
	require.NoError(t, g.SetStart(toolNode.ID()))
	require.NoError(t, g.AddEdge(toolNode.ID(), done.ID(), nil, false))

	var checkpoint *graph.GraphCheckpoint
	g.WithCheckpointing(1, func(candidate *graph.GraphCheckpoint) error {
		if candidate.CompletedNodeID == toolNode.ID() {
			checkpoint = candidate
		}
		return nil
	})

	_, err := g.Execute(context.Background(), core.NewContext())
	require.NoError(t, err)
	require.NotNil(t, checkpoint)
	require.Equal(t, done.ID(), checkpoint.NextNodeID)
	require.Equal(t, 1, tool.calls)

	result, err := g.ResumeFromCheckpoint(context.Background(), checkpoint)
	require.NoError(t, err)
	require.Equal(t, done.ID(), result.NodeID)
	require.Equal(t, 1, tool.calls)
}

func TestResumeFromCheckpointDoesNotReplayHumanNode(t *testing.T) {
	g := graph.NewGraph()
	approvals := 0
	human := &countingNode{
		id:   "approval",
		kind: graph.NodeTypeHuman,
		run: func(state *core.Context) {
			approvals++
			state.Set("approval.count", approvals)
		},
	}
	done := graph.NewTerminalNode("done")

	require.NoError(t, g.AddNode(human))
	require.NoError(t, g.AddNode(done))
	require.NoError(t, g.SetStart(human.ID()))
	require.NoError(t, g.AddEdge(human.ID(), done.ID(), nil, false))

	var checkpoint *graph.GraphCheckpoint
	g.WithCheckpointing(1, func(candidate *graph.GraphCheckpoint) error {
		if candidate.CompletedNodeID == human.ID() {
			checkpoint = candidate
		}
		return nil
	})

	_, err := g.Execute(context.Background(), core.NewContext())
	require.NoError(t, err)
	require.NotNil(t, checkpoint)
	require.Equal(t, 1, approvals)

	result, err := g.ResumeFromCheckpoint(context.Background(), checkpoint)
	require.NoError(t, err)
	require.Equal(t, done.ID(), result.NodeID)
	require.Equal(t, 1, approvals)
}

func TestResumeFromCheckpointUsesConditionalNextNode(t *testing.T) {
	g := graph.NewGraph()
	conditionalRuns := 0
	chosenBranchRuns := 0
	otherBranchRuns := 0

	start := &countingNode{id: "start", kind: graph.NodeTypeSystem}
	conditional := &countingNode{
		id:    "gate",
		kind:  graph.NodeTypeConditional,
		calls: &conditionalRuns,
		run: func(state *core.Context) {
			state.Set("gate.next", "branch-a")
		},
	}
	branchA := &countingNode{id: "branch-a", kind: graph.NodeTypeSystem, calls: &chosenBranchRuns}
	branchB := &countingNode{id: "branch-b", kind: graph.NodeTypeSystem, calls: &otherBranchRuns}
	done := graph.NewTerminalNode("done")

	require.NoError(t, g.AddNode(start))
	require.NoError(t, g.AddNode(conditional))
	require.NoError(t, g.AddNode(branchA))
	require.NoError(t, g.AddNode(branchB))
	require.NoError(t, g.AddNode(done))
	require.NoError(t, g.SetStart(start.ID()))
	require.NoError(t, g.AddEdge(start.ID(), conditional.ID(), nil, false))
	require.NoError(t, g.AddEdge(conditional.ID(), branchA.ID(), func(result *graph.Result, state *graph.Context) bool {
		return state.GetString("gate.next") == "branch-a"
	}, false))
	require.NoError(t, g.AddEdge(conditional.ID(), branchB.ID(), func(result *graph.Result, state *graph.Context) bool {
		return state.GetString("gate.next") == "branch-b"
	}, false))
	require.NoError(t, g.AddEdge(branchA.ID(), done.ID(), nil, false))
	require.NoError(t, g.AddEdge(branchB.ID(), done.ID(), nil, false))

	var checkpoint *graph.GraphCheckpoint
	g.WithCheckpointing(1, func(candidate *graph.GraphCheckpoint) error {
		if candidate.CompletedNodeID == conditional.ID() {
			checkpoint = candidate
		}
		return nil
	})

	_, err := g.Execute(context.Background(), core.NewContext())
	require.NoError(t, err)
	require.NotNil(t, checkpoint)
	require.Equal(t, "branch-a", checkpoint.NextNodeID)
	require.Equal(t, 1, conditionalRuns)

	result, err := g.ResumeFromCheckpoint(context.Background(), checkpoint)
	require.NoError(t, err)
	require.Equal(t, done.ID(), result.NodeID)
	require.Equal(t, 1, conditionalRuns)
	require.Equal(t, 2, chosenBranchRuns)
	require.Equal(t, 0, otherBranchRuns)
}

func TestResumeFromCheckpointAfterParallelBranchCompletionSkipsCompletedBranches(t *testing.T) {
	g := graph.NewGraph()
	startRuns := 0
	branchARuns := 0
	branchBRuns := 0
	doneRuns := 0

	start := &countingNode{id: "start", kind: graph.NodeTypeSystem, calls: &startRuns}
	branchA := &countingNode{id: "branch-a", kind: graph.NodeTypeSystem, calls: &branchARuns, run: func(state *core.Context) {
		state.Set("branch.a", true)
	}}
	branchB := &countingNode{id: "branch-b", kind: graph.NodeTypeSystem, calls: &branchBRuns, run: func(state *core.Context) {
		state.Set("branch.b", true)
	}}
	done := &countingNode{id: "done", kind: graph.NodeTypeTerminal, calls: &doneRuns}

	require.NoError(t, g.AddNode(start))
	require.NoError(t, g.AddNode(branchA))
	require.NoError(t, g.AddNode(branchB))
	require.NoError(t, g.AddNode(done))
	require.NoError(t, g.SetStart(start.ID()))
	require.NoError(t, g.AddEdge(start.ID(), branchA.ID(), nil, true))
	require.NoError(t, g.AddEdge(start.ID(), branchB.ID(), nil, true))
	require.NoError(t, g.AddEdge(start.ID(), done.ID(), nil, false))

	var checkpoint *graph.GraphCheckpoint
	g.WithCheckpointing(1, func(candidate *graph.GraphCheckpoint) error {
		if candidate.CompletedNodeID == start.ID() {
			checkpoint = candidate
		}
		return nil
	})

	_, err := g.Execute(context.Background(), core.NewContext())
	require.NoError(t, err)
	require.NotNil(t, checkpoint)
	require.Equal(t, done.ID(), checkpoint.NextNodeID)
	require.Equal(t, 1, startRuns)
	require.Equal(t, 1, branchARuns)
	require.Equal(t, 1, branchBRuns)

	result, err := g.ResumeFromCheckpoint(context.Background(), checkpoint)
	require.NoError(t, err)
	require.Equal(t, done.ID(), result.NodeID)
	require.Equal(t, 1, startRuns)
	require.Equal(t, 1, branchARuns)
	require.Equal(t, 1, branchBRuns)
}

func TestResumeFromCompletedCheckpointReturnsStoredResultWithoutRestart(t *testing.T) {
	g := graph.NewGraph()
	done := graph.NewTerminalNode("done")
	require.NoError(t, g.AddNode(done))
	require.NoError(t, g.SetStart(done.ID()))

	checkpoint, err := g.CreateCheckpoint("task", done.ID(), "", &graph.Result{NodeID: done.ID(), Success: true}, &graph.NodeTransitionRecord{
		CompletedNodeID:  done.ID(),
		NextNodeID:       "",
		TransitionReason: "terminal",
	}, core.NewContext())
	require.NoError(t, err)

	result, err := g.ResumeFromCheckpoint(context.Background(), checkpoint)
	require.NoError(t, err)
	require.Equal(t, done.ID(), result.NodeID)
	require.True(t, result.Success)
}
