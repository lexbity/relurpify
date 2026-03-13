package goalcon

import "github.com/lexcodex/relurpify/framework/capability"

func (a *GoalConAgent) CapabilityRegistry() *capability.Registry {
	if a == nil {
		return nil
	}
	return a.Tools
}
