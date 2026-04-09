package features

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/agents/htn/authoring"
	"github.com/lexcodex/relurpify/agents/htn/runtime"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/memory/db"
	"github.com/stretchr/testify/require"
)

func htnTestStore(t *testing.T) memory.WorkflowStateStore {
	t.Helper()
	// The store is only used as a non-nil sentinel in these tests.
	return (*db.SQLiteWorkflowStateStore)(nil)
}

func htnTestMethodLibrary(methods ...runtime.Method) *runtime.MethodLibrary {
	lib := &runtime.MethodLibrary{}
	for _, method := range methods {
		lib.Register(method)
	}
	return lib
}

func TestMethodPerformanceTrackingAndRating(t *testing.T) {
	tracker := &MethodPerformanceTracker{}
	require.Nil(t, tracker.AnalyzeMethodPerformance(context.Background(), "method"))

	tracker.Store = htnTestStore(t)
	metrics := tracker.AnalyzeMethodPerformance(context.Background(), "method")
	require.NotNil(t, metrics)
	require.Equal(t, "method", metrics.MethodName)

	cases := []struct {
		name  string
		metrics *PerformanceMetrics
		want  float64
	}{
		{name: "untested", metrics: &PerformanceMetrics{}, want: 0.5},
		{name: "improving boosted", metrics: &PerformanceMetrics{TotalExecutions: 101, SuccessRate: 0.96, TrendDirection: "improving"}, want: 1.0},
		{name: "stable medium boost", metrics: &PerformanceMetrics{TotalExecutions: 51, SuccessRate: 0.4, TrendDirection: "stable"}, want: 0.42},
		{name: "degrading clamped low", metrics: &PerformanceMetrics{TotalExecutions: 2, SuccessRate: 0.02, TrendDirection: "degrading"}, want: 0.0},
		{name: "plain", metrics: &PerformanceMetrics{TotalExecutions: 10, SuccessRate: 0.75, TrendDirection: "unknown"}, want: 0.75},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.metrics.ComputeRecommendationRating()
			require.InDelta(t, tc.want, got, 1e-9)
			if tc.metrics.TotalExecutions == 0 {
				require.Zero(t, tc.metrics.RecommendedRating)
			} else {
				require.Equal(t, got, tc.metrics.RecommendedRating)
			}
		})
	}
}

func TestKnowledgeBasedMethodSelector(t *testing.T) {
	lib := htnTestMethodLibrary(
		runtime.Method{Name: "alpha", TaskType: core.TaskTypeCodeGeneration, Priority: 1},
		runtime.Method{Name: "beta", TaskType: core.TaskTypeCodeGeneration, Priority: 2},
		runtime.Method{Name: "review", TaskType: core.TaskTypeReview, Priority: 1},
	)

	selector := &KnowledgeBasedMethodSelector{}
	method, metrics := selector.SelectBestMethod(context.Background(), &core.Task{Type: core.TaskTypeCodeGeneration})
	require.Nil(t, method)
	require.Nil(t, metrics)

	selector.Library = lib
	selector.MinConfidenceThreshold = 0.6
	method, metrics = selector.SelectBestMethod(context.Background(), &core.Task{Type: core.TaskTypeCodeGeneration})
	require.NotNil(t, method)
	require.Equal(t, "beta", method.Name)
	require.NotNil(t, metrics)
	require.Equal(t, "beta", metrics.MethodName)

	ranked := selector.RankMethodsByPerformance(context.Background(), []*runtime.Method{
		{Name: "first", TaskType: core.TaskTypeCodeGeneration},
		{Name: "second", TaskType: core.TaskTypeCodeGeneration},
	})
	require.Len(t, ranked, 2)
	require.Equal(t, "first", ranked[0].Name)
	require.Equal(t, "second", ranked[1].Name)

	require.Nil(t, selector.RankMethodsByPerformance(context.Background(), nil))

	emptySelector := &KnowledgeBasedMethodSelector{Library: &runtime.MethodLibrary{}}
	method, metrics = emptySelector.SelectBestMethod(context.Background(), &core.Task{Type: core.TaskTypeCodeGeneration})
	require.Nil(t, method)
	require.Nil(t, metrics)
}

func TestMethodLibraryBuilderAndComposition(t *testing.T) {
	builder := NewMethodLibraryBuilder(nil)
	require.NotNil(t, builder)

	builder.AddMethod(nil)
	builder.AddMethod(&runtime.Method{Name: "base", TaskType: core.TaskTypeAnalysis, Priority: 1})
	compiled := builder.Build()
	require.NotNil(t, compiled)
	require.NotNil(t, compiled.FindByName("base"))

	componentMethod := runtime.Method{Name: "component", TaskType: core.TaskTypePlanning}
	builder.ComposeMethod(&ComposedMethod{
		Name:     "composed",
		TaskType: core.TaskTypeCodeGeneration,
		Priority: 7,
		Components: []CompositionComponent{
			{Primitive: &runtime.SubtaskSpec{Name: "primitive", Type: core.TaskTypeAnalysis}},
			{Method: &componentMethod, Name: "delegated"},
			{},
		},
	})
	composed := builder.Build().FindByName("composed")
	require.NotNil(t, composed)
	require.Equal(t, 2, len(composed.Subtasks))
	require.Equal(t, "primitive", composed.Subtasks[0].Name)
	require.Equal(t, "delegated", composed.Subtasks[1].Name)
	require.Equal(t, "htn", composed.Subtasks[1].Executor)

	same := builder.ComposeMethod(nil)
	require.Same(t, builder, same)
	require.Same(t, builder.library, builder.Build())
}

func TestRecursiveDecompositionContext(t *testing.T) {
	ctx := &RecursiveDecompositionContext{
		MaxDepth: 2,
		Visited:   map[string]bool{},
	}

	require.True(t, ctx.CanDecomposeFurther("alpha"))
	ctx.PushDecomposition("alpha")
	require.Equal(t, 1, ctx.CurrentDepth)
	require.Equal(t, []string{"alpha"}, ctx.PathStack)
	require.Equal(t, "alpha", ctx.GetDecompositionPath())
	require.False(t, ctx.CanDecomposeFurther("alpha"))
	require.True(t, ctx.CanDecomposeFurther("beta"))

	ctx.PushDecomposition("beta")
	require.False(t, ctx.CanDecomposeFurther("gamma"))
	ctx.PopDecomposition()
	require.Equal(t, 1, ctx.CurrentDepth)
	ctx.PopDecomposition()
	require.Equal(t, 0, ctx.CurrentDepth)
	ctx.PopDecomposition()
	require.Equal(t, 0, ctx.CurrentDepth)
	require.Equal(t, "", (&RecursiveDecompositionContext{}).GetDecompositionPath())
}

func TestMethodCompositionAnalyzer(t *testing.T) {
	require.Zero(t, (&MethodCompositionAnalyzer{}).AnalyzeCompositionDepth("missing"))
	require.Nil(t, (&MethodCompositionAnalyzer{}).FindCompositionCycles())
	require.Nil(t, (&MethodCompositionAnalyzer{}).AnalyzeComposition("missing"))
	require.Nil(t, (&MethodCompositionAnalyzer{}).CaptureSnapshot())

	lib := htnTestMethodLibrary(
		runtime.Method{Name: "single", TaskType: core.TaskTypeAnalysis, Priority: 1, Subtasks: []runtime.SubtaskSpec{{Name: "one"}}},
		runtime.Method{Name: "pair", TaskType: core.TaskTypePlanning, Priority: 2, Subtasks: []runtime.SubtaskSpec{{Name: "one"}, {Name: "two"}}},
		runtime.Method{Name: "triple", TaskType: core.TaskTypeCodeGeneration, Priority: 3, Subtasks: []runtime.SubtaskSpec{{Name: "one"}, {Name: "two"}, {Name: "three"}}},
	)
	analyzer := &MethodCompositionAnalyzer{Library: lib}

	require.Equal(t, 1, analyzer.AnalyzeCompositionDepth("single"))
	require.Equal(t, 0, analyzer.AnalyzeCompositionDepth("absent"))
	require.Empty(t, analyzer.FindCompositionCycles())

	simple := analyzer.AnalyzeComposition("single")
	require.NotNil(t, simple)
	require.False(t, simple.IsComposed)
	require.Equal(t, "simple", simple.EstimatedComplexity)
	require.Equal(t, 5, simple.EstimatedDuration)

	moderate := analyzer.AnalyzeComposition("pair")
	require.NotNil(t, moderate)
	require.True(t, moderate.IsComposed)
	require.Equal(t, "moderate", moderate.EstimatedComplexity)
	require.Equal(t, 10, moderate.EstimatedDuration)

	complex := analyzer.AnalyzeComposition("triple")
	require.NotNil(t, complex)
	require.Equal(t, "complex", complex.EstimatedComplexity)
	require.Equal(t, 15, complex.EstimatedDuration)

	snapshot := analyzer.CaptureSnapshot()
	require.NotNil(t, snapshot)
	require.Equal(t, 3, snapshot.MethodCount)
	require.Equal(t, 2.0, snapshot.AveragePriority)
	require.Equal(t, 1, snapshot.TaskTypes[string(core.TaskTypeAnalysis)])
	require.Equal(t, 1, snapshot.TaskTypes[string(core.TaskTypePlanning)])
	require.Equal(t, 1, snapshot.TaskTypes[string(core.TaskTypeCodeGeneration)])
}

func TestAutomationHelpers(t *testing.T) {
	store := htnTestStore(t)
	nilSnapshot, nilErr := (&MetricsAggregator{}).AggregateHistoricalMetrics(context.Background(), "operator")
	require.NoError(t, nilErr)
	require.Nil(t, nilSnapshot)
	snapshot, err := (&MetricsAggregator{Store: store}).AggregateHistoricalMetrics(context.Background(), "operator")
	require.NoError(t, err)
	require.NotNil(t, snapshot)
	require.Equal(t, "operator", snapshot.OperatorName)
	require.Equal(t, 999999, snapshot.MinDuration)

	state := core.NewContext()
	EnrichExecutionContextWithHistoricalData(context.Background(), state, "operator", store)
	EnrichExecutionContextWithHistoricalData(context.Background(), nil, "operator", store)
	EnrichExecutionContextWithHistoricalData(context.Background(), state, "operator", nil)
	value, ok := state.Get("htn.operator_history.operator")
	require.True(t, ok)
	require.NotNil(t, value)

	ok, issues := ValidateOutputAgainstSchema(nil, &OutputValidationSchema{ExpectedKeys: []string{"a"}})
	require.False(t, ok)
	require.NotEmpty(t, issues)

	ok, issues = ValidateOutputAgainstSchema(map[string]any{"a": 1}, &OutputValidationSchema{ExpectedKeys: []string{"a", "b"}, MinOutputSize: 2, MaxOutputSize: 3})
	require.False(t, ok)
	require.NotEmpty(t, issues)

	ok, issues = ValidateOutputAgainstSchema(map[string]any{"a": 1, "b": 2}, &OutputValidationSchema{ExpectedKeys: []string{"a", "b"}, MinOutputSize: 1, MaxOutputSize: 2})
	require.True(t, ok)
	require.Empty(t, issues)

	expected := map[string]any{
		"fields":   []any{"x", "y"},
		"min_size": float64(1),
		"max_size": float64(3),
	}
	schema := ExtractOutputValidationSchema(expected)
	require.NotNil(t, schema)
	require.Equal(t, []string{"x", "y"}, schema.ExpectedKeys)
	require.Equal(t, 1, schema.MinOutputSize)
	require.Equal(t, 3, schema.MaxOutputSize)
	require.Equal(t, expected, schema.Schema)
	require.Nil(t, ExtractOutputValidationSchema(nil))

	plan := &core.Plan{Steps: []core.PlanStep{
		{ID: "fast", Tool: "fast-tool"},
		{ID: "medium", Tool: ""},
		{ID: "slow", Tool: "slow-tool"},
		{ID: "unknown", Tool: "unknown-tool"},
		{ID: "fallback"},
	}}
	metadata := map[string]OperatorMetadata{
		"fast-tool":    {CostClass: authoring.CostClassFast},
		"medium":       {CostClass: authoring.CostClassMedium},
		"slow-tool":    {CostClass: authoring.CostClassSlow},
		"unknown-tool":  {CostClass: authoring.CostClassUnknown},
	}
	hints := OptimizeStepOrdering(plan, metadata)
	require.Len(t, hints, 5)
	require.Equal(t, "fast", hints[0].StepID)
	require.True(t, hints[0].CanParallelize)
	require.Equal(t, "medium", hints[1].StepID)
	require.False(t, hints[1].CanParallelize)
	require.Equal(t, "unknown", hints[2].StepID)
	require.Equal(t, "fallback", hints[3].StepID)
	require.Equal(t, "slow", hints[4].StepID)
	require.Equal(t, 1, hints[0].EstimatedDuration)
	require.Equal(t, 5, hints[1].EstimatedDuration)
	require.Equal(t, 30, hints[4].EstimatedDuration)

	require.Nil(t, OptimizeStepOrdering(nil, nil))
	require.Nil(t, OptimizeStepOrdering(&core.Plan{}, nil))

	sortFn := SortStepsByOptimization
	ordered := sortFn(plan.Steps, metadata, "fast_first")
	require.Equal(t, []string{"fast", "medium", "unknown", "fallback", "slow"}, []string{ordered[0].ID, ordered[1].ID, ordered[2].ID, ordered[3].ID, ordered[4].ID})
	ordered = sortFn(plan.Steps, metadata, "slow_first")
	require.Equal(t, []string{"slow", "medium", "unknown", "fallback", "fast"}, []string{ordered[0].ID, ordered[1].ID, ordered[2].ID, ordered[3].ID, ordered[4].ID})
	ordered = sortFn(plan.Steps, metadata, "parallel")
	require.Equal(t, plan.Steps, ordered)
	require.Empty(t, sortFn(nil, metadata, "fast_first"))

	require.Equal(t, authoring.CostClassFast, getCostClassForStep(core.PlanStep{ID: "x", Tool: "fast-tool"}, metadata))
	require.Equal(t, 1, costToValue(authoring.CostClassFast))
	require.Equal(t, 2, costToValue(authoring.CostClassMedium))
	require.Equal(t, 3, costToValue(authoring.CostClassSlow))
	require.Equal(t, 2, costToValue(authoring.CostClassUnknown))

	computeFn := ComputeEstimatedExecutionTime
	estimated := computeFn(plan, metadata)
	require.Equal(t, 46, estimated)

	retryFn := ShouldRetryStep
	require.False(t, retryFn("step", authoring.RetryClassNone, nil, 0, 3))
	require.False(t, retryFn("step", authoring.RetryClassIdempotent, nil, 3, 3))
	require.True(t, retryFn("step", authoring.RetryClassIdempotent, nil, 0, 3))
	require.True(t, retryFn("step", authoring.RetryClassStateless, nil, 0, 3))
	require.True(t, retryFn("step", authoring.RetryClassProbed, nil, 0, 3))
	require.False(t, retryFn("step", authoring.RetryClass("custom"), nil, 0, 3))

	knowledgeFn := RetrieveRelevantKnowledge
	require.Nil(t, knowledgeFn(context.Background(), nil, &KnowledgeQuery{}))
	require.Nil(t, knowledgeFn(context.Background(), store, nil))
	require.Empty(t, knowledgeFn(context.Background(), store, &KnowledgeQuery{MethodName: "m", TaskType: core.TaskTypeAnalysis, MaxResults: 10}))
}

func TestRecoveryEdgeBranches(t *testing.T) {
	state := core.NewContext()
	stateless := &StatelessRecoveryStrategy{}
	require.NoError(t, stateless.Execute(context.Background(), state, "step", nil))
	_, ok := state.Get("operator_state.step")
	require.True(t, ok)

	probed := &ProbedRecoveryStrategy{}
	require.NoError(t, probed.Execute(context.Background(), state, "step", nil))
	probed.Prober = func(context.Context, *core.Context, string) (bool, error) { return false, nil }
	err := probed.Execute(context.Background(), state, "step", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "preconditions not met")
	probed.Prober = func(context.Context, *core.Context, string) (bool, error) { return false, errors.New("boom") }
	err = probed.Execute(context.Background(), state, "step", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "precondition check failed")

	engine := &RecoveryPolicyEngine{MaxRecoveryAttempts: 2}
	_, err = engine.DetermineRecoveryAction("step", RetryClassIdempotent, 2, nil)
	require.Error(t, err)
	_, err = engine.DetermineRecoveryAction("step", RetryClassNone, 0, nil)
	require.Error(t, err)
	_, err = (&RecoveryPolicyEngine{}).DetermineRecoveryAction("step", RetryClassIdempotent, 0, nil)
	require.Error(t, err)

	require.Error(t, engine.ApplyRecoveryStrategy(context.Background(), state, nil, "step", nil))

	success, issues := engine.VerifyStepCompletion(context.Background(), state, nil)
	require.False(t, success)
	require.NotEmpty(t, issues)

	emptyEngine := &RecoveryPolicyEngine{}
	success, issues = emptyEngine.VerifyStepCompletion(context.Background(), state, &VerificationContext{ExecutionResult: &core.Result{Success: true, Data: map[string]any{"ok": true}}})
	require.True(t, success)
	require.Nil(t, issues)
	success, issues = emptyEngine.VerifyStepCompletion(context.Background(), state, &VerificationContext{ExecutionResult: &core.Result{Success: false}})
	require.False(t, success)
	require.NotEmpty(t, issues)

	builder := &RecoveryContextBuilder{}
	ctx := builder.BuildRecoveryContext(context.Background(), "step", "operator", OperatorMetadata{
		RetryClass: RetryClassIdempotent,
		CostClass:  authoring.CostClassFast,
		BranchSafe: true,
		VerificationHint: VerificationHint{
			Description: "verify",
			Criteria:    []string{"a"},
			Files:       []string{"file.go"},
			Timeout:     time.Second,
		},
		FileFocus: FileFocus{
			Primary:   []string{"a.go"},
			Secondary: []string{"b.go"},
			Patterns:  []string{"*.go"},
			Exclude:   []string{"vendor/*"},
		},
		ExpectedOutput: "json",
	}, errors.New("boom"), 2)
	require.Equal(t, "step", ctx["step_id"])
	require.Contains(t, ctx, "verification_hint")
	require.Contains(t, ctx, "file_focus")
	require.Equal(t, "json", ctx["expected_output"])

	empty := builder.BuildRecoveryContext(context.Background(), "step", "operator", OperatorMetadata{}, errors.New("boom"), 0)
	require.NotContains(t, empty, "verification_hint")
	require.NotContains(t, empty, "file_focus")
	require.NotContains(t, empty, "expected_output")
}
