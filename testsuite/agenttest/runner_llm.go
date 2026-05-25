package agenttest

import (
	"time"

	"codeburg.org/lexbit/relurpify/platform/contracts"
	"codeburg.org/lexbit/relurpify/platform/llm"
)

// buildCaseBackend constructs a language model backend using the provider-agnostic factory.
// It creates a ManagedBackend via llm.New(), applies the profile if provided, sets debug logging,
// and returns the underlying LanguageModel.
func buildCaseBackend(execution resolvedCaseExecution, profile *llm.ModelProfile, debug bool) (contracts.LanguageModel, error) {
	cfg := llm.ProviderConfig{
		Provider: execution.Provider,
		Endpoint: execution.Endpoint,
		Model:    execution.Model,
		Timeout:  30 * time.Second,
		Debug:    debug,
	}

	backend, err := llm.New(cfg, llm.ProviderSecrets{})
	if err != nil {
		return nil, err
	}

	if profile != nil {
		backend.SetProfile(profile)
	}

	backend.SetDebugLogging(debug)

	return backend.Model(), nil
}

// buildCaseManagedBackend constructs a ManagedBackend using the provider-agnostic factory.
// Unlike buildCaseBackend, this returns the backend itself rather than extracting the LanguageModel.
// This is useful for operations that need the full backend interface (e.g., preflight, reset).
func buildCaseManagedBackend(execution resolvedCaseExecution, profile *llm.ModelProfile, debug bool) (llm.ManagedBackend, error) {
	cfg := llm.ProviderConfig{
		Provider: execution.Provider,
		Endpoint: execution.Endpoint,
		Model:    execution.Model,
		Timeout:  30 * time.Second,
		Debug:    debug,
	}

	backend, err := llm.New(cfg, llm.ProviderSecrets{})
	if err != nil {
		return nil, err
	}

	if profile != nil {
		backend.SetProfile(profile)
	}

	backend.SetDebugLogging(debug)

	return backend, nil
}
