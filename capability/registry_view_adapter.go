package capability

import (
	"codeburg.org/lexbit/relurpify/governance/policyresolve"
)

// registryViewAdapter wraps *CapabilityRegistry to implement policyresolve.RegistryView.
type registryViewAdapter struct {
	inner *CapabilityRegistry
}

// NewPolicyResolveRegistry returns a policyresolve.RegistryView backed by the
// given CapabilityRegistry. Descriptors are adapted to governanceports.DescriptorView
// so policyresolve can match selectors without importing capability.
func NewPolicyResolveRegistry(inner *CapabilityRegistry) policyresolve.RegistryView {
	return &registryViewAdapter{inner: inner}
}

func (a *registryViewAdapter) GetCapability(idOrName string) (any, bool) {
	if a.inner == nil {
		return nil, false
	}
	desc, ok := a.inner.GetCapability(idOrName)
	if !ok {
		return nil, false
	}
	return CapabilityDescriptorView(desc), true
}

func (a *registryViewAdapter) CallableCapabilities() []any {
	if a.inner == nil {
		return nil
	}
	descs := a.inner.CallableCapabilities()
	out := make([]any, len(descs))
	for i, d := range descs {
		out[i] = CapabilityDescriptorView(d)
	}
	return out
}

func (a *registryViewAdapter) EffectiveExposure(desc any) string {
	if a.inner == nil || desc == nil {
		return ""
	}
	// The desc was wrapped by CapabilityDescriptorView, unwrap to get the original.
	if adapter, ok := desc.(*descriptorViewAdapter); ok {
		return string(a.inner.EffectiveExposure(adapter.d))
	}
	return ""
}
