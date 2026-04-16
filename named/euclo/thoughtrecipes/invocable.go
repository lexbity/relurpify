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

	// Execute the recipe using the new ExecuteWithInput method
	result, err := r.Executor.ExecuteWithInput(ctx, r.Plan, in)
	if err != nil {
		return &core.Result{Success: false, Error: err}, err
	}

	return recipeResultToCoreResult(result), nil
}

// recipeResultToCoreResult converts a RecipeResult to a core.Result.
// This preserves all fields including Warnings and step-level artifacts in Data.
func recipeResultToCoreResult(recipeResult *RecipeResult) *core.Result {
	if recipeResult == nil {
		return &core.Result{Success: false, Error: fmt.Errorf("nil recipe result")}
	}

	data := map[string]any{
		"recipe_id":      recipeResult.RecipeID,
		"artifacts":      recipeResult.Artifacts,
		"warnings":       recipeResult.Warnings,
		"final_captures": recipeResult.FinalCaptures,
		"step_results":   recipeResult.StepResults,
	}

	// Include final result data if present
	if recipeResult.FinalResult != nil && recipeResult.FinalResult.Data != nil {
		for k, v := range recipeResult.FinalResult.Data {
			if _, exists := data[k]; !exists {
				data[k] = v
			}
		}
	}

	return &core.Result{
		Success: recipeResult.Success,
		Data:    data,
		Error:   nil,
	}
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
		RecipePath:             p.Name, // Could be enhanced to store actual file path
		Keywords:               p.IntentKeywords,
		Summary:                p.Description,
	}
}
