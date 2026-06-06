package promptprovider

import "codeburg.org/lexbit/relurpify/framework/prompt"

// RegisterAll registers all built-in prompt providers with r. Calling it more
// than once is safe — duplicate registrations are silently skipped.
func RegisterAll(r prompt.Registry) error {
	providers := []prompt.DescribingProvider{
		reactToolsProvider{},
		reactCapabilityCatalogProvider{},
		reactPhaseProvider{},
		reactPlanGoalProvider{},
		reactCurrentStepProvider{},
		reactPriorStepProvider{},
		reactExternalStateProvider{},
		reactDeclarativeMemoryProvider{},
		reactWorkflowRetrievalProvider{},
		reactStreamedContextProvider{},
		reactObservationsProvider{},
		reactHistoryProvider{},
		reactContextFilesProvider{},
		pipelineStageOutputsProvider{},
		pipelineTaskInstructionProvider{},
	}
	for _, p := range providers {
		if err := r.RegisterProvider(p.Describe().Name, p); err != nil {
			if prompt.IsAlreadyRegistered(err) {
				continue
			}
			return err
		}
	}
	return nil
}
