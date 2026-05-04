package promptprovider

import (
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/prompt"
	"codeburg.org/lexbit/relurpify/named/euclo/state"
)

// recipeStepContextProvider provides context from prior recipe step outputs (captures).
type recipeStepContextProvider struct{}

func (p *recipeStepContextProvider) Provide(ctx prompt.RuntimeContext) prompt.ContextChunk {
	if ctx.Envelope == nil {
		return prompt.ContextChunk{Content: ""}
	}

	// Get current recipe ID to scope captures
	recipeID, hasRecipe := state.GetRecipeID(ctx.Envelope)
	if !hasRecipe {
		return prompt.ContextChunk{Content: ""}
	}

	// Collect all captures for the current recipe
	var captures []string
	prefix := state.KeyRecipePrefix + recipeID + "."
	
	// Get all working values and filter for recipe captures
	snapshot := ctx.Envelope.HandoffSnapshot(contextdata.HandoffPolicy{
		PreserveWorkingMemory: true,
		WorkingKeys:           []string{}, // Get all keys
	})
	
	if snapshot != nil && snapshot.WorkingData != nil {
		for key, value := range snapshot.WorkingData {
			if strings.HasPrefix(key, prefix) {
				// This is a recipe capture
				captureName := strings.TrimPrefix(key, prefix)
				content := fmt.Sprintf("%s: %v", captureName, value)
				captures = append(captures, content)
			}
		}
	}

	if len(captures) == 0 {
		return prompt.ContextChunk{Content: ""}
	}

	// Format captures as a structured context block
	var result strings.Builder
	result.WriteString("Prior Step Outputs:\n")
	for _, capture := range captures {
		result.WriteString("- ")
		result.WriteString(capture)
		result.WriteString("\n")
	}

	return prompt.ContextChunk{Content: strings.TrimSpace(result.String())}
}

func (p *recipeStepContextProvider) Describe() prompt.ProviderMetadata {
	return prompt.ProviderMetadata{
		Name:        "euclo.recipe_step_context",
		Description: "Provides outputs from prior recipe steps (captures) for context",
		Paradigms:   []string{"euclo"},
		ReadsKeys:   []string{"euclo.recipe.*"},
	}
}
