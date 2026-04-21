package architect

import "codeburg.org/lexbit/relurpify/framework/capability"

func (a *ArchitectAgent) CapabilityRegistry() *capability.Registry {
	if a == nil {
		return nil
	}
	if a.ExecutorTools != nil {
		return a.ExecutorTools
	}
	return a.PlannerTools
}
