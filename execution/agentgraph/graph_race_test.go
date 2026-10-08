package agentgraph

import (
	"context"
	"sync"
	"testing"

	capresult "codeburg.org/lexbit/relurpify/capability/result"

	"codeburg.org/lexbit/relurpify/capability/descriptor"
	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/context/contextdata"
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
	if got := node.spanCount.Load(); got != workers {
		t.Fatalf("spanCount = %d, want %d", got, workers)
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

	if got := tool.spanCount.Load(); got != 2 {
		t.Fatalf("shared ToolNode spanCount = %d, want 2", got)
	}
	if tool.traceIDValue() == "" {
		t.Fatal("shared ToolNode trace ID must be initialized")
	}
}
