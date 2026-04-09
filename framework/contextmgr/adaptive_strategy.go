package contextmgr

import (
	"fmt"
	"github.com/lexcodex/relurpify/framework/core"
	"strings"
	"time"
)

// StrategyMode controls adaptive strategy personalities.
type StrategyMode string

const (
	ModeAggressive   StrategyMode = "aggressive"
	ModeBalanced     StrategyMode = "balanced"
	ModeConservative StrategyMode = "conservative"
)

// AdaptiveStrategy adjusts retrieval behaviour using task complexity and past success.
type AdaptiveStrategy struct {
	contextLoadHistory []ContextLoadEvent
	successRate        map[string]float64
	currentMode        StrategyMode
	lowSuccessThreshold,
	highSuccessThreshold float64
	aggressive   *ProfiledStrategy
	balanced     *ProfiledStrategy
	conservative *ProfiledStrategy
}

// NewAdaptiveStrategy returns a ready adaptive strategy.
func NewAdaptiveStrategy() *AdaptiveStrategy {
	return &AdaptiveStrategy{
		contextLoadHistory:   make([]ContextLoadEvent, 0),
		successRate:          make(map[string]float64),
		currentMode:          ModeBalanced,
		lowSuccessThreshold:  0.6,
		highSuccessThreshold: 0.85,
		aggressive:           NewStrategyFromProfile(AggressiveProfile),
		balanced:             NewStrategyFromProfile(BalancedProfile),
		conservative:         NewStrategyFromProfile(ConservativeProfile),
	}
}

func (as *AdaptiveStrategy) activeStrategy() *ProfiledStrategy {
	if as == nil {
		return NewStrategyFromProfile(BalancedProfile)
	}
	switch as.currentMode {
	case ModeAggressive:
		if as.aggressive != nil {
			return as.aggressive
		}
	case ModeConservative:
		if as.conservative != nil {
			return as.conservative
		}
	default:
		if as.balanced != nil {
			return as.balanced
		}
	}
	return NewStrategyFromProfile(BalancedProfile)
}

// SelectContext delegates to the current mode.
func (as *AdaptiveStrategy) SelectContext(task *core.Task, budget *core.ContextBudget) (*ContextRequest, error) {
	if task == nil {
		return nil, fmt.Errorf("task required")
	}
	if budget == nil {
		return nil, fmt.Errorf("budget required")
	}
	complexity := as.analyzeTaskComplexity(task)
	as.adjustMode(complexity)
	return as.activeStrategy().SelectContext(task, budget)
}

// ShouldCompress adapts threshold based on mode.
func (as *AdaptiveStrategy) ShouldCompress(ctx *core.SharedContext) bool {
	return as.activeStrategy().ShouldCompress(ctx)
}

// DetermineDetailLevel returns mode-specific detail.
func (as *AdaptiveStrategy) DetermineDetailLevel(file string, relevance float64) DetailLevel {
	return as.activeStrategy().DetermineDetailLevel(file, relevance)
}

// ShouldExpandContext reacts to failures or uncertainty.
func (as *AdaptiveStrategy) ShouldExpandContext(ctx *core.SharedContext, lastResult *core.Result) bool {
	if lastResult == nil {
		return false
	}
	event := ContextLoadEvent{
		Timestamp: time.Now(),
		Success:   lastResult.Success,
	}
	if !lastResult.Success {
		event.Trigger = "failure"
		as.contextLoadHistory = append(as.contextLoadHistory, event)
		return true
	}
	if output, ok := lastResult.Data["llm_output"].(string); ok {
		markers := []string{
			"not sure", "unclear", "need more information",
			"cannot determine", "insufficient",
		}
		for _, marker := range markers {
			if ContainsInsensitive(output, marker) {
				event.Trigger = "uncertainty"
				as.contextLoadHistory = append(as.contextLoadHistory, event)
				return true
			}
		}
	}
	as.contextLoadHistory = append(as.contextLoadHistory, event)
	return false
}

// PrioritizeContext combines relevance and recency.
func (as *AdaptiveStrategy) PrioritizeContext(items []core.ContextItem) []core.ContextItem {
	return as.activeStrategy().PrioritizeContext(items)
}

func (as *AdaptiveStrategy) analyzeTaskComplexity(task *core.Task) int {
	if task == nil {
		return 0
	}
	complexity := 0
	inst := task.Instruction
	if len(inst) > 500 {
		complexity += 2
	}
	if countKeywords(inst, []string{"refactor", "redesign", "architecture"}) > 0 {
		complexity += 3
	}
	if countKeywords(inst, []string{"fix", "bug", "error", "debug"}) > 0 {
		complexity += 1
	}
	if countKeywords(inst, []string{"add", "implement", "create"}) > 0 {
		complexity += 2
	}
	if task.Metadata != nil {
		if taskType, ok := task.Metadata["type"]; ok {
			switch strings.ToLower(taskType) {
			case "exploration":
				complexity++
			case "modification":
				complexity += 2
			case "creation":
				complexity += 3
			}
		}
	}
	return complexity
}

func (as *AdaptiveStrategy) adjustMode(complexity int) {
	recent := as.contextLoadHistory
	if len(recent) > 10 {
		recent = recent[len(recent)-10:]
	}
	successCount := 0
	for _, event := range recent {
		if event.Success {
			successCount++
		}
	}
	successRate := 0.0
	if len(recent) > 0 {
		successRate = float64(successCount) / float64(len(recent))
	}
	switch {
	case successRate < as.lowSuccessThreshold:
		as.currentMode = ModeConservative
	case successRate > as.highSuccessThreshold && complexity < 3:
		as.currentMode = ModeAggressive
	default:
		as.currentMode = ModeBalanced
	}
}
