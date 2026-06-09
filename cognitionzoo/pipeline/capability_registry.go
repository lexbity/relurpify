package pipeline

import capability "codeburg.org/lexbit/relurpify/capability/registry"

func (a *PipelineAgent) CapabilityRegistry() *capability.CapabilityRegistry {
	if a == nil {
		return nil
	}
	return a.Tools
}
