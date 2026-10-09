package thoughtrecipe

import (
	"context"
	"errors"
	"sync"
	"testing"

	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	rewooagent "codeburg.org/lexbit/relurpify/cognitionzoo/rewoo"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/contextstream"
	"codeburg.org/lexbit/relurpify/context/knowledge"
	contextports "codeburg.org/lexbit/relurpify/context/ports"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/governance/permissions"
	"codeburg.org/lexbit/relurpify/model"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	euclostate "codeburg.org/lexbit/relurpify/named/euclo/state"
	"codeburg.org/lexbit/relurpify/named/euclo/surface"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

// --- AC-19: grounding failure classification uses errors.Is, never strings ---

// TestGroundingFailureClassification is AC-19's entry point: a grounding
// failure (fake sink / injected knowledge.ErrGroundingFailed) classifies as
// grounding_failed under errors.Is and is governed by the policy; a
// non-wrapping error with a matching message does NOT classify (FR-23).
func TestGroundingFailureClassification(t *testing.T) {
	wrapped := errors.New("barrier blew up")
	cases := []struct {
		name string
		err  error
		want euclotypes.FailureKind
	}{
		{
			name: "wrapping grounding sentinel classifies",
			err:  errors.Join(errors.New("epoch 3 grounding"), knowledge.ErrGroundingFailed, wrapped),
			want: euclotypes.FailureGroundingFailed,
		},
		{
			name: "non-wrapping matching message does not classify",
			err:  errors.New("knowledge: grounding failed"),
			want: euclotypes.FailureUnknown,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClassifyFailure(tc.err); got != tc.want {
				t.Fatalf("ClassifyFailure(%v) = %q, want %q", tc.err, got, tc.want)
			}
		})
	}
}

func TestClassifyFailureGroundingUsesErrorsIs(t *testing.T) {
	wrapped := errors.New("barrier blew up")
	cases := []struct {
		name string
		err  error
		want euclotypes.FailureKind
	}{
		{
			name: "wrapping grounding sentinel classifies",
			err:  errors.Join(errors.New("epoch 3 grounding"), knowledge.ErrGroundingFailed, wrapped),
			want: euclotypes.FailureGroundingFailed,
		},
		{
			name: "non-wrapping matching message does not classify",
			err:  errors.New("knowledge: grounding failed"),
			want: euclotypes.FailureUnknown,
		},
		{name: "context cancellation", err: context.Canceled, want: euclotypes.FailureCancelled},
		{name: "deadline", err: context.DeadlineExceeded, want: euclotypes.FailureModelUnavailable},
		{name: "invalid planner output", err: rewooagent.ErrRewooPlanInvalid, want: euclotypes.FailureModelInvalidOutput},
		{
			name: "permission denied",
			err:  &permissions.PermissionDeniedError{Message: "denied"},
			want: euclotypes.FailureCapabilityDenied,
		},
		{name: "unknown", err: errors.New("boom"), want: euclotypes.FailureUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClassifyFailure(tc.err); got != tc.want {
				t.Fatalf("ClassifyFailure(%v) = %q, want %q", tc.err, got, tc.want)
			}
		})
	}
	if got := ClassifyFailure(nil); got != "" {
		t.Fatalf("ClassifyFailure(nil) = %q, want empty", got)
	}
}

// --- AC-19 / FR-9: the node-boundary failure protocol is structured ---

func TestRecordOperationalFailureProtocol(t *testing.T) {
	sink := &recordingTelemetry{}
	core := &stepCore{
		id:   "step.execute",
		deps: &paradigm.Deps{Telemetry: sink},
		step: ExecutionStep{ID: "step", Kind: StepKindRun, Paradigm: "react"},
	}

	t.Run("abort records typed failure and emits", func(t *testing.T) {
		env := contextdata.NewEnvelope("task-fail", "session-fail")
		err := errors.Join(errors.New("grounding barrier"), knowledge.ErrGroundingFailed)
		result := core.recordOperationalFailure(context.Background(), env, err)

		if result == nil || result.Success {
			t.Fatalf("expected a failed structured result, got %+v", result)
		}
		if got := result.Metadata[operationalFailureKindMetadata]; got != string(euclotypes.FailureGroundingFailed) {
			t.Fatalf("result failure_kind = %v, want grounding_failed", got)
		}
		failure, ok := euclostate.GetStepFailure(env)
		if !ok || failure == nil || failure.Kind != euclotypes.FailureGroundingFailed {
			t.Fatalf("envelope step failure = %+v (ok=%v), want grounding_failed", failure, ok)
		}
		if all := euclostate.GetStepFailures(env); len(all) != 1 {
			t.Fatalf("envelope failure list = %d, want 1", len(all))
		}
		events := sink.eventsOfType(telemetry.EventStepOperationalFailure)
		if len(events) != 1 {
			t.Fatalf("operational_failure events = %d, want 1", len(events))
		}
		if got := events[0].Metadata["kind"]; got != string(euclotypes.FailureGroundingFailed) {
			t.Fatalf("telemetry kind = %v, want grounding_failed", got)
		}
		if got := events[0].Metadata["action_taken"]; got != string(policyAbort) {
			t.Fatalf("telemetry action_taken = %v, want abort", got)
		}
	})

	t.Run("continue resolves to success but stays observable", func(t *testing.T) {
		env := contextdata.NewEnvelope("task-skip", "session-skip")
		cont := *core
		cont.step.OnError = &surface.StepErrorPolicy{Action: "skip"}
		result := cont.recordOperationalFailure(context.Background(), env, errors.New("boom"))

		if result == nil || !result.Success {
			t.Fatalf("expected a resolved-success result, got %+v", result)
		}
		skipped, _ := execution.ResultField(result.Data, "skipped")
		if skipped != true {
			t.Fatalf("expected skipped=true, got %v", skipped)
		}
		if _, ok := euclostate.GetStepFailure(env); !ok {
			t.Fatal("expected the resolved failure to remain recorded")
		}
	})

	t.Run("ask degrades to abort until the resolver lands", func(t *testing.T) {
		env := contextdata.NewEnvelope("task-ask", "session-ask")
		ask := *core
		ask.step.OnError = &surface.StepErrorPolicy{Action: "ask"}
		result := ask.recordOperationalFailure(context.Background(), env, errors.New("boom"))

		if result == nil || result.Success {
			t.Fatalf("expected abort-style failure result, got %+v", result)
		}
		if unavailable, _ := result.Metadata["ask_unavailable"].(bool); !unavailable {
			t.Fatalf("expected ask_unavailable metadata, got %#v", result.Metadata)
		}
	})
}

// --- AC-6 / D7: authored fallback inheritance and narrow firing ---

// TestFallbackSemantics is AC-6's entry point: the authored fallback inherits
// Goal/Sources/captures from the parent and fires only on classified
// operational failures.
func TestFallbackSemantics(t *testing.T) {
	t.Run("inheritance", TestFallbackStepInheritance)
	t.Run("edge-gating", TestFallbackEdgeFiresOnlyOnOperationalFailure)
}

func TestFallbackStepInheritance(t *testing.T) {
	parent := ExecutionStep{
		ID:       "primary",
		Kind:     StepKindRun,
		Paradigm: "react",
		Goal:     "complete the obligation",
		Sources:  []string{"input.findings"},
		Scope:    AllowTools([]string{"file_write"}),
		CaptureBindings: []CaptureBinding{{
			Source:      Identifier{Value: "result"},
			Destination: PathExpr{Raw: "state.out"},
		}},
		Directives: []TypedDirective{{Name: "until", TextArgs: []string{"3"}}},
		PromptID:   "parent.prompt",
		Mutation:   "required",
		OnError:    &surface.StepErrorPolicy{Action: "fallback"},
		Fallback: &surface.ThoughtRecipeStepAgent{
			Paradigm: "planner",
			Prompt:   "fallback prompt",
			Context:  surface.ThoughtRecipeStepContext{Inherit: []string{"state.findings"}},
		},
	}

	fallback := buildFallbackStep(parent)

	if fallback.ID != "primary.fallback" {
		t.Fatalf("fallback ID = %q, want primary.fallback", fallback.ID)
	}
	if fallback.FallbackFor != "primary" {
		t.Fatalf("fallback_for = %q, want primary", fallback.FallbackFor)
	}
	if fallback.Fallback != nil {
		t.Fatal("fallback must not recurse")
	}
	// Allowed to differ.
	if fallback.Paradigm != "planner" || fallback.Prompt != "fallback prompt" {
		t.Fatalf("fallback paradigm/prompt = %q/%q, want planner/fallback prompt", fallback.Paradigm, fallback.Prompt)
	}
	if fallback.PromptID != "" {
		t.Fatalf("fallback PromptID = %q, want cleared", fallback.PromptID)
	}
	// Must be inherited.
	if fallback.Goal != parent.Goal {
		t.Fatalf("fallback goal = %q, want inherited %q", fallback.Goal, parent.Goal)
	}
	if len(fallback.Sources) != 1 || fallback.Sources[0] != "input.findings" {
		t.Fatalf("fallback sources = %#v, want inherited [input.findings]", fallback.Sources)
	}
	if len(fallback.CaptureBindings) != 1 || fallback.CaptureBindings[0].Destination.Raw != "state.out" {
		t.Fatalf("fallback captures = %#v, want inherited state.out", fallback.CaptureBindings)
	}
	if len(fallback.Directives) != 1 || fallback.Directives[0].Name != "until" {
		t.Fatalf("fallback directives = %#v, want inherited until", fallback.Directives)
	}
	if !fallback.Scope.Permits("file_write") {
		t.Fatal("fallback must inherit the parent scope")
	}
	if fallback.Mutation != "required" || fallback.OnError == nil || fallback.OnError.Action != "fallback" {
		t.Fatalf("fallback must inherit mutation and error policy, got mutation=%q onError=%+v", fallback.Mutation, fallback.OnError)
	}
}

func TestFallbackEdgeFiresOnlyOnOperationalFailure(t *testing.T) {
	if class := operationalFailureClass(&execution.Result{Success: false}); class != "" {
		t.Fatalf("unclassified failure must not fire fallback, got %q", class)
	}
	if class := operationalFailureClass(&execution.Result{Success: true}); class != "" {
		t.Fatalf("success must not fire fallback, got %q", class)
	}
	classified := &execution.Result{Success: false, Metadata: map[string]any{
		operationalFailureKindMetadata: string(euclotypes.FailureModelUnavailable),
	}}
	if class := operationalFailureClass(classified); class != string(euclotypes.FailureModelUnavailable) {
		t.Fatalf("classified failure = %q, want model_unavailable", class)
	}
}

// --- AC-7: a failing model ends the turn structured, no panic ---

func TestRunNodeOperationalFailureEndsStructured(t *testing.T) {
	sink := &recordingTelemetry{}
	deps := &paradigm.Deps{
		Model:         failingModel{err: errors.New("provider exploded")},
		Config:        &execution.Config{Name: "failure-protocol", Model: "failing"},
		Telemetry:     sink,
		StreamTrigger: contextstream.NewTrigger(noopCompiler{}),
	}
	step := ExecutionStep{
		ID:       "react.step",
		Kind:     StepKindRun,
		Paradigm: "react",
		Goal:     "Do the thing.",
		Prompt:   "Do the thing.",
	}
	node := NewRunNode("react.step.execute", deps, step)
	env := contextdata.NewEnvelope("task-react", "session-react")

	result, err := node.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("operational failure must not surface as a raw error: %v", err)
	}
	if result == nil || result.Success {
		t.Fatalf("expected a failed structured result, got %+v", result)
	}
	if node.step.Paradigm != "react" {
		t.Fatalf("step paradigm changed to %q; no substitution is permitted", node.step.Paradigm)
	}
	if got := result.Metadata[operationalFailureKindMetadata]; got == "" {
		t.Fatalf("expected a classified failure kind, got %#v", result.Metadata)
	}
	if _, ok := euclostate.GetStepFailure(env); !ok {
		t.Fatal("expected the typed step failure on the envelope")
	}
	if len(sink.eventsOfType(telemetry.EventStepOperationalFailure)) != 1 {
		t.Fatalf("expected exactly one step.operational_failure event, got %d",
			len(sink.eventsOfType(telemetry.EventStepOperationalFailure)))
	}
}

// --- helpers ---------------------------------------------------------------

type recordingTelemetry struct {
	mu     sync.Mutex
	events []telemetry.Event
}

func (s *recordingTelemetry) Emit(event telemetry.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
}

func (s *recordingTelemetry) eventsOfType(eventType telemetry.EventType) []telemetry.Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []telemetry.Event
	for _, event := range s.events {
		if event.Type == eventType {
			out = append(out, event)
		}
	}
	return out
}

type failingModel struct {
	err   error
	calls int
}

func (m failingModel) Generate(context.Context, string, *model.LLMOptions) (*model.LLMResponse, error) {
	return nil, m.err
}

func (m failingModel) GenerateStream(context.Context, string, *model.LLMOptions) (<-chan string, error) {
	return nil, m.err
}

func (m failingModel) Chat(context.Context, []model.Message, *model.LLMOptions) (*model.LLMResponse, error) {
	return nil, m.err
}

func (m failingModel) ChatWithTools(context.Context, []model.Message, []model.LLMToolSpec, *model.LLMOptions) (*model.LLMResponse, error) {
	return nil, m.err
}

type noopCompiler struct{}

func (noopCompiler) Compile(context.Context, contextports.CompilationRequest) (*contextports.CompilationResult, error) {
	return &contextports.CompilationResult{}, nil
}
