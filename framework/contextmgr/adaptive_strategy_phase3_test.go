package contextmgr

import (
	"reflect"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
)

func TestAdaptiveStrategyInitialisesProfileDelegates(t *testing.T) {
	strategy := NewAdaptiveStrategy()
	if strategy.aggressive == nil || strategy.balanced == nil || strategy.conservative == nil {
		t.Fatalf("expected all profile delegates to be initialised: %#v", strategy)
	}
	if strategy.aggressive.Profile.Name != AggressiveProfile.Name {
		t.Fatalf("unexpected aggressive profile: %#v", strategy.aggressive.Profile)
	}
	if strategy.balanced.Profile.Name != BalancedProfile.Name {
		t.Fatalf("unexpected balanced profile: %#v", strategy.balanced.Profile)
	}
	if strategy.conservative.Profile.Name != ConservativeProfile.Name {
		t.Fatalf("unexpected conservative profile: %#v", strategy.conservative.Profile)
	}
}

func TestAdaptiveStrategyActiveStrategyMatchesMode(t *testing.T) {
	strategy := NewAdaptiveStrategy()
	if got := strategy.activeStrategy(); got != strategy.balanced {
		t.Fatal("expected balanced profile to be active by default")
	}
	strategy.currentMode = ModeAggressive
	if got := strategy.activeStrategy(); got != strategy.aggressive {
		t.Fatal("expected aggressive profile to be active")
	}
	strategy.currentMode = ModeConservative
	if got := strategy.activeStrategy(); got != strategy.conservative {
		t.Fatal("expected conservative profile to be active")
	}
}

func TestAdaptiveStrategyDelegatesAfterModeTransition(t *testing.T) {
	strategy := NewAdaptiveStrategy()
	strategy.contextLoadHistory = []ContextLoadEvent{
		{Success: true},
		{Success: true},
		{Success: true},
		{Success: true},
		{Success: true},
		{Success: true},
	}
	strategy.currentMode = ModeBalanced
	strategy.adjustMode(0)
	if strategy.currentMode != ModeAggressive {
		t.Fatalf("expected aggressive mode after high success rate, got %s", strategy.currentMode)
	}

	budget := core.NewContextBudget(8000)
	task := &core.Task{Instruction: "Fix bug in pkg/foo.go"}
	request, err := strategy.SelectContext(task, budget)
	if err != nil {
		t.Fatalf("SelectContext: %v", err)
	}
	if request.MaxTokens != budget.AvailableForContext/4 {
		t.Fatalf("expected aggressive profile token budget, got %d", request.MaxTokens)
	}
	if len(request.Files) == 0 || request.Files[0].DetailLevel != DetailSignatureOnly {
		t.Fatalf("expected aggressive profile file handling, got %#v", request.Files)
	}

	strategy.contextLoadHistory = []ContextLoadEvent{{Success: false}, {Success: false}, {Success: false}}
	strategy.currentMode = ModeBalanced
	strategy.adjustMode(0)
	if strategy.currentMode != ModeConservative {
		t.Fatalf("expected conservative mode after low success rate, got %s", strategy.currentMode)
	}
	if got := strategy.DetermineDetailLevel("file.go", 0.55); got != NewStrategyFromProfile(ConservativeProfile).DetermineDetailLevel("file.go", 0.55) {
		t.Fatalf("expected conservative detail behavior, got %v", got)
	}
	if got := strategy.PrioritizeContext([]core.ContextItem{
		&testContextItem{id: "low", relevance: 0.1},
		&testContextItem{id: "high", relevance: 0.9},
	}); !reflect.DeepEqual(got, []core.ContextItem{
		&testContextItem{id: "high", relevance: 0.9},
		&testContextItem{id: "low", relevance: 0.1},
	}) {
		t.Fatalf("unexpected conservative prioritization: %#v", got)
	}
}
