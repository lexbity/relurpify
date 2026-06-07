package rewoo

import "codeburg.org/lexbit/relurpify/capability"

func (a *RewooAgent) CapabilityRegistry() *capability.Registry {
	if a == nil {
		return nil
	}
	return a.Tools
}
