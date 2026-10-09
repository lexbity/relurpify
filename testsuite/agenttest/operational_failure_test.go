package agenttest

import (
	"context"
	"errors"
	"testing"

	registry "codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/contextstream"
	contextports "codeburg.org/lexbit/relurpify/context/ports"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/model"
	euclostate "codeburg.org/lexbit/relurpify/named/euclo/state"
	"codeburg.org/lexbit/relurpify/named/euclo/surface"
	thoughtrecipe "codeburg.org/lexbit/relurpify/named/euclo/thoughtrecipes"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

// TestOperationalFailureProtocol is AC-7: a step driven by a failing model ends
// the turn with a structured degraded result and a step.operational_failure
// telemetry event — never a panic, never a raw graph error, and never a
// paradigm substitution.
func TestOperationalFailureProtocol(t *testing.T) {
	sink := newRecordingTelemetrySink()
	deps := &paradigm.Deps{
		Config:        &execution.Config{Name: "operational-failure", Model: "failing"},
		Model:         alwaysFailingModel{err: errors.New("provider unreachable")},
		Registry:      registry.NewRegistry(),
		Telemetry:     sink,
		StreamTrigger: contextstream.NewTrigger(noopCompiler{}),
	}

	plan := &thoughtrecipe.ExecutionPlan{
		ThoughtRecipe: &surface.ThoughtRecipe{
			ID:   "euclo.thoughtrecipe.test",
			Name: "operational failure probe",
		},
		Agents: map[string]thoughtrecipe.AgentBinding{
			"agent": {Name: "agent", Paradigm: "react"},
		},
		Steps: []thoughtrecipe.ExecutionStep{{
			ID:       "step.execute",
			Kind:     thoughtrecipe.StepKindRun,
			Paradigm: "react",
			Goal:     "Complete the task.",
			Prompt:   "Complete the task.",
			Scope:    thoughtrecipe.AllowAll(),
		}},
	}

	graph, err := thoughtrecipe.BuildThoughtRecipeGraph(plan, deps, nil)
	if err != nil {
		t.Fatalf("BuildThoughtRecipeGraph: %v", err)
	}
	if err := graph.SetTelemetry(sink); err != nil {
		t.Fatalf("SetTelemetry: %v", err)
	}

	env := contextdata.NewEnvelope("task-operational-failure", "session-operational-failure")
	result, err := graph.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("operational failure must not surface as a graph error: %v", err)
	}

	if result == nil {
		t.Fatal("expected a structured result")
	}
	if result.Success {
		t.Fatalf("expected a failed structured result, got %+v", result)
	}
	if _, ok := euclostate.GetStepFailure(env); !ok {
		t.Fatal("expected the typed step failure on the envelope")
	}

	events := sink.Events()
	var operational int
	execNodes := map[string]bool{}
	for _, event := range events {
		switch event.Type {
		case telemetry.EventStepOperationalFailure:
			operational++
			if got := event.Metadata["kind"]; got == "" {
				t.Fatalf("operational failure telemetry missing kind: %#v", event.Metadata)
			}
			if got := event.Metadata["action_taken"]; got != "abort" {
				t.Fatalf("operational failure action_taken = %v, want abort", got)
			}
		case telemetry.EventNodeStart:
			execNodes[event.NodeID] = true
		}
	}
	if operational != 1 {
		t.Fatalf("step.operational_failure events = %d, want 1", operational)
	}
	// No paradigm substitution: only the declared react step executed.
	execNodeID := plan.Steps[0].ID + ".execute"
	if len(execNodes) != 1 || !execNodes[execNodeID] {
		t.Fatalf("executed nodes = %#v, want exactly [%s]", execNodes, execNodeID)
	}
}

func TestOperationalFailureProtocolFallsBackOnOperationalClass(t *testing.T) {
	sink := newRecordingTelemetrySink()
	deps := &paradigm.Deps{
		Config:        &execution.Config{Name: "operational-fallback", Model: "failing"},
		Model:         alwaysFailingModel{err: errors.New("provider unreachable")},
		Registry:      registry.NewRegistry(),
		Telemetry:     sink,
		StreamTrigger: contextstream.NewTrigger(noopCompiler{}),
	}

	plan := &thoughtrecipe.ExecutionPlan{
		ThoughtRecipe: &surface.ThoughtRecipe{
			ID:   "euclo.thoughtrecipe.test",
			Name: "operational fallback probe",
		},
		Agents: map[string]thoughtrecipe.AgentBinding{
			"agent":    {Name: "agent", Paradigm: "react"},
			"fallback": {Name: "fallback", Paradigm: "react"},
		},
		Steps: []thoughtrecipe.ExecutionStep{{
			ID:       "step.execute",
			Kind:     thoughtrecipe.StepKindRun,
			Paradigm: "react",
			Goal:     "Complete the task.",
			Prompt:   "Complete the task.",
			Scope:    thoughtrecipe.AllowAll(),
			Fallback: &surface.ThoughtRecipeStepAgent{
				Paradigm: "react",
				Prompt:   "Fallback completion.",
			},
		}},
	}

	graph, err := thoughtrecipe.BuildThoughtRecipeGraph(plan, deps, nil)
	if err != nil {
		t.Fatalf("BuildThoughtRecipeGraph: %v", err)
	}
	if err := graph.SetTelemetry(sink); err != nil {
		t.Fatalf("SetTelemetry: %v", err)
	}

	env := contextdata.NewEnvelope("task-fallback", "session-fallback")
	result, err := graph.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("operational failure with fallback must stay structured: %v", err)
	}
	if result == nil {
		t.Fatal("expected a structured result")
	}

	// The fallback node ran and recorded activation; the primary failure was
	// still classified and recorded.
	events := sink.Events()
	var fallbackActivated bool
	for _, event := range events {
		if event.Type == telemetry.EventStepFallbackActivated {
			fallbackActivated = true
		}
	}
	if !fallbackActivated {
		t.Fatal("expected step.fallback_activated for the authored fallback")
	}
	if !euclostate.GetFallbackTaken(env) {
		t.Fatal("expected fallback_taken on the envelope")
	}
	if _, ok := euclostate.GetStepFailure(env); !ok {
		t.Fatal("expected the primary failure to remain recorded")
	}
}

// alwaysFailingModel fails every model call.
type alwaysFailingModel struct {
	err error
}

func (m alwaysFailingModel) Generate(context.Context, string, *model.LLMOptions) (*model.LLMResponse, error) {
	return nil, m.err
}

func (m alwaysFailingModel) GenerateStream(context.Context, string, *model.LLMOptions) (<-chan string, error) {
	return nil, m.err
}

func (m alwaysFailingModel) Chat(context.Context, []model.Message, *model.LLMOptions) (*model.LLMResponse, error) {
	return nil, m.err
}

func (m alwaysFailingModel) ChatWithTools(context.Context, []model.Message, []model.LLMToolSpec, *model.LLMOptions) (*model.LLMResponse, error) {
	return nil, m.err
}

// noopCompiler satisfies contextstream.CompilerInvoker offline.
type noopCompiler struct{}

func (noopCompiler) Compile(context.Context, contextports.CompilationRequest) (*contextports.CompilationResult, error) {
	return &contextports.CompilationResult{}, nil
}
