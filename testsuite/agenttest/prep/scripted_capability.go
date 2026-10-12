package prep

import (
	"context"
	"fmt"
	"sync"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/descriptor"
	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/governance/classification"
	"codeburg.org/lexbit/relurpify/model"
)

// prepCapabilityCategory is the fixed category/tag vocabulary of the prep
// dry-run scripted capabilities.
const prepCapabilityCategory = "prep"

// ScriptedCapability is a registered registry capability that returns a fixed
// payload (or error) — the dry run's scripted capability surface. Every
// invocation is recorded onto it so the report can assert exactly which
// capabilities ran, with which arguments.
type ScriptedCapability struct {
	Name     string
	Response map[string]any
	Err      error

	mu   sync.Mutex
	args []map[string]any
}

// CapabilityCall is one recorded invocation of a scripted capability.
type CapabilityCall struct {
	Name     string
	Args     map[string]any
	Response map[string]any
	Error    string
}

// Calls returns the recorded invocations in order.
func (s *ScriptedCapability) Calls() []CapabilityCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]CapabilityCall, len(s.args))
	for i, args := range s.args {
		call := CapabilityCall{Name: s.Name, Args: args, Response: s.Response}
		if s.Err != nil {
			call.Error = s.Err.Error()
		}
		out[i] = call
	}
	return out
}

func (s *ScriptedCapability) record(args map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.args = append(s.args, args)
}

// prepProbe is the scripted capability every prep fixture declares in its
// `may invoke` scope. Fixtures that never invoke it still must register it —
// the loud load (D2) rejects a declared-but-unregistered name, and the harness
// passes through that contract rather than bypassing it.
func prepProbe() *ScriptedCapability {
	return &ScriptedCapability{Name: "prep_probe", Response: map[string]any{"finding": "platypus-quartz 42"}}
}

// scriptedCapabilityHandler adapts a ScriptedCapability to the invocable
// capability-handler surface — the same registration path the production
// relurpic capabilities take. Registering as a legacy tool instead would
// squat the capability ID without the bare-name index entry (D3), so a
// recipe declaring "test_run" could not resolve it even though
// "euclo:cap.test_run" was registered: the loud load (D2) then correctly
// rejects the recipe, and the harness must not construct that broken state.
type scriptedCapabilityHandler struct {
	capability *ScriptedCapability
}

func (h *scriptedCapabilityHandler) Descriptor(context.Context, ports.State) descriptor.CapabilityDescriptor {
	return descriptor.CapabilityDescriptor{
		ID:            h.capability.Name,
		Kind:          agentspec.CapabilityKindTool,
		RuntimeFamily: agentspec.CapabilityRuntimeFamilyRelurpic,
		Name:          h.capability.Name,
		Version:       "1.0.0",
		Description:   "prep dry-run scripted capability",
		Category:      prepCapabilityCategory,
		Tags:          []string{prepCapabilityCategory, "scripted"},
		Source: descriptor.CapabilitySource{
			Scope: classification.CapabilityScopeBuiltin,
		},
		TrustClass: agentspec.TrustClassBuiltinTrusted,
		InputSchema: &model.Schema{
			Type: "object",
			Properties: map[string]*model.Schema{
				"target": {
					Type:        "string",
					Description: "arbitrary scripted argument",
				},
			},
		},
	}
}

func (h *scriptedCapabilityHandler) Invoke(_ context.Context, _ ports.State, args map[string]any) (*ports.ToolResult, error) {
	h.capability.record(args)
	if h.capability.Err != nil {
		return &ports.ToolResult{Success: false, Error: h.capability.Err.Error()}, h.capability.Err
	}
	return &ports.ToolResult{Success: true, Data: h.capability.Response}, nil
}

// registerScripted registers the scripted capabilities as invocable registry
// entries and fails loud when one does not resolve — a silently missing
// capability would look like a paradigm bug instead of a harness bug.
func registerScripted(ctx context.Context, reg *registry.CapabilityRegistry, scripted []*ScriptedCapability) error {
	items := make([]registry.RegistrationBatchItem, 0, len(scripted))
	for _, capability := range scripted {
		if capability == nil || capability.Name == "" {
			return fmt.Errorf("prep: scripted capability requires a name")
		}
		items = append(items, registry.RegistrationBatchItem{
			InvocableHandler: &scriptedCapabilityHandler{capability: capability},
		})
	}
	if err := reg.RegisterBatch(ctx, items); err != nil {
		return err
	}
	for _, capability := range scripted {
		if _, ok := reg.GetCapability(capability.Name); !ok {
			return fmt.Errorf("prep: scripted capability %q did not resolve after registration", capability.Name)
		}
	}
	return nil
}
