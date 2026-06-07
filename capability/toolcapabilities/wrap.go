package toolcapabilities

import (
	"context"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/ports"
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
	risk   []agentspec.RiskClass
	effect []agentspec.EffectClass
}

func (w *withCapability) TrustClass() agentspec.TrustClass {
	if w.trust != "" {
		return w.trust
	}
	return agentspec.TrustClassBuiltinTrusted
}

func (w *withCapability) RiskClasses() []agentspec.RiskClass {
	return append([]agentspec.RiskClass(nil), w.risk...)
}

func (w *withCapability) EffectClasses() []agentspec.EffectClass {
	return append([]agentspec.EffectClass(nil), w.effect...)
}

// wrapWithCapability returns a tool whose capability classes are sourced from
// the manifest. When the manifest has no capability data, the original tool is
// returned unchanged.
func wrapWithCapability(tool ports.Tool, manifest toolcapabilities.ToolManifest) ports.Tool {
	if tool == nil {
		return nil
	}
	cap := manifest.Capability
	if cap.TrustClass == "" && len(cap.RiskClass) == 0 && len(cap.EffectClass) == 0 {
		return tool
	}
	trust := agentspec.TrustClass(cap.TrustClass)
	risk := make([]agentspec.RiskClass, len(cap.RiskClass))
	for i, c := range cap.RiskClass {
		risk[i] = agentspec.RiskClass(c)
	}
	effect := make([]agentspec.EffectClass, len(cap.EffectClass))
	for i, c := range cap.EffectClass {
		effect[i] = agentspec.EffectClass(c)
	}
	return &withCapability{
		Tool:   tool,
		trust:  trust,
		risk:   risk,
		effect: effect,
	}
}

// Compile-time check that withCapability implements the required interfaces.
var _ ports.Tool = (*withCapability)(nil)
var _ interface{ TrustClass() agentspec.TrustClass } = (*withCapability)(nil)
var _ interface{ RiskClasses() []agentspec.RiskClass } = (*withCapability)(nil)
var _ interface {
	EffectClasses() []agentspec.EffectClass
} = (*withCapability)(nil)

// Ensure the wrapped tool is available when wrapped.
func (w *withCapability) IsAvailable(ctx context.Context) bool {
	return w.Tool.IsAvailable(ctx)
}
