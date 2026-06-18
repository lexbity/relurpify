package conformance

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/descriptor"
	"codeburg.org/lexbit/relurpify/capability/ports"
	registry "codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/named/euclo/surface"
	thoughtrecipepkg "codeburg.org/lexbit/relurpify/named/euclo/thoughtrecipes"
)

type grantRecordingCapability struct {
	id     string
	called bool
}

func (h *grantRecordingCapability) Descriptor(ctx context.Context, env ports.State) descriptor.CapabilityDescriptor {
	_ = ctx
	_ = env
	return descriptor.CapabilityDescriptor{
		ID:            h.id,
		Name:          h.id,
		Kind:          agentspec.CapabilityKindTool,
		RuntimeFamily: agentspec.CapabilityRuntimeFamilyRelurpic,
		Availability:  descriptor.AvailabilitySpec{Available: true},
	}
}

func (h *grantRecordingCapability) Invoke(ctx context.Context, env ports.State, args map[string]any) (*ports.ToolResult, error) {
	_ = ctx
	_ = env
	_ = args
	h.called = true
	return &ports.ToolResult{
		Success: true,
		Data: map[string]any{
			"executed": true,
		},
	}, nil
}

func TestRecipeGrant_ExcludedCapabilityDeniedByPolicyWithoutEscalation(t *testing.T) {
	base := registry.NewRegistry()
	handler := &grantRecordingCapability{id: "euclo:cap.restricted"}
	require.NoError(t, base.RegisterInvocableCapability(context.Background(), handler))

	scoped := base.WithAllowlist([]string{"euclo:cap.allowed"})
	require.NotNil(t, scoped)

	env := contextdata.NewEnvelope("task-grant", "session-grant")
	step := thoughtrecipepkg.ExecutionStep{
		ID:           "grant.step",
		Kind:         thoughtrecipepkg.StepKindCapability,
		CapabilityID: "euclo:cap.restricted",
		OnError: &surface.StepErrorPolicy{
			Action:   "skip",
			RetryMax: 0,
			Fallback: "grant.step.recover",
		},
		Config: map[string]any{},
	}

	node := thoughtrecipepkg.NewThoughtRecipeStepNode("grant.step.execute", &paradigm.Deps{Registry: scoped}, step)
	result, err := node.Execute(context.Background(), env)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Success)
	require.False(t, handler.called)

	skipped, ok := execution.ResultField(result.Data, "skipped")
	require.True(t, ok)
	require.Equal(t, true, skipped)

	reason, ok := execution.ResultField(result.Data, "skipped_reason")
	require.True(t, ok)
	require.Contains(t, reason, "not permitted in this context")

	action, ok := execution.ResultField(result.Data, "on_error_action")
	require.True(t, ok)
	require.Equal(t, "skip", action)

	fallback, ok := execution.ResultField(result.Data, "on_error_fallback")
	require.True(t, ok)
	require.Equal(t, "grant.step.recover", fallback)

	require.Equal(t, "skipped", result.Metadata["on_error_resolved"])
}
