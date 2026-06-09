package blackboard

import capability "codeburg.org/lexbit/relurpify/capability/registry"

func (a *BlackboardAgent) CapabilityRegistry() *capability.CapabilityRegistry {
	if a == nil {
		return nil
	}
	return a.Tools
}
