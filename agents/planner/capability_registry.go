package planner

import "codeburg.org/lexbit/relurpify/framework/capability"

func (a *PlannerAgent) CapabilityRegistry() *capability.Registry {
	if a == nil {
		return nil
	}
	return a.Tools
}
