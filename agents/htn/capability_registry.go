package htn

import "github.com/lexcodex/relurpify/framework/capability"

func (a *HTNAgent) CapabilityRegistry() *capability.Registry {
	if a == nil {
		return nil
	}
	return a.Tools
}
