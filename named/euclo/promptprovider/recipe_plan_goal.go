package promptprovider

import (
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/prompt"
	"codeburg.org/lexbit/relurpify/named/euclo/state"
)

// recipePlanGoalProvider provides the current recipe plan or goal context.
type recipePlanGoalProvider struct{}

func (p *recipePlanGoalProvider) Provide(ctx prompt.RuntimeContext) prompt.ContextChunk {
	if ctx.Envelope == nil {
		return prompt.ContextChunk{Content: ""}
	}

	var goalParts []string

	// Try to get recipe information
	if recipeID, hasRecipe := state.GetRecipeID(ctx.Envelope); hasRecipe {
		goalParts = append(goalParts, fmt.Sprintf("Recipe ID: %s", recipeID))

		if version, hasVersion := state.GetRecipeVersion(ctx.Envelope); hasVersion {
			goalParts = append(goalParts, fmt.Sprintf("Recipe Version: %s", version))
		}
	}

	// Try to get task instruction from the runtime context
	if instruction, ok := ctx.Variables["instruction"]; ok && instruction != "" {
		goalParts = append(goalParts, fmt.Sprintf("Task Goal: %s", instruction))
	}

	// Try to get route selection (which may contain plan information)
	if route, hasRoute := state.GetRouteSelection(ctx.Envelope); hasRoute && route != nil {
		goalParts = append(goalParts, fmt.Sprintf("Route Kind: %s", route.RouteKind))
		if route.RecipeID != "" {
			goalParts = append(goalParts, fmt.Sprintf("Recipe ID: %s", route.RecipeID))
		}
		if route.CapabilityID != "" {
			goalParts = append(goalParts, fmt.Sprintf("Capability ID: %s", route.CapabilityID))
		}
	}

	// Try to get intent classification for additional context
	if classification, hasClassification := state.GetIntentClassification(ctx.Envelope); hasClassification && classification != nil {
		if classification.WinningFamily != "" {
			goalParts = append(goalParts, fmt.Sprintf("Winning Family: %s", classification.WinningFamily))
		}
		if classification.Confidence > 0 {
			goalParts = append(goalParts, fmt.Sprintf("Confidence: %.2f", classification.Confidence))
		}
	}

	if len(goalParts) == 0 {
		return prompt.ContextChunk{Content: ""}
	}

	// Format as a structured context block
	return prompt.ContextChunk{Content: "Recipe Plan Goal:\n" + strings.Join(goalParts, "\n")}
}

func (p *recipePlanGoalProvider) Describe() prompt.ProviderMetadata {
	return prompt.ProviderMetadata{
		Name:        "euclo.recipe_plan_goal",
		Description: "Provides the current recipe plan goal and execution context",
		Paradigms:   []string{"euclo"},
		ReadsKeys: []string{
			"euclo.recipe_id",
			"euclo.recipe_version",
			"euclo.route_selection",
			"euclo.intent_classification",
		},
	}
}
