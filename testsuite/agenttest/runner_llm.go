package agenttest

import (
	"time"

	"codeburg.org/lexbit/relurpify/platform/llm"
)

// buildCaseManagedBackend constructs a ManagedBackend using the provider-agnostic factory.
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
