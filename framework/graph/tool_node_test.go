package graph_test

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/authorization"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	"github.com/stretchr/testify/require"
)

type graphToolStub struct {
	name  string
	calls int
}

func (t *graphToolStub) Name() string        { return t.name }
func (t *graphToolStub) Description() string { return "graph tool stub" }
func (t *graphToolStub) Category() string    { return "test" }
func (t *graphToolStub) Parameters() []core.ToolParameter {
	return []core.ToolParameter{{Name: "value", Type: "string"}}
}
func (t *graphToolStub) Execute(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.ToolResult, error) {
	t.calls++
	return &core.ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"value": args["value"],
		},
	}, nil
}
func (t *graphToolStub) IsAvailable(ctx context.Context, state *core.Context) bool {
	return true
}
func (t *graphToolStub) Permissions() core.ToolPermissions { return core.ToolPermissions{} }
func (t *graphToolStub) Tags() []string                    { return []string{core.TagExecute} }

type denyPolicyEngine struct{}

func (denyPolicyEngine) Evaluate(ctx context.Context, req core.PolicyRequest) (core.PolicyDecision, error) {
	return core.PolicyDecisionDeny("denied by test policy"), nil
}

func TestToolNodeUsesCapabilityRegistryAndAttachesEnvelope(t *testing.T) {
	registry := capability.NewRegistry()
	tool := &graphToolStub{name: "registry_tool"}
	require.NoError(t, registry.Register(tool))

	node := graph.NewToolNode("run-tool", tool, map[string]interface{}{"value": "ok"}, registry)

	result, err := node.Execute(context.Background(), core.NewContext())
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Success)
	require.Equal(t, "ok", result.Data["value"])

	envelope, ok := result.Metadata["capability_result_envelope"].(*core.CapabilityResultEnvelope)
	require.True(t, ok)
	require.Equal(t, "tool:registry_tool", envelope.Descriptor.ID)
	require.Equal(t, "registry_tool", envelope.Descriptor.Name)
	require.Equal(t, core.InsertionActionDirect, envelope.Insertion.Action)

	decision, ok := result.Metadata["insertion_decision"].(core.InsertionDecision)
	require.True(t, ok)
	require.Equal(t, envelope.Insertion.Action, decision.Action)
	require.Equal(t, 1, tool.calls)
}

func TestToolNodeRegistryPathEnforcesCapabilityPolicy(t *testing.T) {
	registry := capability.NewRegistry()
	tool := &graphToolStub{name: "policy_tool"}
	require.NoError(t, registry.Register(tool))
	registry.SetPolicyEngine(authorization.PolicyEngine(denyPolicyEngine{}))

	node := graph.NewToolNode("run-tool", tool, map[string]interface{}{"value": "blocked"}, registry)

	_, err := node.Execute(context.Background(), core.NewContext())
	require.Error(t, err)
	require.Contains(t, err.Error(), "denied by test policy")
	require.Equal(t, 0, tool.calls)
}
