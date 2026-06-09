package react

import capability "codeburg.org/lexbit/relurpify/capability/registry"

func (a *ReActAgent) CapabilityRegistry() *capability.CapabilityRegistry {
	if a == nil {
		return nil
	}
	return a.Tools
}
