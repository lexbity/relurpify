package rewoo

import capability "codeburg.org/lexbit/relurpify/capability/registry"

func (a *RewooAgent) CapabilityRegistry() *capability.CapabilityRegistry {
	if a == nil {
		return nil
	}
	return a.Tools
}
