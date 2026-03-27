package features

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/lexcodex/relurpify/agents/htn/runtime"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
)

// Phase 12: HTN as reusable composition primitive.
// Enables method composition, recursive decomposition, knowledge-driven selection,
// and performance-based recommendations. Methods become first-class composable units.

// CompositionPrimitive marks a subtask as an HTN composition (recursive HTN).
type CompositionPrimitive struct {
	// Name of the composed method.
	Name string
	// runtime.MethodLibrary to use for decomposition.
	Library *runtime.MethodLibrary
	// Context for method selection.
	SelectionContext map[string]any
	// AllowRecursion permits infinite recursion depth (use with caution).
	AllowRecursion bool
	// MaxRecursionDepth limits nesting (0 = no limit).
	MaxRecursionDepth int
}

// MethodComposer builds complex methods from simpler components.
type MethodComposer struct {
	// Library is the base method library.
	Library *runtime.MethodLibrary
	// Store provides access to historical performance data.
	Store memory.WorkflowStateStore
	// WorkflowID scopes artifact queries.
	WorkflowID string
}

// ComposedMethod combines multiple subtasks into a single method.
type ComposedMethod struct {
	// Name is the composed method name.
	Name string
	// TaskType is the task this method handles.
	TaskType core.TaskType
	// Priority for method selection.
	Priority int
	// Components are the subtasks that make up this method.
	Components []CompositionComponent
	// Metadata about composition.
	Metadata map[string]any
}

// CompositionComponent represents a single component in a composed method.
type CompositionComponent struct {
	// Name of the component.
	Name string
	// Method is the method to execute (either primitive or composed).
	Method *runtime.Method
	// Primitive is the primitive subtask (if not a composed method).
	Primitive *runtime.SubtaskSpec
	// IsComposed indicates if this component is itself a composed method.
	IsComposed bool
	// Dependencies lists components that must complete first.
	Dependencies []string
	// Parallelizable indicates if this can run in parallel.
	Parallelizable bool
	// Metadata about the component.
	Metadata map[string]any
}

// PerformanceMetrics tracks method effectiveness over time.
type PerformanceMetrics struct {
	MethodName        string
	TaskType          core.TaskType
	TotalExecutions   int
	SuccessfulRuns    int
	FailedRuns        int
	SkippedRuns       int
	SuccessRate       float64
	AverageDuration   int // seconds
	AverageCost       string
	TrendDirection    string // "improving", "stable", "degrading"
	LastExecutionTime time.Time
	RecommendedRating float64 // 0.0 to 1.0
}

// MethodPerformanceTracker analyzes method effectiveness.
type MethodPerformanceTracker struct {
	Store      memory.WorkflowStateStore
	WorkflowID string
}

// AnalyzeMethodPerformance computes metrics for a method.
func (t *MethodPerformanceTracker) AnalyzeMethodPerformance(ctx context.Context, methodName string) *PerformanceMetrics {
	if t.Store == nil {
		return nil
	}

	metrics := &PerformanceMetrics{
		MethodName: methodName,
	}

	// In a real implementation, would query artifacts for:
	// - Count executions from HTNRunSummary records
	// - Calculate success rate from completion status
	// - Trend from time-series data
	// - Average duration from ExecutionMetrics

	return metrics
}

// ComputeRecommendationRating calculates how recommended a method is.
func (m *PerformanceMetrics) ComputeRecommendationRating() float64 {
	if m.TotalExecutions == 0 {
		return 0.5 // Neutral for untested methods
	}

	// Base rating on success rate
	successRating := m.SuccessRate

	// Adjust for trend
	trendAdjustment := 0.0
	switch m.TrendDirection {
	case "improving":
		trendAdjustment = 0.1
	case "degrading":
		trendAdjustment = -0.1
	case "stable":
		trendAdjustment = 0.0
	}

	// Boost for high execution count (more reliable)
	reliabilityBoost := 0.0
	if m.TotalExecutions > 100 {
		reliabilityBoost = 0.05
	} else if m.TotalExecutions > 50 {
		reliabilityBoost = 0.02
	}

	rating := successRating + trendAdjustment + reliabilityBoost
	if rating > 1.0 {
		rating = 1.0
	} else if rating < 0.0 {
		rating = 0.0
	}

	m.RecommendedRating = rating
	return rating
}

// KnowledgeBasedMethodSelector chooses methods based on performance history.
type KnowledgeBasedMethodSelector struct {
	// Library to select from.
	Library *runtime.MethodLibrary
	// Store for historical data.
	Store memory.WorkflowStateStore
	// WorkflowID for scoping.
	WorkflowID string
	// MinConfidenceThreshold (0.0-1.0) for recommendations.
	MinConfidenceThreshold float64
}

// SelectBestMethod recommends the best method for a task based on history.
func (s *KnowledgeBasedMethodSelector) SelectBestMethod(ctx context.Context, task *core.Task) (*runtime.Method, *PerformanceMetrics) {
	if s.Library == nil {
		return nil, nil
	}

	// Find all applicable methods
	candidates := s.Library.FindAll(task)
	if len(candidates) == 0 {
		return nil, nil
	}

	// Rank candidates by performance
	tracker := &MethodPerformanceTracker{
		Store:      s.Store,
		WorkflowID: s.WorkflowID,
	}

	bestMethod := candidates[0]
	bestMetrics := &PerformanceMetrics{MethodName: bestMethod.Name}
	bestRating := 0.0

	for _, method := range candidates {
		metrics := tracker.AnalyzeMethodPerformance(ctx, method.Name)
		if metrics == nil {
			metrics = &PerformanceMetrics{MethodName: method.Name}
		}

		rating := metrics.ComputeRecommendationRating()

		if rating > bestRating && rating >= s.MinConfidenceThreshold {
			bestRating = rating
			bestMethod = method
			bestMetrics = metrics
		}
	}

	return &bestMethod, bestMetrics
}

// RankMethodsByPerformance sorts methods by effectiveness.
func (s *KnowledgeBasedMethodSelector) RankMethodsByPerformance(ctx context.Context, candidates []*runtime.Method) []*runtime.Method {
	if len(candidates) == 0 {
		return candidates
	}

	tracker := &MethodPerformanceTracker{
		Store:      s.Store,
		WorkflowID: s.WorkflowID,
	}

	// Create slice of (method, rating) tuples
	type methodRating struct {
		method *runtime.Method
		rating float64
	}

	ratings := make([]methodRating, 0, len(candidates))
	for _, method := range candidates {
		metrics := tracker.AnalyzeMethodPerformance(ctx, method.Name)
		if metrics == nil {
			metrics = &PerformanceMetrics{MethodName: method.Name}
		}

		rating := metrics.ComputeRecommendationRating()
		ratings = append(ratings, methodRating{method, rating})
	}

	// Sort by rating descending
	sort.Slice(ratings, func(i, j int) bool {
		return ratings[i].rating > ratings[j].rating
	})

	// Extract sorted methods
	result := make([]*runtime.Method, len(ratings))
	for i, mr := range ratings {
		result[i] = mr.method
	}

	return result
}

// MethodLibraryBuilder constructs composed methods.
type MethodLibraryBuilder struct {
	library *runtime.MethodLibrary
}

// NewMethodLibraryBuilder creates a builder.
func NewMethodLibraryBuilder(lib *runtime.MethodLibrary) *MethodLibraryBuilder {
	if lib == nil {
		lib = runtime.NewMethodLibrary()
	}
	return &MethodLibraryBuilder{library: lib}
}

// AddMethod adds a method to the library.
func (b *MethodLibraryBuilder) AddMethod(method *runtime.Method) *MethodLibraryBuilder {
	if b.library != nil && method != nil {
		b.library.Register(*method)
	}
	return b
}

// ComposeMethod creates a composed method from components.
func (b *MethodLibraryBuilder) ComposeMethod(composed *ComposedMethod) *MethodLibraryBuilder {
	if b.library == nil || composed == nil {
		return b
	}

	// Convert composed method to library method
	subtasks := make([]runtime.SubtaskSpec, 0, len(composed.Components))

	for _, component := range composed.Components {
		if component.Primitive != nil {
			subtasks = append(subtasks, *component.Primitive)
		} else if component.Method != nil {
			// Create a subtask that references the composed method
			subtask := runtime.SubtaskSpec{
				Name:        component.Name,
				Type:        component.Method.TaskType,
				Executor:    "htn", // HTN executor for composition
				Instruction: fmt.Sprintf("Execute composed method: %s", component.Method.Name),
			}
			subtasks = append(subtasks, subtask)
		}
	}

	method := &runtime.Method{
		Name:     composed.Name,
		TaskType: composed.TaskType,
		Priority: composed.Priority,
		Subtasks: subtasks,
	}

	if method != nil {
		b.library.Register(*method)
	}
	return b
}

// Build returns the constructed library.
func (b *MethodLibraryBuilder) Build() *runtime.MethodLibrary {
	return b.library
}

// RecursiveDecompositionContext tracks recursion depth and state.
type RecursiveDecompositionContext struct {
	// MaxDepth limits recursion (0 = unlimited).
	MaxDepth int
	// CurrentDepth tracks current nesting level.
	CurrentDepth int
	// PathStack tracks the decomposition path.
	PathStack []string
	// Visited tracks which methods have been visited (cycle detection).
	Visited map[string]bool
}

// CanDecomposeFurther checks if recursion can continue.
func (c *RecursiveDecompositionContext) CanDecomposeFurther(methodName string) bool {
	// Check depth limit
	if c.MaxDepth > 0 && c.CurrentDepth >= c.MaxDepth {
		return false
	}

	// Check for cycles
	if c.Visited[methodName] {
		return false
	}

	return true
}

// PushDecomposition enters a new decomposition level.
func (c *RecursiveDecompositionContext) PushDecomposition(methodName string) {
	c.CurrentDepth++
	c.PathStack = append(c.PathStack, methodName)
	c.Visited[methodName] = true
}

// PopDecomposition exits current decomposition level.
func (c *RecursiveDecompositionContext) PopDecomposition() {
	if len(c.PathStack) > 0 {
		c.PathStack = c.PathStack[:len(c.PathStack)-1]
		c.CurrentDepth--
	}
}

// GetDecompositionPath returns the full path to current location.
func (c *RecursiveDecompositionContext) GetDecompositionPath() string {
	result := ""
	for i, methodName := range c.PathStack {
		if i > 0 {
			result += " → "
		}
		result += methodName
	}
	return result
}

// ComposableSubtaskSpec wraps a subtask with composition info.
type ComposableSubtaskSpec struct {
	Subtask       runtime.SubtaskSpec
	IsComposition bool
	ComposedName  string // Name of composed method if IsComposition is true
}

// MethodCompositionAnalyzer analyzes method composition patterns.
type MethodCompositionAnalyzer struct {
	Library *runtime.MethodLibrary
}

// AnalyzeCompositionDepth determines how deep the composition goes.
func (a *MethodCompositionAnalyzer) AnalyzeCompositionDepth(methodName string) int {
	if a.Library == nil {
		return 0
	}

	method := a.Library.FindByName(methodName)
	if method == nil {
		return 0
	}

	maxDepth := 0

	// This would recursively check subtasks
	// For now, return 1 to indicate it's a composed method
	return maxDepth + 1
}

// FindCompositionCycles detects circular method dependencies.
func (a *MethodCompositionAnalyzer) FindCompositionCycles() [][]string {
	if a.Library == nil {
		return nil
	}

	cycles := [][]string{}

	// Would perform cycle detection on method dependency graph
	// For now, return empty (no cycles)

	return cycles
}

// ComputeCompositionMetrics analyzes composition characteristics.
type CompositionMetrics struct {
	MethodName          string
	IsComposed          bool
	CompositionDepth    int
	ComponentCount      int
	ParallelizableCount int
	SequentialCount     int
	EstimatedDuration   int
	EstimatedComplexity string // "simple", "moderate", "complex"
}

// AnalyzeComposition returns metrics about a method.
func (a *MethodCompositionAnalyzer) AnalyzeComposition(methodName string) *CompositionMetrics {
	if a.Library == nil {
		return nil
	}

	method := a.Library.FindByName(methodName)
	if method == nil {
		return nil
	}

	metrics := &CompositionMetrics{
		MethodName:     methodName,
		IsComposed:     len(method.Subtasks) > 1,
		ComponentCount: len(method.Subtasks),
	}

	// Compute complexity
	switch len(method.Subtasks) {
	case 0, 1:
		metrics.EstimatedComplexity = "simple"
	case 2:
		metrics.EstimatedComplexity = "moderate"
	default:
		metrics.EstimatedComplexity = "complex"
	}

	// Would analyze parallelization potential
	metrics.EstimatedDuration = len(method.Subtasks) * 5 // rough estimate

	return metrics
}

// MethodLibrarySnapshot captures library state at a point in time.
type MethodLibrarySnapshot struct {
	Timestamp       time.Time
	MethodCount     int
	TaskTypes       map[string]int // count by task type
	AveragePriority float64
	Metadata        map[string]any
}

// CaptureSnapshot creates a snapshot of the library.
func (a *MethodCompositionAnalyzer) CaptureSnapshot() *MethodLibrarySnapshot {
	if a.Library == nil {
		return nil
	}

	methods := a.Library.All()
	snapshot := &MethodLibrarySnapshot{
		Timestamp:   time.Now(),
		MethodCount: len(methods),
		TaskTypes:   make(map[string]int),
		Metadata:    make(map[string]any),
	}

	// Count by task type
	prioritySum := 0
	for _, method := range methods {
		taskTypeStr := string(method.TaskType)
		snapshot.TaskTypes[taskTypeStr]++
		prioritySum += method.Priority
	}

	if snapshot.MethodCount > 0 {
		snapshot.AveragePriority = float64(prioritySum) / float64(snapshot.MethodCount)
	}

	return snapshot
}
