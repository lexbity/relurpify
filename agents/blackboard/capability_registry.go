package blackboard

import "codeburg.org/lexbit/relurpify/framework/capability"

func (a *BlackboardAgent) CapabilityRegistry() *capability.Registry {
	if a == nil {
		return nil
	}
	return a.Tools
}
