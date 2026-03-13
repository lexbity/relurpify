package rewoo

import "github.com/lexcodex/relurpify/framework/capability"

func (a *RewooAgent) CapabilityRegistry() *capability.Registry {
	if a == nil {
		return nil
	}
	return a.Tools
}
