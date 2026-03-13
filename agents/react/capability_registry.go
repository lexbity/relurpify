package react

import "github.com/lexcodex/relurpify/framework/capability"

func (a *ReActAgent) CapabilityRegistry() *capability.Registry {
	if a == nil {
		return nil
	}
	return a.Tools
}
