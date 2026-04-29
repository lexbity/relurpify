package analysis

import (
	"context"
	"time"

	"codeburg.org/lexbit/relurpify/agents/goalcon/types"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
)

// ClassifierConfig controls LLM-based classification behavior.
type ClassifierConfig struct {
	Enabled        bool
	Timeout        time.Duration
	FallbackOnFail bool
	Cache          *GoalCache
	MinConfidence  float32
}

// DefaultClassifierConfig returns sensible defaults.
func DefaultClassifierConfig() ClassifierConfig {
	return ClassifierConfig{
		Enabled:        true,
		Timeout:        5 * time.Second,
		FallbackOnFail: true,
		Cache:          NewGoalCache(256),
		MinConfidence:  0.5,
	}
}

// ClassifyGoalWithLLM attempts to classify a goal using the language model.
// Falls back to keyword-based classification if LLM fails or is disabled.
func ClassifyGoalWithLLM(
	taskInstruction string,
	model core.LanguageModel,
	operators *types.OperatorRegistry,
	config ClassifierConfig,
) types.GoalCondition {
	if taskInstruction == "" {
		return types.GoalCondition{}
	}

	// Check cache first
	if config.Cache != nil {
		if cached := config.Cache.Get(taskInstruction); cached != nil {
			return *cached
		}
	}

	// Try LLM classification if enabled
	if config.Enabled && model != nil {
		goal := classifyViaLLM(taskInstruction, model, operators, config)
		if goal != nil {
			// Cache successful classification
			if config.Cache != nil {
				config.Cache.Set(taskInstruction, goal)
			}
			return *goal
		}

		// If LLM fails but fallback is disabled, return empty
		if !config.FallbackOnFail {
			return types.GoalCondition{Description: taskInstruction}
		}
	}

	// Fallback to keyword-based classification
	goal := ClassifyGoal(taskInstruction, operators)
	if config.Cache != nil {
		config.Cache.Set(taskInstruction, &goal)
	}
	return goal
}

// classifyViaLLM handles the actual LLM invocation with timeout.
func classifyViaLLM(
	instruction string,
	model core.LanguageModel,
	operators *types.OperatorRegistry,
	config ClassifierConfig,
) *types.GoalCondition {
	// Extract available predicates from operator registry
	availablePredicates := PredicatesFromRegistry(operators)

	// Set up timeout
	ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
	defer cancel()

	// Create a channel for the result to support timeout
	type result struct {
		resp *ClassificationResponse
		err  error
	}
	resultCh := make(chan result, 1)

	go func() {
		resp, err := classifyWithLLM(model, instruction, availablePredicates)
		resultCh <- result{resp, err}
	}()

	select {
	case res := <-resultCh:
		if res.err != nil {
			// Log error would go here in a real implementation
			return nil
		}
		if res.resp == nil {
			return nil
		}

		// Check confidence threshold
		if res.resp.Confidence < config.MinConfidence {
			// Log: low confidence, will fall back
			return nil
		}

		// Convert response to types.GoalCondition
		goal := &types.GoalCondition{
			Description: instruction,
			Predicates:  make([]types.Predicate, 0, len(res.resp.Predicates)),
		}
		for _, p := range res.resp.Predicates {
			goal.Predicates = append(goal.Predicates, types.Predicate(p))
		}

		return goal

	case <-ctx.Done():
		// Timeout occurred
		return nil
	}
}

// ClassifyGoalWithContext is a context-aware wrapper for ClassifyGoalWithLLM.
func ClassifyGoalWithContext(
	coreCtx *contextdata.Envelope,
	instruction string,
	model core.LanguageModel,
	operators *types.OperatorRegistry,
) types.GoalCondition {
	config := DefaultClassifierConfig()

	// Use cached classifier config if available in context
	if coreCtx != nil {
		if raw, ok := coreCtx.GetWorkingValue("goalcon.classifier_config"); ok {
			if cachedConfig, ok := raw.(ClassifierConfig); ok {
				config = cachedConfig
			}
		}
	}

	return ClassifyGoalWithLLM(instruction, model, operators, config)
}
