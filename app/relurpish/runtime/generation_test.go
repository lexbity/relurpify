package runtime

import (
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/userconfig/config"
)

// TestGenerationLifecycle tests the generation lifecycle states.
func TestGenerationLifecycle(t *testing.T) {
	// Create a workspace config snapshot for testing
	// For P05 tests, we create a minimal valid DocumentSnapshot
	snapshot := &config.DocumentSnapshot{
		Document: &config.Document{
			APIVersion: "v1",
			Kind:       "Workspace",
			Metadata:   config.DocumentMetadata{Name: "test-workspace"},
		},
		SourcePath: "test-workspace.yaml",
		LoadedAt:   time.Now().UTC(),
	}

	// Create a new generation
	gen := NewGeneration("gen_1", "test-workspace", snapshot)

	// Check initial state
	if gen.State() != GenerationStateBuilding {
		t.Errorf("Expected initial state to be Building, got %s", gen.State())
	}

	// Test state transitions
	validTransitions := []GenerationState{
		GenerationStateValidated,
	}

	for _, nextState := range validTransitions {
		if err := gen.SetState(nextState); err != nil {
			t.Errorf("Unexpected error transitioning from %s to %s: %v",
				gen.State(), nextState, err)
		}
	}

	// Test invalid transition
	if err := gen.SetState(GenerationStateActive); err == nil {
		t.Error("Expected error transitioning from Validated to Active directly")
	}

	// Complete the lifecycle
	_ = gen.SetState(GenerationStateValidated)
	_ = gen.SetState(GenerationStateQuiescing)
	_ = gen.SetState(GenerationStateActive)
	_ = gen.SetState(GenerationStateRetiring)
	_ = gen.SetState(GenerationStateClosed)

	if gen.State() != GenerationStateClosed {
		t.Errorf("Expected final state to be Closed, got %s", gen.State())
	}

	// Verify timestamps
	if gen.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
	if gen.ActivatedAt.IsZero() {
		t.Error("ActivatedAt should not be zero after activation")
	}
	if gen.RetiredAt.IsZero() {
		t.Error("RetiredAt should not be zero after retirement")
	}
}

// TestGenerationManager tests the generation manager.
func TestGenerationManager(t *testing.T) {
	workspace := "test-workspace"
	manager := NewGenerationManager(workspace)

	// Create a generation
	snapshot := &config.DocumentSnapshot{
		Document: &config.Document{
			APIVersion: "v1",
			Kind:       "Workspace",
			Metadata:   config.DocumentMetadata{Name: workspace},
		},
		SourcePath: workspace + ".yaml",
		LoadedAt:   time.Now().UTC(),
	}
	gen1 := manager.CreateGeneration(snapshot)

	// Check it was created
	if retrieved := manager.GetGeneration(gen1.ID); retrieved == nil {
		t.Error("Failed to retrieve created generation")
	}

	// Create another generation (used for testing)
	_ = manager.CreateGeneration(snapshot)

	// Test setting active
	if err := manager.SetActive(gen1); err != nil {
		t.Errorf("Failed to set active generation: %v", err)
	}

	// Check active generation
	active := manager.ActiveGeneration()
	if active == nil {
		t.Error("Active generation should not be nil")
	}
	if active.ID != gen1.ID {
		t.Errorf("Expected active generation %s, got %s", gen1.ID, active.ID)
	}

	// Test that we can't set active a generation from different workspace
	otherSnapshot := &config.DocumentSnapshot{
		Document: &config.Document{
			APIVersion: "v1",
			Kind:       "Workspace",
			Metadata:   config.DocumentMetadata{Name: "other-workspace"},
		},
		SourcePath: "other-workspace.yaml",
		LoadedAt:   time.Now().UTC(),
	}
	otherManager := NewGenerationManager("other-workspace")
	otherGen := otherManager.CreateGeneration(otherSnapshot)
	if err := manager.SetActive(otherGen); err == nil {
		t.Error("Expected error setting active generation from different workspace")
	}

	// Test reference counting
	gen1.IncrementReference()
	gen1.IncrementReference()
	if gen1.DecrementReference() != false {
		t.Error("Expected false after first decrement (count should be 1)")
	}
	if gen1.DecrementReference() != true {
		t.Error("Expected true after second decrement (count should be 0)")
	}

	// Test run counting
	gen1.IncrementActiveRun()
	gen1.IncrementActiveRun()
	if gen1.ActiveRunCount() != 2 {
		t.Errorf("Expected active run count 2, got %d", gen1.ActiveRunCount())
	}
	gen1.DecrementActiveRun()
	if gen1.ActiveRunCount() != 1 {
		t.Errorf("Expected active run count 1, got %d", gen1.ActiveRunCount())
	}
}

// TestIsValidTransition tests the state transition validation.
func TestIsValidTransition(t *testing.T) {
	testCases := []struct {
		from, to   GenerationState
		valid     bool
	}{
		{GenerationStateBuilding, GenerationStateValidated, true},
		{GenerationStateBuilding, GenerationStateFailed, true},
		{GenerationStateBuilding, GenerationStateActive, false},
		{GenerationStateValidated, GenerationStateQuiescing, true},
		{GenerationStateValidated, GenerationStateFailed, true},
		{GenerationStateQuiescing, GenerationStateActive, true},
		{GenerationStateQuiescing, GenerationStateFailed, true},
		{GenerationStateActive, GenerationStateRetiring, true},
		{GenerationStateRetiring, GenerationStateClosed, true},
		{GenerationStateClosed, GenerationStateActive, false},
		{GenerationStateFailed, GenerationStateBuilding, false},
	}

	for _, tc := range testCases {
		result := isValidTransition(tc.from, tc.to)
		if result != tc.valid {
			t.Errorf("Transition from %s to %s: expected %v, got %v",
				tc.from, tc.to, tc.valid, result)
		}
	}
}

// TestGenerationManagerQuiescence tests generation quiescence.
func TestGenerationManagerQuiescence(t *testing.T) {
	workspace := "test-workspace"
	manager := NewGenerationManager(workspace)

	// Create a generation with proper snapshot
	snapshot := &config.DocumentSnapshot{
		Document: &config.Document{
			APIVersion: "v1",
			Kind:       "Workspace",
			Metadata:   config.DocumentMetadata{Name: workspace},
		},
		SourcePath: workspace + ".yaml",
		LoadedAt:   time.Now().UTC(),
	}
	gen := manager.CreateGeneration(snapshot)

	// Set it to validated state so it can be activated
	_ = gen.SetState(GenerationStateValidated)

	// Create an old active generation with active runs
	oldSnapshot := &config.DocumentSnapshot{
		Document: &config.Document{
			APIVersion: "v1",
			Kind:       "Workspace",
			Metadata:   config.DocumentMetadata{Name: workspace + "-old"},
		},
		SourcePath: workspace + "-old.yaml",
		LoadedAt:   time.Now().UTC(),
	}
	oldGen := manager.CreateGeneration(oldSnapshot)
	_ = oldGen.SetState(GenerationStateValidated)
	_ = oldGen.SetState(GenerationStateQuiescing)
	_ = oldGen.SetState(GenerationStateActive)
	oldGen.IncrementActiveRun()
	oldGen.IncrementActiveRun()
	_ = manager.SetActive(oldGen)

	// Try to set new generation active - should fail because old has active runs
	if err := manager.SetActive(gen); err == nil {
		t.Error("Expected error setting active with active runs in old generation")
	}

	// Test quiescence waiting on old generation
	ch := manager.QuiesceGeneration(oldGen.ID)
	select {
	case <-ch:
		t.Error("Quiescence channel should not be closed yet")
	case <-time.After(50 * time.Millisecond):
		// Expected - still has active runs
	}

	// Decrement runs on old generation
	oldGen.DecrementActiveRun()
	oldGen.DecrementActiveRun()

	// Now quiescence should be immediate
	ch = manager.QuiesceGeneration(oldGen.ID)
	select {
	case <-ch:
		// Expected
	case <-time.After(50 * time.Millisecond):
		t.Error("Quiescence channel should be closed when no active runs")
	}

	// Now should be able to set new generation active
	if err := manager.SetActive(gen); err != nil {
		t.Errorf("Failed to set active after quiescence: %v", err)
	}

	// Transition to active state
	_ = gen.SetState(GenerationStateQuiescing)
	_ = gen.SetState(GenerationStateActive)

	// Test retire
	if err := manager.RetireActive(); err != nil {
		t.Errorf("Failed to retire active generation: %v", err)
	}

	// Check that active is now nil
	if manager.ActiveGeneration() != nil {
		t.Error("Active generation should be nil after retirement")
	}
}

// TestSettingsReview tests the SettingsReview structure.
func TestSettingsReview(t *testing.T) {
	review := SettingsReview{
		ReviewID:          "review_1",
		Valid:             true,
		ValidationErrors:  make([]string, 0),
		Warnings:          make([]string, 0),
		FilesChanged:      []string{"file1.yaml", "file2.yaml"},
		OldHashes:         map[string]string{"file1.yaml": "hash1", "file2.yaml": "hash2"},
		NewHashes:         map[string]string{"file1.yaml": "hash3", "file2.yaml": "hash4"},
		ReadyForActivation: true,
		RequiresRestart:   false,
		ActivationPlan:    "Test activation plan",
		CreatedAt:         time.Now().UTC(),
	}

	// Verify all fields are set correctly
	if review.ReviewID != "review_1" {
		t.Errorf("Expected ReviewID 'review_1', got '%s'", review.ReviewID)
	}
	if !review.Valid {
		t.Error("Expected Valid to be true")
	}
	if len(review.FilesChanged) != 2 {
		t.Errorf("Expected 2 files changed, got %d", len(review.FilesChanged))
	}
	if !review.ReadyForActivation {
		t.Error("Expected ReadyForActivation to be true")
	}
}

// TestActivationCommand tests the ActivationCommand structure.
func TestActivationCommand(t *testing.T) {
	cmd := ActivationCommand{
		StagedGeneration: "gen_1",
		Force:             true,
		Deadline:         30 * time.Second,
	}

	if cmd.StagedGeneration != "gen_1" {
		t.Errorf("Expected StagedGeneration 'gen_1', got '%s'", cmd.StagedGeneration)
	}
	if !cmd.Force {
		t.Error("Expected Force to be true")
	}
	if cmd.Deadline != 30*time.Second {
		t.Errorf("Expected Deadline 30s, got %v", cmd.Deadline)
	}
}

// TestWorkspaceReadiness tests the WorkspaceReadiness structure.
func TestWorkspaceReadiness(t *testing.T) {
	readiness := WorkspaceReadiness{
		Ready:        true,
		Degraded:     false,
		BlockingIssues: []BlockingIssue{
			{
				ID:       "test_issue",
				Category: "config",
				Message:  "Test issue",
				CheckID:  "check_1",
				Severity: SeverityError,
				Action:   "Fix the issue",
			},
		},
		Warnings: []string{"Test warning"},
		CheckedAt: time.Now().UTC(),
		SandboxReady: true,
		ModelReady: true,
		ConfigValid: true,
	}

	if readiness.IsReady() {
		t.Error("Expected IsReady to return false with blocking issues")
	}

	if !readiness.HasBlockingIssues() {
		t.Error("Expected HasBlockingIssues to return true")
	}

	if readiness.NeedsInitialization() {
		t.Error("Expected NeedsInitialization to return false")
	}

	// Test with no blocking issues
	readiness2 := WorkspaceReadiness{
		Ready:           true,
		Degraded:        true,
		BlockingIssues:  []BlockingIssue{},
		Warnings:        []string{"Test warning"},
		CheckedAt:       time.Now().UTC(),
		SandboxReady:    true,
		ModelReady:      true,
		ConfigValid:     true,
	}

	if !readiness2.IsReady() {
		t.Error("Expected IsReady to return true with no blocking issues")
	}

	if readiness2.HasBlockingIssues() {
		t.Error("Expected HasBlockingIssues to return false")
	}

	if !readiness2.CanActivateGeneration() {
		t.Error("Expected CanActivateGeneration to return true")
	}
}
