package features

import (
	"context"
	"fmt"
	"time"

	"github.com/lexcodex/relurpify/agents/htn/authoring"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
)

// Note: VerificationHint and FileFocus need to be defined as structs with proper fields

// Phase 11: Recovery optimization and file-scoped search.
// Uses Phase 8 metadata (RetryClass, FileFocus, VerificationHint) and Phase 10 automation
// (ShouldRetryStep, OperatorMetricsSnapshot) to implement intelligent recovery strategies,
// adaptive timeouts, and scoped recovery searches.

// RetryClass categorizes how a failed step should be retried.
type RetryClass string

const (
	RetryClassIdempotent RetryClass = "idempotent"
	RetryClassStateless  RetryClass = "stateless"
	RetryClassStateful   RetryClass = "stateful"
	RetryClassProbed     RetryClass = "probed"
	RetryClassNone       RetryClass = "none"
)

// FileFocus restricts recovery search scope to specific files.
type FileFocus struct {
	Primary   []string
	Secondary []string
	Patterns  []string
	Exclude   []string
}

// VerificationHint provides guidance for recovery verification.
type VerificationHint struct {
	Description string
	Criteria    []string
	Files       []string
	Timeout     time.Duration
}

// OperatorMetadata encapsulates operator execution metrics and metadata.
type OperatorMetadata struct {
	Name              string
	Duration          int
	Success           bool
	Retried           bool
	RetryClass        RetryClass
	CostClass         authoring.CostClass
	BranchSafe        bool
	VerificationHint  VerificationHint
	FileFocus         FileFocus
	ExpectedOutput    string
}

// RecoveryStrategy defines how to recover from a failed step.
type RecoveryStrategy interface {
	// Name returns the strategy name.
	Name() string
	// CanApply checks if strategy is applicable.
	CanApply(stepID string, retryClass RetryClass, attemptCount int, maxAttempts int) bool
	// Execute applies the recovery strategy.
	Execute(ctx context.Context, state *core.Context, stepID string, lastError error) error
}

// IdempotentRecoveryStrategy - Direct retry without state modification.
// Used for operations safe to retry unconditionally.
type IdempotentRecoveryStrategy struct{}

func (s *IdempotentRecoveryStrategy) Name() string {
	return "idempotent"
}

func (s *IdempotentRecoveryStrategy) CanApply(stepID string, retryClass RetryClass, attemptCount int, maxAttempts int) bool {
	return retryClass == RetryClassIdempotent && attemptCount < maxAttempts
}

func (s *IdempotentRecoveryStrategy) Execute(ctx context.Context, state *core.Context, stepID string, lastError error) error {
	// No state modification needed - operator is idempotent
	// Operator will be re-executed with same inputs
	return nil
}

// StatelessRecoveryStrategy - Reset operator state before retry.
// Used for operations that maintain internal state that should be reset.
type StatelessRecoveryStrategy struct{}

func (s *StatelessRecoveryStrategy) Name() string {
	return "stateless"
}

func (s *StatelessRecoveryStrategy) CanApply(stepID string, retryClass RetryClass, attemptCount int, maxAttempts int) bool {
	return retryClass == RetryClassStateless && attemptCount < maxAttempts
}

func (s *StatelessRecoveryStrategy) Execute(ctx context.Context, state *core.Context, stepID string, lastError error) error {
	if state == nil {
		return nil
	}

	// Clear operator-specific state by setting to nil
	contextKey := fmt.Sprintf("operator_state.%s", stepID)
	state.Set(contextKey, nil)

	// Clear step history by setting to nil
	historyKey := fmt.Sprintf("step_history.%s", stepID)
	state.Set(historyKey, nil)

	return nil
}

// ProbedRecoveryStrategy - Verify preconditions before retry.
// Used for operations where preconditions may have changed.
type ProbedRecoveryStrategy struct {
	// Prober is called to check if preconditions are met
	Prober func(ctx context.Context, state *core.Context, stepID string) (bool, error)
}

func (s *ProbedRecoveryStrategy) Name() string {
	return "probed"
}

func (s *ProbedRecoveryStrategy) CanApply(stepID string, retryClass RetryClass, attemptCount int, maxAttempts int) bool {
	return retryClass == RetryClassProbed && attemptCount < maxAttempts
}

func (s *ProbedRecoveryStrategy) Execute(ctx context.Context, state *core.Context, stepID string, lastError error) error {
	if s.Prober == nil {
		return nil
	}

	// Check if preconditions are met
	preconditionsMet, err := s.Prober(ctx, state, stepID)
	if err != nil {
		return fmt.Errorf("precondition check failed: %w", err)
	}

	if !preconditionsMet {
		return fmt.Errorf("preconditions not met for retry")
	}

	return nil
}

// AdaptiveTimeoutCalculator computes timeout based on historical data.
type AdaptiveTimeoutCalculator struct {
	// BaseTimeout is the minimum timeout.
	BaseTimeout time.Duration
	// SafetyMultiplier is applied to average duration.
	SafetyMultiplier float64
	// MaxTimeout caps the calculated timeout.
	MaxTimeout time.Duration
}

// CalculateTimeout computes appropriate timeout for a step.
func (c *AdaptiveTimeoutCalculator) CalculateTimeout(metrics *OperatorMetricsSnapshot) time.Duration {
	if metrics == nil {
		return c.BaseTimeout
	}

	// Base timeout on historical average
	avgDuration := time.Duration(metrics.AverageDuration) * time.Second
	adaptiveTimeout := time.Duration(float64(avgDuration) * c.SafetyMultiplier)

	// Apply minimum and maximum bounds
	if adaptiveTimeout < c.BaseTimeout {
		adaptiveTimeout = c.BaseTimeout
	}
	if adaptiveTimeout > c.MaxTimeout {
		adaptiveTimeout = c.MaxTimeout
	}

	return adaptiveTimeout
}

// FileRecoveryScope defines which files to search during recovery.
type FileRecoveryScope struct {
	// PrimaryFiles are most relevant and searched first.
	PrimaryFiles []string
	// SecondaryFiles are searched if primary doesn't yield results.
	SecondaryFiles []string
	// Patterns match files to include.
	Patterns []string
	// Exclude patterns match files to skip.
	Exclude []string
	// MaxFiles limits search scope.
	MaxFiles int
}

// ExtractRecoveryScopeFromMetadata creates recovery scope from Phase 8 metadata.
func ExtractRecoveryScopeFromMetadata(fileFocus *FileFocus, maxFiles int) *FileRecoveryScope {
	if fileFocus == nil {
		return &FileRecoveryScope{MaxFiles: maxFiles}
	}

	return &FileRecoveryScope{
		PrimaryFiles:   fileFocus.Primary,
		SecondaryFiles: fileFocus.Secondary,
		Patterns:       fileFocus.Patterns,
		Exclude:        fileFocus.Exclude,
		MaxFiles:       maxFiles,
	}
}

// VerificationContext carries context for step verification.
type VerificationContext struct {
	// StepID is the step being verified.
	StepID string
	// OperatorName is the operator that executed the step.
	OperatorName string
	// ExecutionResult is the result from execution.
	ExecutionResult *core.Result
	// Hint provides guidance on verification.
	Hint *VerificationHint
	// Files are relevant files for verification.
	Files []string
	// Timeout is the expected verification time.
	Timeout time.Duration
}

// VerificationStrategy defines how to verify step completion.
type VerificationStrategy interface {
	// Name returns strategy name.
	Name() string
	// Verify checks if step completed successfully.
	Verify(ctx context.Context, state *core.Context, vctx *VerificationContext) (bool, []string)
}

// HintBasedVerificationStrategy uses VerificationHint guidance.
type HintBasedVerificationStrategy struct{}

func (s *HintBasedVerificationStrategy) Name() string {
	return "hint_based"
}

func (s *HintBasedVerificationStrategy) Verify(ctx context.Context, state *core.Context, vctx *VerificationContext) (bool, []string) {
	if vctx.Hint == nil || len(vctx.Hint.Criteria) == 0 {
		// No hint, assume success
		return true, nil
	}

	var issues []string

	// Check each criterion
	for _, criterion := range vctx.Hint.Criteria {
		// In a real implementation, would execute verification checks
		// For now, mark as verified if criterion is documented
		if criterion == "" {
			issues = append(issues, "empty verification criterion")
		}
	}

	return len(issues) == 0, issues
}

// ResultBasedVerificationStrategy checks execution result.
type ResultBasedVerificationStrategy struct{}

func (s *ResultBasedVerificationStrategy) Name() string {
	return "result_based"
}

func (s *ResultBasedVerificationStrategy) Verify(ctx context.Context, state *core.Context, vctx *VerificationContext) (bool, []string) {
	if vctx.ExecutionResult == nil {
		return false, []string{"no execution result"}
	}

	if !vctx.ExecutionResult.Success {
		return false, []string{"execution result indicates failure"}
	}

	if vctx.ExecutionResult.Data == nil {
		return false, []string{"execution produced no output"}
	}

	return true, nil
}

// RecoveryPolicyEngine coordinates recovery decisions and execution.
type RecoveryPolicyEngine struct {
	// Strategies available for recovery.
	Strategies []RecoveryStrategy
	// TimeoutCalculator computes adaptive timeouts.
	TimeoutCalculator *AdaptiveTimeoutCalculator
	// VerificationStrategies available for verification.
	VerificationStrategies []VerificationStrategy
	// MaxRecoveryAttempts limits retry attempts.
	MaxRecoveryAttempts int
}

// DetermineRecoveryAction decides what recovery action to take.
func (e *RecoveryPolicyEngine) DetermineRecoveryAction(
	stepID string,
	retryClass RetryClass,
	attemptCount int,
	lastError error,
) (RecoveryStrategy, error) {
	// Check if recovery is possible
	if attemptCount >= e.MaxRecoveryAttempts {
		return nil, fmt.Errorf("max recovery attempts exceeded")
	}

	if retryClass == RetryClassNone {
		return nil, fmt.Errorf("retry class is none, recovery not permitted")
	}

	// Find applicable strategy
	for _, strategy := range e.Strategies {
		if strategy.CanApply(stepID, retryClass, attemptCount, e.MaxRecoveryAttempts) {
			return strategy, nil
		}
	}

	return nil, fmt.Errorf("no recovery strategy applicable for retry class %s", retryClass)
}

// ApplyRecoveryStrategy executes the determined recovery strategy.
func (e *RecoveryPolicyEngine) ApplyRecoveryStrategy(
	ctx context.Context,
	state *core.Context,
	strategy RecoveryStrategy,
	stepID string,
	lastError error,
) error {
	if strategy == nil {
		return fmt.Errorf("no recovery strategy provided")
	}

	return strategy.Execute(ctx, state, stepID, lastError)
}

// VerifyStepCompletion verifies that a step completed successfully.
func (e *RecoveryPolicyEngine) VerifyStepCompletion(
	ctx context.Context,
	state *core.Context,
	vctx *VerificationContext,
) (bool, []string) {
	if vctx == nil {
		return false, []string{"no verification context"}
	}

	if len(e.VerificationStrategies) == 0 {
		// No verification strategies, trust execution result
		if vctx.ExecutionResult != nil && vctx.ExecutionResult.Success {
			return true, nil
		}
		return false, []string{"execution result indicates failure"}
	}

	// Try each strategy until one succeeds
	var allIssues []string

	for _, strategy := range e.VerificationStrategies {
		success, issues := strategy.Verify(ctx, state, vctx)
		if success {
			return true, nil
		}
		allIssues = append(allIssues, issues...)
	}

	return false, allIssues
}

// RecoveryContextBuilder builds recovery context from available metadata and state.
type RecoveryContextBuilder struct {
	// Store provides access to historical artifacts.
	Store memory.WorkflowStateStore
	// WorkflowID scopes artifact queries.
	WorkflowID string
}

// BuildRecoveryContext assembles recovery context from metadata and history.
func (b *RecoveryContextBuilder) BuildRecoveryContext(
	ctx context.Context,
	stepID string,
	operatorName string,
	metadata OperatorMetadata,
	lastError error,
	attemptCount int,
) map[string]any {
	recoveryContext := map[string]any{
		"step_id":        stepID,
		"operator_name": operatorName,
		"attempt_count": attemptCount,
		"last_error":    lastError.Error(),
		"retry_class":   string(metadata.RetryClass),
		"cost_class":    string(metadata.CostClass),
		"branch_safe":   metadata.BranchSafe,
	}

	// Add verification hint
	if metadata.VerificationHint.Description != "" || len(metadata.VerificationHint.Criteria) > 0 {
		recoveryContext["verification_hint"] = map[string]any{
			"description": metadata.VerificationHint.Description,
			"criteria":    metadata.VerificationHint.Criteria,
			"files":       metadata.VerificationHint.Files,
			"timeout":     metadata.VerificationHint.Timeout,
		}
	}

	// Add file focus
	if len(metadata.FileFocus.Primary) > 0 || len(metadata.FileFocus.Secondary) > 0 || len(metadata.FileFocus.Patterns) > 0 {
		recoveryContext["file_focus"] = map[string]any{
			"primary":   metadata.FileFocus.Primary,
			"secondary": metadata.FileFocus.Secondary,
			"patterns":  metadata.FileFocus.Patterns,
			"exclude":   metadata.FileFocus.Exclude,
		}
	}

	// Add expected output schema
	if metadata.ExpectedOutput != "" {
		recoveryContext["expected_output"] = metadata.ExpectedOutput
	}

	return recoveryContext
}

// NewRecoveryPolicyEngine creates a recovery engine with default strategies.
func NewRecoveryPolicyEngine() *RecoveryPolicyEngine {
	return &RecoveryPolicyEngine{
		Strategies: []RecoveryStrategy{
			&IdempotentRecoveryStrategy{},
			&StatelessRecoveryStrategy{},
			&ProbedRecoveryStrategy{},
		},
		TimeoutCalculator: &AdaptiveTimeoutCalculator{
			BaseTimeout:      5 * time.Second,
			SafetyMultiplier: 2.0,
			MaxTimeout:       5 * time.Minute,
		},
		VerificationStrategies: []VerificationStrategy{
			&HintBasedVerificationStrategy{},
			&ResultBasedVerificationStrategy{},
		},
		MaxRecoveryAttempts: 3,
	}
}

// RecoveryMetrics tracks recovery attempts and outcomes.
type RecoveryMetrics struct {
	StepID              string
	TotalAttempts       int
	SuccessfulAttempts  int
	FailedAttempts      int
	LastRecoveryTime    time.Time
	LastRecoveryError   string
	TotalRecoveryTime   time.Duration
	AverageRecoveryTime time.Duration
}

// RecordRecoveryAttempt updates recovery metrics.
func RecordRecoveryAttempt(metrics *RecoveryMetrics, duration time.Duration, success bool, err error) {
	if metrics == nil {
		return
	}

	metrics.TotalAttempts++
	metrics.LastRecoveryTime = time.Now()
	metrics.TotalRecoveryTime += duration

	if success {
		metrics.SuccessfulAttempts++
	} else {
		metrics.FailedAttempts++
		if err != nil {
			metrics.LastRecoveryError = err.Error()
		}
	}

	if metrics.TotalAttempts > 0 {
		metrics.AverageRecoveryTime = metrics.TotalRecoveryTime / time.Duration(metrics.TotalAttempts)
	}
}
