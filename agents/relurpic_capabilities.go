package agents

import (
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
)

// RegisterBuiltinRelurpicCapabilities installs framework-native orchestrated
// capabilities that are reusable across agents without treating them as local tools.
func RegisterBuiltinRelurpicCapabilities(registry *capability.Registry, model core.LanguageModel, cfg *core.Config) error {
	if registry == nil || model == nil {
		return nil
	}

	handlers := []core.InvocableCapabilityHandler{
		plannerPlanCapabilityHandler{model: model, registry: registry, config: cfg},
		architectExecuteCapabilityHandler{model: model, registry: registry, config: cfg},
		reviewerReviewCapabilityHandler{model: model, config: cfg},
		verifierVerifyCapabilityHandler{model: model, config: cfg},
		executorInvokeCapabilityHandler{registry: registry},
	}
	for _, handler := range handlers {
		if err := registry.RegisterInvocableCapability(handler); err != nil {
			return err
		}
	}
	return nil
}
