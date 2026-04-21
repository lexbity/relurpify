package pipeline

import "codeburg.org/lexbit/relurpify/framework/capability"

func (a *PipelineAgent) CapabilityRegistry() *capability.Registry {
	if a == nil {
		return nil
	}
	return a.Tools
}
