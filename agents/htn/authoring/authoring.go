package authoring

import (
	"codeburg.org/lexbit/relurpify/agents/htn/runtime"
	"codeburg.org/lexbit/relurpify/framework/core"
)

// Type aliases for convenience.
type SubtaskSpec = runtime.SubtaskSpec
type OperatorSpec = runtime.OperatorSpec
type MethodSpec = runtime.MethodSpec

// Phase 8: Extended method authoring model supporting richer runtime hints
// without changing HTN internals. These types enrich SubtaskSpec, MethodSpec,
// and OperatorSpec with metadata for verification, file focus, cost class,
// branch safety, retry strategy, and expected outputs.

// VerificationHint describes how to verify a step's success.
type VerificationHint struct {
	// Description is a human-readable guide on how to verify success.
	Description string `json:"description,omitempty"`
	// Criteria lists specific verification checks to perform.
	Criteria []string `json:"criteria,omitempty"`
	// Files to inspect for verification.
	Files []string `json:"files,omitempty"`
	// Timeout is the expected time for verification in seconds.
	Timeout int `json:"timeout,omitempty"`
}

// FileFocus describes which files are relevant to a step.
type FileFocus struct {
	// Primary files most relevant to this step.
	Primary []string `json:"primary,omitempty"`
	// Secondary files that may be affected.
	Secondary []string `json:"secondary,omitempty"`
	// Patterns to match files (e.g., "*.go", "test_*.py").
	Patterns []string `json:"patterns,omitempty"`
	// Exclude patterns to skip files.
	Exclude []string `json:"exclude,omitempty"`
}

// CostClass categorizes the execution cost/latency characteristics.
type CostClass string

const (
	CostClassFast    CostClass = "fast"    // < 1 second expected
	CostClassMedium  CostClass = "medium"  // 1-10 seconds expected
	CostClassSlow    CostClass = "slow"    // > 10 seconds expected
	CostClassUnknown CostClass = "unknown" // Unknown cost
)

// RetryClass categorizes when and how a step can be retried.
type RetryClass string

const (
	RetryClassNone       RetryClass = "none"       // Never retry
	RetryClassIdempotent RetryClass = "idempotent" // Safe to retry unconditionally
	RetryClassStateless  RetryClass = "stateless"  // Retry after state reset
	RetryClassProbed     RetryClass = "probed"     // Retry after probe/validation
	RetryClassUnknown    RetryClass = "unknown"    // Unknown retry semantics
)

// EnhancedSubtaskSpec extends SubtaskSpec with Phase 8 runtime hints.
// This type is used internally; SubtaskSpec remains the primary user-facing type.
type EnhancedSubtaskSpec struct {
	runtime.SubtaskSpec
	// VerificationHint guides how to verify step success.
	VerificationHint *VerificationHint `json:"verification_hint,omitempty"`
	// FileFocus describes relevant files for this step.
	FileFocus *FileFocus `json:"file_focus,omitempty"`
	// CostClass hints at execution cost/latency.
	CostClass CostClass `json:"cost_class,omitempty"`
	// BranchSafe indicates whether this step can safely execute in parallel.
	BranchSafe bool `json:"branch_safe,omitempty"`
	// RetryClass describes retry semantics for this step.
	RetryClass RetryClass `json:"retry_class,omitempty"`
	// ExpectedOutput describes expected output structure/schema.
	ExpectedOutput map[string]any `json:"expected_output,omitempty"`
}

// EnhancedOperatorSpec extends OperatorSpec with Phase 8 runtime hints.
type EnhancedOperatorSpec struct {
	runtime.OperatorSpec
	// VerificationHint guides verification of operator completion.
	VerificationHint *VerificationHint `json:"verification_hint,omitempty"`
	// FileFocus describes files affected by this operator.
	FileFocus *FileFocus `json:"file_focus,omitempty"`
	// CostClass hints at execution cost.
	CostClass CostClass `json:"cost_class,omitempty"`
	// BranchSafe indicates parallel execution safety.
	BranchSafe bool `json:"branch_safe,omitempty"`
	// RetryClass describes retry semantics.
	RetryClass RetryClass `json:"retry_class,omitempty"`
	// ExpectedOutput describes expected output structure.
	ExpectedOutput map[string]any `json:"expected_output,omitempty"`
}

// EnhancedMethodSpec extends MethodSpec with Phase 8 runtime hints.
type EnhancedMethodSpec struct {
	runtime.MethodSpec
	// VerificationHint guides verification of method completion.
	VerificationHint *VerificationHint `json:"verification_hint,omitempty"`
	// FileFocus describes files relevant to this method.
	FileFocus *FileFocus `json:"file_focus,omitempty"`
	// CostClass aggregated from operators.
	CostClass CostClass `json:"cost_class,omitempty"`
	// BranchSafe indicates if all operators support parallel execution.
	BranchSafe bool `json:"branch_safe,omitempty"`
	// ExpectedOutput describes method completion output.
	ExpectedOutput map[string]any `json:"expected_output,omitempty"`
}

// EnhancedSubtaskSpecProvider allows a SubtaskSpec to provide extended metadata.
type EnhancedSubtaskSpecProvider interface {
	EnhancedSubtaskSpec() EnhancedSubtaskSpec
}

// EnhancedOperatorSpecProvider allows an operator to provide extended metadata.
type EnhancedOperatorSpecProvider interface {
	EnhancedOperatorSpec() EnhancedOperatorSpec
}

// EnhancedMethodSpecProvider allows a method to provide extended metadata.
type EnhancedMethodSpecProvider interface {
	EnhancedMethodSpec() EnhancedMethodSpec
}

// GetVerificationHint extracts verification hint from a subtask or returns nil.
func GetVerificationHint(subtask SubtaskSpec) *VerificationHint {
	if provider, ok := any(subtask).(EnhancedSubtaskSpecProvider); ok {
		enhanced := provider.EnhancedSubtaskSpec()
		return enhanced.VerificationHint
	}
	return nil
}

// GetFileFocus extracts file focus from a subtask or returns nil.
func GetFileFocus(subtask SubtaskSpec) *FileFocus {
	if provider, ok := any(subtask).(EnhancedSubtaskSpecProvider); ok {
		enhanced := provider.EnhancedSubtaskSpec()
		return enhanced.FileFocus
	}
	return nil
}

// GetCostClass extracts cost class from a subtask, defaults to Unknown.
func GetCostClass(subtask SubtaskSpec) CostClass {
	if provider, ok := any(subtask).(EnhancedSubtaskSpecProvider); ok {
		enhanced := provider.EnhancedSubtaskSpec()
		if enhanced.CostClass != "" {
			return enhanced.CostClass
		}
	}
	return CostClassUnknown
}

// IsBranchSafe checks if a subtask is safe for parallel execution.
func IsBranchSafe(subtask SubtaskSpec) bool {
	if provider, ok := any(subtask).(EnhancedSubtaskSpecProvider); ok {
		enhanced := provider.EnhancedSubtaskSpec()
		return enhanced.BranchSafe
	}
	return false
}

// GetRetryClass extracts retry class from a subtask, defaults to Unknown.
func GetRetryClass(subtask SubtaskSpec) RetryClass {
	if provider, ok := any(subtask).(EnhancedSubtaskSpecProvider); ok {
		enhanced := provider.EnhancedSubtaskSpec()
		if enhanced.RetryClass != "" {
			return enhanced.RetryClass
		}
	}
	return RetryClassUnknown
}

// IsRetryable checks if a subtask can be retried.
func IsRetryable(subtask SubtaskSpec) bool {
	rc := GetRetryClass(subtask)
	return rc != RetryClassNone && rc != RetryClassUnknown
}

// GetExpectedOutput extracts expected output schema from a subtask or returns nil.
func GetExpectedOutput(subtask SubtaskSpec) map[string]any {
	if provider, ok := any(subtask).(EnhancedSubtaskSpecProvider); ok {
		enhanced := provider.EnhancedSubtaskSpec()
		return enhanced.ExpectedOutput
	}
	return nil
}

// OperatorMetadata captures Phase 8 metadata for an operator.
type OperatorMetadata struct {
	VerificationHint *VerificationHint
	FileFocus        *FileFocus
	CostClass        CostClass
	BranchSafe       bool
	RetryClass       RetryClass
	ExpectedOutput   map[string]any
}

// ExtractOperatorMetadata extracts Phase 8 metadata from an operator spec.
func ExtractOperatorMetadata(operator OperatorSpec) OperatorMetadata {
	metadata := OperatorMetadata{
		CostClass:  CostClassUnknown,
		RetryClass: RetryClassUnknown,
	}

	if provider, ok := any(operator).(EnhancedOperatorSpecProvider); ok {
		enhanced := provider.EnhancedOperatorSpec()
		metadata.VerificationHint = enhanced.VerificationHint
		metadata.FileFocus = enhanced.FileFocus
		metadata.CostClass = enhanced.CostClass
		metadata.BranchSafe = enhanced.BranchSafe
		metadata.RetryClass = enhanced.RetryClass
		metadata.ExpectedOutput = enhanced.ExpectedOutput
	}

	return metadata
}

// MethodMetadata captures Phase 8 metadata for a method.
type MethodMetadata struct {
	VerificationHint *VerificationHint
	FileFocus        *FileFocus
	CostClass        CostClass
	BranchSafe       bool
	ExpectedOutput   map[string]any
}

// ExtractMethodMetadata extracts Phase 8 metadata from a method spec.
func ExtractMethodMetadata(spec MethodSpec) MethodMetadata {
	metadata := MethodMetadata{
		CostClass: CostClassUnknown,
	}

	if provider, ok := any(spec).(EnhancedMethodSpecProvider); ok {
		enhanced := provider.EnhancedMethodSpec()
		metadata.VerificationHint = enhanced.VerificationHint
		metadata.FileFocus = enhanced.FileFocus
		metadata.CostClass = enhanced.CostClass
		metadata.BranchSafe = enhanced.BranchSafe
		metadata.ExpectedOutput = enhanced.ExpectedOutput
	}

	return metadata
}

// AggregateOperatorMetadata combines metadata from multiple operators.
// Used to determine method-level metadata from operator metadata.
func AggregateOperatorMetadata(operators []OperatorSpec) OperatorMetadata {
	if len(operators) == 0 {
		return OperatorMetadata{
			CostClass:  CostClassUnknown,
			RetryClass: RetryClassUnknown,
		}
	}

	// Extract metadata from all operators
	metadataList := make([]OperatorMetadata, len(operators))
	for i, op := range operators {
		metadataList[i] = ExtractOperatorMetadata(op)
	}

	// Aggregate
	aggregated := OperatorMetadata{
		CostClass:  CostClassUnknown,
		RetryClass: RetryClassUnknown,
		BranchSafe: true, // Start as true, set false if any operator isn't
	}

	// Cost class: use slowest
	costRank := map[CostClass]int{
		CostClassFast:    1,
		CostClassMedium:  2,
		CostClassSlow:    3,
		CostClassUnknown: 0,
	}

	maxRank := 0
	for _, meta := range metadataList {
		if rank := costRank[meta.CostClass]; rank > maxRank {
			maxRank = rank
			aggregated.CostClass = meta.CostClass
		}
	}

	// Branch safety: all must be true
	for _, meta := range metadataList {
		if !meta.BranchSafe {
			aggregated.BranchSafe = false
			break
		}
	}

	return aggregated
}

// CostClassFromString converts string to CostClass, returns Unknown if invalid.
func CostClassFromString(s string) CostClass {
	switch s {
	case string(CostClassFast):
		return CostClassFast
	case string(CostClassMedium):
		return CostClassMedium
	case string(CostClassSlow):
		return CostClassSlow
	default:
		return CostClassUnknown
	}
}

// RetryClassFromString converts string to RetryClass, returns Unknown if invalid.
func RetryClassFromString(s string) RetryClass {
	switch s {
	case string(RetryClassNone):
		return RetryClassNone
	case string(RetryClassIdempotent):
		return RetryClassIdempotent
	case string(RetryClassStateless):
		return RetryClassStateless
	case string(RetryClassProbed):
		return RetryClassProbed
	default:
		return RetryClassUnknown
	}
}

// PublishOperatorMetadata publishes operator metadata to the execution context.
func PublishOperatorMetadata(state *core.Context, operator string, metadata OperatorMetadata) {
	if state == nil {
		return
	}
	key := "htn.operator_metadata." + operator
	state.Set(key, metadata)
}

// GetPublishedOperatorMetadata retrieves operator metadata from context.
func GetPublishedOperatorMetadata(state *core.Context, operator string) (OperatorMetadata, bool) {
	if state == nil {
		return OperatorMetadata{}, false
	}
	key := "htn.operator_metadata." + operator
	if raw, ok := state.Get(key); ok {
		if typed, ok := raw.(OperatorMetadata); ok {
			return typed, true
		}
	}
	return OperatorMetadata{}, false
}
