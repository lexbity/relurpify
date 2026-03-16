package analysis

import (
	"github.com/lexcodex/relurpify/agents/goalcon/types"
)

import (
	"testing"
	"time"
)

// TestNewGoalDecomposer tests decomposer creation.
func TestNewGoalDecomposer(t *testing.T) {
	decomposer := NewGoalDecomposer()

	if decomposer == nil {
		t.Fatal("Expected non-nil decomposer")
	}

	if decomposer.maxSubGoals != 8 {
		t.Errorf("Expected maxSubGoals=8, got %d", decomposer.maxSubGoals)
	}

	if len(decomposer.strategies) < 3 {
		t.Errorf("Expected at least 3 strategies, got %d", len(decomposer.strategies))
	}
}

// TestGoalDecomposer_SetMaxSubGoals tests configuration.
func TestGoalDecomposer_SetMaxSubGoals(t *testing.T) {
	decomposer := NewGoalDecomposer()

	decomposer.SetMaxSubGoals(5)
	if decomposer.maxSubGoals != 5 {
		t.Errorf("Expected maxSubGoals=5, got %d", decomposer.maxSubGoals)
	}

	// Invalid max should not change value
	decomposer.SetMaxSubGoals(0)
	if decomposer.maxSubGoals != 5 {
		t.Error("Expected maxSubGoals to remain unchanged for invalid value")
	}
}

// TestGoalDecomposer_DecomposeByPredicates tests predicate-based decomposition.
func TestGoalDecomposer_DecomposeByPredicates(t *testing.T) {
	decomposer := NewGoalDecomposer()

	goal := types.GoalCondition{
		Description: "Complete system setup",
		Predicates: []types.Predicate{
			"database=installed",
			"server=running",
			"config=loaded",
			"monitoring=enabled",
		},
	}

	decomp := decomposer.Decompose(goal, DecompositionStrategyPredicates, nil)

	if decomp == nil {
		t.Fatal("Expected non-nil decomposition")
	}

	if len(decomp.SubGoals) != 4 {
		t.Errorf("Expected 4 sub-goals, got %d", len(decomp.SubGoals))
	}

	if decomp.Strategy != DecompositionStrategyPredicates {
		t.Errorf("Strategy mismatch: expected %s", DecompositionStrategyPredicates)
	}
}

// TestGoalDecomposer_DecomposeSequential tests sequential decomposition.
func TestGoalDecomposer_DecomposeSequential(t *testing.T) {
	decomposer := NewGoalDecomposer()

	goal := types.GoalCondition{
		Description: "Build project",
		Predicates: []types.Predicate{
			"compile=done",
			"test=pass",
			"package=ready",
		},
	}

	decomp := decomposer.Decompose(goal, DecompositionStrategySequential, nil)

	if decomp == nil {
		t.Fatal("Expected non-nil decomposition")
	}

	if len(decomp.SubGoals) != 3 {
		t.Errorf("Expected 3 sub-goals, got %d", len(decomp.SubGoals))
	}

	// Check dependency chain
	if len(decomp.SubGoals[1].Dependencies) != 1 {
		t.Error("Expected second sub-goal to depend on first")
	}

	if len(decomp.SubGoals[2].Dependencies) != 1 {
		t.Error("Expected third sub-goal to depend on second")
	}
}

// TestGoalDecomposer_DecomposeHierarchical tests hierarchical decomposition.
func TestGoalDecomposer_DecomposeHierarchical(t *testing.T) {
	decomposer := NewGoalDecomposer()

	goal := types.GoalCondition{
		Description: "Deploy application",
		Predicates: []types.Predicate{
			"code=reviewed",
			"tests=passed",
			"image=built",
			"registry=pushed",
		},
	}

	decomp := decomposer.Decompose(goal, DecompositionStrategyHierarchical, nil)

	if decomp == nil {
		t.Fatal("Expected non-nil decomposition")
	}

	// Hierarchical should split into 2 levels
	if len(decomp.SubGoals) < 2 {
		t.Errorf("Expected at least 2 hierarchical objectives, got %d", len(decomp.SubGoals))
	}
}

// TestGoalDecomposer_ShouldDecompose_Complex tests decomposition decision.
func TestGoalDecomposer_ShouldDecompose_Complex(t *testing.T) {
	decomposer := NewGoalDecomposer()

	goal := types.GoalCondition{
		Description: "Complex goal",
		Predicates: []types.Predicate{
			"p1=true", "p2=true", "p3=true", "p4=true", "p5=true",
		},
	}

	if !decomposer.ShouldDecompose(goal, nil) {
		t.Error("Expected decomposition for complex goal")
	}
}

// TestGoalDecomposer_ShouldDecompose_Simple tests no decomposition needed.
func TestGoalDecomposer_ShouldDecompose_Simple(t *testing.T) {
	decomposer := NewGoalDecomposer()

	goal := types.GoalCondition{
		Description: "Simple goal",
		Predicates: []types.Predicate{"p1=true"},
	}

	if decomposer.ShouldDecompose(goal, nil) {
		t.Error("Expected no decomposition for simple goal")
	}
}

// TestGoalDecomposer_ShouldDecompose_Ambiguous tests decomposition for ambiguous goals.
func TestGoalDecomposer_ShouldDecompose_Ambiguous(t *testing.T) {
	decomposer := NewGoalDecomposer()

	goal := types.GoalCondition{
		Description: "Ambiguous goal",
		Predicates:  []types.Predicate{"p1=true", "p2=true"},
	}

	ambiguity := &AmbiguityScore{
		OverallScore: 0.8,
		ShouldRefine: true,
	}

	if !decomposer.ShouldDecompose(goal, ambiguity) {
		t.Error("Expected decomposition for ambiguous goal")
	}
}

// TestGoalDecomposer_ChooseBestStrategy tests strategy selection.
func TestGoalDecomposer_ChooseBestStrategy(t *testing.T) {
	decomposer := NewGoalDecomposer()

	tests := []struct {
		name           string
		goal           types.GoalCondition
		ambiguity      *AmbiguityScore
		expectedStrat  DecompositionStrategy
	}{
		{
			name: "High ambiguity",
			goal: types.GoalCondition{Predicates: []types.Predicate{"p1", "p2"}},
			ambiguity: &AmbiguityScore{OverallScore: 0.8},
			expectedStrat: DecompositionStrategyHierarchical,
		},
		{
			name: "Many predicates",
			goal: types.GoalCondition{Predicates: []types.Predicate{"p1", "p2", "p3", "p4", "p5"}},
			ambiguity: nil,
			expectedStrat: DecompositionStrategySequential,
		},
		{
			name: "Default",
			goal: types.GoalCondition{Predicates: []types.Predicate{"p1", "p2", "p3"}},
			ambiguity: nil,
			expectedStrat: DecompositionStrategyPredicates,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			strategy := decomposer.ChooseBestStrategy(test.goal, test.ambiguity)
			if strategy != test.expectedStrat {
				t.Errorf("Expected %s, got %s", test.expectedStrat, strategy)
			}
		})
	}
}

// TestGoalDecomposer_RegisterStrategy tests custom strategy registration.
func TestGoalDecomposer_RegisterStrategy(t *testing.T) {
	decomposer := NewGoalDecomposer()

	customStrat := DecompositionStrategy("custom")
	customFn := func(goal types.GoalCondition, ambiguity *AmbiguityScore) []*SubGoal {
		return []*SubGoal{
			{
				ID:          "custom_1",
				Description: "Custom decomposition",
				Predicates:  goal.Predicates,
			},
		}
	}

	decomposer.RegisterStrategy(customStrat, customFn)

	goal := types.GoalCondition{
		Description: "Test",
		Predicates:  []types.Predicate{"p1=true"},
	}

	decomp := decomposer.Decompose(goal, customStrat, nil)

	if decomp == nil {
		t.Fatal("Expected decomposition with custom strategy")
	}

	if len(decomp.SubGoals) != 1 || decomp.SubGoals[0].ID != "custom_1" {
		t.Error("Custom strategy not applied correctly")
	}
}

// TestSubGoal_Fields tests sub-goal struct.
func TestSubGoal_Fields(t *testing.T) {
	subGoal := &SubGoal{
		ID:           "sg_1",
		Description:  "Test sub-goal",
		Predicates:   []types.Predicate{"p1=true"},
		Importance:   0.8,
		Dependencies: []string{"sg_0"},
		Timestamp:    time.Now(),
	}

	if subGoal.ID != "sg_1" {
		t.Error("ID mismatch")
	}

	if subGoal.Importance != 0.8 {
		t.Error("Importance mismatch")
	}

	if len(subGoal.Dependencies) != 1 {
		t.Error("Dependencies mismatch")
	}
}

// TestGoalDecomposition_Fields tests decomposition struct.
func TestGoalDecomposition_Fields(t *testing.T) {
	goal := types.GoalCondition{Description: "Test goal"}
	subGoals := []*SubGoal{
		{ID: "sg_1", Description: "Sub 1"},
		{ID: "sg_2", Description: "Sub 2"},
	}

	decomp := &GoalDecomposition{
		OriginalGoal:   goal,
		SubGoals:       subGoals,
		Strategy:       DecompositionStrategyPredicates,
		ExecutionOrder: []string{"sg_1", "sg_2"},
		Timestamp:      time.Now(),
	}

	if decomp.OriginalGoal.Description != "Test goal" {
		t.Error("Goal mismatch")
	}

	if len(decomp.SubGoals) != 2 {
		t.Error("SubGoals count mismatch")
	}

	if len(decomp.ExecutionOrder) != 2 {
		t.Error("ExecutionOrder count mismatch")
	}
}

// TestGoalDecomposer_ComputeExecutionOrder tests dependency resolution.
func TestGoalDecomposer_ComputeExecutionOrder(t *testing.T) {
	decomposer := NewGoalDecomposer()

	subGoals := []*SubGoal{
		{ID: "sg_3", Description: "Third", Dependencies: []string{"sg_1"}},
		{ID: "sg_1", Description: "First", Dependencies: []string{}},
		{ID: "sg_2", Description: "Second", Dependencies: []string{"sg_1"}},
	}

	order := decomposer.computeExecutionOrder(subGoals)

	if len(order) != 3 {
		t.Errorf("Expected 3 items in order, got %d", len(order))
	}

	// sg_1 should come before sg_2 and sg_3
	sg1Idx := -1
	sg2Idx := -1
	sg3Idx := -1
	for i, id := range order {
		if id == "sg_1" {
			sg1Idx = i
		} else if id == "sg_2" {
			sg2Idx = i
		} else if id == "sg_3" {
			sg3Idx = i
		}
	}

	if sg1Idx >= sg2Idx || sg1Idx >= sg3Idx {
		t.Error("Execution order violates dependencies")
	}
}

// TestGoalDecomposer_FormatReport tests report generation.
func TestGoalDecomposer_FormatReport(t *testing.T) {
	decomposer := NewGoalDecomposer()

	goal := types.GoalCondition{Description: "Test goal"}
	subGoals := []*SubGoal{
		{
			ID:          "sg_1",
			Description: "Sub 1",
			Importance:  0.6,
			Predicates:  []types.Predicate{"p1"},
		},
		{
			ID:          "sg_2",
			Description: "Sub 2",
			Importance:  0.4,
			Predicates:  []types.Predicate{"p2"},
		},
	}

	decomp := &GoalDecomposition{
		OriginalGoal:   goal,
		SubGoals:       subGoals,
		Strategy:       DecompositionStrategyPredicates,
		ExecutionOrder: []string{"sg_1", "sg_2"},
		Rationale:      "Goal complexity requires decomposition",
		Timestamp:      time.Now(),
	}

	report := decomposer.FormatDecompositionReport(decomp)

	if report == "" {
		t.Fatal("Expected non-empty report")
	}

	requiredFields := []string{
		"Test goal",
		"Predicates",
		"Sub-Goals",
		"Sub 1",
		"Sub 2",
	}

	for _, field := range requiredFields {
		if !contains(report, field) {
			t.Errorf("Report missing field: %s", field)
		}
	}
}

// TestGoalDecomposer_MaxSubGoalsLimit tests that decomposition respects limit.
func TestGoalDecomposer_MaxSubGoalsLimit(t *testing.T) {
	decomposer := NewGoalDecomposer()
	decomposer.SetMaxSubGoals(2)

	goal := types.GoalCondition{
		Description: "Complex goal",
		Predicates: []types.Predicate{
			"p1=true", "p2=true", "p3=true",
			"p4=true", "p5=true",
		},
	}

	decomp := decomposer.Decompose(goal, DecompositionStrategyPredicates, nil)

	if decomp == nil {
		t.Fatal("Expected decomposition")
	}

	if len(decomp.SubGoals) > 2 {
		t.Errorf("Expected at most 2 sub-goals, got %d", len(decomp.SubGoals))
	}
}

// TestGoalDecomposer_GenerateRationale tests rationale generation.
func TestGoalDecomposer_GenerateRationale(t *testing.T) {
	decomposer := NewGoalDecomposer()

	tests := []struct {
		strategy DecompositionStrategy
		expected string
	}{
		{DecompositionStrategyPredicates, "independent"},
		{DecompositionStrategySequential, "sequential"},
		{DecompositionStrategyHierarchical, "hierarchical"},
	}

	for _, test := range tests {
		rationale := decomposer.generateRationale(test.strategy, 5)
		if !contains(rationale, test.expected) {
			t.Errorf("Rationale for %s missing '%s'", test.strategy, test.expected)
		}
	}
}

// TestDecomposition_Importance_Distribution tests importance weights.
func TestDecomposition_Importance_Distribution(t *testing.T) {
	decomposer := NewGoalDecomposer()

	goal := types.GoalCondition{
		Description: "Test",
		Predicates: []types.Predicate{"p1", "p2", "p3"},
	}

	decomp := decomposer.Decompose(goal, DecompositionStrategyPredicates, nil)

	if decomp == nil {
		t.Fatal("Expected decomposition")
	}

	// Check importance values sum to approximately 1.0
	totalImportance := float32(0)
	for _, sg := range decomp.SubGoals {
		totalImportance += sg.Importance
	}

	if totalImportance < 0.9 || totalImportance > 1.1 {
		t.Errorf("Expected total importance ~1.0, got %.2f", totalImportance)
	}
}
