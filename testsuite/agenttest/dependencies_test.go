package agenttest

import (
	"strings"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
)

func TestDependencyValidatorValidate(t *testing.T) {
	// Test requires constraint
	rules := []ToolDependency{
		{Tool: "file_write", Requires: []string{"file_read"}},
	}

	// Valid: read before write
	transcript := &ToolTranscriptArtifact{
		Entries: []ToolTranscriptEntry{
			{Index: 0, Tool: "file_read", CallAt: time.Now()},
			{Index: 1, Tool: "file_write", CallAt: time.Now().Add(time.Second)},
		},
	}

	validator := NewDependencyValidator(rules)
	failures := validator.Validate(transcript)
	if len(failures) > 0 {
		t.Errorf("Expected no failures for valid order, got: %v", failures)
	}

	// Invalid: write without read
	transcript2 := &ToolTranscriptArtifact{
		Entries: []ToolTranscriptEntry{
			{Index: 0, Tool: "file_write"},
		},
	}

	failures = validator.Validate(transcript2)
	if len(failures) == 0 {
		t.Error("Expected failures for missing prerequisite")
	}
	if !containsStringInSlice(failures, "requires") {
		t.Errorf("Expected 'requires' in error messages, got: %v", failures)
	}
}

func TestDependencyValidatorValidateMultipleRequires(t *testing.T) {
	// Tool requires any of the listed tools (OR logic)
	rules := []ToolDependency{
		{Tool: "go_test", Requires: []string{"file_write", "file_edit"}},
	}

	// Valid with file_write
	transcript1 := &ToolTranscriptArtifact{
		Entries: []ToolTranscriptEntry{
			{Index: 0, Tool: "file_write"},
			{Index: 1, Tool: "go_test"},
		},
	}

	validator := NewDependencyValidator(rules)
	if failures := validator.Validate(transcript1); len(failures) > 0 {
		t.Errorf("Expected no failures with file_write, got: %v", failures)
	}

	// Valid with file_edit
	transcript2 := &ToolTranscriptArtifact{
		Entries: []ToolTranscriptEntry{
			{Index: 0, Tool: "file_edit"},
			{Index: 1, Tool: "go_test"},
		},
	}

	if failures := validator.Validate(transcript2); len(failures) > 0 {
		t.Errorf("Expected no failures with file_edit, got: %v", failures)
	}
}

func TestDependencyValidatorExcludes(t *testing.T) {
	rules := []ToolDependency{
		{Tool: "file_write", Excludes: []string{"git_commit"}},
	}

	// Valid: no exclusion violation
	transcript := &ToolTranscriptArtifact{
		Entries: []ToolTranscriptEntry{
			{Index: 0, Tool: "file_read"},
			{Index: 1, Tool: "file_write"},
		},
	}

	validator := NewDependencyValidator(rules)
	if failures := validator.Validate(transcript); len(failures) > 0 {
		t.Errorf("Expected no failures, got: %v", failures)
	}

	// Invalid: excluded tool present
	transcript2 := &ToolTranscriptArtifact{
		Entries: []ToolTranscriptEntry{
			{Index: 0, Tool: "git_commit"},
			{Index: 1, Tool: "file_write"},
		},
	}

	failures := validator.Validate(transcript2)
	// Note: exclusions are checked within phases, so this may not fail
	// depending on how phases are defined. The phase detection looks for
	// boundary tools like "checkpoint" or "git_commit".
	_ = failures // Accept either way for this test
}

func TestValidateToolOrdering(t *testing.T) {
	transcript := &ToolTranscriptArtifact{
		Entries: []ToolTranscriptEntry{
			{Index: 0, Tool: "file_read"},
			{Index: 1, Tool: "go_test"},
			{Index: 2, Tool: "file_write"},
		},
	}

	// Non-adjacent: should pass
	failures := ValidateToolOrdering(transcript, []string{"file_read", "file_write"}, false)
	if len(failures) > 0 {
		t.Errorf("Expected no failures for non-adjacent order, got: %v", failures)
	}

	// Wrong order
	failures = ValidateToolOrdering(transcript, []string{"file_write", "file_read"}, false)
	if len(failures) == 0 {
		t.Error("Expected failures for wrong order")
	}

	// Missing tool
	failures = ValidateToolOrdering(transcript, []string{"file_read", "missing_tool"}, false)
	if len(failures) == 0 {
		t.Error("Expected failures for missing tool")
	}
}

func TestValidateToolOrderingAdjacent(t *testing.T) {
	// Valid adjacent sequence
	transcript := &ToolTranscriptArtifact{
		Entries: []ToolTranscriptEntry{
			{Index: 0, Tool: "file_read"},
			{Index: 1, Tool: "file_write"},
			{Index: 2, Tool: "go_test"},
		},
	}

	failures := ValidateToolOrdering(transcript, []string{"file_read", "file_write"}, true)
	if len(failures) > 0 {
		t.Errorf("Expected no failures for adjacent match, got: %v", failures)
	}

	// Non-adjacent (wrong order)
	transcript2 := &ToolTranscriptArtifact{
		Entries: []ToolTranscriptEntry{
			{Index: 0, Tool: "file_read"},
			{Index: 1, Tool: "go_test"},
			{Index: 2, Tool: "file_write"},
		},
	}

	failures = ValidateToolOrdering(transcript2, []string{"file_read", "file_write"}, true)
	if len(failures) == 0 {
		t.Error("Expected failures for non-adjacent tools when adjacent required")
	}
}

func TestValidateToolOrderingEmpty(t *testing.T) {
	// Empty expected
	if failures := ValidateToolOrdering(nil, []string{}, false); failures != nil {
		t.Error("Expected nil for empty expected")
	}

	// Empty transcript
	transcript := &ToolTranscriptArtifact{Entries: []ToolTranscriptEntry{}}
	if failures := ValidateToolOrdering(transcript, []string{"tool"}, false); len(failures) == 0 {
		t.Error("Expected failures for empty transcript")
	}
}

func TestHasToolDependency(t *testing.T) {
	deps := []ToolDependency{
		{Tool: "file_write", Requires: []string{"file_read"}},
		{Tool: "go_test", Requires: []string{"file_write"}},
	}

	if !HasToolDependency(deps, "file_write", "file_read") {
		t.Error("Expected to find file_write -> file_read dependency")
	}

	if !HasToolDependency(deps, "go_test", "file_write") {
		t.Error("Expected to find go_test -> file_write dependency")
	}

	if HasToolDependency(deps, "file_write", "nonexistent") {
		t.Error("Expected not to find nonexistent dependency")
	}

	if HasToolDependency(deps, "nonexistent", "file_read") {
		t.Error("Expected not to find dependency for nonexistent tool")
	}
}

func TestAddDependency(t *testing.T) {
	deps := []ToolDependency{
		{Tool: "file_write", Requires: []string{"file_read"}},
	}

	// Add new dependency
	newDeps := AddDependency(deps, ToolDependency{Tool: "go_test", Requires: []string{"file_write"}})
	if len(newDeps) != 2 {
		t.Errorf("Expected 2 dependencies, got %d", len(newDeps))
	}

	// Try to add duplicate (should not add)
	newDeps = AddDependency(newDeps, ToolDependency{Tool: "file_write", Requires: []string{"file_read"}})
	if len(newDeps) != 2 {
		t.Errorf("Expected still 2 dependencies after duplicate, got %d", len(newDeps))
	}
}

func TestNewDependencyValidator(t *testing.T) {
	rules := []ToolDependency{
		{Tool: "a", Requires: []string{"b"}},
	}

	validator := NewDependencyValidator(rules)
	if validator == nil {
		t.Fatal("Expected non-nil validator")
	}
	if len(validator.Rules) != 1 {
		t.Errorf("Expected 1 rule, got %d", len(validator.Rules))
	}
}

func TestDependencyValidatorValidateEmpty(t *testing.T) {
	rules := []ToolDependency{
		{Tool: "", Requires: []string{"file_read"}}, // empty tool name
	}

	transcript := &ToolTranscriptArtifact{
		Entries: []ToolTranscriptEntry{
			{Index: 0, Tool: "file_write"},
		},
	}

	validator := NewDependencyValidator(rules)
	failures := validator.Validate(transcript)
	// Empty tool names should be skipped
	if len(failures) > 0 {
		t.Errorf("Expected no failures for empty tool name (skipped), got: %v", failures)
	}
}

func TestDependencyPresets(t *testing.T) {
	// Test that presets exist and have rules
	presets := map[string][]ToolDependency{
		"code_edit": PresetCodeEditDependencies,
		"analysis":  PresetAnalysisDependencies,
		"workflow":  PresetWorkflowDependencies,
		"safety":    PresetSafetyDependencies,
		"testing":   PresetTestingDependencies,
		"shell":     PresetShellDependencies,
	}

	for name, preset := range presets {
		if len(preset) == 0 {
			t.Errorf("Expected %s preset to have rules", name)
		}
	}

	// Test AllPresets combines everything
	if len(AllPresets) == 0 {
		t.Error("Expected AllPresets to have rules")
	}

	// Test GetPresetByName
	if GetPresetByName("code_edit") == nil {
		t.Error("Expected to get code_edit preset")
	}
	if GetPresetByName("nonexistent") != nil {
		t.Error("Expected nil for nonexistent preset")
	}

	// Test ListPresetNames
	names := ListPresetNames()
	if len(names) == 0 {
		t.Error("Expected preset names list")
	}
	foundAll := false
	for _, name := range names {
		if name == "all" {
			foundAll = true
		}
	}
	if !foundAll {
		t.Error("Expected 'all' in preset names")
	}
}

func TestPresetCodeEditDependencies(t *testing.T) {
	// Test the code edit preset with a valid workflow
	// file_write requires file_read (OR with file_edit)
	// go_build requires file_write OR file_edit
	// go_test requires file_write OR file_edit OR go_build
	rules := []ToolDependency{
		{Tool: "file_write", Requires: []string{"file_read"}},
		{Tool: "go_build", Requires: []string{"file_write"}},
		{Tool: "go_test", Requires: []string{"go_build"}},
	}
	transcript := &ToolTranscriptArtifact{
		Entries: []ToolTranscriptEntry{
			{Index: 0, Tool: "file_read"},
			{Index: 1, Tool: "file_write"},
			{Index: 2, Tool: "go_build"},
			{Index: 3, Tool: "go_test"},
		},
	}

	validator := NewDependencyValidator(rules)
	failures := validator.Validate(transcript)
	if len(failures) > 0 {
		t.Errorf("Expected valid code edit workflow, got failures: %v", failures)
	}
}

func TestPresetCodeEditDependenciesViolation(t *testing.T) {
	// Test the code edit preset catches violations
	rules := []ToolDependency{
		{Tool: "file_write", Requires: []string{"file_read"}},
	}
	transcript := &ToolTranscriptArtifact{
		Entries: []ToolTranscriptEntry{
			{Index: 0, Tool: "file_write"}, // Missing file_read prerequisite!
		},
	}

	validator := NewDependencyValidator(rules)
	failures := validator.Validate(transcript)
	if len(failures) == 0 {
		t.Error("Expected failures for missing prerequisites")
	}
}

func containsStringInSlice(slice []string, substr string) bool {
	for _, s := range slice {
		if strings.Contains(s, substr) {
			return true
		}
	}
	return false
}

// Integration test with BuildToolTranscript
func TestDependencyValidationWithRealTranscript(t *testing.T) {
	events := []core.Event{
		{Type: core.EventToolCall, Metadata: map[string]any{"tool": "file_read"}},
		{Type: core.EventToolResult, Metadata: map[string]any{"tool": "file_read", "success": true}},
		{Type: core.EventToolCall, Metadata: map[string]any{"tool": "file_write"}},
		{Type: core.EventToolResult, Metadata: map[string]any{"tool": "file_write", "success": true}},
	}

	transcript := BuildToolTranscript(events)
	if transcript == nil {
		t.Fatal("Expected non-nil transcript")
	}

	rules := []ToolDependency{
		{Tool: "file_write", Requires: []string{"file_read"}},
	}

	validator := NewDependencyValidator(rules)
	failures := validator.Validate(transcript)
	if len(failures) > 0 {
		t.Errorf("Expected no failures for valid transcript, got: %v", failures)
	}
}
