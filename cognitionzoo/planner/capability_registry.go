package planner

import capability "codeburg.org/lexbit/relurpify/capability/registry"

func (a *PlannerAgent) CapabilityRegistry() *capability.CapabilityRegistry {
	if a == nil {
		return nil
	}
	return a.Tools
}
