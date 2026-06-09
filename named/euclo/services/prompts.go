package services

import (
	zoopromptprovider "codeburg.org/lexbit/relurpify/cognitionzoo/promptprovider"
	"codeburg.org/lexbit/relurpify/execution/prompt"
	eucloprovider "codeburg.org/lexbit/relurpify/named/euclo/promptprovider"
)

// defaultPromptRegistrar implements PromptRegistrar using Euclo's prompt providers.
type defaultPromptRegistrar struct{}

func (r *defaultPromptRegistrar) RegisterAll(registry prompt.Registry) error {
	if registry == nil {
		return nil // No registry to register with.
	}
	// Register generic paradigm providers.
	if err := zoopromptprovider.RegisterAll(registry); err != nil {
		return err
	}
	// Register Euclo-specific providers.
	if err := eucloprovider.RegisterAll(registry); err != nil {
		return err
	}
	return nil
}
