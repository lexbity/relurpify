package envcomposition

import (
	"fmt"

	"codeburg.org/lexbit/relurpify/model"
	"codeburg.org/lexbit/relurpify/platform/llm"
	"codeburg.org/lexbit/relurpify/telemetry"
)

// ModelRuntime bundles the app-composed LLM backend. The instrumented model
// is created at workspace-open time when telemetry is available.
type ModelRuntime struct {
	Backend      llm.ManagedBackend
	ModelFactory model.ModelFactory
}

// ModelRuntimeInput carries parameters for BuildModelRuntime.
type ModelRuntimeInput struct {
	Provider          string
	Endpoint          string
	ModelName         string
	NativeToolCalling bool
	Secrets           llm.ProviderSecrets
	Profile           *model.ModelProfile
}

// BuildModelRuntime constructs the LLM backend and applies the profile.
func BuildModelRuntime(input ModelRuntimeInput) (*ModelRuntime, error) {
	if input.Provider == "" {
		return nil, fmt.Errorf("inference provider required")
	}
	providerCfg := llm.ProviderConfig{
		Provider:          input.Provider,
		Endpoint:          input.Endpoint,
		Model:             input.ModelName,
		NativeToolCalling: input.NativeToolCalling,
	}
	backend, err := llm.New(providerCfg, input.Secrets)
	if err != nil {
		return nil, fmt.Errorf("build inference backend: %w", err)
	}
	if input.Profile != nil {
		_ = llm.ApplyProfile(backend, input.Profile)
	}
	return &ModelRuntime{
		Backend: backend,
		ModelFactory: func(tel telemetry.Telemetry, debug bool) model.LanguageModel {
			backend.SetDebugLogging(debug)
			instrumented := llm.NewInstrumentedModel(backend.Model(), tel, debug)
			_ = llm.ApplyProfile(instrumented, input.Profile)
			return instrumented
		},
	}, nil
}
