package chainer

import capability "codeburg.org/lexbit/relurpify/capability/registry"

func (a *ChainerAgent) CapabilityRegistry() *capability.CapabilityRegistry {
	if a == nil {
		return nil
	}
	return a.Tools
}
