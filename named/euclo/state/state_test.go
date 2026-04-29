package state

import (
	"testing"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/named/euclo/intake"
	"codeburg.org/lexbit/relurpify/named/euclo/orchestrate"
)

// === Phase 2 Unit Tests ===

func TestAllKeysUnique(t *testing.T) {
	keys := []string{
		KeyTaskEnvelope,
		KeyIntentClassification,
		KeyRouteSelection,
		KeyClassificationMetadata,
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
		KeyRecipeID,
		KeyRecipeVersion,
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
		KeyCapabilitySequence,
	}

	seen := make(map[string]bool)
	for _, key := range keys {
		if seen[key] {
			t.Errorf("Duplicate key: %s", key)
		}
		seen[key] = true
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

	rs := &orchestrate.RouteSelection{
		RouteKind:    "recipe",
		RecipeID:     "recipe-123",
		CapabilityID: "",
	}

	SetRouteSelection(env, rs)

	retrieved, ok := GetRouteSelection(env)
	if !ok {
		t.Fatal("Failed to retrieve RouteSelection")
	}
	if retrieved.RouteKind != rs.RouteKind {
		t.Errorf("RouteKind mismatch: got %q, want %q", retrieved.RouteKind, rs.RouteKind)
	}
	if retrieved.RecipeID != rs.RecipeID {
		t.Errorf("RecipeID mismatch: got %q, want %q", retrieved.RecipeID, rs.RecipeID)
	}
}

func TestSetGetJobRecords(t *testing.T) {
	env := contextdata.NewEnvelope("test-task", "test-session")

	records := []JobRecord{
		{ID: "job-1", Type: "analysis", Status: "completed"},
		{ID: "job-2", Type: "execution", Status: "pending"},
	}

	SetJobRecords(env, records)

	retrieved, ok := GetJobRecords(env)
	if !ok {
		t.Fatal("Failed to retrieve JobRecords")
	}
	if len(retrieved) != 2 {
		t.Errorf("Expected 2 records, got %d", len(retrieved))
	}

	// Test append
	AppendJobRecord(env, JobRecord{ID: "job-3", Type: "reporting", Status: "running"})

	retrieved, ok = GetJobRecords(env)
	if !ok {
		t.Fatal("Failed to retrieve JobRecords after append")
	}
	if len(retrieved) != 3 {
		t.Errorf("Expected 3 records after append, got %d", len(retrieved))
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

func TestRecipeCaptureKeyConstruction(t *testing.T) {
	key := RecipeCaptureKey("tdd", "test_output")
	expected := "euclo.recipe.tdd.test_output"
	if key != expected {
		t.Errorf("RecipeCaptureKey = %q, want %q", key, expected)
	}
}

func TestSetGetClassificationMetadata(t *testing.T) {
	env := contextdata.NewEnvelope("test-task", "test-session")

	metadata := map[string]any{
		"family":    "debug",
		"score":     0.85,
		"ambiguous": false,
	}

	SetClassificationMetadata(env, metadata)

	retrieved, ok := GetClassificationMetadata(env)
	if !ok {
		t.Fatal("Failed to retrieve ClassificationMetadata")
	}
	if len(retrieved) != 3 {
		t.Errorf("Expected 3 metadata entries, got %d", len(retrieved))
	}
	if retrieved["family"] != "debug" {
		t.Errorf("family = %v, want debug", retrieved["family"])
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
}
