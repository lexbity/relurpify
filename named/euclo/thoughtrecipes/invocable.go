package thoughtrecipes

import (
	"context"
	"fmt"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/execution"
	euclorelurpic "github.com/lexcodex/relurpify/named/euclo/relurpicabilities"
)

// RecipeInvocable wraps a thoughtrecipes.ExecutionPlan as an execution.Invocable.
// It implements the full Invocable interface so recipes participate in the
// unified registry and dispatch path.
type RecipeInvocable struct {
	Plan     *ExecutionPlan
	Executor *Executor
}

// ID returns the capability ID for this recipe (euclo:recipe.<name>).
func (r *RecipeInvocable) ID() string {
	if r == nil || r.Plan == nil {
		return ""
	}
	return "euclo:recipe." + r.Plan.Name
}

// IsPrimary returns true as recipes are primary dispatch targets.
func (r *RecipeInvocable) IsPrimary() bool {
	return true
}

// Invoke executes the recipe plan and returns a core.Result.
// This implements the execution.Invocable interface.
func (r *RecipeInvocable) Invoke(ctx context.Context, in execution.InvokeInput) (*core.Result, error) {
	if r == nil {
		return &core.Result{Success: false, Error: fmt.Errorf("nil RecipeInvocable")}, fmt.Errorf("nil RecipeInvocable")
	}
	if r.Plan == nil {
		return &core.Result{Success: false, Error: fmt.Errorf("nil ExecutionPlan")}, fmt.Errorf("nil ExecutionPlan")
	}
	if r.Executor == nil {
		return &core.Result{Success: false, Error: fmt.Errorf("nil Executor")}, fmt.Errorf("nil Executor")
	}

	// Execute the recipe
	result, err := r.Executor.Execute(ctx, r.Plan, in.Task, in.Environment)
	if err != nil {
		return &core.Result{Success: false, Error: err}, err
	}

	if result == nil {
		return &core.Result{Success: false, Error: fmt.Errorf("nil recipe result")}, nil
	}

	data := map[string]any{
		"recipe_id":      result.RecipeID,
		"artifacts":      result.Artifacts,
		"warnings":       result.Warnings,
		"final_captures": result.FinalCaptures,
		"step_results":   result.StepResults,
	}

	// Include final result data if present
	if result.FinalResult != nil && result.FinalResult.Data != nil {
		for k, v := range result.FinalResult.Data {
			if _, exists := data[k]; !exists {
				data[k] = v
			}
		}
	}

	return &core.Result{
		Success: result.Success,
		Data:    data,
		Error:   nil,
	}, nil
}

// ToDescriptor converts an ExecutionPlan to a euclorelurpic.Descriptor.
// This allows recipes to participate in Tier-1 keyword classification.
func (p *ExecutionPlan) ToDescriptor() euclorelurpic.Descriptor {
	if p == nil {
		return euclorelurpic.Descriptor{}
	}

	return euclorelurpic.Descriptor{
		ID:                     "euclo:recipe." + p.Name,
		DisplayName:            p.Name,
		ModeFamilies:           p.Modes,
		PrimaryCapable:         true,
		AllowDynamicResolution: true,
		IsUserDefined:          true,
		RecipePath:             p.FilePath,
		Keywords:               p.IntentKeywords,
		Summary:                p.Description,
	}
}
