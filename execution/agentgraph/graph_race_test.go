package agentgraph

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	capresult "codeburg.org/lexbit/relurpify/capability/result"

	"codeburg.org/lexbit/relurpify/capability/descriptor"
	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
	"github.com/stretchr/testify/require"
)

// statelessInvoker is a concurrency-safe CapabilityInvoker for race tests.
type statelessInvoker struct{}

func (statelessInvoker) InvokeCapability(context.Context, *contextdata.Envelope, string, map[string]any) (*ports.ToolResult, error) {
	return &ports.ToolResult{Success: true, Data: map[string]any{"stdout": "ok"}}, nil
}
func (statelessInvoker) CapturePolicySnapshot() *capresult.PolicySnapshot { return nil }
func (statelessInvoker) GetCapability(string) (descriptor.CapabilityDescriptor, bool) {
	return descriptor.CapabilityDescriptor{}, false
}

// TestToolNodeConcurrentExecute drives the same ToolNode from many goroutines.
// Run with -race; it fails if traceID/spanCount are unsynchronized.
func TestToolNodeConcurrentExecute(t *testing.T) {
	node := NewToolNode("shared", &traceTestTool{name: "shared"}, nil, statelessInvoker{})

	const workers = 32
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			env := contextdata.NewEnvelope("task", "session")
			if _, err := node.Execute(context.Background(), env); err != nil {
				t.Errorf("Execute: %v", err)
			}
		}()
	}
	wg.Wait()

	if node.traceIDValue() == "" {
		t.Fatal("trace ID must be initialized after execution")
	}
}

// TestGraphParallelBranchesShareToolNode exercises the real parallel-branch
// path, where both branches traverse the same ToolNode instance concurrently.
func TestGraphParallelBranchesShareToolNode(t *testing.T) {
	g := NewGraph()
	fork := internalContractNode{id: "fork", kind: NodeTypeSystem}
	left := internalContractNode{id: "left", kind: NodeTypeSystem}
	right := internalContractNode{id: "right", kind: NodeTypeSystem}
	tool := NewToolNode("tool", &traceTestTool{name: "shared"}, nil, statelessInvoker{})

	require.NoError(t, g.AddNode(fork))
	require.NoError(t, g.AddNode(left))
	require.NoError(t, g.AddNode(right))
	require.NoError(t, g.AddNode(tool))
	require.NoError(t, g.AddEdge("fork", "left", nil, true))
	require.NoError(t, g.AddEdge("fork", "right", nil, true))
	require.NoError(t, g.AddEdge("left", "tool", nil, false))
	require.NoError(t, g.AddEdge("right", "tool", nil, false))
	require.NoError(t, g.SetStart("fork"))

	env := contextdata.NewEnvelope("task", "session")
	_, err := g.Execute(context.Background(), env)
	require.NoError(t, err)

	if tool.traceIDValue() == "" {
		t.Fatal("shared ToolNode trace ID must be initialized")
	}
}

// TestGraphParallelBranchesWithEnvelopeReader drives the parallel fan-out while
// a background goroutine reads the parent envelope. Run with -race; the locked
// merge and the envelope accessors must keep this free of data races.
func TestGraphParallelBranchesWithEnvelopeReader(t *testing.T) {
	g, env := buildFanOut(t, nil,
		&branchActionNode{id: "b0", apply: writeKey("shared", "b0")},
		&branchActionNode{id: "b1", apply: writeKey("shared", "b1")},
	)
	env.SetWorkingValueWithClass("shared", "base", contextdata.MemoryClassTask)

	stop := make(chan struct{})
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		for {
			select {
			case <-stop:
				return
			default:
				_ = env.WorkingDataSnapshot()
				_ = env.GetExecutionPhase()
				_ = env.GetInteractions()
			}
		}
	}()

	_, err := g.Execute(context.Background(), env)
	close(stop)
	<-readerDone
	require.NoError(t, err)
	require.Equal(t, "b1", env.WorkingDataSnapshot()["shared"])
}

// TestGraphBranchReadPathsDoNotDeadlock asserts that a node reading the parent
// graph from inside a parallel branch cannot deadlock against the parent run
// loop: read paths take g.mu.RLock and never touch execMu.
func TestGraphBranchReadPathsDoNotDeadlock(t *testing.T) {
	g := NewGraph()
	var reads atomic.Int64
	require.NoError(t, g.AddNode(&branchActionNode{id: fanOutRootID}))
	require.NoError(t, g.AddNode(NewTerminalNode(fanOutDoneID)))
	for _, id := range []string{"readerA", "readerB"} {
		node := &graphReadingNode{id: id, graph: g, reads: &reads}
		require.NoError(t, g.AddNode(node))
		require.NoError(t, g.AddEdge(fanOutRootID, node.ID(), nil, true))
		require.NoError(t, g.AddEdge(node.ID(), fanOutDoneID, nil, false))
	}
	require.NoError(t, g.SetStart(fanOutRootID))

	completed := make(chan error, 1)
	go func() {
		_, err := g.Execute(context.Background(), contextdata.NewEnvelope("task-read", "session"))
		completed <- err
	}()
	select {
	case err := <-completed:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("parallel branch read path deadlocked against execMu")
	}
	require.EqualValues(t, 2, reads.Load())
}

type graphReadingNode struct {
	id    string
	graph *Graph
	reads *atomic.Int64
}

func (n *graphReadingNode) ID() string     { return n.id }
func (n *graphReadingNode) Type() NodeType { return NodeTypeSystem }

func (n *graphReadingNode) Execute(_ context.Context, _ *contextdata.Envelope) (*execution.Result, error) {
	_ = n.graph.HasNode(fanOutDoneID)
	_ = n.graph.OutgoingEdges(fanOutRootID)
	_ = n.graph.NodeIDs()
	n.reads.Add(1)
	return &execution.Result{NodeID: n.id, Success: true}, nil
}

// TestGraphSnapshotIndependence asserts that a node inside a parallel branch
// cannot mutate the sealed parent: the attempted AddNode fails with
// ErrGraphSealed and the structure stays whole across a second execution.
func TestGraphSnapshotIndependence(t *testing.T) {
	g, env := buildFanOut(t, nil,
		&branchActionNode{id: "b0", apply: writeKey("k", "b0")},
		&branchActionNode{id: "b1", apply: writeKey("k", "b1")},
	)
	mutator := &graphMutatingNode{id: "mutator", graph: g}
	require.NoError(t, g.AddNode(mutator))
	require.NoError(t, g.AddEdge(fanOutRootID, "mutator", nil, true))
	require.NoError(t, g.AddEdge("mutator", fanOutDoneID, nil, false))

	_, err := g.Execute(context.Background(), env)
	require.NoError(t, err)
	require.ErrorIs(t, mutator.errValue(), ErrGraphSealed)
	require.False(t, g.HasNode("intruder"), "mid-run mutation must not take effect")
	require.Equal(t, "b1", env.WorkingDataSnapshot()["k"])

	// A second run still sees the sealed, whole structure. A fresh envelope keeps
	// the branch inputs identical, so the declaration-order winner is stable.
	_, err = g.Execute(context.Background(), contextdata.NewEnvelope("task-merge", "session-2"))
	require.NoError(t, err)
	require.False(t, g.HasNode("intruder"))
	require.ErrorIs(t, mutator.errValue(), ErrGraphSealed)
}

type graphMutatingNode struct {
	id    string
	graph *Graph
	mu    sync.Mutex
	err   error
}

func (n *graphMutatingNode) ID() string     { return n.id }
func (n *graphMutatingNode) Type() NodeType { return NodeTypeSystem }

func (n *graphMutatingNode) Execute(_ context.Context, _ *contextdata.Envelope) (*execution.Result, error) {
	err := n.graph.AddNode(NewTerminalNode("intruder"))
	n.mu.Lock()
	n.err = err
	n.mu.Unlock()
	return &execution.Result{NodeID: n.id, Success: true}, nil
}

func (n *graphMutatingNode) errValue() error {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.err
}
