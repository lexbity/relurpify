package graph_test

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/graph"
	"github.com/stretchr/testify/require"
)

type preflightNode struct {
	id       string
	contract graph.NodeContract
}

func (n preflightNode) ID() string           { return n.id }
func (n preflightNode) Type() graph.NodeType { return graph.NodeTypeTool }
func (n preflightNode) Execute(context.Context, *core.Context) (*core.Result, error) {
	return &core.Result{NodeID: n.id, Success: true}, nil
}
func (n preflightNode) Contract() graph.NodeContract { return n.contract }

type preflightTool struct {
	name string
	desc core.CapabilityDescriptor
}

func (t preflightTool) Name() string                     { return t.name }
func (t preflightTool) Description() string              { return "preflight tool" }
func (t preflightTool) Category() string                 { return "test" }
func (t preflightTool) Parameters() []core.ToolParameter { return nil }
func (t preflightTool) Execute(context.Context, *core.Context, map[string]interface{}) (*core.ToolResult, error) {
	return &core.ToolResult{Success: true}, nil
}
func (t preflightTool) IsAvailable(context.Context, *core.Context) bool { return true }
func (t preflightTool) Permissions() core.ToolPermissions               { return core.ToolPermissions{} }
func (t preflightTool) Tags() []string                                  { return nil }
func (t preflightTool) CapabilityDescriptor() core.CapabilityDescriptor { return t.desc }

func TestGraphPreflightRejectsMissingCapability(t *testing.T) {
	g := graph.NewGraph()
	node := preflightNode{
		id: "tool",
		contract: graph.NodeContract{
			RequiredCapabilities: []core.CapabilitySelector{{Kind: core.CapabilityKindTool, Name: "missing.tool"}},
			SideEffectClass:      graph.SideEffectExternal,
			Idempotency:          graph.IdempotencySingleShot,
		},
	}
	done := graph.NewTerminalNode("done")
	require.NoError(t, g.AddNode(node))
	require.NoError(t, g.AddNode(done))
	require.NoError(t, g.SetStart(node.ID()))
	require.NoError(t, g.AddEdge(node.ID(), done.ID(), nil, false))
	g.SetCapabilityCatalog(capability.NewRegistry())

	_, err := g.Execute(context.Background(), core.NewContext())
	require.Error(t, err)
	require.Contains(t, err.Error(), "graph preflight failed")

	report := g.LastPreflightReport()
	require.NotNil(t, report)
	require.True(t, report.HasBlockingIssues())
}

func TestGraphPreflightProducesDeterministicPlacementDecision(t *testing.T) {
	registry := capability.NewRegistry()
	require.NoError(t, registry.Register(preflightTool{
		name: "local.echo",
		desc: core.CapabilityDescriptor{
			ID:          "tool:local.echo",
			Name:        "echo",
			Kind:        core.CapabilityKindTool,
			TrustClass:  core.TrustClassWorkspaceTrusted,
			RiskClasses: []core.RiskClass{core.RiskClassReadOnly},
		},
	}))
	require.NoError(t, registry.Register(preflightTool{
		name: "remote.echo",
		desc: core.CapabilityDescriptor{
			ID:          "tool:remote.echo",
			Name:        "echo",
			Kind:        core.CapabilityKindTool,
			TrustClass:  core.TrustClassRemoteApproved,
			RiskClasses: []core.RiskClass{core.RiskClassReadOnly},
			Source:      core.CapabilitySource{ProviderID: "remote-1"},
		},
	}))

	g := graph.NewGraph()
	node := preflightNode{
		id: "tool",
		contract: graph.NodeContract{
			RequiredCapabilities: []core.CapabilitySelector{{Kind: core.CapabilityKindTool, Name: "echo"}},
			PreferredPlacement:   graph.PlacementPreferenceLocal,
			RequiredTrustClass:   core.TrustClassRemoteApproved,
			MaxRiskClass:         core.RiskClassExecute,
			SideEffectClass:      graph.SideEffectExternal,
			Idempotency:          graph.IdempotencySingleShot,
		},
	}
	done := graph.NewTerminalNode("done")
	require.NoError(t, g.AddNode(node))
	require.NoError(t, g.AddNode(done))
	require.NoError(t, g.SetStart(node.ID()))
	require.NoError(t, g.AddEdge(node.ID(), done.ID(), nil, false))
	g.SetCapabilityCatalog(registry)

	report, err := g.Preflight()
	require.NoError(t, err)
	require.NotNil(t, report)
	require.Len(t, report.Placements, 1)
	require.Equal(t, "tool:local.echo", report.Placements[0].SelectedCapabilityID)
}

func TestGraphPreflightRequiresCheckpointForPersistedRecoverability(t *testing.T) {
	registry := capability.NewRegistry()
	require.NoError(t, registry.Register(preflightTool{
		name: "local.echo",
		desc: core.CapabilityDescriptor{
			ID:         "tool:local.echo",
			Name:       "echo",
			Kind:       core.CapabilityKindTool,
			TrustClass: core.TrustClassRemoteDeclared,
		},
	}))
	g := graph.NewGraph()
	node := preflightNode{
		id: "tool",
		contract: graph.NodeContract{
			RequiredCapabilities: []core.CapabilitySelector{{Kind: core.CapabilityKindTool, Name: "echo"}},
			Recoverability:       graph.NodeRecoverabilityPersisted,
			CheckpointPolicy:     graph.CheckpointPolicyRequired,
			SideEffectClass:      graph.SideEffectExternal,
			Idempotency:          graph.IdempotencySingleShot,
		},
	}
	done := graph.NewTerminalNode("done")
	require.NoError(t, g.AddNode(node))
	require.NoError(t, g.AddNode(done))
	require.NoError(t, g.SetStart(node.ID()))
	require.NoError(t, g.AddEdge(node.ID(), done.ID(), nil, false))
	g.SetCapabilityCatalog(registry)

	_, err := g.Preflight()
	require.Error(t, err)
	require.Contains(t, err.Error(), "persisted recovery required")
}

func TestGraphPreflightProducesRemoteAndStickyPlacementDecisions(t *testing.T) {
	registry := capability.NewRegistry()
	require.NoError(t, registry.RegisterCapability(core.CapabilityDescriptor{
		ID:         "tool:local.echo",
		Name:       "echo",
		Kind:       core.CapabilityKindTool,
		TrustClass: core.TrustClassRemoteDeclared,
	}))
	require.NoError(t, registry.RegisterCapability(core.CapabilityDescriptor{
		ID:              "tool:remote.echo",
		Name:            "echo",
		Kind:            core.CapabilityKindTool,
		TrustClass:      core.TrustClassRemoteApproved,
		Source:          core.CapabilitySource{ProviderID: "remote-1"},
		SessionAffinity: "session-a",
	}))

	g := graph.NewGraph()
	remote := preflightNode{
		id: "remote",
		contract: graph.NodeContract{
			RequiredCapabilities: []core.CapabilitySelector{{Kind: core.CapabilityKindTool, Name: "echo"}},
			PreferredPlacement:   graph.PlacementPreferenceRemote,
			SideEffectClass:      graph.SideEffectExternal,
			Idempotency:          graph.IdempotencySingleShot,
		},
	}
	sticky := preflightNode{
		id: "sticky",
		contract: graph.NodeContract{
			RequiredCapabilities: []core.CapabilitySelector{{Kind: core.CapabilityKindTool, Name: "echo"}},
			PreferredPlacement:   graph.PlacementPreferenceSticky,
			SideEffectClass:      graph.SideEffectExternal,
			Idempotency:          graph.IdempotencySingleShot,
		},
	}
	done := graph.NewTerminalNode("done")
	require.NoError(t, g.AddNode(remote))
	require.NoError(t, g.AddNode(sticky))
	require.NoError(t, g.AddNode(done))
	require.NoError(t, g.SetStart(remote.ID()))
	require.NoError(t, g.AddEdge(remote.ID(), sticky.ID(), nil, false))
	require.NoError(t, g.AddEdge(sticky.ID(), done.ID(), nil, false))
	g.SetCapabilityCatalog(registry)

	report, err := g.Preflight()
	require.NoError(t, err)
	require.Len(t, report.Placements, 2)
	require.Equal(t, "tool:remote.echo", report.Placements[0].SelectedCapabilityID)
	require.Equal(t, "tool:remote.echo", report.Placements[1].SelectedCapabilityID)
}

func TestGraphPreflightRejectsCapabilitiesAboveMaxRisk(t *testing.T) {
	registry := capability.NewRegistry()
	require.NoError(t, registry.Register(preflightTool{
		name: "dangerous.echo",
		desc: core.CapabilityDescriptor{
			ID:          "tool:dangerous.echo",
			Name:        "echo",
			Kind:        core.CapabilityKindTool,
			TrustClass:  core.TrustClassWorkspaceTrusted,
			RiskClasses: []core.RiskClass{core.RiskClassDestructive},
		},
	}))

	g := graph.NewGraph()
	node := preflightNode{
		id: "tool",
		contract: graph.NodeContract{
			RequiredCapabilities: []core.CapabilitySelector{{Kind: core.CapabilityKindTool, Name: "echo"}},
			MaxRiskClass:         core.RiskClassReadOnly,
			SideEffectClass:      graph.SideEffectExternal,
			Idempotency:          graph.IdempotencySingleShot,
		},
	}
	done := graph.NewTerminalNode("done")
	require.NoError(t, g.AddNode(node))
	require.NoError(t, g.AddNode(done))
	require.NoError(t, g.SetStart(node.ID()))
	require.NoError(t, g.AddEdge(node.ID(), done.ID(), nil, false))
	g.SetCapabilityCatalog(registry)

	_, err := g.Preflight()
	require.Error(t, err)
	require.Contains(t, err.Error(), "trust/risk constraints")
}

func TestPreflightReportHasBlockingIssuesFalseForNonBlockingOnly(t *testing.T) {
	report := graph.PreflightReport{
		Issues: []graph.PreflightIssue{
			{NodeID: "n1", Code: "warning", Message: "non-blocking", Blocking: false},
		},
	}
	require.False(t, report.HasBlockingIssues())
}
