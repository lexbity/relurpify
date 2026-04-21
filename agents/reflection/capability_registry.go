package reflection

import "codeburg.org/lexbit/relurpify/framework/capability"

func (a *ReflectionAgent) CapabilityRegistry() *capability.Registry {
	if a == nil || a.Delegate == nil {
		return nil
	}
	if provider, ok := a.Delegate.(interface{ CapabilityRegistry() *capability.Registry }); ok {
		return provider.CapabilityRegistry()
	}
	return nil
}
