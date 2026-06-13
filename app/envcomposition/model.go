package envcomposition

import (
	"fmt"

	"codeburg.org/lexbit/relurpify/model"
	"codeburg.org/lexbit/relurpify/platform/llm"
	"codeburg.org/lexbit/relurpify/platform/observability"
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
	TapePath          string
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
		TapePath:          input.TapePath,
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
		ModelFactory: func(tel model.Telemetry, debug bool) model.LanguageModel {
			backend.SetDebugLogging(debug)
			instrumented := llm.NewInstrumentedModel(backend.Model(), modelTelemetryAdapter{inner: tel}, debug)
			_ = llm.ApplyProfile(instrumented, input.Profile)
			return instrumented
		},
	}, nil
}

type modelTelemetryAdapter struct {
	inner model.Telemetry
}

func (a modelTelemetryAdapter) Emit(event observability.Event) {
	if a.inner == nil {
		return
	}
	a.inner.Emit(telemetry.Event{
		Type:      telemetry.EventType(event.Type),
		NodeID:    event.NodeID,
		TaskID:    event.TaskID,
		Message:   event.Message,
		Timestamp: event.Timestamp,
		Metadata:  event.Metadata,
		Seq:       event.Seq,
		Partition: event.Partition,
		Payload:   event.Payload,
		Actor:     actorID(event.Actor),
	})
}

func actorID(actor observability.Actor) string {
	if actor.ID != "" {
		return actor.ID
	}
	if actor.Label != "" {
		return actor.Label
	}
	return actor.Kind
}
