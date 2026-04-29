package features

import (
	"context"
	"fmt"
	"sort"
	"time"

	"codeburg.org/lexbit/relurpify/agents/htn/authoring"
	"codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/memory"
)

// Phase 10: Persistence automation and knowledge integration.
// Uses Phase 8 metadata (authoring.CostClass, authoring.RetryClass, VerificationHint, FileFocus)
// and Phase 9 artifacts (HTNRunSummary, OperatorOutcome, ExecutionMetrics)
// to optimize scheduling, enable intelligent recovery, and enrich execution context.

// MetricsAggregator collects and analyzes historical execution metrics.
type MetricsAggregator struct {
	// TODO: Replace with agentlifecycle.Repository
	// per the agentlifecycle workflow-store removal plan
	Store interface{}
	// WorkflowID is the current workflow ID for scoping artifact queries.
	WorkflowID string
}

// OperatorMetricsSnapshot captures aggregated metrics for an operator across runs.
type OperatorMetricsSnapshot struct {
	OperatorName      string
	TotalRuns         int
	SuccessfulRuns    int
	FailedRuns        int
	AverageDuration   int // seconds
	MinDuration       int
	MaxDuration       int
	SuccessRate       float64 // 0.0 to 1.0
	MostCommonCost    authoring.CostClass
	MostCommonRetry   authoring.RetryClass
	LastObservedError string
	LastRunTime       time.Time
}

// OutputValidationSchema defines expected output structure.
type OutputValidationSchema struct {
	ExpectedKeys  []string
	OptionalKeys  []string
	Schema        map[string]any
	MinOutputSize int
	MaxOutputSize int
}

// SchedulingHint provides recommendations for step ordering and parallelization.
type SchedulingHint struct {
	StepID            string
	RecommendedOrder  int // Lower = execute first
	CanParallelize    bool
	EstimatedDuration int // seconds
	DependsOn         []string
	CostOptimization  string // "fast_first", "slow_first", "parallel"
}

// AggregateHistoricalMetrics retrieves and analyzes metrics for an operator from workflow artifacts.
// Returns snapshot of aggregated metrics across all runs.
func (ma *MetricsAggregator) AggregateHistoricalMetrics(ctx context.Context, operatorName string) (*OperatorMetricsSnapshot, error) {
	if ma.Store == nil {
		return nil, nil
	}

	snapshot := &OperatorMetricsSnapshot{
		OperatorName: operatorName,
		MinDuration:  999999,
	}

	// In a real implementation, this would query the store for historical artifacts.
	// For now, return a default snapshot structure.
	return snapshot, nil
}

// ValidateOutputAgainstSchema checks if step output matches expected schema.
func ValidateOutputAgainstSchema(output map[string]any, schema *OutputValidationSchema) (bool, []string) {
	var issues []string

	if output == nil {
		issues = append(issues, "output is nil")
		return false, issues
	}

	// Check expected keys exist
	for _, key := range schema.ExpectedKeys {
		if _, ok := output[key]; !ok {
			issues = append(issues, fmt.Sprintf("expected key missing: %s", key))
		}
	}

	// Check output size constraints
	if schema.MinOutputSize > 0 && len(output) < schema.MinOutputSize {
		issues = append(issues, fmt.Sprintf("output size %d below minimum %d", len(output), schema.MinOutputSize))
	}
	if schema.MaxOutputSize > 0 && len(output) > schema.MaxOutputSize {
		issues = append(issues, fmt.Sprintf("output size %d exceeds maximum %d", len(output), schema.MaxOutputSize))
	}

	return len(issues) == 0, issues
}

// OptimizeStepOrdering generates scheduling hints for a plan based on cost classes and dependencies.
// Returns steps ordered for optimal execution considering cost and parallelization safety.
func OptimizeStepOrdering(plan *agentgraph.Plan, metadata map[string]OperatorMetadata) []SchedulingHint {
	if plan == nil || len(plan.Steps) == 0 {
		return nil
	}

	hints := make([]SchedulingHint, 0, len(plan.Steps))

	// Group steps by cost class
	fastSteps := make([]agentgraph.PlanStep, 0)
	mediumSteps := make([]agentgraph.PlanStep, 0)
	slowSteps := make([]agentgraph.PlanStep, 0)

	for _, step := range plan.Steps {
		operatorName := step.Tool
		if operatorName == "" {
			operatorName = step.ID
		}

		meta, ok := metadata[operatorName]
		if !ok {
			mediumSteps = append(mediumSteps, step)
			continue
		}

		switch meta.CostClass {
		case authoring.CostClassFast:
			fastSteps = append(fastSteps, step)
		case authoring.CostClassSlow:
			slowSteps = append(slowSteps, step)
		default:
			mediumSteps = append(mediumSteps, step)
		}
	}

	// Generate hints: fast first, then medium, then slow
	// This allows fast steps to complete quickly and free up resources
	order := 0
	costOptimization := "fast_first"

	// Add fast steps first
	for _, step := range fastSteps {
		hints = append(hints, SchedulingHint{
			StepID:            step.ID,
			RecommendedOrder:  order,
			CanParallelize:    true, // Fast steps are good candidates for parallelization
			EstimatedDuration: 1,
			CostOptimization:  costOptimization,
		})
		order++
	}

	// Add medium steps next
	for _, step := range mediumSteps {
		hints = append(hints, SchedulingHint{
			StepID:            step.ID,
			RecommendedOrder:  order,
			CanParallelize:    false, // Medium steps are sequential by default
			EstimatedDuration: 5,
			CostOptimization:  costOptimization,
		})
		order++
	}

	// Add slow steps last
	for _, step := range slowSteps {
		hints = append(hints, SchedulingHint{
			StepID:            step.ID,
			RecommendedOrder:  order,
			CanParallelize:    false, // Slow steps are sequential
			EstimatedDuration: 30,
			CostOptimization:  costOptimization,
		})
		order++
	}

	return hints
}

// SortStepsByOptimization reorders plan steps based on cost class optimization strategy.
// Returns reordered slice of steps without modifying the original plan.
func SortStepsByOptimization(steps []agentgraph.PlanStep, metadata map[string]OperatorMetadata, strategy string) []agentgraph.PlanStep {
	if len(steps) == 0 {
		return steps
	}

	// Create a copy to avoid modifying original
	sorted := make([]agentgraph.PlanStep, len(steps))
	copy(sorted, steps)

	// Sort based on strategy
	switch strategy {
	case "fast_first":
		sort.Slice(sorted, func(i, j int) bool {
			costI := getCostClassForStep(sorted[i], metadata)
			costJ := getCostClassForStep(sorted[j], metadata)
			return costToValue(costI) < costToValue(costJ)
		})

	case "slow_first":
		sort.Slice(sorted, func(i, j int) bool {
			costI := getCostClassForStep(sorted[i], metadata)
			costJ := getCostClassForStep(sorted[j], metadata)
			return costToValue(costI) > costToValue(costJ)
		})

	case "parallel":
		// Already handled by CanParallelize flag in hints
		// Just return as-is since parallelization is handled by executor
	}

	return sorted
}

// Helper to get cost class for a step
func getCostClassForStep(step agentgraph.PlanStep, metadata map[string]OperatorMetadata) authoring.CostClass {
	operatorName := step.Tool
	if operatorName == "" {
		operatorName = step.ID
	}

	if meta, ok := metadata[operatorName]; ok {
		return meta.CostClass
	}
	return authoring.CostClassUnknown
}

// Helper to convert cost class to numeric value for sorting
func costToValue(cost authoring.CostClass) int {
	switch cost {
	case authoring.CostClassFast:
		return 1
	case authoring.CostClassMedium:
		return 2
	case authoring.CostClassSlow:
		return 3
	default:
		return 2 // Unknown defaults to medium
	}
}

// EnrichExecutionContextWithHistoricalData augments execution context with historical metrics.
// This enables informed decisions about timeouts, retries, and resource allocation.
// TODO: Reimplement without WorkflowStateStore dependency
// per the agentlifecycle workflow-store removal plan
func EnrichExecutionContextWithHistoricalData(ctx context.Context, state *contextdata.Envelope, operatorName string, store interface{}) {
	if state == nil || store == nil {
		return
	}
	// Placeholder - historical metrics enrichment to be reimplemented
	// using agentlifecycle.Repository or WorkingMemory
}

// ExtractOutputValidationSchema creates a schema validator from Phase 8 metadata.
func ExtractOutputValidationSchema(expectedOutput map[string]any) *OutputValidationSchema {
	if expectedOutput == nil {
		return nil
	}

	schema := &OutputValidationSchema{
		ExpectedKeys: make([]string, 0),
		OptionalKeys: make([]string, 0),
		Schema:       expectedOutput,
	}

	// Extract expected keys from schema if defined
	if fieldsAny, ok := expectedOutput["fields"]; ok {
		if fields, ok := fieldsAny.([]string); ok {
			schema.ExpectedKeys = fields
		} else if fields, ok := fieldsAny.([]any); ok {
			for _, f := range fields {
				if s, ok := f.(string); ok {
					schema.ExpectedKeys = append(schema.ExpectedKeys, s)
				}
			}
		}
	}

	// Extract size constraints
	if minSizeAny, ok := expectedOutput["min_size"]; ok {
		if size, ok := minSizeAny.(float64); ok {
			schema.MinOutputSize = int(size)
		}
	}
	if maxSizeAny, ok := expectedOutput["max_size"]; ok {
		if size, ok := maxSizeAny.(float64); ok {
			schema.MaxOutputSize = int(size)
		}
	}

	return schema
}

// ComputeEstimatedExecutionTime calculates expected duration for a plan based on historical data.
func ComputeEstimatedExecutionTime(plan *agentgraph.Plan, metadata map[string]OperatorMetadata) int {
	totalTime := 0

	for _, step := range plan.Steps {
		operatorName := step.Tool
		if operatorName == "" {
			operatorName = step.ID
		}

		// Get estimated time based on cost class
		var estimatedDuration int
		if meta, ok := metadata[operatorName]; ok {
			switch meta.CostClass {
			case authoring.CostClassFast:
				estimatedDuration = 1
			case authoring.CostClassMedium:
				estimatedDuration = 5
			case authoring.CostClassSlow:
				estimatedDuration = 30
			default:
				estimatedDuration = 5
			}
		} else {
			estimatedDuration = 5 // Default to medium
		}

		totalTime += estimatedDuration
	}

	return totalTime
}

// ShouldRetryStep determines if a step should be retried based on historical data and retry class.
func ShouldRetryStep(stepID string, retryClass authoring.RetryClass, lastError error, attemptCount int, maxAttempts int) bool {
	// Don't retry if max attempts exceeded
	if attemptCount >= maxAttempts {
		return false
	}

	// Don't retry if retry class is "none"
	if retryClass == authoring.RetryClassNone {
		return false
	}

	// Allow retry for other classes
	switch retryClass {
	case authoring.RetryClassIdempotent:
		// Always safe to retry
		return true
	case authoring.RetryClassStateless:
		// Safe to retry after state reset
		return true
	case authoring.RetryClassProbed:
		// Retry after verification/probe
		return true
	default:
		// For unknown retry class, don't retry
		return false
	}
}

// BuildKnowledgeQuery creates a structured query for retrieving relevant past executions.
type KnowledgeQuery struct {
	MethodName   string
	TaskType     core.TaskType
	OperatorName string
	SuccessOnly  bool
	SinceTime    time.Time
	MaxResults   int
}

// RetrieveRelevantKnowledge queries the knowledge base for relevant past executions.
// This enables learning from similar previous runs.
// TODO: Reimplement without WorkflowStateStore dependency
// per the agentlifecycle workflow-store removal plan
func RetrieveRelevantKnowledge(ctx context.Context, store interface{}, query *KnowledgeQuery) []memory.KnowledgeRecord {
	if store == nil || query == nil {
		return nil
	}
	// Placeholder - knowledge retrieval to be reimplemented
	// using agentlifecycle.Repository or WorkingMemory
	return []memory.KnowledgeRecord{}
}
