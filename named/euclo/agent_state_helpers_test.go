package euclo

import (
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	agentstate "codeburg.org/lexbit/relurpify/named/euclo/internal/agentstate"
	"codeburg.org/lexbit/relurpify/named/euclo/runtime"
	euclointake "codeburg.org/lexbit/relurpify/named/euclo/runtime/intake"
	euclopretask "codeburg.org/lexbit/relurpify/named/euclo/runtime/pretask"
)

func TestEnrichBundleWithContextKnowledge(t *testing.T) {
	state := core.NewContext()
	state.Set("context.knowledge_items", []euclopretask.KnowledgeEvidenceItem{
		{
			RefID:   "ref-1",
			Kind:    euclopretask.KnowledgeKindDecision,
			Title:   "  Decision title  ",
			Summary: "summary",
		},
	})

	bundle := agentstate.EnrichBundleWithContextKnowledge(runtime.SemanticInputBundle{}, state)
	if len(bundle.PatternFindings) != 1 {
		t.Fatalf("expected one pattern finding, got %#v", bundle.PatternFindings)
	}
	if got := bundle.PatternFindings[0].Kind; got != "context_retrieved_decision" {
		t.Fatalf("unexpected finding kind: %q", got)
	}
}

func TestSeedInteractionPrepassAndEvidenceHelpers(t *testing.T) {
	state := core.NewContext()
	agentstate.SeedInteractionPrepass(state, &core.Task{Instruction: "Please just do it"}, runtime.TaskClassification{}, euclotypes.ModeResolution{ModeID: "code"})
	if got, _ := state.Get("just_do_it"); got != true {
		t.Fatalf("expected just_do_it flag, got %#v", got)
	}

	if !agentstate.HasInstructionEvidence("panic: boom", nil) {
		t.Fatal("expected panic text to count as evidence")
	}
	if agentstate.HasInstructionEvidence("plain text", nil) {
		t.Fatal("expected plain text to not count as evidence")
	}
}

func TestInteractionScriptAndTransitionHelpers(t *testing.T) {
	task := &core.Task{
		Context: map[string]any{
			"euclo.interaction_script": []map[string]any{
				{"action": "step-1", "phase": "review", "kind": "probe", "text": "hello"},
			},
			"euclo.max_interactive_transitions": "7",
		},
	}

	script := agentstate.InteractionScriptFromTask(task)
	if len(script) != 1 || script[0].ActionID != "step-1" {
		t.Fatalf("unexpected scripted responses: %#v", script)
	}
	emitter, withTransitions := agentstate.InteractionEmitterForTask(task)
	if emitter == nil || !withTransitions {
		t.Fatalf("expected emitter with transitions, got %#v %v", emitter, withTransitions)
	}
	if got := agentstate.InteractionMaxTransitions(task); got != 7 {
		t.Fatalf("unexpected max transitions: %d", got)
	}
	if got := agentstate.InteractionMaxTransitions(nil); got != 5 {
		t.Fatalf("expected default max transitions, got %d", got)
	}
}

func TestSeedClassifiedEnvelope(t *testing.T) {
	state := core.NewContext()
	state.Set("euclo.unit_of_work", runtime.UnitOfWork{ExecutionDescriptor: runtime.ExecutionDescriptor{UpdatedAt: testNow()}, ID: "prev"})

	classified := euclointake.ClassifiedEnvelope{
		Envelope:       runtime.TaskEnvelope{ResolvedMode: "code"},
		Classification: runtime.TaskClassification{},
		Mode:           euclotypes.ModeResolution{ModeID: "code"},
		Profile:        euclotypes.ExecutionProfileSelection{ProfileID: "plan_stage_execute"},
		Work:           runtime.UnitOfWork{ExecutionDescriptor: runtime.ExecutionDescriptor{UpdatedAt: testNow()}, ID: "uow-1", RootID: "root-1"},
	}

	euclointake.SeedClassifiedEnvelope(state, classified)

	if got, ok := state.Get("euclo.unit_of_work"); !ok || got == nil {
		t.Fatal("expected unit of work in state")
	}
	if got, ok := state.Get("euclo.unit_of_work_history"); !ok || got == nil {
		t.Fatal("expected unit of work history in state")
	}
	if got := state.GetString("euclo.mode"); got != "code" {
		t.Fatalf("unexpected mode: %q", got)
	}
}

func testNow() time.Time {
	return time.Unix(1700000000, 0).UTC()
}
