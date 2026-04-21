package execution_test

import (
	"errors"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/euclo/execution"
	eucloruntime "codeburg.org/lexbit/relurpify/named/euclo/runtime"
)

// ---------------------------------------------------------------------------
// UniqueStrings
// ---------------------------------------------------------------------------

func TestUniqueStrings_NilInputReturnsNil(t *testing.T) {
	if got := execution.UniqueStrings(nil); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestUniqueStrings_EmptySliceReturnsNil(t *testing.T) {
	if got := execution.UniqueStrings([]string{}); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestUniqueStrings_RemovesDuplicates(t *testing.T) {
	got := execution.UniqueStrings([]string{"a", "b", "a", "c", "b"})
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i, v := range want {
		if got[i] != v {
			t.Fatalf("got[%d] = %q, want %q", i, got[i], v)
		}
	}
}

func TestUniqueStrings_TrimsWhitespace(t *testing.T) {
	got := execution.UniqueStrings([]string{"  a  ", "b", " a "})
	if len(got) != 2 {
		t.Fatalf("expected 2 unique values after trim, got %v", got)
	}
}

func TestUniqueStrings_SkipsEmptyAfterTrim(t *testing.T) {
	got := execution.UniqueStrings([]string{"  ", "", "a"})
	if len(got) != 1 || got[0] != "a" {
		t.Fatalf("expected only 'a', got %v", got)
	}
}

// ---------------------------------------------------------------------------
// ContainsString
// ---------------------------------------------------------------------------

func TestContainsString_FoundReturnTrue(t *testing.T) {
	if !execution.ContainsString([]string{"x", "y", "z"}, "y") {
		t.Fatal("expected true")
	}
}

func TestContainsString_NotFoundReturnsFalse(t *testing.T) {
	if execution.ContainsString([]string{"x", "y"}, "z") {
		t.Fatal("expected false")
	}
}

func TestContainsString_EmptySliceReturnsFalse(t *testing.T) {
	if execution.ContainsString(nil, "x") {
		t.Fatal("expected false for nil slice")
	}
}

func TestContainsString_TrimsWhitespace(t *testing.T) {
	if !execution.ContainsString([]string{"  x  "}, "x") {
		t.Fatal("expected true with whitespace trim")
	}
}

// ---------------------------------------------------------------------------
// StringValue
// ---------------------------------------------------------------------------

func TestStringValue_StringInput(t *testing.T) {
	if got := execution.StringValue("hello"); got != "hello" {
		t.Fatalf("got %q", got)
	}
}

func TestStringValue_TrimsWhitespace(t *testing.T) {
	if got := execution.StringValue("  hi  "); got != "hi" {
		t.Fatalf("got %q", got)
	}
}

func TestStringValue_NonStringReturnsEmpty(t *testing.T) {
	if got := execution.StringValue(42); got != "" {
		t.Fatalf("expected empty string for non-string input, got %q", got)
	}
}

func TestStringValue_NilReturnsEmpty(t *testing.T) {
	if got := execution.StringValue(nil); got != "" {
		t.Fatalf("expected empty string for nil, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// CapabilityTaskInstruction
// ---------------------------------------------------------------------------

func TestCapabilityTaskInstruction_NilTaskReturnsDefault(t *testing.T) {
	got := execution.CapabilityTaskInstruction(nil)
	if got != "the requested change" {
		t.Fatalf("got %q", got)
	}
}

func TestCapabilityTaskInstruction_EmptyInstructionReturnsDefault(t *testing.T) {
	got := execution.CapabilityTaskInstruction(&core.Task{Instruction: "  "})
	if got != "the requested change" {
		t.Fatalf("got %q", got)
	}
}

func TestCapabilityTaskInstruction_ReturnsTrimmmedInstruction(t *testing.T) {
	got := execution.CapabilityTaskInstruction(&core.Task{Instruction: "  fix bug  "})
	if got != "fix bug" {
		t.Fatalf("got %q", got)
	}
}

// ---------------------------------------------------------------------------
// ResultSummary
// ---------------------------------------------------------------------------

func TestResultSummary_NilReturnsEmpty(t *testing.T) {
	if got := execution.ResultSummary(nil); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestResultSummary_DataSummaryField(t *testing.T) {
	r := &core.Result{Data: map[string]any{"summary": "all good"}}
	if got := execution.ResultSummary(r); got != "all good" {
		t.Fatalf("got %q", got)
	}
}

func TestResultSummary_ErrorFallback(t *testing.T) {
	r := &core.Result{Error: errors.New("something broke")}
	if got := execution.ResultSummary(r); got != "something broke" {
		t.Fatalf("got %q", got)
	}
}

func TestResultSummary_DefaultCompleted(t *testing.T) {
	r := &core.Result{Success: true}
	if got := execution.ResultSummary(r); got != "completed" {
		t.Fatalf("got %q", got)
	}
}

// ---------------------------------------------------------------------------
// ErrorMessage
// ---------------------------------------------------------------------------

func TestErrorMessage_ErrTakesPriority(t *testing.T) {
	err := errors.New("primary err")
	r := &core.Result{Error: errors.New("result err")}
	if got := execution.ErrorMessage(err, r); got != "primary err" {
		t.Fatalf("got %q", got)
	}
}

func TestErrorMessage_ResultErrorFallback(t *testing.T) {
	r := &core.Result{Error: errors.New("result err")}
	if got := execution.ErrorMessage(nil, r); got != "result err" {
		t.Fatalf("got %q", got)
	}
}

func TestErrorMessage_NoErrorReturnsUnknown(t *testing.T) {
	if got := execution.ErrorMessage(nil, nil); got != "unknown error" {
		t.Fatalf("got %q", got)
	}
}

// ---------------------------------------------------------------------------
// SuccessResult
// ---------------------------------------------------------------------------

func TestSuccessResult_ReturnsTrueResult(t *testing.T) {
	r, err := execution.SuccessResult("done", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !r.Success {
		t.Fatal("expected Success=true")
	}
	if r.Data["summary"] != "done" {
		t.Fatalf("expected summary 'done', got %v", r.Data["summary"])
	}
}

// ---------------------------------------------------------------------------
// AppendDiagnostic
// ---------------------------------------------------------------------------

func TestAppendDiagnostic_NilStateDoesNotPanic(t *testing.T) {
	execution.AppendDiagnostic(nil, "key", "msg")
}

func TestAppendDiagnostic_EmptyKeyDoesNothing(t *testing.T) {
	state := core.NewContext()
	execution.AppendDiagnostic(state, "", "msg")
	if _, ok := state.Get(""); ok {
		t.Fatal("unexpected key set")
	}
}

func TestAppendDiagnostic_AppendsMessage(t *testing.T) {
	state := core.NewContext()
	execution.AppendDiagnostic(state, "diag.key", "first")
	execution.AppendDiagnostic(state, "diag.key", "second")
	raw, ok := state.Get("diag.key")
	if !ok {
		t.Fatal("expected diag.key to be set")
	}
	payload := raw.(map[string]any)
	diags := payload["diagnostics"].([]string)
	if len(diags) != 2 {
		t.Fatalf("expected 2 diagnostics, got %v", diags)
	}
	if diags[0] != "first" || diags[1] != "second" {
		t.Fatalf("unexpected diagnostic values: %v", diags)
	}
}

func TestAppendDiagnostic_DeduplicatesMessages(t *testing.T) {
	state := core.NewContext()
	execution.AppendDiagnostic(state, "diag.key", "same")
	execution.AppendDiagnostic(state, "diag.key", "same")
	raw, _ := state.Get("diag.key")
	payload := raw.(map[string]any)
	diags := payload["diagnostics"].([]string)
	if len(diags) != 1 {
		t.Fatalf("expected 1 deduplicated diagnostic, got %v", diags)
	}
}

// ---------------------------------------------------------------------------
// SupportingIDs
// ---------------------------------------------------------------------------

func TestSupportingIDs_FiltersByPrefix(t *testing.T) {
	work := eucloruntime.UnitOfWork{ExecutionDescriptor: eucloruntime.ExecutionDescriptor{SupportingRelurpicCapabilityIDs: []string{
		"euclo:chat.local-review",
		"euclo:debug.root-cause",
		"euclo:chat.inspect",
	}},
	}
	got := execution.SupportingIDs(work, "euclo:chat.")
	if len(got) != 2 {
		t.Fatalf("expected 2 chat IDs, got %v", got)
	}
}

func TestSupportingIDs_EmptyWorkReturnsEmpty(t *testing.T) {
	if got := execution.SupportingIDs(eucloruntime.UnitOfWork{}, "euclo:"); len(got) != 0 {
		t.Fatalf("expected empty, got %v", got)
	}
}

// ---------------------------------------------------------------------------
// SetBehaviorTrace
// ---------------------------------------------------------------------------

func TestSetBehaviorTrace_NilStateDoesNotPanic(t *testing.T) {
	execution.SetBehaviorTrace(nil, eucloruntime.UnitOfWork{}, nil)
}

func TestSetBehaviorTrace_StoresTrace(t *testing.T) {
	state := core.NewContext()
	work := eucloruntime.UnitOfWork{ExecutionDescriptor: eucloruntime.ExecutionDescriptor{PrimaryRelurpicCapabilityID: "euclo:chat.ask",
		ExecutorDescriptor: eucloruntime.WorkUnitExecutorDescriptor{
			Family:   "react",
			RecipeID: "chat.ask.inquiry",
		}},
	}
	execution.SetBehaviorTrace(state, work, []string{"euclo:chat.inspect"})
	raw, ok := state.Get("euclo.relurpic_behavior_trace")
	if !ok {
		t.Fatal("expected behavior trace in state")
	}
	trace := raw.(execution.Trace)
	if trace.PrimaryCapabilityID != "euclo:chat.ask" {
		t.Fatalf("unexpected primary capability ID: %q", trace.PrimaryCapabilityID)
	}
	if len(trace.SupportingRoutines) != 1 || trace.SupportingRoutines[0] != "euclo:chat.inspect" {
		t.Fatalf("unexpected supporting routines: %v", trace.SupportingRoutines)
	}
	if trace.ExecutorFamily != "react" {
		t.Fatalf("unexpected executor family: %q", trace.ExecutorFamily)
	}
}

func TestSetBehaviorTrace_DeduplicatesRecipeIDs(t *testing.T) {
	state := core.NewContext()
	work := eucloruntime.UnitOfWork{ExecutionDescriptor: eucloruntime.ExecutionDescriptor{ExecutorDescriptor: eucloruntime.WorkUnitExecutorDescriptor{RecipeID: "r1"}}}
	execution.SetBehaviorTrace(state, work, nil)
	execution.SetBehaviorTrace(state, work, nil) // same recipe ID again
	raw, _ := state.Get("euclo.relurpic_behavior_trace")
	trace := raw.(execution.Trace)
	if len(trace.RecipeIDs) != 1 {
		t.Fatalf("expected 1 recipe ID after dedup, got %v", trace.RecipeIDs)
	}
}

// ---------------------------------------------------------------------------
// AddSpecializedCapabilityTrace
// ---------------------------------------------------------------------------

func TestAddSpecializedCapabilityTrace_NilStateDoesNotPanic(t *testing.T) {
	execution.AddSpecializedCapabilityTrace(nil, "x")
}

func TestAddSpecializedCapabilityTrace_EmptyIDDoesNothing(t *testing.T) {
	state := core.NewContext()
	execution.AddSpecializedCapabilityTrace(state, "  ")
	if _, ok := state.Get("euclo.relurpic_behavior_trace"); ok {
		t.Fatal("did not expect trace to be set for empty ID")
	}
}

func TestAddSpecializedCapabilityTrace_AddsAndDeduplicates(t *testing.T) {
	state := core.NewContext()
	execution.AddSpecializedCapabilityTrace(state, "euclo:local.diff-summary")
	execution.AddSpecializedCapabilityTrace(state, "euclo:local.diff-summary")
	execution.AddSpecializedCapabilityTrace(state, "euclo:local.other")
	raw, _ := state.Get("euclo.relurpic_behavior_trace")
	trace := raw.(execution.Trace)
	if len(trace.SpecializedCapabilityIDs) != 2 {
		t.Fatalf("expected 2 unique specialized IDs, got %v", trace.SpecializedCapabilityIDs)
	}
}

// ---------------------------------------------------------------------------
// EnsureRoutineArtifacts
// ---------------------------------------------------------------------------

func TestEnsureRoutineArtifacts_NilStateDoesNotPanic(t *testing.T) {
	execution.EnsureRoutineArtifacts(nil, "euclo:chat.local-review", eucloruntime.UnitOfWork{})
}

func TestEnsureRoutineArtifacts_LocalReviewSeedsKey(t *testing.T) {
	state := core.NewContext()
	execution.EnsureRoutineArtifacts(state, "euclo:chat.local-review", eucloruntime.UnitOfWork{})
	if _, ok := state.Get("euclo.review_findings"); !ok {
		t.Fatal("expected euclo.review_findings to be seeded")
	}
}

func TestEnsureRoutineArtifacts_LocalReviewDoesNotOverwriteExisting(t *testing.T) {
	state := core.NewContext()
	state.Set("euclo.review_findings", map[string]any{"custom": true})
	execution.EnsureRoutineArtifacts(state, "euclo:chat.local-review", eucloruntime.UnitOfWork{})
	raw, _ := state.Get("euclo.review_findings")
	payload := raw.(map[string]any)
	if payload["custom"] != true {
		t.Fatal("expected existing value to be preserved")
	}
}

func TestEnsureRoutineArtifacts_TargetedVerificationRepairSeedsKey(t *testing.T) {
	state := core.NewContext()
	execution.EnsureRoutineArtifacts(state, "euclo:chat.targeted-verification-repair", eucloruntime.UnitOfWork{})
	if _, ok := state.Get("euclo.verification_summary"); !ok {
		t.Fatal("expected euclo.verification_summary to be seeded")
	}
}

func TestEnsureRoutineArtifacts_DebugRootCauseSeedsKey(t *testing.T) {
	state := core.NewContext()
	execution.EnsureRoutineArtifacts(state, "euclo:debug.root-cause", eucloruntime.UnitOfWork{})
	if _, ok := state.Get("euclo.root_cause_candidates"); !ok {
		t.Fatal("expected euclo.root_cause_candidates to be seeded")
	}
}

func TestEnsureRoutineArtifacts_DebugLocalizationSeedsKey(t *testing.T) {
	state := core.NewContext()
	execution.EnsureRoutineArtifacts(state, "euclo:debug.localization", eucloruntime.UnitOfWork{})
	if _, ok := state.Get("euclo.root_cause"); !ok {
		t.Fatal("expected euclo.root_cause to be seeded")
	}
}

func TestEnsureRoutineArtifacts_DebugVerificationRepairSeedsKey(t *testing.T) {
	state := core.NewContext()
	execution.EnsureRoutineArtifacts(state, "euclo:debug.verification-repair", eucloruntime.UnitOfWork{})
	if _, ok := state.Get("euclo.regression_analysis"); !ok {
		t.Fatal("expected euclo.regression_analysis to be seeded")
	}
}

func TestEnsureRoutineArtifacts_PatternSurfaceSeedsKey(t *testing.T) {
	state := core.NewContext()
	execution.EnsureRoutineArtifacts(state, "euclo:archaeology.pattern-surface", eucloruntime.UnitOfWork{})
	if _, ok := state.Get("pipeline.explore"); !ok {
		t.Fatal("expected pipeline.explore to be seeded")
	}
}

func TestEnsureRoutineArtifacts_ProspectiveAssessSeedsPlanCandidates(t *testing.T) {
	state := core.NewContext()
	execution.EnsureRoutineArtifacts(state, "euclo:archaeology.prospective-assess", eucloruntime.UnitOfWork{})
	raw, ok := state.Get("euclo.plan_candidates")
	if !ok {
		t.Fatal("expected euclo.plan_candidates to be seeded")
	}
	payload := raw.(map[string]any)
	ops := payload["operations"].([]string)
	if len(ops) != 1 || ops[0] != "euclo:archaeology.prospective-assess" {
		t.Fatalf("unexpected operations: %v", ops)
	}
}

func TestEnsureRoutineArtifacts_ConvergenceGuardAppendsOperation(t *testing.T) {
	state := core.NewContext()
	execution.EnsureRoutineArtifacts(state, "euclo:archaeology.prospective-assess", eucloruntime.UnitOfWork{})
	execution.EnsureRoutineArtifacts(state, "euclo:archaeology.convergence-guard", eucloruntime.UnitOfWork{})
	raw, _ := state.Get("euclo.plan_candidates")
	payload := raw.(map[string]any)
	ops := payload["operations"].([]string)
	if len(ops) != 2 {
		t.Fatalf("expected 2 operations, got %v", ops)
	}
}

// ---------------------------------------------------------------------------
// ExecuteWorkflow — nil executor path
// ---------------------------------------------------------------------------

func TestExecuteWorkflow_NilExecutorReturnsError(t *testing.T) {
	in := execution.ExecuteInput{State: core.NewContext()}
	r, err := execution.ExecuteWorkflow(nil, in) //nolint:staticcheck
	if err == nil {
		t.Fatal("expected error when workflow executor is nil")
	}
	if r.Success {
		t.Fatal("expected Success=false")
	}
}

// ---------------------------------------------------------------------------
// MergeStateArtifactsToContext
// ---------------------------------------------------------------------------

func TestMergeStateArtifactsToContext_NilStateDoesNotPanic(t *testing.T) {
	execution.MergeStateArtifactsToContext(nil, nil)
}

func TestMergeStateArtifactsToContext_SetsArtifactsKey(t *testing.T) {
	state := core.NewContext()
	artifacts := []interface{}{} // use euclo artifacts
	_ = artifacts
	// Use a concrete euclotypes artifact via state key
	state.Set("euclo.artifacts", []map[string]any{})
	execution.MergeStateArtifactsToContext(state, nil)
}

// ---------------------------------------------------------------------------
// CompilePlanFallback
// ---------------------------------------------------------------------------

func TestCompilePlanFallback_EmptyInputsReturnsNil(t *testing.T) {
	got := execution.CompilePlanFallback(eucloruntime.UnitOfWork{})
	if got != nil {
		t.Fatalf("expected nil for empty work, got %v", got)
	}
}

func TestCompilePlanFallback_PatternProposalsProduceSteps(t *testing.T) {
	work := eucloruntime.UnitOfWork{ExecutionDescriptor: eucloruntime.ExecutionDescriptor{PrimaryRelurpicCapabilityID: "euclo:archaeology.compile-plan",
		SemanticInputs: eucloruntime.SemanticInputBundle{
			PatternProposals: []eucloruntime.PatternProposalSummary{
				{Title: "step 1", Summary: "do first thing", PatternRefs: []string{"ref-a"}},
			},
		}},
	}
	got := execution.CompilePlanFallback(work)
	if got == nil {
		t.Fatal("expected non-nil plan fallback")
	}
	steps := got["steps"].([]map[string]any)
	if len(steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(steps))
	}
	if steps[0]["title"] != "step 1" {
		t.Fatalf("unexpected step title: %v", steps[0]["title"])
	}
}

func TestCompilePlanFallback_CoherenceSuggestionsProduceSteps(t *testing.T) {
	work := eucloruntime.UnitOfWork{ExecutionDescriptor: eucloruntime.ExecutionDescriptor{SemanticInputs: eucloruntime.SemanticInputBundle{
		CoherenceSuggestions: []eucloruntime.CoherenceSuggestion{
			{Title: "coherence fix", Summary: "align things", SuggestedAction: "refactor"},
		},
	}},
	}
	got := execution.CompilePlanFallback(work)
	if got == nil {
		t.Fatal("expected non-nil plan fallback")
	}
	steps := got["steps"].([]map[string]any)
	if steps[0]["suggested_action"] != "refactor" {
		t.Fatalf("unexpected suggested_action: %v", steps[0]["suggested_action"])
	}
}

// ---------------------------------------------------------------------------
// ExecuteSupportingRoutines — InvokeSupporting path
// ---------------------------------------------------------------------------

func TestExecuteSupportingRoutines_EmptyListReturnsEmpty(t *testing.T) {
	artifacts, executed, err := execution.ExecuteSupportingRoutines(nil, execution.ExecuteInput{}, nil)
	if err != nil || len(artifacts) != 0 || len(executed) != 0 {
		t.Fatalf("expected empty result, got artifacts=%v executed=%v err=%v", artifacts, executed, err)
	}
}

func TestExecuteSupportingRoutines_SeedsArtifactsWhenNoRunner(t *testing.T) {
	state := core.NewContext()
	in := execution.ExecuteInput{State: state}
	_, executed, err := execution.ExecuteSupportingRoutines(nil, in, []string{"euclo:chat.local-review"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(executed) != 1 || executed[0] != "euclo:chat.local-review" {
		t.Fatalf("unexpected executed list: %v", executed)
	}
	if _, ok := state.Get("euclo.review_findings"); !ok {
		t.Fatal("expected review findings seeded")
	}
}

func TestExecuteSupportingRoutines_SkipsEmptyRoutineIDs(t *testing.T) {
	state := core.NewContext()
	in := execution.ExecuteInput{State: state}
	_, executed, err := execution.ExecuteSupportingRoutines(nil, in, []string{"  ", "", "euclo:debug.root-cause"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(executed) != 1 {
		t.Fatalf("expected 1 executed routine, got %v", executed)
	}
}

// ---------------------------------------------------------------------------
// PropagateBehaviorTrace
// ---------------------------------------------------------------------------

func TestPropagateBehaviorTrace_NilSrcDoesNothing(t *testing.T) {
	dst := core.NewContext()
	execution.PropagateBehaviorTrace(dst, nil)
	if _, ok := dst.Get("euclo.relurpic_behavior_trace"); ok {
		t.Fatal("expected no trace propagated from nil src")
	}
}

func TestPropagateBehaviorTrace_CopiesTraceFromSrcToDst(t *testing.T) {
	src := core.NewContext()
	src.Set("euclo.relurpic_behavior_trace", execution.Trace{PrimaryCapabilityID: "euclo:chat.ask"})
	dst := core.NewContext()
	execution.PropagateBehaviorTrace(dst, src)
	raw, ok := dst.Get("euclo.relurpic_behavior_trace")
	if !ok {
		t.Fatal("expected trace propagated to dst")
	}
	trace := raw.(execution.Trace)
	if trace.PrimaryCapabilityID != "euclo:chat.ask" {
		t.Fatalf("unexpected primary capability ID: %q", trace.PrimaryCapabilityID)
	}
}
