package agenttest

import (
	"time"

	"codeburg.org/lexbit/relurpify/model"
	"codeburg.org/lexbit/relurpify/platform/llm"
)

// buildCaseManagedBackend constructs a ManagedBackend using the provider-agnostic factory.
func buildCaseManagedBackend(execution resolvedCaseExecution, profile *model.ModelProfile, debug bool) (llm.ManagedBackend, error) {
	cfg := llm.ProviderConfig{
		Provider: execution.Provider,
		Endpoint: execution.Endpoint,
		Model:    execution.Model,
		Timeout:  30 * time.Second,
		Debug:    debug,
	}

	// For P03: pass nil gate for test backend (tests manage their own gate)
	backend, err := llm.New(cfg, llm.ProviderSecrets{}, nil)
	if err != nil {
		return nil, err
	}

	if profile != nil {
		backend.SetProfile(profile)
	}

	backend.SetDebugLogging(debug)

	return backend, nil
}
