package reflection

import "codeburg.org/lexbit/relurpify/capability"

func (a *ReflectionAgent) CapabilityRegistry() *capability.CapabilityRegistry {
	if a == nil || a.Delegate == nil {
		return nil
	}
	if provider, ok := a.Delegate.(interface{ CapabilityRegistry() *capability.CapabilityRegistry }); ok {
		return provider.CapabilityRegistry()
	}
	return nil
}
