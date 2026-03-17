package authoring

import (
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
)

// Phase 8: Test verification hint structure.
func TestVerificationHintStructure(t *testing.T) {
	hint := &VerificationHint{
		Description: "Check output file",
		Criteria:    []string{"exists", "readable"},
		Files:       []string{"output.txt"},
		Timeout:     5,
	}

	if hint.Description == "" {
		t.Error("Description should not be empty")
	}
	if len(hint.Criteria) != 2 {
		t.Errorf("Expected 2 criteria, got %d", len(hint.Criteria))
	}
}

// Phase 8: Test file focus structure.
func TestFileFocusStructure(t *testing.T) {
	focus := &FileFocus{
		Primary:   []string{"main.go"},
		Secondary: []string{"config.go"},
		Patterns:  []string{"*.go"},
		Exclude:   []string{"vendor/*"},
	}

	if len(focus.Primary) != 1 {
		t.Errorf("Expected 1 primary file, got %d", len(focus.Primary))
	}
	if focus.Primary[0] != "main.go" {
		t.Errorf("Expected main.go, got %s", focus.Primary[0])
	}
}

// Phase 8: Test cost class constants.
func TestCostClassConstants(t *testing.T) {
	if CostClassFast == "" {
		t.Error("CostClassFast should not be empty")
	}
	if CostClassMedium == "" {
		t.Error("CostClassMedium should not be empty")
	}
	if CostClassSlow == "" {
		t.Error("CostClassSlow should not be empty")
	}
}

// Phase 8: Test retry class constants.
func TestRetryClassConstants(t *testing.T) {
	if RetryClassNone == "" {
		t.Error("RetryClassNone should not be empty")
	}
	if RetryClassIdempotent == "" {
		t.Error("RetryClassIdempotent should not be empty")
	}
	if RetryClassStateless == "" {
		t.Error("RetryClassStateless should not be empty")
	}
	if RetryClassProbed == "" {
		t.Error("RetryClassProbed should not be empty")
	}
}

func TestCostClassFromString(t *testing.T) {
	tests := []struct {
		input    string
		expected CostClass
	}{
		{"fast", CostClassFast},
		{"medium", CostClassMedium},
		{"slow", CostClassSlow},
		{"unknown", CostClassUnknown},
		{"invalid", CostClassUnknown},
		{"", CostClassUnknown},
	}

	for _, test := range tests {
		result := CostClassFromString(test.input)
		if result != test.expected {
			t.Errorf("Input %q: expected %s, got %s", test.input, test.expected, result)
		}
	}
}

func TestRetryClassFromString(t *testing.T) {
	tests := []struct {
		input    string
		expected RetryClass
	}{
		{"none", RetryClassNone},
		{"idempotent", RetryClassIdempotent},
		{"stateless", RetryClassStateless},
		{"probed", RetryClassProbed},
		{"unknown", RetryClassUnknown},
		{"invalid", RetryClassUnknown},
		{"", RetryClassUnknown},
	}

	for _, test := range tests {
		result := RetryClassFromString(test.input)
		if result != test.expected {
			t.Errorf("Input %q: expected %s, got %s", test.input, test.expected, result)
		}
	}
}

// Phase 8: Test aggregate metadata with empty operator list.
func TestAggregateOperatorMetadataEmpty(t *testing.T) {
	aggregated := AggregateOperatorMetadata([]OperatorSpec{})

	if aggregated.CostClass != CostClassUnknown {
		t.Errorf("Empty list: expected Unknown cost class, got %s", aggregated.CostClass)
	}
	if aggregated.RetryClass != RetryClassUnknown {
		t.Errorf("Empty list: expected Unknown retry class, got %s", aggregated.RetryClass)
	}
}

// Phase 8: Test aggregate metadata with operators.
func TestAggregateOperatorMetadataWithOperators(t *testing.T) {
	operators := []OperatorSpec{
		{
			Name:     "op1",
			TaskType: core.TaskType("code"),
			Executor: "react",
		},
		{
			Name:     "op2",
			TaskType: core.TaskType("code"),
			Executor: "react",
		},
	}

	aggregated := AggregateOperatorMetadata(operators)

	// Should have valid metadata structure.
	if aggregated.CostClass == "" {
		t.Error("Cost class should not be empty")
	}
	if aggregated.RetryClass == "" {
		t.Error("Retry class should not be empty")
	}
}

func TestExtractOperatorMetadata(t *testing.T) {
	operator := OperatorSpec{
		Name:     "test_op",
		TaskType: core.TaskType("code"),
		Executor: "react",
	}

	// Default extraction.
	metadata := ExtractOperatorMetadata(operator)

	if metadata.CostClass != CostClassUnknown {
		t.Errorf("Expected Unknown cost class, got %s", metadata.CostClass)
	}
	if metadata.RetryClass != RetryClassUnknown {
		t.Errorf("Expected Unknown retry class, got %s", metadata.RetryClass)
	}
	if metadata.VerificationHint != nil {
		t.Error("Expected no verification hint, got one")
	}
}

func TestPublishAndRetrieveOperatorMetadata(t *testing.T) {
	state := core.NewContext()
	operator := "test_operator"

	metadata := OperatorMetadata{
		CostClass:  CostClassFast,
		RetryClass: RetryClassIdempotent,
		BranchSafe: true,
	}

	PublishOperatorMetadata(state, operator, metadata)

	retrieved, ok := GetPublishedOperatorMetadata(state, operator)
	if !ok {
		t.Fatal("Failed to retrieve published metadata")
	}

	if retrieved.CostClass != metadata.CostClass {
		t.Errorf("Cost class mismatch: expected %s, got %s", metadata.CostClass, retrieved.CostClass)
	}
	if retrieved.RetryClass != metadata.RetryClass {
		t.Errorf("Retry class mismatch: expected %s, got %s", metadata.RetryClass, retrieved.RetryClass)
	}
	if retrieved.BranchSafe != metadata.BranchSafe {
		t.Errorf("Branch safety mismatch: expected %v, got %v", metadata.BranchSafe, retrieved.BranchSafe)
	}
}

func TestMetadataWithNilValues(t *testing.T) {
	enhanced := EnhancedSubtaskSpec{
		SubtaskSpec: SubtaskSpec{
			Name:     "test",
			Type:     core.TaskType("code"),
			Executor: "react",
		},
		// All extended fields are nil/zero.
	}

	// Should handle gracefully without panicking.
	_ = GetVerificationHint(SubtaskSpec(enhanced.SubtaskSpec))
	_ = GetFileFocus(SubtaskSpec(enhanced.SubtaskSpec))
	_ = GetCostClass(SubtaskSpec(enhanced.SubtaskSpec))
	_ = GetRetryClass(SubtaskSpec(enhanced.SubtaskSpec))
	_ = IsBranchSafe(SubtaskSpec(enhanced.SubtaskSpec))
	_ = GetExpectedOutput(SubtaskSpec(enhanced.SubtaskSpec))
}

func TestExtractMethodMetadata(t *testing.T) {
	spec := MethodSpec{
		Name:          "test_method",
		TaskType:      core.TaskType("code"),
		Priority:      1,
		OperatorCount: 3,
	}

	// Default extraction.
	metadata := ExtractMethodMetadata(spec)

	if metadata.CostClass != CostClassUnknown {
		t.Errorf("Expected Unknown cost class, got %s", metadata.CostClass)
	}
	if metadata.VerificationHint != nil {
		t.Error("Expected no verification hint, got one")
	}
}

func TestOperatorMetadataRoundtrip(t *testing.T) {
	original := OperatorMetadata{
		CostClass:  CostClassMedium,
		RetryClass: RetryClassStateless,
		BranchSafe: true,
	}

	state := core.NewContext()
	PublishOperatorMetadata(state, "op", original)

	retrieved, ok := GetPublishedOperatorMetadata(state, "op")
	if !ok {
		t.Fatal("Failed to retrieve metadata")
	}

	// Verify round-trip.
	if retrieved.CostClass != original.CostClass {
		t.Errorf("Cost class mismatch after round-trip")
	}
	if retrieved.RetryClass != original.RetryClass {
		t.Errorf("Retry class mismatch after round-trip")
	}
	if retrieved.BranchSafe != original.BranchSafe {
		t.Errorf("Branch safety mismatch after round-trip")
	}
}
