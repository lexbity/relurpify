package execution

import (
	"github.com/lexcodex/relurpify/agents/goalcon/types"
)

import (
	"context"
	"fmt"
	"time"

	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
)

// StepExecutionRequest encapsulates parameters for executing a single plan step.
type StepExecutionRequest struct {
	Step             core.PlanStep
	Context          *core.Context
	CapabilityRegistry *capability.Registry
	Timeout          time.Duration
	OnFailure        FailureMode // How to handle step failure
}

// FailureMode determines behavior when a step fails.
type FailureMode int

const (
	FailureModeAbort    FailureMode = iota // Stop execution immediately
	FailureModeContinue                      // Log error and continue
	FailureModeRetry                         // Retry with backoff
)

// StepExecutionResult captures the outcome of executing a step.
type StepExecutionResult struct {
	StepID        string
	Success       bool
	Duration      time.Duration
	ToolName      string
	Error         error
	Output        map[string]any
	ExecutedAt    time.Time
	Retries       int
	CapabilityID  string
}

// StepExecutor executes individual plan steps via capability invocation.
type StepExecutor struct {
	registry   *capability.Registry
	timeout    time.Duration
	metrics    *types.MetricsRecorder          // Optional metrics recording
	auditTrail *types.CapabilityAuditTrail     // Optional audit trail (Phase 5)
}

// NewStepExecutor creates a new step executor.
func NewStepExecutor(registry *capability.Registry) *StepExecutor {
	return &StepExecutor{
		registry: registry,
		timeout:  30 * time.Second, // Default timeout
	}
}

// SetTimeout sets the default timeout for step execution.
func (e *StepExecutor) SetTimeout(d time.Duration) {
	if e != nil && d > 0 {
		e.timeout = d
	}
}

// SetMetricsRecorder optionally records execution metrics.
func (e *StepExecutor) SetMetricsRecorder(recorder *types.MetricsRecorder) {
	if e != nil {
		e.metrics = recorder
	}
}

// SetAuditTrail optionally records capability invocations to an audit trail.
func (e *StepExecutor) SetAuditTrail(trail *types.CapabilityAuditTrail) {
	if e != nil {
		e.auditTrail = trail
	}
}

// Execute runs a single plan step and returns the result.
func (e *StepExecutor) Execute(ctx context.Context, req StepExecutionRequest) *StepExecutionResult {
	if e == nil {
		return &StepExecutionResult{
			Success: false,
			Error:   fmt.Errorf("executor is nil"),
		}
	}

	if req.Step.Tool == "" {
		return &StepExecutionResult{
			StepID:  req.Step.ID,
			Success: false,
			Error:   fmt.Errorf("step has no tool name"),
		}
	}

	start := time.Now()
	result := &StepExecutionResult{
		StepID:     req.Step.ID,
		ToolName:   req.Step.Tool,
		ExecutedAt: start,
	}

	// Determine timeout
	timeout := e.timeout
	if req.Timeout > 0 {
		timeout = req.Timeout
	}

	// Create execution context
	if req.Context == nil {
		req.Context = core.NewContext()
	}

	// Execute step with timeout
	ctxWithTimeout, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Look up capability
	cap := e.lookupCapability(req.Step.Tool)
	if cap == nil {
		result.Error = fmt.Errorf("capability not found: %s", req.Step.Tool)
		result.Duration = time.Since(start)
		e.recordMetrics(result)
		return result
	}

	result.CapabilityID = cap.ID

	// Execute step via agent/tool
	toolResult, err := e.executeToolStep(ctxWithTimeout, req.Step, req.Context, cap)
	result.Duration = time.Since(start)

	if err != nil {
		result.Error = err
		result.Success = false
		e.recordMetrics(result)
		return result
	}

	result.Success = toolResult.Success
	result.Output = toolResult.Data

	// Update world state based on success
	if result.Success && req.Context != nil {
		e.updateWorldState(req.Context, req.Step)
	}

	// Record audit trail if enabled (Phase 5)
	e.recordAudit(result, req.Step, cap, toolResult)

	e.recordMetrics(result)
	return result
}

// lookupCapability finds a capability by name.
func (e *StepExecutor) lookupCapability(toolName string) *core.CapabilityDescriptor {
	if e == nil || e.registry == nil {
		return nil
	}

	// Try to find capability by name in invocable capabilities
	caps := e.registry.InvocableCapabilities()
	for _, cap := range caps {
		if cap.Name == toolName || cap.ID == toolName {
			return &cap
		}
	}

	return nil
}

// executeToolStep invokes a capability via the registry.
// In Phase 4, this is a placeholder that returns success.
// In Phase 5+, this would integrate with real agents to execute the capability.
func (e *StepExecutor) executeToolStep(
	ctx context.Context,
	step core.PlanStep,
	state *core.Context,
	cap *core.CapabilityDescriptor,
) (*core.Result, error) {
	if cap == nil {
		return nil, fmt.Errorf("capability is nil")
	}

	// Phase 4: Placeholder implementation
	// Returns success for all capabilities
	// In Phase 5+, would actually invoke the capability via agent
	return &core.Result{
		Success: true,
		Data: map[string]any{
			"step_id":      step.ID,
			"tool":         step.Tool,
			"capability":   cap.Name,
			"executed":     true,
			"placeholder":  true, // Indicates this is not a real execution
		},
	}, nil
}

// updateWorldState marks predicates as satisfied based on step success.
func (e *StepExecutor) updateWorldState(ctx *core.Context, step core.PlanStep) {
	if ctx == nil {
		return
	}

	// Store execution result in context
	executionKey := fmt.Sprintf("goalcon.step_result.%s", step.ID)
	ctx.Set(executionKey, map[string]any{
		"tool":     step.Tool,
		"params":   step.Params,
		"executed": true,
	})
}

// recordAudit records the capability invocation to the audit trail (Phase 5).
func (e *StepExecutor) recordAudit(result *StepExecutionResult, step core.PlanStep, cap *core.CapabilityDescriptor, toolResult *core.Result) {
	if e == nil || e.auditTrail == nil || cap == nil {
		return
	}

	// Convert core.Result to core.ToolResult for the envelope
	toolResultEnv := &core.ToolResult{
		Success: toolResult.Success,
		Data:    toolResult.Data,
		Error:   "",
	}
	if toolResult.Error != nil {
		toolResultEnv.Error = fmt.Sprintf("%v", toolResult.Error)
	}

	// Create a minimal CapabilityResultEnvelope from the step execution
	envelope := &core.CapabilityResultEnvelope{
		Descriptor:  *cap,
		Result:      toolResultEnv,
		Disposition: core.ContentDispositionMetadataOnly,
		RecordedAt:  time.Now().UTC(),
	}

	// For Phase 5 placeholder, we use metadata-only insertion
	decision := core.InsertionDecision{
		Action: core.InsertionActionMetadataOnly,
		Reason: "phase-5-placeholder-execution",
	}

	e.auditTrail.RecordInvocation(step.ID, envelope, decision)
}

// recordMetrics records step execution metrics if recorder available.
func (e *StepExecutor) recordMetrics(result *StepExecutionResult) {
	if e == nil || e.metrics == nil || result == nil {
		return
	}

	execMetrics := ExecutionMetrics{
		OperatorName: result.ToolName,
		Success:      result.Success,
		Duration:     result.Duration,
	}

	_ = e.metrics.RecordExecution(execMetrics)
}

// ExecutorChain allows sequential execution of multiple steps with error handling.
type ExecutorChain struct {
	executor  *StepExecutor
	failureMode FailureMode
	results   []*StepExecutionResult
}

// NewExecutorChain creates a new execution chain.
func NewExecutorChain(executor *StepExecutor) *ExecutorChain {
	return &ExecutorChain{
		executor:    executor,
		failureMode: FailureModeContinue,
		results:     make([]*StepExecutionResult, 0),
	}
}

// SetFailureMode sets how the chain handles step failures.
func (ec *ExecutorChain) SetFailureMode(mode FailureMode) {
	if ec != nil {
		ec.failureMode = mode
	}
}

// ExecuteSteps executes a sequence of steps.
func (ec *ExecutorChain) ExecuteSteps(
	ctx context.Context,
	steps []core.PlanStep,
	planContext *core.Context,
	registry *capability.Registry,
) []*StepExecutionResult {
	if ec == nil || ec.executor == nil {
		return nil
	}

	ec.results = make([]*StepExecutionResult, 0, len(steps))

	for _, step := range steps {
		req := StepExecutionRequest{
			Step:               step,
			Context:            planContext,
			CapabilityRegistry: registry,
			OnFailure:          ec.failureMode,
		}

		result := ec.executor.Execute(ctx, req)
		ec.results = append(ec.results, result)

		// Handle failure based on mode
		if !result.Success {
			switch ec.failureMode {
			case FailureModeAbort:
				return ec.results
			case FailureModeContinue:
				// Log and continue
				continue
			case FailureModeRetry:
				// Retry logic would go here (Phase 6)
				continue
			}
		}
	}

	return ec.results
}

// Results returns all execution results.
func (ec *ExecutorChain) Results() []*StepExecutionResult {
	if ec == nil {
		return nil
	}
	return ec.results
}

// SuccessCount returns number of successful steps.
func (ec *ExecutorChain) SuccessCount() int {
	if ec == nil {
		return 0
	}
	count := 0
	for _, r := range ec.results {
		if r != nil && r.Success {
			count++
		}
	}
	return count
}

// FailureCount returns number of failed steps.
func (ec *ExecutorChain) FailureCount() int {
	if ec == nil {
		return 0
	}
	return len(ec.results) - ec.SuccessCount()
}

// Summary returns a human-readable execution summary.
func (ec *ExecutorChain) Summary() string {
	if ec == nil || len(ec.results) == 0 {
		return "No steps executed"
	}

	total := len(ec.results)
	success := ec.SuccessCount()
	fail := ec.FailureCount()

	var totalDuration time.Duration
	for _, r := range ec.results {
		if r != nil {
			totalDuration += r.Duration
		}
	}

	return fmt.Sprintf(
		"Executed %d steps: %d succeeded, %d failed (total time: %v)",
		total, success, fail, totalDuration,
	)
}
