// Package prep is the hermetic dry-run tier: it drives real .erpe recipe text
// through the real loader, the real euclo.Agent.Execute, the real RootGraph,
// real paradigm agents, and scripted capabilities against a scripted model — no
// LLM, no network, no security bundle. It proves dispatch, execution,
// capability invocation, prompt content, and the full read/write knowledge
// loop (spec §5.8). It is NOT a governance tier: nothing here asserts
// authorization or sandbox behavior; the live tier stays the security tier.
package prep

import (
	"context"
	"fmt"
	"sync"

	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/capability/registry"
)

// ScriptedCapability is a registered registry capability that returns a fixed
// payload (or error) — the dry run's scripted capability surface. Every
// invocation is recorded onto it so the report can assert exactly which
// capabilities ran, with which arguments. It registers as a legacy tool so
// paradigm agents reach it through the model-callable tool surface — the same
// path a real capability takes.
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

// registerScripted registers the scripted capabilities as invocable registry
// entries and fails loud when admission drops one — a silently missing
// capability would look like a paradigm bug instead of a harness bug.
func registerScripted(ctx context.Context, reg *registry.CapabilityRegistry, scripted []*ScriptedCapability) error {
	items := make([]registry.RegistrationBatchItem, 0, len(scripted))
	for _, capability := range scripted {
		if capability == nil || capability.Name == "" {
			return fmt.Errorf("prep: scripted capability requires a name")
		}
		items = append(items, registry.RegistrationBatchItem{
			LegacyTool: &dryRunTool{capability: capability},
		})
	}
	if err := reg.RegisterBatch(ctx, items); err != nil {
		return err
	}
	for _, capability := range scripted {
		if _, ok := reg.GetCapability(capability.Name); !ok {
			return fmt.Errorf("prep: scripted capability %q was not admitted by the registry (missing tool manifest?)", capability.Name)
		}
	}
	return nil
}

// dryRunTool adapts a ScriptedCapability to the ports.Tool surface.
type dryRunTool struct {
	capability *ScriptedCapability
}

func (t *dryRunTool) Name() string        { return t.capability.Name }
func (t *dryRunTool) Description() string { return "prep dry-run scripted capability" }
func (t *dryRunTool) Category() string    { return "prep" }
func (t *dryRunTool) Parameters() []ports.ToolParameter {
	return []ports.ToolParameter{
		{Name: "target", Type: ports.ToolParamString, Description: "arbitrary scripted argument", Required: false},
	}
}

func (t *dryRunTool) Execute(_ context.Context, args map[string]any) (*ports.ToolResult, error) {
	t.capability.record(args)
	if t.capability.Err != nil {
		return &ports.ToolResult{Success: false, Error: t.capability.Err.Error()}, t.capability.Err
	}
	return &ports.ToolResult{Success: true, Data: t.capability.Response}, nil
}

func (t *dryRunTool) IsAvailable(context.Context) bool   { return true }
func (t *dryRunTool) Permissions() ports.ToolPermissions { return ports.ToolPermissions{} }
func (t *dryRunTool) Tags() []string                     { return []string{"prep", "scripted"} }
