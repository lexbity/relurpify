package runtime

import (
	"testing"

	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
)

func TestClassifyTask_BackwardCompat(t *testing.T) {
	// Verify ClassifyTask still returns a valid TaskClassification.
	envelope := TaskEnvelope{
		Instruction:        "fix the broken handler",
		EditPermitted:      true,
		CapabilitySnapshot: euclotypes.CapabilitySnapshot{HasWriteTools: true},
	}
	c := ClassifyTask(envelope)
	if c.RecommendedMode == "" {
		t.Error("expected non-empty recommended mode")
	}
	if len(c.IntentFamilies) == 0 {
		t.Error("expected non-empty intent families")
	}
}

func TestClassifyTaskScored_DebugClear(t *testing.T) {
	envelope := TaskEnvelope{
		Instruction:   "debug why this panic happens: goroutine 1 [running]",
		EditPermitted: true,
	}
	scored := ClassifyTaskScored(envelope)
	if scored.RecommendedMode != "debug" {
		t.Errorf("mode: got %q, want debug", scored.RecommendedMode)
	}
	if len(scored.Candidates) == 0 {
		t.Fatal("expected candidates")
	}
	if scored.Candidates[0].Mode != "debug" {
		t.Errorf("top candidate: got %q", scored.Candidates[0].Mode)
	}
	if scored.Ambiguous {
		t.Error("should not be ambiguous — debug has many signals")
	}
}

func TestClassifyTaskScored_CodeClear(t *testing.T) {
	envelope := TaskEnvelope{
		Instruction:   "implement a new HTTP handler for /api/users",
		EditPermitted: true,
	}
	scored := ClassifyTaskScored(envelope)
	if scored.RecommendedMode != "code" {
		t.Errorf("mode: got %q, want code", scored.RecommendedMode)
	}
}

func TestClassifyTaskScored_Ambiguous(t *testing.T) {
	// "fix" → code, "failing" → debug. Close enough to be ambiguous.
	envelope := TaskEnvelope{
		Instruction:   "fix the failing assertion",
		EditPermitted: true,
	}
	scored := ClassifyTaskScored(envelope)
	if len(scored.Candidates) < 2 {
		t.Fatalf("expected at least 2 candidates, got %d", len(scored.Candidates))
	}
	// Both code and debug should be present.
	modes := map[string]bool{}
	for _, c := range scored.Candidates {
		modes[c.Mode] = true
	}
	if !modes["code"] || !modes["debug"] {
		t.Errorf("expected both code and debug, got %v", modes)
	}
}

func TestClassifyTaskScored_ModeHintOverrides(t *testing.T) {
	envelope := TaskEnvelope{
		Instruction:   "fix this",
		ModeHint:      "debug",
		EditPermitted: true,
	}
	scored := ClassifyTaskScored(envelope)
	// Mode hint should push debug to top due to high weight.
	if scored.Candidates[0].Mode != "debug" {
		t.Errorf("top: got %q, want debug (mode hint)", scored.Candidates[0].Mode)
	}
}

func TestClassifyTaskScored_NoSignals_DefaultCode(t *testing.T) {
	envelope := TaskEnvelope{
		Instruction:   "hello world",
		EditPermitted: true,
	}
	scored := ClassifyTaskScored(envelope)
	if scored.RecommendedMode != "code" {
		t.Errorf("mode: got %q, want code (default)", scored.RecommendedMode)
	}
}

func TestClassifyTaskScored_ErrorText(t *testing.T) {
	envelope := TaskEnvelope{
		Instruction:   "panic: nil pointer dereference at server.go:42",
		EditPermitted: true,
	}
	scored := ClassifyTaskScored(envelope)
	if scored.RecommendedMode != "debug" {
		t.Errorf("mode: got %q, want debug", scored.RecommendedMode)
	}
	// Should have error_text signals.
	hasErrorText := false
	for _, s := range scored.Signals {
		if s.Kind == "error_text" {
			hasErrorText = true
			break
		}
	}
	if !hasErrorText {
		t.Error("expected error_text signals")
	}
}

func TestClassifyTaskScored_TaskStructure(t *testing.T) {
	envelope := TaskEnvelope{
		Instruction:   "the login page used to work but now shows a blank screen",
		EditPermitted: true,
	}
	scored := ClassifyTaskScored(envelope)
	if scored.RecommendedMode != "debug" {
		t.Errorf("mode: got %q, want debug (task structure)", scored.RecommendedMode)
	}
}

func TestClassifyTaskScored_TDD(t *testing.T) {
	envelope := TaskEnvelope{
		Instruction:   "write tests for handler_test.go first, then implement",
		EditPermitted: true,
	}
	scored := ClassifyTaskScored(envelope)
	// TDD should be a candidate.
	hasTDD := false
	for _, c := range scored.Candidates {
		if c.Mode == "tdd" {
			hasTDD = true
			break
		}
	}
	if !hasTDD {
		t.Error("expected tdd candidate")
	}
}

func TestClassifyTaskScored_Planning(t *testing.T) {
	envelope := TaskEnvelope{
		Instruction:   "how should we redesign the authentication system",
		EditPermitted: true,
	}
	scored := ClassifyTaskScored(envelope)
	if scored.RecommendedMode != "planning" {
		t.Errorf("mode: got %q, want planning", scored.RecommendedMode)
	}
}

func TestClassifyTaskScored_AmbiguousDebugOrCodePrefersDebug(t *testing.T) {
	envelope := TaskEnvelope{
		Instruction:   "this test used to pass but now it doesn't after I changed the handler",
		EditPermitted: true,
	}
	scored := ClassifyTaskScored(envelope)
	if scored.RecommendedMode != "debug" {
		t.Fatalf("mode: got %q, want debug", scored.RecommendedMode)
	}
	if len(scored.Candidates) < 2 {
		t.Fatalf("expected multiple candidates, got %d", len(scored.Candidates))
	}
	if scored.Candidates[0].Mode != "debug" {
		t.Fatalf("top candidate: got %q, want debug", scored.Candidates[0].Mode)
	}
}

func TestClassifyTaskScored_AmbiguousReviewOrCodeKeepsBothCandidates(t *testing.T) {
	envelope := TaskEnvelope{
		Instruction:   "look at the error handling in parser.go and fix anything that's wrong",
		EditPermitted: true,
	}
	scored := ClassifyTaskScored(envelope)
	if len(scored.Candidates) < 2 {
		t.Fatalf("expected multiple candidates, got %d", len(scored.Candidates))
	}
	modes := map[string]bool{}
	for _, c := range scored.Candidates {
		modes[c.Mode] = true
	}
	if !modes["review"] || !modes["code"] {
		t.Fatalf("expected review and code candidates, got %+v", scored.Candidates)
	}
}

func TestClassifyTaskScored_AmbiguousTDDOrCodeIncludesTDD(t *testing.T) {
	envelope := TaskEnvelope{
		Instruction:   "add input validation to CreateUser and make sure it's tested",
		EditPermitted: true,
	}
	scored := ClassifyTaskScored(envelope)
	modes := map[string]bool{}
	for _, c := range scored.Candidates {
		modes[c.Mode] = true
	}
	if !modes["code"] || !modes["tdd"] {
		t.Fatalf("expected code and tdd candidates, got %+v", scored.Candidates)
	}
}

func TestClassifyTaskScored_AmbiguousPlanningOrCodePrefersPlanning(t *testing.T) {
	envelope := TaskEnvelope{
		Instruction:   "design the fix for the authentication bug and then implement it across multiple handlers",
		EditPermitted: true,
	}
	scored := ClassifyTaskScored(envelope)
	if scored.RecommendedMode != "planning" {
		t.Fatalf("mode: got %q, want planning", scored.RecommendedMode)
	}
	modes := map[string]bool{}
	for _, c := range scored.Candidates {
		modes[c.Mode] = true
	}
	if !modes["planning"] || !modes["code"] {
		t.Fatalf("expected planning and code candidates, got %+v", scored.Candidates)
	}
}

func TestResolveMode_ExplicitOverrideBeatsClassifier(t *testing.T) {
	envelope := TaskEnvelope{
		Instruction:   "review the code in handler.go",
		ModeHint:      "code",
		EditPermitted: true,
	}
	classification := ClassifyTask(envelope)
	mode := ResolveMode(envelope, classification, euclotypes.DefaultModeRegistry())
	if mode.ModeID != "code" {
		t.Fatalf("mode: got %q, want code", mode.ModeID)
	}
	if mode.Source != "explicit" {
		t.Fatalf("source: got %q, want explicit", mode.Source)
	}
}

func TestClassifyTaskScored_ReadOnly(t *testing.T) {
	envelope := TaskEnvelope{
		Instruction:   "review the code",
		EditPermitted: false,
	}
	scored := ClassifyTaskScored(envelope)
	// Should have workspace_state read_only signal.
	hasReadOnly := false
	for _, s := range scored.Signals {
		if s.Value == "read_only" {
			hasReadOnly = true
		}
	}
	if !hasReadOnly {
		t.Error("expected read_only signal")
	}
}

func TestClassifyTaskScored_ConfidenceRange(t *testing.T) {
	envelope := TaskEnvelope{
		Instruction:   "debug this panic",
		EditPermitted: true,
	}
	scored := ClassifyTaskScored(envelope)
	if scored.Confidence < 0 || scored.Confidence > 1 {
		t.Errorf("confidence out of range: %f", scored.Confidence)
	}
}

// ──────────────────────────────────────────────────────────────
// Profile selection tests
// ──────────────────────────────────────────────────────────────

func TestProfileForCodeMode_Default(t *testing.T) {
	profile := profileForCodeMode(
		TaskEnvelope{EditPermitted: true},
		TaskClassification{Scope: "local"},
	)
	if profile != "edit_verify_repair" {
		t.Errorf("got %q", profile)
	}
}

func TestProfileForCodeMode_ReadOnly(t *testing.T) {
	profile := profileForCodeMode(
		TaskEnvelope{EditPermitted: false},
		TaskClassification{},
	)
	if profile != "plan_stage_execute" {
		t.Errorf("got %q", profile)
	}
}

func TestProfileForCodeMode_EvidenceFirst(t *testing.T) {
	profile := profileForCodeMode(
		TaskEnvelope{EditPermitted: true},
		TaskClassification{RequiresEvidenceBeforeMutation: true},
	)
	if profile != "reproduce_localize_patch" {
		t.Errorf("got %q", profile)
	}
}

func TestProfileForCodeMode_ExistingPlan(t *testing.T) {
	profile := profileForCodeMode(
		TaskEnvelope{
			EditPermitted:         true,
			PreviousArtifactKinds: []string{"euclo.plan"},
		},
		TaskClassification{Scope: "local"},
	)
	if profile != "edit_verify_repair" {
		t.Errorf("got %q, want edit_verify_repair (plan exists)", profile)
	}
}

func TestProfileForCodeMode_ExistingExploration(t *testing.T) {
	profile := profileForCodeMode(
		TaskEnvelope{
			EditPermitted:         true,
			PreviousArtifactKinds: []string{"euclo.explore"},
		},
		TaskClassification{Scope: "local"},
	)
	if profile != "reproduce_localize_patch" {
		t.Errorf("got %q, want reproduce_localize_patch (exploration exists)", profile)
	}
}

func TestProfileForCodeMode_CrossCutting(t *testing.T) {
	profile := profileForCodeMode(
		TaskEnvelope{EditPermitted: true},
		TaskClassification{Scope: "cross_cutting"},
	)
	if profile != "plan_stage_execute" {
		t.Errorf("got %q, want plan_stage_execute (cross-cutting)", profile)
	}
}
