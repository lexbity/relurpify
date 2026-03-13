package pipeline

import "github.com/lexcodex/relurpify/framework/capability"

func (a *PipelineAgent) CapabilityRegistry() *capability.Registry {
	if a == nil {
		return nil
	}
	return a.Tools
}
