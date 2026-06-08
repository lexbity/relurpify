package goalcon

import "codeburg.org/lexbit/relurpify/capability"

func (a *GoalConAgent) CapabilityRegistry() *capability.CapabilityRegistry {
	if a == nil {
		return nil
	}
	return a.Tools
}
