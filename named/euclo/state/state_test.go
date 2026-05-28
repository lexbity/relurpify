package state

import (
	"testing"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	"codeburg.org/lexbit/relurpify/named/euclo/intake"
	"codeburg.org/lexbit/relurpify/named/euclo/intentcontext"
)

// === Phase 2 Unit Tests ===

func TestAllKeysUnique(t *testing.T) {
	keys := []string{
		KeyTaskEnvelope,
		KeyIntentClassification,
		KeyIntentEvidence,
		KeyIntentInterpretation,
		KeyRouteSelection,
		KeyRouteResolution,
		KeyContextHint,
		KeyWorkspaceScopes,
		KeySessionHint,
		KeyFollowUpHint,
		KeyAgentModeHint,
		KeyUserSelectedFiles,
		KeyExplicitIngestPaths,
		KeyIncrementalSinceRef,
		KeyIngestPolicy,
		KeyIntentSignals,
		KeyFamilyScores,
		KeyThoughtRecipeID,
		KeyThoughtRecipeVersion,
		KeyPolicyDecision,
		KeyHITLTriggered,
		KeyHITLResponse,
		KeyDryRunMode,
		KeyOutcomeCategory,
		KeyOutcomeArtifacts,
		KeyOutcomeTelemetry,
		KeyResumeClassification,
		KeyResumeRoute,
		KeyStreamResult,
		KeyStreamTokenUsage,
		KeyFrameHistory,
		KeyJobRecords,
		KeyIngestionResult,
		KeyNegativeConstraints,
		KeyFamilySelection,
	}

	seen := make(map[string]bool)
	for _, key := range keys {
		if seen[key] {
			t.Errorf("Duplicate key: %s", key)
		}
		seen[key] = true
	}
}

func TestCanonicalWorkingMemoryKeysIncludeClarificationAndRouteState(t *testing.T) {
	keys := intentcontext.CanonicalWorkingMemoryKeys()
	want := map[string]bool{
		intentcontext.ClarificationStateKey:   true,
		intentcontext.IntentEvidenceKey:       true,
		intentcontext.IntentInterpretationKey: true,
		intentcontext.RouteResolutionKey:      true,
	}
	for _, key := range keys {
		delete(want, key)
	}
	if len(want) != 0 {
		t.Fatalf("canonical keys missing entries: %#v", want)
	}
}

func TestSetGetTaskEnvelope(t *testing.T) {
	env := contextdata.NewEnvelope("test-task", "test-session")

	envelope := &intake.TaskEnvelope{
		TaskID:      "task-123",
		Instruction: "Test instruction",
	}

	SetTaskEnvelope(env, envelope)

	retrieved, ok := GetTaskEnvelope(env)
	if !ok {
		t.Fatal("Failed to retrieve TaskEnvelope")
	}
	if retrieved.TaskID != envelope.TaskID {
		t.Errorf("TaskID mismatch: got %q, want %q", retrieved.TaskID, envelope.TaskID)
	}
	if retrieved.Instruction != envelope.Instruction {
		t.Errorf("Instruction mismatch: got %q, want %q", retrieved.Instruction, envelope.Instruction)
	}
}

func TestSetGetIntentClassification(t *testing.T) {
	env := contextdata.NewEnvelope("test-task", "test-session")

	ic := &intake.IntentClassification{
		WinningFamily: "debug",
		Confidence:    0.85,
		Ambiguous:     false,
	}

	SetIntentClassification(env, ic)

	retrieved, ok := GetIntentClassification(env)
	if !ok {
		t.Fatal("Failed to retrieve IntentClassification")
	}
	if retrieved.WinningFamily != ic.WinningFamily {
		t.Errorf("WinningFamily mismatch: got %q, want %q", retrieved.WinningFamily, ic.WinningFamily)
	}
	if retrieved.Confidence != ic.Confidence {
		t.Errorf("Confidence mismatch: got %f, want %f", retrieved.Confidence, ic.Confidence)
	}
}

func TestSetGetRouteSelection(t *testing.T) {
	env := contextdata.NewEnvelope("test-task", "test-session")

	rs := &euclotypes.RouteSelection{
		RouteKind:       "thoughtrecipe",
		ThoughtRecipeID: "thoughtrecipe-123",
		CapabilityID:    "",
	}

	SetRouteSelection(env, rs)

	retrieved, ok := GetRouteSelection(env)
	if !ok {
		t.Fatal("Failed to retrieve RouteSelection")
	}
	if retrieved.RouteKind != rs.RouteKind {
		t.Errorf("RouteKind mismatch: got %q, want %q", retrieved.RouteKind, rs.RouteKind)
	}
	if retrieved.ThoughtRecipeID != rs.ThoughtRecipeID {
		t.Errorf("ThoughtRecipeID mismatch: got %q, want %q", retrieved.ThoughtRecipeID, rs.ThoughtRecipeID)
	}
}

func TestSetGetIntentEvidence(t *testing.T) {
	env := contextdata.NewEnvelope("test-task", "test-session")

	evidence := &intentcontext.IntentEvidence{
		ActionType:            "implement",
		Target:                "named/euclo",
		Scope:                 "package",
		RiskLevel:             "medium",
		ExpectedVerb:          "change",
		ExplicitFiles:         []string{"named/euclo/state/accessors.go"},
		WorkspaceScopes:       []string{"named/euclo"},
		SessionPins:           []string{"checkpoint-1"},
		ContextHints:          []string{"clarify"},
		SessionContinuation:   "continue",
		FollowUp:              "update state keys",
		NegativeConstraints:   []string{"no stubs"},
		RequiresClarification: true,
		MissingFields:         []string{"route"},
		ReasonCodes:           []string{"missing_route"},
	}

	SetIntentEvidence(env, evidence)

	retrieved, ok := GetIntentEvidence(env)
	if !ok {
		t.Fatal("Failed to retrieve IntentEvidence")
	}
	if retrieved.ActionType != evidence.ActionType {
		t.Fatalf("ActionType = %q, want %q", retrieved.ActionType, evidence.ActionType)
	}
	if len(retrieved.ExplicitFiles) != 1 || retrieved.ExplicitFiles[0] != evidence.ExplicitFiles[0] {
		t.Fatalf("unexpected explicit files: %#v", retrieved.ExplicitFiles)
	}
}

func TestSetGetIntentInterpretation(t *testing.T) {
	env := contextdata.NewEnvelope("test-task", "test-session")

	interpretation := &intentcontext.IntentInterpretation{
		ActionType:     "review",
		Target:         "orchestrate",
		Scope:          "package",
		RiskLevel:      "low",
		MissingInfo:    []string{"route"},
		Rationale:      "route state should be typed",
		ConfidenceNote: "high confidence",
		ReasonCodes:    []string{"typed_route_state"},
	}

	SetIntentInterpretation(env, interpretation)

	retrieved, ok := GetIntentInterpretation(env)
	if !ok {
		t.Fatal("Failed to retrieve IntentInterpretation")
	}
	if retrieved.Target != interpretation.Target {
		t.Fatalf("Target = %q, want %q", retrieved.Target, interpretation.Target)
	}
	if len(retrieved.MissingInfo) != 1 || retrieved.MissingInfo[0] != "route" {
		t.Fatalf("unexpected missing info: %#v", retrieved.MissingInfo)
	}
}

func TestSetGetRouteResolution(t *testing.T) {
	env := contextdata.NewEnvelope("test-task", "test-session")

	resolution := &euclotypes.RouteResolution{
		RouteKind:                 euclotypes.RouteKindCapability,
		CapabilityID:              "euclo:cap.ast_query",
		ResolutionSource:          "deterministic",
		FallbackTaken:             true,
		ClarificationStateVersion: 7,
		ReasonCodes:               []string{"explicit_route", "registry_match"},
	}

	SetRouteResolution(env, resolution)

	retrieved, ok := GetRouteResolution(env)
	if !ok {
		t.Fatal("Failed to retrieve RouteResolution")
	}
	if retrieved.CapabilityID != resolution.CapabilityID {
		t.Fatalf("CapabilityID = %q, want %q", retrieved.CapabilityID, resolution.CapabilityID)
	}
	if !retrieved.FallbackTaken {
		t.Fatal("expected fallback flag to round-trip")
	}
	if got := retrieved.RouteID(); got != resolution.CapabilityID {
		t.Fatalf("RouteID = %q, want %q", got, resolution.CapabilityID)
	}
}

func TestSetGetNegativeConstraints(t *testing.T) {
	env := contextdata.NewEnvelope("test-task", "test-session")

	constraints := []string{"don't change API", "don't add dependencies"}

	SetNegativeConstraints(env, constraints)

	retrieved, ok := GetNegativeConstraints(env)
	if !ok {
		t.Fatal("Failed to retrieve NegativeConstraints")
	}
	if len(retrieved) != 2 {
		t.Errorf("Expected 2 constraints, got %d", len(retrieved))
	}
	if retrieved[0] != constraints[0] {
		t.Errorf("First constraint mismatch: got %q, want %q", retrieved[0], constraints[0])
	}
}

func TestThoughtRecipeCaptureKeyConstruction(t *testing.T) {
	key := ThoughtRecipeCaptureKey("tdd", "test_output")
	expected := "euclo.thoughtrecipe.tdd.test_output"
	if key != expected {
		t.Errorf("ThoughtRecipeCaptureKey = %q, want %q", key, expected)
	}
}

func TestSetGetContextHint(t *testing.T) {
	env := contextdata.NewEnvelope("test-task", "test-session")

	SetContextHint(env, "typescript-react")

	retrieved, ok := GetContextHint(env)
	if !ok {
		t.Fatal("Failed to retrieve ContextHint")
	}
	if retrieved != "typescript-react" {
		t.Errorf("ContextHint = %q, want %q", retrieved, "typescript-react")
	}
}

func TestSetGetWorkspaceScopes(t *testing.T) {
	env := contextdata.NewEnvelope("test-task", "test-session")

	scopes := []string{"backend", "frontend", "infra"}
	SetWorkspaceScopes(env, scopes)

	retrieved, ok := GetWorkspaceScopes(env)
	if !ok {
		t.Fatal("Failed to retrieve WorkspaceScopes")
	}
	if len(retrieved) != 3 {
		t.Errorf("WorkspaceScopes length = %d, want 3", len(retrieved))
	}
	if retrieved[1] != "frontend" {
		t.Errorf("WorkspaceScopes[1] = %q, want frontend", retrieved[1])
	}
}

func TestMissingKeyReturnsZero(t *testing.T) {
	env := contextdata.NewEnvelope("test-task", "test-session")

	// Try to get TaskEnvelope from empty envelope
	_, ok := GetTaskEnvelope(env)
	if ok {
		t.Error("Expected GetTaskEnvelope to return false for empty envelope")
	}

	// Try to get IntentClassification from empty envelope
	_, ok = GetIntentClassification(env)
	if ok {
		t.Error("Expected GetIntentClassification to return false for empty envelope")
	}

	// Try to get RouteSelection from empty envelope
	_, ok = GetRouteSelection(env)
	if ok {
		t.Error("Expected GetRouteSelection to return false for empty envelope")
	}
	_, ok = GetIntentEvidence(env)
	if ok {
		t.Error("Expected GetIntentEvidence to return false for empty envelope")
	}
	_, ok = GetIntentInterpretation(env)
	if ok {
		t.Error("Expected GetIntentInterpretation to return false for empty envelope")
	}
	_, ok = GetRouteResolution(env)
	if ok {
		t.Error("Expected GetRouteResolution to return false for empty envelope")
	}
}
