package toolcapabilities

import (
	"context"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/governance/taxonomy"
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
	risk   []taxonomy.RiskClass
	effect []taxonomy.EffectClass
}

func (w *withCapability) TrustClass() agentspec.TrustClass {
	if w.trust != "" {
		return w.trust
	}
	return agentspec.TrustClassBuiltinTrusted
}

func (w *withCapability) RiskClasses() []taxonomy.RiskClass {
	return append([]taxonomy.RiskClass(nil), w.risk...)
}

func (w *withCapability) EffectClasses() []taxonomy.EffectClass {
	return append([]taxonomy.EffectClass(nil), w.effect...)
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
	risk := make([]taxonomy.RiskClass, len(cap.RiskClass))
	for i, c := range cap.RiskClass {
		risk[i] = taxonomy.RiskClass(c)
	}
	effect := make([]taxonomy.EffectClass, len(cap.EffectClass))
	for i, c := range cap.EffectClass {
		effect[i] = taxonomy.EffectClass(c)
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
var _ interface{ RiskClasses() []taxonomy.RiskClass } = (*withCapability)(nil)
var _ interface {
	EffectClasses() []taxonomy.EffectClass
} = (*withCapability)(nil)

// Ensure the wrapped tool is available when wrapped.
func (w *withCapability) IsAvailable(ctx context.Context) bool {
	return w.Tool.IsAvailable(ctx)
}
