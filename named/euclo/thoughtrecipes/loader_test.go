package thoughtrecipes

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFileValid(t *testing.T) {
	// Create a minimal valid recipe YAML
	content := `
apiVersion: euclo/v1alpha1
kind: ThoughtRecipe
metadata:
  name: test-recipe
  description: A test recipe
sequence:
  - id: step1
    parent:
      paradigm: react
      prompt: Analyze the code
`
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "valid.yaml")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	plan, warnings, err := LoadFile(tmpFile)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if plan == nil {
		t.Error("expected non-nil plan")
	}
	if len(warnings) != 0 {
		t.Errorf("expected no warnings, got %v", warnings)
	}
	if plan.FilePath != tmpFile {
		t.Errorf("expected FilePath %q, got %q", tmpFile, plan.FilePath)
	}
	if desc := plan.ToDescriptor(); desc.RecipePath != tmpFile {
		t.Errorf("expected descriptor RecipePath %q, got %q", tmpFile, desc.RecipePath)
	}

	// Verify plan fields
	if plan.Name != "test-recipe" {
		t.Errorf("expected name 'test-recipe', got %q", plan.Name)
	}
	if len(plan.Steps) != 1 {
		t.Errorf("expected 1 step, got %d", len(plan.Steps))
	}
}

func TestLoadFileInvalidAPIVersion(t *testing.T) {
	content := `
apiVersion: wrong/v1
kind: ThoughtRecipe
metadata:
  name: test-recipe
  description: A test recipe
sequence:
  - id: step1
    parent:
      paradigm: react
`

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "invalid.yaml")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	_, _, err := LoadFile(tmpFile)
	if err == nil {
		t.Error("expected error for invalid apiVersion")
	}
}

func TestLoadFileInvalidKind(t *testing.T) {
	content := `
apiVersion: euclo/v1alpha1
kind: WrongKind
metadata:
  name: test-recipe
  description: A test recipe
sequence:
  - id: step1
    parent:
      paradigm: react
`

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "invalid.yaml")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	_, _, err := LoadFile(tmpFile)
	if err == nil {
		t.Error("expected error for invalid kind")
	}
}

func TestLoadFileMissingName(t *testing.T) {
	content := `
apiVersion: euclo/v1alpha1
kind: ThoughtRecipe
metadata:
  description: A test recipe
sequence:
  - id: step1
    parent:
      paradigm: react
`

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "invalid.yaml")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	_, _, err := LoadFile(tmpFile)
	if err == nil {
		t.Error("expected error for missing name")
	}
}

func TestLoadFileMissingDescription(t *testing.T) {
	content := `
apiVersion: euclo/v1alpha1
kind: ThoughtRecipe
metadata:
  name: test-recipe
sequence:
  - id: step1
    parent:
      paradigm: react
`

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "invalid.yaml")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	_, _, err := LoadFile(tmpFile)
	if err == nil {
		t.Error("expected error for missing description")
	}
}

func TestLoadFileEmptySequence(t *testing.T) {
	content := `
apiVersion: euclo/v1alpha1
kind: ThoughtRecipe
metadata:
  name: test-recipe
  description: A test recipe
sequence: []
`

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "invalid.yaml")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	_, _, err := LoadFile(tmpFile)
	if err == nil {
		t.Error("expected error for empty sequence")
	}
}

func TestLoadFileDuplicateStepID(t *testing.T) {
	content := `
apiVersion: euclo/v1alpha1
kind: ThoughtRecipe
metadata:
  name: test-recipe
  description: A test recipe
sequence:
  - id: step1
    parent:
      paradigm: react
  - id: step1
    parent:
      paradigm: planner
`

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "invalid.yaml")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	_, _, err := LoadFile(tmpFile)
	if err == nil {
		t.Error("expected error for duplicate step ID")
	}
}

func TestLoadFileInvalidParadigm(t *testing.T) {
	content := `
apiVersion: euclo/v1alpha1
kind: ThoughtRecipe
metadata:
  name: test-recipe
  description: A test recipe
sequence:
  - id: step1
    parent:
      paradigm: unknown-paradigm
`

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "invalid.yaml")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	_, _, err := LoadFile(tmpFile)
	if err == nil {
		t.Error("expected error for invalid paradigm")
	}
}

func TestLoadFileChildNoDelegation(t *testing.T) {
	content := `
apiVersion: euclo/v1alpha1
kind: ThoughtRecipe
metadata:
  name: test-recipe
  description: A test recipe
sequence:
  - id: step1
    parent:
      paradigm: react
      prompt: Analyze
    child:
      paradigm: planner
      prompt: Plan it
`

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "warning.yaml")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	plan, warnings, err := LoadFile(tmpFile)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if plan == nil {
		t.Error("expected non-nil plan")
	}
	if len(warnings) == 0 {
		t.Error("expected warning for child on non-delegating paradigm")
	}
}

func TestLoadFileAliasShadowWarning(t *testing.T) {
	content := `
apiVersion: euclo/v1alpha1
kind: ThoughtRecipe
metadata:
  name: test-recipe
  description: A test recipe
global:
  context:
    aliases:
      explore_findings: "custom.key"
sequence:
  - id: step1
    parent:
      paradigm: react
`

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "warning.yaml")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	_, warnings, err := LoadFile(tmpFile)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if len(warnings) == 0 {
		t.Error("expected warning for shadowed alias")
	}
}

func TestLoadFilePriorityClamp(t *testing.T) {
	content := `
apiVersion: euclo/v1alpha1
kind: ThoughtRecipe
metadata:
  name: test-recipe
  description: A test recipe
global:
  configuration:
    trigger_priority: 200
sequence:
  - id: step1
    parent:
      paradigm: react
`

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "clamp.yaml")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	plan, warnings, err := LoadFile(tmpFile)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if plan.TriggerPriority != MaxTriggerPriority {
		t.Errorf("expected priority clamped to %d, got %d", MaxTriggerPriority, plan.TriggerPriority)
	}
	if len(warnings) == 0 {
		t.Error("expected warning for priority clamp")
	}
}

func TestLoadFilePriorityDefault(t *testing.T) {
	content := `
apiVersion: euclo/v1alpha1
kind: ThoughtRecipe
metadata:
  name: test-recipe
  description: A test recipe
sequence:
  - id: step1
    parent:
      paradigm: react
`

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "default.yaml")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	plan, _, err := LoadFile(tmpFile)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if plan.TriggerPriority != DefaultTriggerPriority {
		t.Errorf("expected default priority %d, got %d", DefaultTriggerPriority, plan.TriggerPriority)
	}
}

func TestCompileExecutionPlanAliasResolution(t *testing.T) {
	recipe := &ThoughtRecipe{
		APIVersion: ExpectedAPIVersion,
		Kind:       ExpectedKind,
		Metadata: RecipeMetadata{
			Name:        "test",
			Description: "test recipe",
		},
		Global: RecipeGlobal{
			Context: RecipeContextSpec{
				Aliases: map[string]string{
					"my_alias": "custom.state.key",
				},
			},
		},
		Sequence: []RecipeStep{
			{
				ID: "step1",
				Parent: RecipeStepAgent{
					Paradigm: "react",
					Context: RecipeStepContext{
						Inherit: []string{"explore_findings"},
						Capture: []string{"analysis_result"},
					},
				},
			},
		},
	}

	plan, _, err := CompileExecutionPlan(recipe, "")
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if len(plan.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(plan.Steps))
	}

	step := plan.Steps[0]
	if len(step.Inherit) != 1 || step.Inherit[0] != "explore_findings" {
		t.Errorf("expected inherit ['explore_findings'], got %v", step.Inherit)
	}
	if len(step.Capture) != 1 || step.Capture[0] != "analysis_result" {
		t.Errorf("expected capture ['analysis_result'], got %v", step.Capture)
	}
}

func TestCompileExecutionPlanGlobalCapabilities(t *testing.T) {
	recipe := &ThoughtRecipe{
		APIVersion: ExpectedAPIVersion,
		Kind:       ExpectedKind,
		Metadata: RecipeMetadata{
			Name:        "test",
			Description: "test recipe",
		},
		Global: RecipeGlobal{
			Capabilities: RecipeCapabilitySpec{
				Allowed: []string{"cap:file_read", "cap:file_write"},
			},
		},
		Sequence: []RecipeStep{
			{
				ID: "step1",
				Parent: RecipeStepAgent{
					Paradigm: "react",
				},
			},
		},
	}

	plan, _, err := CompileExecutionPlan(recipe, "")
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if len(plan.GlobalCapabilities) != 2 {
		t.Errorf("expected 2 global capabilities, got %d", len(plan.GlobalCapabilities))
	}
}

func TestCompileExecutionPlanStepIntersect(t *testing.T) {
	recipe := &ThoughtRecipe{
		APIVersion: ExpectedAPIVersion,
		Kind:       ExpectedKind,
		Metadata: RecipeMetadata{
			Name:        "test",
			Description: "test recipe",
		},
		Global: RecipeGlobal{
			Capabilities: RecipeCapabilitySpec{
				Allowed: []string{"cap:file_read", "cap:file_write"},
			},
		},
		Sequence: []RecipeStep{
			{
				ID: "step1",
				Parent: RecipeStepAgent{
					Paradigm: "react",
					Capabilities: RecipeCapabilitySpec{
						Allowed: []string{"cap:file_read", "cap:go_test"},
					},
				},
			},
		},
	}

	plan, _, err := CompileExecutionPlan(recipe, "")
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	step := plan.Steps[0]
	// Should be intersection: only cap:file_read is in both
	if len(step.AllowedCapabilities) != 1 || step.AllowedCapabilities[0] != "cap:file_read" {
		t.Errorf("expected intersection [cap:file_read], got %v", step.AllowedCapabilities)
	}
}

func TestCompileExecutionPlanSharingDefault(t *testing.T) {
	recipe := &ThoughtRecipe{
		APIVersion: ExpectedAPIVersion,
		Kind:       ExpectedKind,
		Metadata: RecipeMetadata{
			Name:        "test",
			Description: "test recipe",
		},
		Global: RecipeGlobal{
			Context: RecipeContextSpec{}, // No sharing.default specified
		},
		Sequence: []RecipeStep{
			{
				ID: "step1",
				Parent: RecipeStepAgent{
					Paradigm: "react",
				},
			},
		},
	}

	plan, _, err := CompileExecutionPlan(recipe, "")
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if plan.GlobalSharing != SharingModeExplicit {
		t.Errorf("expected default sharing mode %q, got %q", SharingModeExplicit, plan.GlobalSharing)
	}
}

func TestLoadAllEmpty(t *testing.T) {
	tmpDir := t.TempDir()

	result, err := LoadAll(tmpDir)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Plans) != 0 {
		t.Errorf("expected 0 plans, got %d", len(result.Plans))
	}
	if len(result.Errors) != 0 {
		t.Errorf("expected 0 errors, got %d", len(result.Errors))
	}
}

func TestLoadAllMixed(t *testing.T) {
	tmpDir := t.TempDir()

	// Write a valid recipe
	valid := `
apiVersion: euclo/v1alpha1
kind: ThoughtRecipe
metadata:
  name: valid-recipe
  description: A valid recipe
sequence:
  - id: step1
    parent:
      paradigm: react
`
	if err := os.WriteFile(filepath.Join(tmpDir, "valid.yaml"), []byte(valid), 0644); err != nil {
		t.Fatalf("failed to write valid file: %v", err)
	}

	// Write an invalid recipe
	invalid := `
apiVersion: wrong/v1
kind: ThoughtRecipe
metadata:
  name: invalid-recipe
  description: An invalid recipe
sequence:
  - id: step1
    parent:
      paradigm: react
`
	if err := os.WriteFile(filepath.Join(tmpDir, "invalid.yaml"), []byte(invalid), 0644); err != nil {
		t.Fatalf("failed to write invalid file: %v", err)
	}

	result, err := LoadAll(tmpDir)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if len(result.Plans) != 1 {
		t.Errorf("expected 1 plan, got %d", len(result.Plans))
	}
	if len(result.Errors) != 1 {
		t.Errorf("expected 1 error, got %d", len(result.Errors))
	}
	if result.Plans[0].Name != "valid-recipe" {
		t.Errorf("expected plan name 'valid-recipe', got %q", result.Plans[0].Name)
	}
}

func TestLoadAllSkipsNonYAML(t *testing.T) {
	tmpDir := t.TempDir()

	// Write a valid recipe
	valid := `
apiVersion: euclo/v1alpha1
kind: ThoughtRecipe
metadata:
  name: valid-recipe
  description: A valid recipe
sequence:
  - id: step1
    parent:
      paradigm: react
`
	if err := os.WriteFile(filepath.Join(tmpDir, "valid.yaml"), []byte(valid), 0644); err != nil {
		t.Fatalf("failed to write valid file: %v", err)
	}

	// Write a non-YAML file
	if err := os.WriteFile(filepath.Join(tmpDir, "readme.txt"), []byte("not yaml"), 0644); err != nil {
		t.Fatalf("failed to write txt file: %v", err)
	}

	// Write a .yml file (should also be processed)
	ymlContent := `
apiVersion: euclo/v1alpha1
kind: ThoughtRecipe
metadata:
  name: yml-recipe
  description: A yml recipe
sequence:
  - id: step1
    parent:
      paradigm: react
`
	if err := os.WriteFile(filepath.Join(tmpDir, "also-valid.yml"), []byte(ymlContent), 0644); err != nil {
		t.Fatalf("failed to write yml file: %v", err)
	}

	result, err := LoadAll(tmpDir)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if len(result.Plans) != 2 {
		t.Errorf("expected 2 plans, got %d", len(result.Plans))
	}
	if len(result.Errors) != 0 {
		t.Errorf("expected 0 errors, got %d", len(result.Errors))
	}
}

func TestValidateEmpty(t *testing.T) {
	errs := Validate(&ThoughtRecipe{})
	if len(errs) == 0 {
		t.Error("expected validation errors for empty recipe")
	}
}

func TestValidateNil(t *testing.T) {
	errs := Validate(nil)
	if len(errs) == 0 {
		t.Error("expected validation error for nil recipe")
	}
}

func TestValidateInvalidNameFormat(t *testing.T) {
	recipe := &ThoughtRecipe{
		APIVersion: ExpectedAPIVersion,
		Kind:       ExpectedKind,
		Metadata: RecipeMetadata{
			Name:        "InvalidName", // uppercase not allowed
			Description: "test",
		},
		Sequence: []RecipeStep{
			{ID: "step1", Parent: RecipeStepAgent{Paradigm: "react"}},
		},
	}

	errs := Validate(recipe)
	found := false
	for _, err := range errs {
		if err != nil && err.Error() != "" && contains(err.Error(), "metadata.name") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected error about invalid name format, got %v", errs)
	}
}

func TestPlanRegistry(t *testing.T) {
	reg := NewPlanRegistry()

	// Register a plan
	plan := &ExecutionPlan{
		Name:        "test-plan",
		Description: "A test plan",
		Steps:       []ExecutionStep{},
	}

	if err := reg.Register(plan); err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	// Retrieve the plan
	retrieved, ok := reg.Get("test-plan")
	if !ok {
		t.Error("expected to find registered plan")
	}
	if retrieved.Name != "test-plan" {
		t.Errorf("expected name 'test-plan', got %q", retrieved.Name)
	}

	// List should include the plan
	list := reg.List()
	if len(list) != 1 || list[0] != "test-plan" {
		t.Errorf("expected list ['test-plan'], got %v", list)
	}

	// Count should be 1
	if reg.Count() != 1 {
		t.Errorf("expected count 1, got %d", reg.Count())
	}
}

func TestPlanRegistryNil(t *testing.T) {
	var reg *PlanRegistry

	// Get on nil registry should return false
	_, ok := reg.Get("anything")
	if ok {
		t.Error("expected Get on nil registry to return false")
	}

	// Count on nil registry should return 0
	if reg.Count() != 0 {
		t.Errorf("expected Count on nil registry to return 0, got %d", reg.Count())
	}

	// List on nil registry should return nil
	if reg.List() != nil {
		t.Errorf("expected List on nil registry to return nil, got %v", reg.List())
	}
}

func TestPlanRegisterNilPlan(t *testing.T) {
	reg := NewPlanRegistry()
	err := reg.Register(nil)
	if err == nil {
		t.Error("expected error for nil plan")
	}
}

func TestResolveInherit(t *testing.T) {
	resolver := NewAliasResolver(nil)

	// Test resolving standard aliases
	aliases := []string{"explore_findings", "analysis_result", "unknown_alias"}
	stateKeys := ResolveInherit(aliases, resolver)

	if len(stateKeys) != 3 {
		t.Errorf("expected 3 state keys, got %d", len(stateKeys))
	}

	// explore_findings should resolve to pipeline.explore
	if stateKeys[0] != "pipeline.explore" {
		t.Errorf("expected 'pipeline.explore', got %q", stateKeys[0])
	}

	// unknown_alias should be passed through as-is
	if stateKeys[2] != "unknown_alias" {
		t.Errorf("expected 'unknown_alias', got %q", stateKeys[2])
	}
}

func TestResolveCapture(t *testing.T) {
	// Capture uses same logic as inherit
	resolver := NewAliasResolver(nil)
	aliases := []string{"plan_output"}

	stateKeys := ResolveCapture(aliases, resolver)
	if len(stateKeys) != 1 || stateKeys[0] != "pipeline.plan" {
		t.Errorf("expected ['pipeline.plan'], got %v", stateKeys)
	}
}

func TestValidateEnrichmentSource(t *testing.T) {
	tests := []struct {
		input    string
		expected EnrichmentSource
		valid    bool
	}{
		{"ast", EnrichmentAST, true},
		{"archaeology", EnrichmentArchaeology, true},
		{"bkc", EnrichmentBKC, true},
		{"unknown", "", false},
		{"", "", false},
	}

	for _, tt := range tests {
		result, valid := ValidateEnrichmentSource(tt.input)
		if valid != tt.valid {
			t.Errorf("ValidateEnrichmentSource(%q): expected valid=%v, got valid=%v", tt.input, tt.valid, valid)
		}
		if valid && result != tt.expected {
			t.Errorf("ValidateEnrichmentSource(%q): expected %q, got %q", tt.input, tt.expected, result)
		}
	}
}

func TestValidateSharingMode(t *testing.T) {
	tests := []struct {
		input    string
		expected SharingMode
		valid    bool
	}{
		{"carry_forward", SharingModeCarryForward, true},
		{"isolated", SharingModeIsolated, true},
		{"explicit", SharingModeExplicit, true},
		{"unknown", "", false},
		{"", "", false},
	}

	for _, tt := range tests {
		result, valid := ValidateSharingMode(tt.input)
		if valid != tt.valid {
			t.Errorf("ValidateSharingMode(%q): expected valid=%v, got valid=%v", tt.input, tt.valid, valid)
		}
		if valid && result != tt.expected {
			t.Errorf("ValidateSharingMode(%q): expected %q, got %q", tt.input, tt.expected, result)
		}
	}
}

func TestIsParadigmWithDelegation(t *testing.T) {
	tests := []struct {
		paradigm string
		expected bool
	}{
		{"planner", false}, // planner has no delegation slot
		{"htn", true},
		{"reflection", true},
		{"goalcon", true},
		{"react", false},
		{"blackboard", false},
		{"architect", false},
		{"chainer", false},
	}

	for _, tt := range tests {
		result := IsParadigmWithDelegation(tt.paradigm)
		if result != tt.expected {
			t.Errorf("IsParadigmWithDelegation(%q): expected %v, got %v", tt.paradigm, tt.expected, result)
		}
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || containsAt(s, substr))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
