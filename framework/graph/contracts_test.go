package graph_test

import (
	"context"
	"strings"
	"testing"

	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	"github.com/stretchr/testify/require"
)

type contractTestNode struct {
	id       string
	nodeType graph.NodeType
	contract graph.NodeContract
}

func (n contractTestNode) ID() string { return n.id }
func (n contractTestNode) Type() graph.NodeType {
	if n.nodeType == "" {
		return graph.NodeTypeSystem
	}
	return n.nodeType
}
func (n contractTestNode) Execute(ctx context.Context, state *graph.Context) (*graph.Result, error) {
	return &graph.Result{NodeID: n.id, Success: true, Data: map[string]interface{}{}}, nil
}
func (n contractTestNode) Contract() graph.NodeContract { return n.contract }

type plainTestNode struct {
	id       string
	nodeType graph.NodeType
}

func (n plainTestNode) ID() string           { return n.id }
func (n plainTestNode) Type() graph.NodeType { return n.nodeType }
func (n plainTestNode) Execute(context.Context, *graph.Context) (*graph.Result, error) {
	return &graph.Result{NodeID: n.id, Success: true, Data: map[string]interface{}{}}, nil
}

type readOnlyToolStub struct{}

func (readOnlyToolStub) Name() string        { return "read_only" }
func (readOnlyToolStub) Description() string { return "read-only test tool" }
func (readOnlyToolStub) Category() string    { return "test" }
func (readOnlyToolStub) Parameters() []core.ToolParameter {
	return nil
}
func (readOnlyToolStub) Execute(context.Context, *core.Context, map[string]interface{}) (*core.ToolResult, error) {
	return &core.ToolResult{Success: true, Data: map[string]interface{}{}}, nil
}
func (readOnlyToolStub) IsAvailable(context.Context, *core.Context) bool { return true }
func (readOnlyToolStub) Permissions() core.ToolPermissions               { return core.ToolPermissions{} }
func (readOnlyToolStub) Tags() []string                                  { return []string{core.TagReadOnly} }

func TestResolveNodeContractUsesToolDefaults(t *testing.T) {
	registry := capability.NewRegistry()
	require.NoError(t, registry.Register(readOnlyToolStub{}))
	node := graph.NewToolNode("tool", readOnlyToolStub{}, nil, registry)

	contract := graph.ResolveNodeContract(node)
	require.Len(t, contract.RequiredCapabilities, 1)
	require.Equal(t, "tool:read_only", contract.RequiredCapabilities[0].ID)
	require.Equal(t, graph.SideEffectNone, contract.SideEffectClass)
	require.Equal(t, graph.IdempotencyReplaySafe, contract.Idempotency)
	require.True(t, contract.ContextPolicy.PreferArtifactReferences)
	require.Contains(t, contract.ContextPolicy.AllowedDataClasses, core.StateDataClassArtifactRef)
}

func TestResolveNodeContractUsesLLMAndHumanDefaults(t *testing.T) {
	llmContract := graph.ResolveNodeContract(&graph.LLMNode{})
	require.Equal(t, graph.SideEffectNone, llmContract.SideEffectClass)
	require.Equal(t, graph.IdempotencyReplaySafe, llmContract.Idempotency)
	require.False(t, llmContract.ContextPolicy.AllowHistoryAccess)

	humanContract := graph.ResolveNodeContract(&graph.HumanNode{})
	require.Equal(t, graph.SideEffectHuman, humanContract.SideEffectClass)
	require.Equal(t, graph.IdempotencySingleShot, humanContract.Idempotency)
	require.True(t, humanContract.ContextPolicy.AllowHistoryAccess)
}

func TestResolveNodeContractFallsBackForPlainNode(t *testing.T) {
	contract := graph.ResolveNodeContract(plainTestNode{id: "observe", nodeType: graph.NodeTypeObservation})
	require.Equal(t, graph.SideEffectNone, contract.SideEffectClass)
	require.Equal(t, graph.IdempotencyReplaySafe, contract.Idempotency)
	require.Contains(t, contract.ContextPolicy.AllowedDataClasses, core.StateDataClassRoutingFlag)
}

func TestGraphValidateRejectsInvalidContractSelector(t *testing.T) {
	g := graph.NewGraph()
	start := contractTestNode{
		id:       "start",
		nodeType: graph.NodeTypeSystem,
		contract: graph.NodeContract{
			RequiredCapabilities: []core.CapabilitySelector{{}},
			SideEffectClass:      graph.SideEffectNone,
			Idempotency:          graph.IdempotencyReplaySafe,
		},
	}
	done := graph.NewTerminalNode("done")

	require.NoError(t, g.AddNode(start))
	require.NoError(t, g.AddNode(done))
	require.NoError(t, g.SetStart(start.ID()))
	require.NoError(t, g.AddEdge(start.ID(), done.ID(), nil, false))

	err := g.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid capability selector")
}

func TestGraphValidateRejectsToolContractWithoutRequiredCapabilities(t *testing.T) {
	g := graph.NewGraph()
	start := contractTestNode{
		id:       "start",
		nodeType: graph.NodeTypeTool,
		contract: graph.NodeContract{
			SideEffectClass: graph.SideEffectExternal,
			Idempotency:     graph.IdempotencyUnknown,
		},
	}
	done := graph.NewTerminalNode("done")

	require.NoError(t, g.AddNode(start))
	require.NoError(t, g.AddNode(done))
	require.NoError(t, g.SetStart(start.ID()))
	require.NoError(t, g.AddEdge(start.ID(), done.ID(), nil, false))

	err := g.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "declares no required capabilities")
}

func TestGraphValidateAllowsNonContractNodesViaDefaults(t *testing.T) {
	g := graph.NewGraph()
	start := plainTestNode{id: "start", nodeType: graph.NodeTypeObservation}
	done := graph.NewTerminalNode("done")

	require.NoError(t, g.AddNode(start))
	require.NoError(t, g.AddNode(done))
	require.NoError(t, g.SetStart(start.ID()))
	require.NoError(t, g.AddEdge(start.ID(), done.ID(), nil, false))

	require.NoError(t, g.Validate())
}

func TestGraphValidateRejectsInvalidContextPolicy(t *testing.T) {
	g := graph.NewGraph()
	start := contractTestNode{
		id:       "start",
		nodeType: graph.NodeTypeSystem,
		contract: graph.NodeContract{
			SideEffectClass: graph.SideEffectNone,
			Idempotency:     graph.IdempotencyReplaySafe,
			ContextPolicy: core.StateBoundaryPolicy{
				AllowedMemoryClasses: []core.MemoryClass{"invalid"},
			},
		},
	}
	done := graph.NewTerminalNode("done")

	require.NoError(t, g.AddNode(start))
	require.NoError(t, g.AddNode(done))
	require.NoError(t, g.SetStart(start.ID()))
	require.NoError(t, g.AddEdge(start.ID(), done.ID(), nil, false))

	err := g.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid context policy")
}

func TestLintNodeStateFlagsOversizeRawPayload(t *testing.T) {
	registry := capability.NewRegistry()
	require.NoError(t, registry.Register(readOnlyToolStub{}))
	node := graph.NewToolNode("tool", readOnlyToolStub{}, nil, registry)
	state := core.NewContext()
	state.Set("tool.payload", map[string]interface{}{"body": strings.Repeat("x", 5000)})

	violations := graph.LintNodeState(node, state)
	require.NotEmpty(t, violations)
}

type externalEffectToolStub struct{}

func (externalEffectToolStub) Name() string                     { return "external_tool" }
func (externalEffectToolStub) Description() string              { return "external effect tool" }
func (externalEffectToolStub) Category() string                 { return "test" }
func (externalEffectToolStub) Parameters() []core.ToolParameter { return nil }
func (externalEffectToolStub) Execute(context.Context, *core.Context, map[string]interface{}) (*core.ToolResult, error) {
	return &core.ToolResult{Success: true}, nil
}
func (externalEffectToolStub) IsAvailable(context.Context, *core.Context) bool { return true }
func (externalEffectToolStub) Permissions() core.ToolPermissions               { return core.ToolPermissions{} }
func (externalEffectToolStub) Tags() []string                                  { return []string{core.TagExecute} }
func (externalEffectToolStub) CapabilityDescriptor() core.CapabilityDescriptor {
	return core.CapabilityDescriptor{
		ID:            "tool:external_tool",
		Name:          "external_tool",
		Kind:          core.CapabilityKindTool,
		EffectClasses: []core.EffectClass{core.EffectClassExternalState},
	}
}

func TestResolveNodeContractClassifiesExternalToolAsSingleShot(t *testing.T) {
	registry := capability.NewRegistry()
	tool := externalEffectToolStub{}
	require.NoError(t, registry.Register(tool))

	contract := graph.ResolveNodeContract(graph.NewToolNode("tool", tool, nil, registry))
	require.Equal(t, graph.SideEffectExternal, contract.SideEffectClass)
	require.Equal(t, graph.IdempotencySingleShot, contract.Idempotency)
}
