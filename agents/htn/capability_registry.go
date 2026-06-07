package htn

import "codeburg.org/lexbit/relurpify/capability"

func (a *HTNAgent) CapabilityRegistry() *capability.Registry {
	if a == nil {
		return nil
	}
	return a.Tools
}
