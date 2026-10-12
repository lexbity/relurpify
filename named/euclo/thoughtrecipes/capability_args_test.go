package thoughtrecipe

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
	"codeburg.org/lexbit/relurpify/model"
)

const capTargetParam = "target"

// recordingCapabilityHandler records the exact args it was invoked with so a
// test can assert the step passed only the capability's declared parameters.
type recordingCapabilityHandler struct {
	id     string
	schema *model.Schema
	args   map[string]any
}

func (h *recordingCapabilityHandler) Descriptor(context.Context, ports.State) descriptor.CapabilityDescriptor {
	return descriptor.CapabilityDescriptor{
		ID:            h.id,
		Name:          h.id,
		Kind:          agentspec.CapabilityKindTool,
		RuntimeFamily: agentspec.CapabilityRuntimeFamilyRelurpic,
		Availability:  descriptor.AvailabilitySpec{Available: true},
		InputSchema:   h.schema,
	}
}

func (h *recordingCapabilityHandler) Invoke(_ context.Context, _ ports.State, args map[string]any) (*ports.ToolResult, error) {
	h.args = args
	return &ports.ToolResult{Success: true, Data: map[string]any{"ok": true}}, nil
}

// runCapabilityStep registers the handler, builds a capability step whose
// Config is exactly config, executes it against a fresh envelope seeded by
// seed, and returns the args the handler recorded. It is the shared harness
// for the capability-args hygiene tests.
func runCapabilityStep(t *testing.T, handler *recordingCapabilityHandler, config map[string]any, seed func(*contextdata.Envelope)) map[string]any {
	t.Helper()
	reg := registry.NewRegistry()
	require.NoError(t, reg.RegisterInvocableCapability(context.Background(), handler))

	deps := &paradigm.Deps{Registry: reg}
	step := ExecutionStep{
		ID:           "capability.step",
		Kind:         StepKindCapability,
		Paradigm:     KeyEuclo,
		CapabilityID: handler.id,
		Scope:        AllowTools([]string{handler.id}),
		Config:       config,
	}
	node := NewCapabilityNode("capability.step", deps, step)

	env := contextdata.NewEnvelope("task-args-hygiene", "session-args-hygiene")
	seed(env)
	result, err := node.Execute(context.Background(), env)
	require.NoError(t, err)
	require.True(t, result.Success, "capability invocation must succeed: %s", result.Error)
	return handler.args
}

// TestCapabilityStepArgsDeclaredOnly is the capability-step argument hygiene
// gate (the O3 root-cause pin): a `do relurpic:` step must pass the handler
// ONLY the capability's declared schema parameters — with `on`/`with` refs
// resolved to their concrete values — never the run envelope's internal keys
// (task metadata, policy-gate results, captures). The pre-fix implementation
// dumped the whole envelope into the args, so every strict-schema capability
// invocation failed on incidental keys like euclo.policy_gate.*, and the
// handler could read state pollution.
func TestCapabilityStepArgsDeclaredOnly(t *testing.T) {
	handler := &recordingCapabilityHandler{
		id: "euclo:cap.greeter",
		schema: &model.Schema{
			Type: "object",
			Properties: map[string]*model.Schema{
				capTargetParam: {Type: "string", Description: "the captured state ref value"},
			},
		},
	}
	config := map[string]any{
		"capability_id": handler.id,
		capTargetParam:  "state.repair",
	}
	args := runCapabilityStep(t, handler, config, func(env *contextdata.Envelope) {
		contextdata.SetTyped(env, "state.repair", "the repaired summary")
		// Incidental envelope keys that must NOT leak into the invocation.
		contextdata.SetTyped(env, "euclo.policy_gate.verification_required", true)
		contextdata.SetTyped(env, "euclo.policy.hitl_required", true)
		contextdata.SetTyped(env, "task.id", "task-args-hygiene")
	})
	require.Equal(t, map[string]any{capTargetParam: "the repaired summary"}, args,
		"handler must receive exactly the declared parameter with the ref resolved")
}

// TestCapabilityStepArgsDropsUndeclaredRef is the strict-schema posture: a
// `on`/`with` ref the capability does not declare in its input schema is not
// passed (the invocation must not crash on it), and the handler still runs.
func TestCapabilityStepArgsDropsUndeclaredRef(t *testing.T) {
	handler := &recordingCapabilityHandler{
		id: "euclo:cap.strict",
		schema: &model.Schema{
			Type: "object",
			Properties: map[string]*model.Schema{
				"command": {Type: "string", Description: "the only declared parameter"},
			},
		},
	}
	config := map[string]any{
		"capability_id": handler.id,
		capTargetParam:  "state.repair", // not declared by this capability
	}
	args := runCapabilityStep(t, handler, config, func(env *contextdata.Envelope) {
		contextdata.SetTyped(env, "state.repair", "value")
		contextdata.SetTyped(env, "euclo.policy_gate.verification_required", true)
	})
	require.Equal(t, map[string]any{}, args,
		"the undeclared ref must be dropped; no envelope keys may leak")
}
