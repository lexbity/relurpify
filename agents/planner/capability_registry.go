package planner

import "codeburg.org/lexbit/relurpify/capability"

func (a *PlannerAgent) CapabilityRegistry() *capability.CapabilityRegistry {
	if a == nil {
		return nil
	}
	return a.Tools
}
