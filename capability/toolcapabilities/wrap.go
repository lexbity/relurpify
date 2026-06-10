package toolcapabilities

import (
	"context"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/governance/classification"
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
	return append([]risk.RiskClass(nil), w.risk...)
}

func (w *withCapability) EffectClasses() []classification.EffectClass {
	return append([]classification.EffectClass(nil), w.effect...)
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
	trust := agentspec.TrustClass(cap.TrustClass)
	riskClasses := make([]risk.RiskClass, len(cap.RiskClass))
	for i, c := range cap.RiskClass {
		riskClasses[i] = risk.RiskClass(c)
	}
	effect := make([]classification.EffectClass, len(cap.EffectClass))
	for i, c := range cap.EffectClass {
		effect[i] = classification.EffectClass(c)
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
var _ interface{ TrustClass() agentspec.TrustClass } = (*withCapability)(nil)
var _ interface{ RiskClasses() []risk.RiskClass } = (*withCapability)(nil)
var _ interface {
	EffectClasses() []classification.EffectClass
} = (*withCapability)(nil)

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
