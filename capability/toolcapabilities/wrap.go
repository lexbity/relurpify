package toolcapabilities

import (
	"context"
	"slices"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/governance/classification"
	"codeburg.org/lexbit/relurpify/governance/permissions"
	governanceports "codeburg.org/lexbit/relurpify/governance/ports"
	"codeburg.org/lexbit/relurpify/governance/risk"
)

// withCapability wraps a ports.Tool and overrides its capability class
// providers to source from the manifest, not from the Tool interface's Tags()
// or Permissions().
//
// This ensures the CapabilityDescriptor built by core.ToolDescriptor reflects
// the manifest's capability.* fields rather than Go-derived defaults.
type withCapability struct {
	ports.Tool
	trust  agentspec.TrustClass
	risk   []risk.RiskClass
	effect []classification.EffectClass
}

func (w *withCapability) TrustClass() agentspec.TrustClass {
	if w.trust != "" {
		return w.trust
	}
	return agentspec.TrustClassBuiltinTrusted
}

func (w *withCapability) RiskClasses() []risk.RiskClass {
	return slices.Clone(w.risk)
}

func (w *withCapability) EffectClasses() []classification.EffectClass {
	return slices.Clone(w.effect)
}

// wrapWithCapability returns a tool whose capability classes are sourced from
// the manifest. When the manifest has no capability data, the original tool is
// returned unchanged.
func wrapWithCapability(tool ports.Tool, manifest ports.ToolManifest) ports.Tool {
	if tool == nil {
		return nil
	}
	cap := manifest.Capability
	if cap.TrustClass == "" && len(cap.RiskClass) == 0 && len(cap.EffectClass) == 0 {
		return tool
	}
	trust := agentspec.TrustClass(classification.NormalizeClassString(cap.TrustClass))
	riskClasses := make([]risk.RiskClass, len(cap.RiskClass))
	for i, c := range cap.RiskClass {
		riskClasses[i] = risk.RiskClass(classification.NormalizeClassString(c))
	}
	effect := make([]classification.EffectClass, len(cap.EffectClass))
	for i, c := range cap.EffectClass {
		effect[i] = classification.EffectClass(classification.NormalizeClassString(c))
	}
	return &withCapability{
		Tool:   tool,
		trust:  trust,
		risk:   riskClasses,
		effect: effect,
	}
}

// Compile-time check that withCapability implements the required interfaces.
var _ ports.Tool = (*withCapability)(nil)
var _ ports.ToolUnwrapper = (*withCapability)(nil)
var _ interface{ TrustClass() agentspec.TrustClass } = (*withCapability)(nil)
var _ interface{ RiskClasses() []risk.RiskClass } = (*withCapability)(nil)
var _ interface {
	EffectClasses() []classification.EffectClass
} = (*withCapability)(nil)

// Unwrap exposes the decorated tool so registry stamping can reach the
// concrete implementation (the decoration contract, capability/ports).
func (w *withCapability) Unwrap() ports.Tool { return w.Tool }

// SetSandboxScope forwards the sandbox-scope stamp to the decorated tool.
// The scope lives on the concrete tool (its enforcement reads its own field),
// never on this wrapper. Belt-and-braces depth-one forwarding: recursive
// unwrap in the registry is the general path.
func (w *withCapability) SetSandboxScope(scope *permissions.FileScopePolicy) {
	if aware, ok := w.Tool.(interface {
		SetSandboxScope(*permissions.FileScopePolicy)
	}); ok {
		aware.SetSandboxScope(scope)
	}
}

// SetToolName forwards the tool-name stamp to the decorated tool.
func (w *withCapability) SetToolName(name string) {
	if aware, ok := w.Tool.(interface{ SetToolName(string) }); ok {
		aware.SetToolName(name)
	}
}

// SetCommandRunner forwards the sandbox command runner to the decorated tool.
// The composition root injects the runner through the tool Build returns (the
// wrapper), before registration; without this forwarding a runner-backed
// go_native tool (the shipped test runners) stays unavailable, so a recipe's
// `may invoke` resolves but the model's tool list never contains it.
func (w *withCapability) SetCommandRunner(runner ports.CommandRunner) {
	if aware, ok := w.Tool.(interface{ SetCommandRunner(ports.CommandRunner) }); ok {
		aware.SetCommandRunner(runner)
	}
}

// Ensure the wrapped tool is available when wrapped.
func (w *withCapability) IsAvailable(ctx context.Context) bool {
	return w.Tool.IsAvailable(ctx)
}

// ToolAccessRequest builds a governance AccessRequest from a tool descriptor
// for authorization checking. This is one of ~3 per-caller adapters (execution,
// toolcapabilities, TUI) that translate domain types into governance's own
// request vocabulary. There is deliberately no shared adapters package.
func ToolAccessRequest(tool ports.Tool, principal governanceports.Principal) governanceports.AccessRequest {
	if tool == nil {
		return governanceports.AccessRequest{
			Principal: principal,
			Action:    governanceports.ActionToolInvoke,
		}
	}
	return governanceports.AccessRequest{
		Principal: principal,
		Action:    governanceports.ActionToolInvoke,
		Resource:  governanceports.Resource{Kind: "tool", ID: tool.Name()},
	}
}
