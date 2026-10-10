package envcomposition

import (
	"context"
	"fmt"
	"time"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/model"
	"codeburg.org/lexbit/relurpify/platform/llm"
	// Blank imports register the managed backend kinds with the llm factory;
	// the composition root is where the set of available providers is fixed.
	_ "codeburg.org/lexbit/relurpify/platform/llm/lmstudio"
	_ "codeburg.org/lexbit/relurpify/platform/llm/ollama"
	_ "codeburg.org/lexbit/relurpify/platform/llm/openaicompat"
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
	Kind              string
	Endpoint          string
	ModelName         string
	TapePath          string
	NativeToolCalling bool
	Timeout           time.Duration
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
		Kind:              input.Kind,
		Endpoint:          input.Endpoint,
		Model:             input.ModelName,
		TapePath:          input.TapePath,
		Timeout:           input.Timeout,
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
		ModelFactory: func(tel model.EventSink, debug bool) model.LanguageModel {
			backend.SetDebugLogging(debug)
			instrumented := llm.NewInstrumentedModel(backend.Model(), tel, debug)
			_ = llm.ApplyProfile(instrumented, input.Profile)
			return instrumented
		},
	}, nil
}

// IdentityTriple is the node/session/task identity supplied at the wiring
// point that already knows it. The adapter stamps it onto events whose
// fields the emitter left empty; the per-call context envelope backfills
// whatever the triple does not carry.
type IdentityTriple struct {
	NodeID    string
	SessionID string
	TaskID    string
}

// NewEventSinkAdapter adapts the workspace telemetry chain into the
// model.EventSink the ModelFactory consumes (Q9). It lives at the
// composition root — not in platform/llm — so the identity backfill can
// read the context envelope without giving platform a context edge.
func NewEventSinkAdapter(tel telemetry.Telemetry, ident IdentityTriple) model.EventSink {
	if tel == nil {
		return nil
	}
	return eventSinkAdapter{inner: tel, ident: ident}
}

type eventSinkAdapter struct {
	inner telemetry.Telemetry
	ident IdentityTriple
}

func (a eventSinkAdapter) Emit(ctx context.Context, event any) {
	if a.inner == nil {
		return
	}
	ev, ok := event.(telemetry.Event)
	if !ok {
		return
	}
	telemetry.StampCorrelation(ctx, &ev)
	// Identity enrichment: the wiring point's triple first, then the
	// per-call context envelope (which carries the authoritative
	// node/session/task of the emitting step).
	if ev.NodeID == "" && a.ident.NodeID != "" {
		ev.NodeID = a.ident.NodeID
	}
	if ev.SessionID == "" && a.ident.SessionID != "" {
		ev.SessionID = a.ident.SessionID
	}
	if ev.TaskID == "" && a.ident.TaskID != "" {
		ev.TaskID = a.ident.TaskID
	}
	if env, ok := contextdata.EnvelopeFrom(ctx); ok {
		if ev.NodeID == "" && env.NodeIDSnapshot() != "" {
			ev.NodeID = env.NodeIDSnapshot()
		}
		if ev.SessionID == "" && env.SessionIDSnapshot() != "" {
			ev.SessionID = env.SessionIDSnapshot()
		}
		if ev.TaskID == "" && env.TaskIDSnapshot() != "" {
			ev.TaskID = env.TaskIDSnapshot()
		}
	}
	a.inner.Emit(ev)
}
