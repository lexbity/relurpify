package goalcon

import "codeburg.org/lexbit/relurpify/framework/capability"

func (a *GoalConAgent) CapabilityRegistry() *capability.Registry {
	if a == nil {
		return nil
	}
	return a.Tools
}
