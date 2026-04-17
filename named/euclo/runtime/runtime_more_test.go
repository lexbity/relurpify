package runtime

import (
	"context"
	"reflect"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/retrieval"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/named/euclo/interaction"
)

type fakeRetrievalService struct {
	blocks []core.ContentBlock
	event  retrieval.RetrievalEvent
	err    error
	query  retrieval.RetrievalQuery
}

func (f *fakeRetrievalService) Retrieve(_ context.Context, q retrieval.RetrievalQuery) ([]core.ContentBlock, retrieval.RetrievalEvent, error) {
	f.query = q
	return append([]core.ContentBlock(nil), f.blocks...), f.event, f.err
}

type fakeRetrievalProvider struct {
	service retrieval.RetrieverService
	records []memory.KnowledgeRecord
}

func (f *fakeRetrievalProvider) RetrievalService() retrieval.RetrieverService {
	return f.service
}

func (f *fakeRetrievalProvider) ListKnowledge(_ context.Context, workflowID string, kind memory.KnowledgeKind, unresolvedOnly bool) ([]memory.KnowledgeRecord, error) {
	_ = workflowID
	_ = kind
	_ = unresolvedOnly
	return append([]memory.KnowledgeRecord(nil), f.records...), nil
}

func TestRetrievalPolicyAndExpansionHelpers(t *testing.T) {
	policyCases := map[string]RetrievalPolicy{
		"planning": ResolveRetrievalPolicy(ModeResolution{ModeID: "planning"}, ExecutionProfileSelection{ProfileID: "plan_stage_execute"}),
		"debug":    ResolveRetrievalPolicy(ModeResolution{ModeID: "debug"}, ExecutionProfileSelection{ProfileID: "reproduce_localize_patch"}),
		"chat":     ResolveRetrievalPolicy(ModeResolution{ModeID: "chat"}, ExecutionProfileSelection{ProfileID: "chat_ask_respond"}),
		"code":     ResolveRetrievalPolicy(ModeResolution{ModeID: "code"}, ExecutionProfileSelection{ProfileID: "edit_verify_repair"}),
	}
	if !policyCases["planning"].WidenToWorkflow || policyCases["planning"].WorkflowLimit != 6 {
		t.Fatalf("unexpected planning policy: %#v", policyCases["planning"])
	}
	if !policyCases["debug"].WidenToWorkflow || policyCases["debug"].ExpansionStrategy != "local_then_targeted_workflow" {
		t.Fatalf("unexpected debug policy: %#v", policyCases["debug"])
	}
	if !policyCases["chat"].WidenToWorkflow || policyCases["chat"].WorkflowMaxTokens != 600 {
		t.Fatalf("unexpected chat policy: %#v", policyCases["chat"])
	}
	if policyCases["code"].WidenToWorkflow {
		t.Fatalf("expected code policy to stay local: %#v", policyCases["code"])
	}

	query := buildWorkflowRetrievalQuery(workflowRetrievalQuery{
		Primary:       "  inspect the plan  ",
		TaskText:      "inspect the plan",
		Expected:      "results",
		Verification:  "results",
		PreviousNotes: []string{"  note-a ", "note-a", "note-b"},
	})
	if query != "inspect the plan\nresults\nnote-a\nnote-b" {
		t.Fatalf("unexpected query: %q", query)
	}

	blocks := []core.ContentBlock{
		core.TextContentBlock{Text: "  first result  "},
		core.StructuredContentBlock{Data: map[string]any{
			"text":      " second result ",
			"citations": []any{retrieval.PackedCitation{DocID: "src-1"}, "ignored"},
		}},
	}
	results := contentBlockResults(blocks)
	if len(results) != 2 || len(results[1].Citations) != 1 {
		t.Fatalf("unexpected content block results: %#v", results)
	}
	if got := parseWorkflowRetrievalCitations([]retrieval.PackedCitation{{DocID: "src-2"}}); len(got) != 1 {
		t.Fatalf("unexpected packed citations: %#v", got)
	}
	if got := parseWorkflowRetrievalCitations([]any{retrieval.PackedCitation{DocID: "src-3"}, "skip"}); len(got) != 1 {
		t.Fatalf("unexpected mixed citations: %#v", got)
	}

	task := &core.Task{
		Instruction: "inspect the repository",
		Context: map[string]any{
			"path":         "a.go",
			"file":         "b.go",
			"target_path":  "c.go",
			"paths":        []any{"d.go", "d.go", 1},
			"verification": "go test ./...",
		},
	}
	if got := taskPaths(task); !reflect.DeepEqual(got, []string{"a.go", "b.go", "c.go"}) {
		t.Fatalf("unexpected task paths: %#v", got)
	}
	if got := taskInstruction(task); got != "inspect the repository" {
		t.Fatalf("unexpected task instruction: %q", got)
	}
	if got := taskVerification(task); got != "go test ./..." {
		t.Fatalf("unexpected task verification: %q", got)
	}

	expansion := ContextExpansion{
		LocalPaths:        []string{"a.go"},
		WorkflowRetrieval: map[string]any{"summary": "workflow summary"},
		WidenedToWorkflow: true,
		ExpansionStrategy: "local_then_workflow",
	}
	if got := summarizeExpansion(expansion); got != "local_paths=1 workflow_retrieval" {
		t.Fatalf("unexpected summary: %q", got)
	}

	state := core.NewContext()
	cloned := ApplyContextExpansion(state, task, expansion)
	expansion = cloned.Context["euclo.context_expansion"].(ContextExpansion)
	if state.GetString("pipeline.workflow_retrieval") == "" {
		t.Fatal("expected workflow retrieval stored in state")
	}
	if len(expansion.WorkflowRetrieval) == 0 {
		t.Fatal("expected context expansion to round-trip")
	}

	provider := &fakeRetrievalProvider{
		service: &fakeRetrievalService{
			blocks: []core.ContentBlock{
				core.TextContentBlock{Text: "workflow text"},
			},
			event: retrieval.RetrievalEvent{CacheTier: "l3_main", QueryID: "rq-1"},
		},
		records: []memory.KnowledgeRecord{
			{Title: "title-a", Content: "content-a"},
		},
	}
	exp, err := ExpandContext(context.Background(), provider, "wf-1", task, state, ResolveRetrievalPolicy(ModeResolution{ModeID: "planning"}, ExecutionProfileSelection{ProfileID: "plan_stage_execute"}))
	if err != nil {
		t.Fatalf("ExpandContext() error = %v", err)
	}
	if !exp.WidenedToWorkflow || len(exp.WorkflowRetrieval) == 0 {
		t.Fatalf("expected widened expansion, got %#v", exp)
	}

	fallbackProvider := &fakeRetrievalProvider{
		service: &fakeRetrievalService{},
		records: []memory.KnowledgeRecord{{Title: "fallback", Content: "record"}},
	}
	exp, err = ExpandContext(context.Background(), fallbackProvider, "wf-1", task, nil, ResolveRetrievalPolicy(ModeResolution{ModeID: "planning"}, ExecutionProfileSelection{ProfileID: "plan_stage_execute"}))
	if err != nil {
		t.Fatalf("ExpandContext() fallback error = %v", err)
	}
	if exp.WorkflowRetrieval["results"] == nil {
		t.Fatalf("expected fallback knowledge records in expansion: %#v", exp)
	}
}

func TestUnitOfWorkAndBindingHelpers(t *testing.T) {
	modeRegistry := euclotypes.NewModeRegistry()
	requireNoError(t, modeRegistry.Register(euclotypes.ModeDescriptor{
		ModeID:                   "planning",
		DefaultExecutionProfiles: []string{"plan_stage_execute"},
		ContextStrategy:          "custom-planning",
	}))

	task := &core.Task{
		ID:          "task-1",
		Instruction: "implement the plan for the new API",
		Context: map[string]any{
			"workflow_id":  "wf-task",
			"run_id":       "run-task",
			"skills":       []any{"skill-task", "skill-task"},
			"path":         "a.go",
			"paths":        []any{"b.go", "b.go"},
			"verification": "go test ./...",
		},
	}
	state := core.NewContext()
	state.Set("euclo.workflow_id", "wf-state")
	state.Set("euclo.run_id", "run-state")
	state.Set("euclo.execution_id", "exec-state")
	state.Set("euclo.unit_of_work_id", "uow-state")
	state.Set("euclo.root_unit_of_work_id", "root-state")
	state.Set("euclo.current_plan_step_id", "step-1")
	state.Set("pipeline.workflow_retrieval", map[string]any{"summary": "retrieved"})
	state.Set("euclo.pending_learning_ids", []any{"learn-1"})
	state.Set("euclo.blocking_learning_ids", []string{"learn-2"})
	state.Set("euclo.active_exploration_id", "pattern-1")
	state.Set("euclo.archaeo_phase_state", map[string]any{"x": 1})
	state.Set("euclo.skills", []any{"skill-state", "skill-task"})
	state.Set("euclo.deferred_issue_ids", []string{"issue-1", "issue-1", "issue-2"})
	state.Set("pipeline.plan", map[string]any{
		"steps": []any{map[string]any{"id": "step-1"}},
	})
	state.Set("euclo.unit_of_work", UnitOfWork{ExecutionDescriptor: ExecutionDescriptor{PlanBinding: &UnitOfWorkPlanBinding{WorkflowID: "wf-existing", PlanID: "plan-existing", ActiveStepID: "step-existing", IsPlanBacked: true},
		ResolvedPolicy:              ResolvedExecutionPolicy{ResolvedFromSkillPolicy: true},
		ExecutorDescriptor:          WorkUnitExecutorDescriptor{ExecutorID: "executor-existing"},
		ContextBundle:               UnitOfWorkContextBundle{ContextBudgetClass: "legacy"},
		RoutineBindings:             []UnitOfWorkRoutineBinding{{RoutineID: "legacy"}},
		SkillBindings:               []UnitOfWorkSkillBinding{{SkillID: "skill-legacy"}},
		ToolBindings:                []UnitOfWorkToolBinding{{ToolID: "tool-legacy"}},
		CapabilityBindings:          []UnitOfWorkCapabilityBinding{{CapabilityID: "cap-legacy"}},
		DeferredIssueIDs:            []string{"legacy"},
		PredecessorUnitOfWorkID:     "predecessor",
		PrimaryRelurpicCapabilityID: "cap-primary"}, ID: "uow-existing",
		RootID: "root-existing",
	})

	envelope := TaskEnvelope{
		TaskID:                "task-1",
		Instruction:           task.Instruction,
		Workspace:             "/workspace",
		EditPermitted:         true,
		PreviousArtifactKinds: []string{"plan_output"},
		ExecutionProfile:      "plan_stage_execute",
		CapabilitySnapshot:    euclotypes.CapabilitySnapshot{HasWriteTools: true, HasVerificationTools: true},
	}
	classification := TaskClassification{
		IntentFamilies:                 []string{"planning"},
		RecommendedMode:                "planning",
		RequiresEvidenceBeforeMutation: true,
		RequiresDeterministicStages:    true,
		EditPermitted:                  true,
	}
	mode := ModeResolution{ModeID: "planning"}
	profile := ExecutionProfileSelection{
		ProfileID:            "plan_stage_execute",
		VerificationRequired: true,
		PhaseRoutes:          map[string]string{"plan": "planner"},
	}
	policy := ResolvedExecutionPolicy{
		ResolvedFromSkillPolicy:       true,
		PreferredPlanningCapabilities: []string{"plan-cap"},
		PreferredVerifyCapabilities:   []string{"verify-cap"},
		PhaseCapabilityConstraints:    map[string][]string{"plan": []string{"cap-a", "cap-a"}},
		RequireVerificationStep:       true,
	}
	executor := WorkUnitExecutorDescriptor{}
	uow := BuildUnitOfWork(task, state, envelope, classification, mode, profile, modeRegistry, SemanticInputBundle{
		PatternRefs:             []string{"pattern-1"},
		TensionRefs:             []string{"tension-1"},
		ProvenanceRefs:          []string{"prov-1"},
		LearningInteractionRefs: []string{"learn-1"},
	}, policy, executor)

	if uow.ID != "uow-existing" || uow.RootID != "root-existing" {
		t.Fatalf("unexpected work unit identity fields: %#v", uow)
	}
	if uow.ContextStrategyID != "custom-planning" {
		t.Fatalf("expected registry-derived context strategy, got %#v", uow.ContextStrategyID)
	}
	if uow.PlanBinding == nil || !uow.PlanBinding.IsPlanBacked || !uow.PlanBinding.IsLongRunning {
		t.Fatalf("expected plan binding to be restored and marked long running: %#v", uow.PlanBinding)
	}
	if len(uow.SkillBindings) < 2 || len(uow.SkillBindings) != len(dedupeSkillBindings(uow.SkillBindings)) {
		t.Fatalf("expected deduped skill bindings: %#v", uow.SkillBindings)
	}
	if len(uow.CapabilityBindings) == 0 || len(uow.CapabilityBindings) != len(dedupeCapabilityBindings(uow.CapabilityBindings)) {
		t.Fatalf("expected deduped capability bindings: %#v", uow.CapabilityBindings)
	}
	if uow.ExecutorDescriptor.ExecutorID == "" {
		t.Fatal("expected executor descriptor to be resolved")
	}
	if uow.ContextBundle.ContextBudgetClass != "heavy" {
		t.Fatalf("expected heavy context budget, got %#v", uow.ContextBundle.ContextBudgetClass)
	}
	if !reflect.DeepEqual(deferredIssueIDsFromState(state), []string{"issue-1", "issue-2"}) {
		t.Fatalf("unexpected deferred issue ids: %#v", deferredIssueIDsFromState(state))
	}
	if got := contextBudgetClass(TaskEnvelope{ExecutionProfile: "plan_stage_execute"}); got != "heavy" {
		t.Fatalf("unexpected budget class: %q", got)
	}
	if got := contextBudgetClass(TaskEnvelope{PreviousArtifactKinds: []string{"a"}}); got != "medium" {
		t.Fatalf("unexpected budget class: %q", got)
	}
	if got := contextBudgetClass(TaskEnvelope{}); got != "light" {
		t.Fatalf("unexpected budget class: %q", got)
	}
	if got := stringSliceAny([]any{"a", "a", 1, nil, " "}); !reflect.DeepEqual(got, []string{"a", "a", "1"}) {
		t.Fatalf("unexpected string slice conversion: %#v", got)
	}
	if got := uniqueStrings([]string{"a", "a", " b ", ""}); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("unexpected unique strings: %#v", got)
	}
	if got := summarizeMapSummary(map[string]any{"summary": "  hello  "}); got != "hello" {
		t.Fatalf("unexpected map summary: %q", got)
	}
	if got, ok := existingUnitOfWork(state); !ok || got.ID != "uow-existing" {
		t.Fatalf("unexpected existing unit of work: %#v", got)
	}
}

func TestRuntimeClassificationVerificationAndAmbiguityHelpers(t *testing.T) {
	state := core.NewContext()
	state.Set("euclo.mode", "debug")
	state.Set("euclo.interaction_state", map[string]any{"mode": "review"})
	state.Set("euclo.artifacts", []Artifact{{Kind: "plan"}, {Kind: ""}, {Kind: "report"}})
	if got := resumedModeFromState(state); got != "debug" {
		t.Fatalf("unexpected resumed mode: %q", got)
	}
	if got := previousArtifactKinds(state); !reflect.DeepEqual(got, []string{"plan", "report"}) {
		t.Fatalf("unexpected previous artifact kinds: %#v", got)
	}

	modeRegistry := euclotypes.DefaultModeRegistry()
	profileRegistry := euclotypes.DefaultExecutionProfileRegistry()
	env := TaskEnvelope{Instruction: "please review this", EditPermitted: false}
	classification := TaskClassification{RecommendedMode: "review", RequiresDeterministicStages: true, ReasonCodes: []string{"reason"}}
	resolved := ResolveMode(env, classification, modeRegistry)
	if resolved.ModeID != "review" {
		t.Fatalf("unexpected resolved mode: %#v", resolved)
	}
	selection := SelectExecutionProfile(env, classification, resolved, profileRegistry)
	if selection.ProfileID != "review_suggest_implement" {
		t.Fatalf("unexpected profile selection: %#v", selection)
	}
	if looksLikeSummaryOnlyTask("report status for current status") != true {
		t.Fatal("expected summary-only detection")
	}

	if !verificationBoolValue("TRUE") || verificationBoolValue("nope") {
		t.Fatal("unexpected verification bool parsing")
	}
	parsed := verificationEvidenceFromMap(map[string]any{
		"status":    "pass",
		"summary":   "ok",
		"run_id":    "run-1",
		"source":    "fallback",
		"timestamp": "2026-04-08T19:00:00Z",
		"checks": []any{
			map[string]any{"name": "go test", "status": "pass", "run_id": "run-1", "working_directory": "/w"},
		},
	})
	if parsed.Provenance != VerificationProvenanceFallback || len(parsed.Checks) != 1 || parsed.RunID != "run-1" {
		t.Fatalf("unexpected verification evidence: %#v", parsed)
	}
	if got := verificationProvenanceFromRaw("Manual"); got != VerificationProvenanceManual {
		t.Fatalf("unexpected provenance: %q", got)
	}
	if got := inferVerificationProvenance(map[string]any{"summary": "reused verification"}); got != VerificationProvenanceReused {
		t.Fatalf("unexpected inferred provenance: %q", got)
	}
	if got := stringSliceAnyVerification([]any{"a", 1, nil}); !reflect.DeepEqual(got, []string{"a", "1"}) {
		t.Fatalf("unexpected string slice verification conversion: %#v", got)
	}
	if intValueAny("42") != 42 || intValueAny(nil) != 0 {
		t.Fatal("unexpected int conversion")
	}

	ambiguous := ScoredClassification{
		Candidates: []ModeCandidate{{Mode: "review", Score: 1.0}, {Mode: "code", Score: 0.2}},
	}
	frame := BuildAmbiguityFrame(ambiguous)
	if frame.Mode != "classification" || len(frame.Actions) < 2 {
		t.Fatalf("unexpected ambiguity frame: %#v", frame)
	}
	if ResolveAmbiguity(ambiguous, interaction.UserResponse{ActionID: "planning"}) != "planning" {
		t.Fatal("expected planning override")
	}
	if ResolveAmbiguity(ambiguous, interaction.UserResponse{ActionID: "review"}) != "review" {
		t.Fatal("expected candidate match")
	}
	if ResolveAmbiguity(ScoredClassification{}, interaction.UserResponse{}) != "code" {
		t.Fatal("expected default code fallback")
	}
	if modeLabel("debug") == "" || modeDescription("planning") == "" {
		t.Fatal("expected mode label/description strings")
	}
}

func TestTaskSelectionAndVerificationPolicyHelpers(t *testing.T) {
	profileRegistry := euclotypes.DefaultExecutionProfileRegistry()
	env := TaskEnvelope{
		Instruction:        "fix the failing test",
		CapabilitySnapshot: euclotypes.CapabilitySnapshot{HasWriteTools: true, HasVerificationTools: true},
		EditPermitted:      true,
	}
	classification := TaskClassification{RecommendedMode: "debug", RequiresEvidenceBeforeMutation: true}
	mode := ModeResolution{ModeID: "debug"}
	selection := SelectExecutionProfile(env, classification, mode, profileRegistry)
	if selection.ProfileID != "reproduce_localize_patch" {
		t.Fatalf("unexpected evidence-first selection: %#v", selection)
	}
	verifyPolicy := ResolveVerificationPolicy(mode, selection)
	if verifyPolicy.ManualOutcomeAllowed {
		t.Fatalf("unexpected manual outcome allowance: %#v", verifyPolicy)
	}
	if got := NormalizeVerificationEvidence(nil); got.Status != "not_verified" {
		t.Fatalf("unexpected nil evidence normalization: %#v", got)
	}
	if got := NormalizeVerificationEvidence(core.NewContext()); got.Status != "not_verified" {
		t.Fatalf("unexpected empty evidence normalization: %#v", got)
	}
}

func requireNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
