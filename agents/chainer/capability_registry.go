package chainer

import "codeburg.org/lexbit/relurpify/capability"

func (a *ChainerAgent) CapabilityRegistry() *capability.CapabilityRegistry {
	if a == nil {
		return nil
	}
	return a.Tools
}
