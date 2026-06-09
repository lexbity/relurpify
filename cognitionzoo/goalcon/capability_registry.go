package goalcon

import capability "codeburg.org/lexbit/relurpify/capability/registry"

func (a *GoalConAgent) CapabilityRegistry() *capability.CapabilityRegistry {
	if a == nil {
		return nil
	}
	return a.Tools
}
