package chainer

import "codeburg.org/lexbit/relurpify/capability"

func (a *ChainerAgent) CapabilityRegistry() *capability.Registry {
	if a == nil {
		return nil
	}
	return a.Tools
}
