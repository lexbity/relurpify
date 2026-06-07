package planner

import "codeburg.org/lexbit/relurpify/capability"

func (a *PlannerAgent) CapabilityRegistry() *capability.Registry {
	if a == nil {
		return nil
	}
	return a.Tools
}
