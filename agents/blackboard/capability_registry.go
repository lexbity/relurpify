package blackboard

import "github.com/lexcodex/relurpify/framework/capability"

func (a *BlackboardAgent) CapabilityRegistry() *capability.Registry {
	if a == nil {
		return nil
	}
	return a.Tools
}
