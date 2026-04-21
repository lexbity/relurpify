package react

import "codeburg.org/lexbit/relurpify/framework/capability"

func (a *ReActAgent) CapabilityRegistry() *capability.Registry {
	if a == nil {
		return nil
	}
	return a.Tools
}
