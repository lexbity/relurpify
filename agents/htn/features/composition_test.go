package features

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
)

// TestPerformanceMetricsComputation tests metric calculation.
func TestPerformanceMetricsComputation(t *testing.T) {
	metrics := &PerformanceMetrics{
		MethodName:      "analyze",
		TotalExecutions: 100,
		SuccessfulRuns:  95,
		FailedRuns:      5,
		SuccessRate:     0.95,
		TrendDirection:  "improving",
	}

	if metrics.MethodName != "analyze" {
		t.Errorf("Expected method name 'analyze', got %s", metrics.MethodName)
	}
	if metrics.SuccessRate != 0.95 {
		t.Errorf("Expected success rate 0.95, got %f", metrics.SuccessRate)
	}
}

// TestComputeRecommendationRating calculates method recommendation.
func TestComputeRecommendationRating(t *testing.T) {
	metrics := &PerformanceMetrics{
		MethodName:      "analyze",
		TotalExecutions: 100,
		SuccessfulRuns:  95,
		SuccessRate:     0.95,
		TrendDirection:  "improving",
	}

	rating := metrics.ComputeRecommendationRating()
	// Expected: 0.95 (success) + 0.1 (improving trend) + 0.05 (reliability) = 1.0 (capped)
	if rating != 1.0 {
		t.Errorf("Expected rating 1.0, got %f", rating)
	}

	// Test degrading trend
	metrics.TrendDirection = "degrading"
	rating = metrics.ComputeRecommendationRating()
	// Expected: 0.95 - 0.1 = 0.85
	if rating != 0.85 {
		t.Errorf("Expected rating 0.85 with degrading trend, got %f", rating)
	}

	// Test low execution count
	metrics.TotalExecutions = 10
	metrics.TrendDirection = "stable"
	rating = metrics.ComputeRecommendationRating()
	// Expected: 0.95 + 0.0 = 0.95 (no reliability boost at 10 executions)
	if rating < 0.9 || rating > 1.0 {
		t.Errorf("Expected rating between 0.9 and 1.0, got %f", rating)
	}
}

// TestComputeRecommendationRatingNoExecutions handles untested method.
func TestComputeRecommendationRatingNoExecutions(t *testing.T) {
	metrics := &PerformanceMetrics{
		MethodName:      "unknown",
		TotalExecutions: 0,
	}

	rating := metrics.ComputeRecommendationRating()
	if rating != 0.5 {
		t.Errorf("Expected neutral rating 0.5 for untested method, got %f", rating)
	}
}

// TestMethodPerformanceTracker analyzes method performance.
func TestMethodPerformanceTracker(t *testing.T) {
	tracker := &MethodPerformanceTracker{
		Store:      nil,
		WorkflowID: "workflow_1",
	}

	metrics := tracker.AnalyzeMethodPerformance(context.Background(), "analyze")

	if metrics == nil {
		t.Error("Expected non-nil metrics")
	}
	if metrics.MethodName != "analyze" {
		t.Errorf("Expected method name 'analyze', got %s", metrics.MethodName)
	}
}

// TestKnowledgeBasedMethodSelectorSelectBestMethod selects best method.
func TestKnowledgeBasedMethodSelectorSelectBestMethod(t *testing.T) {
	library := NewMethodLibrary()
	library.Add(&Method{
		Name:     "analyze_fast",
		TaskType: core.TaskTypeAnalysis,
		Priority: 1,
	})
	library.Add(&Method{
		Name:     "analyze_slow",
		TaskType: core.TaskTypeAnalysis,
		Priority: 2,
	})

	selector := &KnowledgeBasedMethodSelector{
		Library:                library,
		Store:                  nil,
		WorkflowID:             "workflow_1",
		MinConfidenceThreshold: 0.0,
	}

	task := &core.Task{
		Type: core.TaskTypeAnalysis,
	}

	method, metrics := selector.SelectBestMethod(context.Background(), task)

	if method == nil {
		t.Error("Expected selected method")
	}
	if metrics == nil {
		t.Error("Expected performance metrics")
	}
}

// TestKnowledgeBasedMethodSelectorSelectBestMethodNoMatch handles no candidates.
func TestKnowledgeBasedMethodSelectorSelectBestMethodNoMatch(t *testing.T) {
	library := NewMethodLibrary()
	selector := &KnowledgeBasedMethodSelector{
		Library: library,
	}

	task := &core.Task{
		Type: core.TaskTypeAnalysis,
	}

	method, metrics := selector.SelectBestMethod(context.Background(), task)

	if method != nil {
		t.Error("Expected nil method for no candidates")
	}
	if metrics != nil {
		t.Error("Expected nil metrics for no candidates")
	}
}

// TestKnowledgeBasedMethodSelectorRankByPerformance ranks methods.
func TestKnowledgeBasedMethodSelectorRankByPerformance(t *testing.T) {
	library := NewMethodLibrary()
	method1 := &Method{Name: "m1", TaskType: core.TaskTypeAnalysis}
	method2 := &Method{Name: "m2", TaskType: core.TaskTypeAnalysis}
	method3 := &Method{Name: "m3", TaskType: core.TaskTypeAnalysis}

	candidates := []*Method{method1, method2, method3}

	selector := &KnowledgeBasedMethodSelector{
		Library: library,
	}

	ranked := selector.RankMethodsByPerformance(context.Background(), candidates)

	if len(ranked) != 3 {
		t.Errorf("Expected 3 ranked methods, got %d", len(ranked))
	}
}

// TestKnowledgeBasedMethodSelectorRankEmpty handles empty input.
func TestKnowledgeBasedMethodSelectorRankEmpty(t *testing.T) {
	selector := &KnowledgeBasedMethodSelector{
		Library: NewMethodLibrary(),
	}

	ranked := selector.RankMethodsByPerformance(context.Background(), []*Method{})

	if len(ranked) != 0 {
		t.Errorf("Expected 0 ranked methods, got %d", len(ranked))
	}
}

// TestMethodLibraryBuilder constructs library.
func TestMethodLibraryBuilder(t *testing.T) {
	builder := NewMethodLibraryBuilder(nil)

	if builder == nil {
		t.Error("Expected non-nil builder")
	}

	method := &Method{
		Name:     "test",
		TaskType: core.TaskTypeAnalysis,
		Priority: 1,
	}

	built := builder.AddMethod(method).Build()

	if built == nil {
		t.Error("Expected non-nil built library")
	}
	if len(built.FindAll(&core.Task{Type: core.TaskTypeAnalysis})) != 1 {
		t.Error("Expected 1 method in library")
	}
}

// TestMethodLibraryBuilderFluent tests fluent API.
func TestMethodLibraryBuilderFluent(t *testing.T) {
	builder := NewMethodLibraryBuilder(nil)

	library := builder.
		AddMethod(&Method{
			Name:     "step1",
			TaskType: core.TaskTypeAnalysis,
		}).
		AddMethod(&Method{
			Name:     "step2",
			TaskType: core.TaskTypePlanning,
		}).
		Build()

	if len(library.methods) != 2 {
		t.Errorf("Expected 2 methods, got %d", len(library.methods))
	}
}

// TestMethodLibraryBuilderComposeMethod adds composed method.
func TestMethodLibraryBuilderComposeMethod(t *testing.T) {
	library := NewMethodLibrary()
	builder := NewMethodLibraryBuilder(library)

	composed := &ComposedMethod{
		Name:     "composite",
		TaskType: core.TaskTypeAnalysis,
		Priority: 1,
		Components: []CompositionComponent{
			{
				Name: "step1",
				Primitive: &SubtaskSpec{
					Name:        "step1",
					Type:        core.TaskTypeAnalysis,
					Executor:    "test",
					Instruction: "Do step 1",
				},
			},
		},
	}

	built := builder.ComposeMethod(composed).Build()

	method := built.FindByName("composite")
	if method == nil {
		t.Error("Expected composed method in library")
	}
	if len(method.Subtasks) != 1 {
		t.Errorf("Expected 1 subtask, got %d", len(method.Subtasks))
	}
}

// TestRecursiveDecompositionContext tracks recursion.
func TestRecursiveDecompositionContext(t *testing.T) {
	ctx := &RecursiveDecompositionContext{
		MaxDepth: 5,
		Visited:  make(map[string]bool),
	}

	if !ctx.CanDecomposeFurther("method1") {
		t.Error("Should allow first decomposition")
	}

	ctx.PushDecomposition("method1")

	if ctx.CurrentDepth != 1 {
		t.Errorf("Expected depth 1, got %d", ctx.CurrentDepth)
	}

	if ctx.Visited["method1"] != true {
		t.Error("Expected method1 marked visited")
	}
}

// TestRecursiveDecompositionContextMaxDepth respects limit.
func TestRecursiveDecompositionContextMaxDepth(t *testing.T) {
	ctx := &RecursiveDecompositionContext{
		MaxDepth:     2,
		CurrentDepth: 2,
		Visited:      make(map[string]bool),
	}

	if ctx.CanDecomposeFurther("method1") {
		t.Error("Should not allow decomposition at max depth")
	}
}

// TestRecursiveDecompositionContextCycles detects cycles.
func TestRecursiveDecompositionContextCycles(t *testing.T) {
	ctx := &RecursiveDecompositionContext{
		MaxDepth: 5,
		Visited: map[string]bool{
			"method1": true,
		},
	}

	if ctx.CanDecomposeFurther("method1") {
		t.Error("Should detect cycle")
	}
}

// TestRecursiveDecompositionContextPath tracks path.
func TestRecursiveDecompositionContextPath(t *testing.T) {
	ctx := &RecursiveDecompositionContext{
		MaxDepth: 10,
		Visited:  make(map[string]bool),
	}

	ctx.PushDecomposition("methodA")
	ctx.PushDecomposition("methodB")
	ctx.PushDecomposition("methodC")

	path := ctx.GetDecompositionPath()
	if path != "methodA → methodB → methodC" {
		t.Errorf("Expected path 'methodA → methodB → methodC', got %s", path)
	}

	ctx.PopDecomposition()
	if len(ctx.PathStack) != 2 {
		t.Errorf("Expected path length 2 after pop, got %d", len(ctx.PathStack))
	}
}

// TestMethodCompositionAnalyzerAnalyzeDepth analyzes depth.
func TestMethodCompositionAnalyzerAnalyzeDepth(t *testing.T) {
	library := NewMethodLibrary()
	library.Add(&Method{
		Name:     "simple",
		TaskType: core.TaskTypeAnalysis,
	})

	analyzer := &MethodCompositionAnalyzer{
		Library: library,
	}

	depth := analyzer.AnalyzeCompositionDepth("simple")
	if depth != 1 {
		t.Errorf("Expected depth 1, got %d", depth)
	}

	depth = analyzer.AnalyzeCompositionDepth("unknown")
	if depth != 0 {
		t.Errorf("Expected depth 0 for unknown method, got %d", depth)
	}
}

// TestMethodCompositionAnalyzerFindCycles detects cycles.
func TestMethodCompositionAnalyzerFindCycles(t *testing.T) {
	library := NewMethodLibrary()
	analyzer := &MethodCompositionAnalyzer{
		Library: library,
	}

	cycles := analyzer.FindCompositionCycles()
	if len(cycles) != 0 {
		t.Errorf("Expected no cycles in empty library, got %d", len(cycles))
	}
}

// TestMethodCompositionAnalyzerAnalyzeComposition analyzes composition.
func TestMethodCompositionAnalyzerAnalyzeComposition(t *testing.T) {
	library := NewMethodLibrary()
	library.Add(&Method{
		Name:     "complex",
		TaskType: core.TaskTypeAnalysis,
		Subtasks: []SubtaskSpec{
			{Name: "s1", Type: core.TaskTypeAnalysis},
			{Name: "s2", Type: core.TaskTypeAnalysis},
			{Name: "s3", Type: core.TaskTypeAnalysis},
		},
	})

	analyzer := &MethodCompositionAnalyzer{
		Library: library,
	}

	metrics := analyzer.AnalyzeComposition("complex")

	if metrics == nil {
		t.Error("Expected metrics")
	}
	if metrics.ComponentCount != 3 {
		t.Errorf("Expected 3 components, got %d", metrics.ComponentCount)
	}
	if metrics.EstimatedComplexity != "complex" {
		t.Errorf("Expected 'complex' complexity, got %s", metrics.EstimatedComplexity)
	}
}

// TestMethodCompositionAnalyzerAnalyzeCompositionSimple analyzes simple method.
func TestMethodCompositionAnalyzerAnalyzeCompositionSimple(t *testing.T) {
	library := NewMethodLibrary()
	library.Add(&Method{
		Name:     "simple",
		TaskType: core.TaskTypeAnalysis,
		Subtasks: []SubtaskSpec{},
	})

	analyzer := &MethodCompositionAnalyzer{
		Library: library,
	}

	metrics := analyzer.AnalyzeComposition("simple")

	if metrics.EstimatedComplexity != "simple" {
		t.Errorf("Expected 'simple' complexity, got %s", metrics.EstimatedComplexity)
	}
}

// TestMethodCompositionAnalyzerAnalyzeCompositionModerate analyzes moderate method.
func TestMethodCompositionAnalyzerAnalyzeCompositionModerate(t *testing.T) {
	library := NewMethodLibrary()
	library.Add(&Method{
		Name:     "moderate",
		TaskType: core.TaskTypeAnalysis,
		Subtasks: []SubtaskSpec{
			{Name: "s1", Type: core.TaskTypeAnalysis},
			{Name: "s2", Type: core.TaskTypeAnalysis},
		},
	})

	analyzer := &MethodCompositionAnalyzer{
		Library: library,
	}

	metrics := analyzer.AnalyzeComposition("moderate")

	if metrics.EstimatedComplexity != "moderate" {
		t.Errorf("Expected 'moderate' complexity, got %s", metrics.EstimatedComplexity)
	}
}

// TestMethodLibrarySnapshot captures state.
func TestMethodLibrarySnapshot(t *testing.T) {
	library := NewMethodLibrary()
	library.Add(&Method{
		Name:     "m1",
		TaskType: core.TaskTypeAnalysis,
		Priority: 1,
	})
	library.Add(&Method{
		Name:     "m2",
		TaskType: core.TaskTypePlanning,
		Priority: 2,
	})

	analyzer := &MethodCompositionAnalyzer{
		Library: library,
	}

	snapshot := analyzer.CaptureSnapshot()

	if snapshot == nil {
		t.Error("Expected non-nil snapshot")
	}
	if snapshot.MethodCount != 2 {
		t.Errorf("Expected 2 methods in snapshot, got %d", snapshot.MethodCount)
	}
	if len(snapshot.TaskTypes) != 2 {
		t.Errorf("Expected 2 task types, got %d", len(snapshot.TaskTypes))
	}

	expectedAverage := float64(1+2) / 2.0
	if snapshot.AveragePriority != expectedAverage {
		t.Errorf("Expected average priority %f, got %f", expectedAverage, snapshot.AveragePriority)
	}
}

// TestMethodLibrarySnapshotEmpty handles empty library.
func TestMethodLibrarySnapshotEmpty(t *testing.T) {
	library := NewMethodLibrary()
	analyzer := &MethodCompositionAnalyzer{
		Library: library,
	}

	snapshot := analyzer.CaptureSnapshot()

	if snapshot == nil {
		t.Error("Expected non-nil snapshot")
	}
	if snapshot.MethodCount != 0 {
		t.Errorf("Expected 0 methods, got %d", snapshot.MethodCount)
	}
}

// TestCompositionPrimitive validates structure.
func TestCompositionPrimitive(t *testing.T) {
	library := NewMethodLibrary()
	primitive := &CompositionPrimitive{
		Name:              "composed",
		Library:           library,
		AllowRecursion:    true,
		MaxRecursionDepth: 10,
	}

	if primitive.Name != "composed" {
		t.Errorf("Expected name 'composed', got %s", primitive.Name)
	}
	if primitive.Library == nil {
		t.Error("Expected non-nil library")
	}
	if primitive.MaxRecursionDepth != 10 {
		t.Errorf("Expected depth 10, got %d", primitive.MaxRecursionDepth)
	}
}

// TestCompositionComponent validates structure.
func TestCompositionComponent(t *testing.T) {
	component := &CompositionComponent{
		Name:            "step1",
		IsComposed:      false,
		Parallelizable:  true,
		Dependencies:    []string{"step0"},
		Metadata:        make(map[string]any),
	}

	if component.Name != "step1" {
		t.Errorf("Expected name 'step1', got %s", component.Name)
	}
	if !component.Parallelizable {
		t.Error("Expected parallelizable")
	}
	if len(component.Dependencies) != 1 {
		t.Errorf("Expected 1 dependency, got %d", len(component.Dependencies))
	}
}

// TestCompositionMetrics validates structure.
func TestCompositionMetrics(t *testing.T) {
	metrics := &CompositionMetrics{
		MethodName:          "composite",
		IsComposed:          true,
		CompositionDepth:    2,
		ComponentCount:      5,
		ParallelizableCount: 3,
		SequentialCount:     2,
		EstimatedComplexity: "complex",
	}

	if metrics.MethodName != "composite" {
		t.Errorf("Expected name 'composite', got %s", metrics.MethodName)
	}
	if !metrics.IsComposed {
		t.Error("Expected is_composed true")
	}
	if metrics.ComponentCount != 5 {
		t.Errorf("Expected 5 components, got %d", metrics.ComponentCount)
	}
}

// TestComposableSubtaskSpec validates structure.
func TestComposableSubtaskSpec(t *testing.T) {
	spec := &ComposableSubtaskSpec{
		Subtask: SubtaskSpec{
			Name:     "step1",
			Type:     core.TaskTypeAnalysis,
			Executor: "htn",
		},
		IsComposition: true,
		ComposedName:  "composite_method",
	}

	if spec.Subtask.Name != "step1" {
		t.Errorf("Expected name 'step1', got %s", spec.Subtask.Name)
	}
	if !spec.IsComposition {
		t.Error("Expected is_composition true")
	}
	if spec.ComposedName != "composite_method" {
		t.Errorf("Expected composed name 'composite_method', got %s", spec.ComposedName)
	}
}

// TestMethodComposer orchestrates composition.
func TestMethodComposer(t *testing.T) {
	composer := &MethodComposer{
		Library:    NewMethodLibrary(),
		Store:      nil,
		WorkflowID: "workflow_1",
	}

	if composer.Library == nil {
		t.Error("Expected non-nil library")
	}
	if composer.WorkflowID != "workflow_1" {
		t.Errorf("Expected workflow_1, got %s", composer.WorkflowID)
	}
}

// TestComposedMethod validates structure.
func TestComposedMethod(t *testing.T) {
	composed := &ComposedMethod{
		Name:     "multi_step",
		TaskType: core.TaskTypeAnalysis,
		Priority: 5,
		Components: []CompositionComponent{
			{Name: "s1"},
			{Name: "s2"},
		},
		Metadata: make(map[string]any),
	}

	if composed.Name != "multi_step" {
		t.Errorf("Expected name 'multi_step', got %s", composed.Name)
	}
	if composed.Priority != 5 {
		t.Errorf("Expected priority 5, got %d", composed.Priority)
	}
	if len(composed.Components) != 2 {
		t.Errorf("Expected 2 components, got %d", len(composed.Components))
	}
}
