package chainer

import "github.com/lexcodex/relurpify/framework/capability"

func (a *ChainerAgent) CapabilityRegistry() *capability.Registry {
	if a == nil {
		return nil
	}
	return a.Tools
}
