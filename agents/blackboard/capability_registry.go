package blackboard

import "codeburg.org/lexbit/relurpify/capability"

func (a *BlackboardAgent) CapabilityRegistry() *capability.CapabilityRegistry {
	if a == nil {
		return nil
	}
	return a.Tools
}
