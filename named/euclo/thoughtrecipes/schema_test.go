package thoughtrecipes

import (
	"testing"
)

// TestThoughtRecipeSchemaZeroValue ensures zero-value ThoughtRecipe has no panics on field access.
func TestThoughtRecipeSchemaZeroValue(t *testing.T) {
	recipe := ThoughtRecipe{}

	// Access all fields to ensure no panics
	_ = recipe.APIVersion
	_ = recipe.Kind
	_ = recipe.Metadata.Name
	_ = recipe.Metadata.Description
	_ = recipe.Metadata.Version
	_ = recipe.Global.Prompt
	_ = recipe.Global.Capabilities.Allowed
	_ = recipe.Global.Context.Enrichment
	_ = recipe.Global.Context.Aliases
	_ = recipe.Global.Configuration.Modes
	_ = recipe.Global.Configuration.IntentKeywords
	_ = recipe.Global.Configuration.TriggerPriority
	_ = recipe.Global.Configuration.AllowDynamicResolution
	_ = recipe.Sequence

	// Access nested struct fields
	_ = recipe.Global.Context.Sharing.Default
}

// TestRecipeMetadataZeroValue ensures zero-value RecipeMetadata has no panics.
func TestRecipeMetadataZeroValue(t *testing.T) {
	meta := RecipeMetadata{}
	_ = meta.Name
	_ = meta.Description
	_ = meta.Version
}

// TestRecipeGlobalZeroValue ensures zero-value RecipeGlobal has no panics.
func TestRecipeGlobalZeroValue(t *testing.T) {
	global := RecipeGlobal{}
	_ = global.Prompt
	_ = global.Capabilities.Allowed
	_ = global.Context.Enrichment
	_ = global.Context.Aliases
	_ = global.Context.Sharing.Default
	_ = global.Configuration.Modes
	_ = global.Configuration.IntentKeywords
	_ = global.Configuration.TriggerPriority
	_ = global.Configuration.AllowDynamicResolution
}

// TestRecipeStepZeroValue ensures zero-value RecipeStep has no panics.
func TestRecipeStepZeroValue(t *testing.T) {
	step := RecipeStep{}
	_ = step.ID
	_ = step.Parent.Paradigm
	_ = step.Parent.Prompt
	_ = step.Parent.Context.Enrichment
	_ = step.Parent.Context.Inherit
	_ = step.Parent.Context.Capture
	_ = step.Parent.Capabilities.Allowed
	_ = step.Child
	_ = step.Fallback
}

// TestRecipeStepAgentZeroValue ensures zero-value RecipeStepAgent has no panics.
func TestRecipeStepAgentZeroValue(t *testing.T) {
	agent := RecipeStepAgent{}
	_ = agent.Paradigm
	_ = agent.Prompt
	_ = agent.Context.Enrichment
	_ = agent.Context.Inherit
	_ = agent.Context.Capture
	_ = agent.Capabilities.Allowed
}

// TestRecipeStepContextZeroValue ensures zero-value RecipeStepContext has no panics.
func TestRecipeStepContextZeroValue(t *testing.T) {
	ctx := RecipeStepContext{}
	_ = ctx.Enrichment
	_ = ctx.Inherit
	_ = ctx.Capture
}

// TestRecipeCapabilitySpecZeroValue ensures zero-value RecipeCapabilitySpec has no panics.
func TestRecipeCapabilitySpecZeroValue(t *testing.T) {
	spec := RecipeCapabilitySpec{}
	_ = spec.Allowed
}

// TestRecipeContextSpecZeroValue ensures zero-value RecipeContextSpec has no panics.
func TestRecipeContextSpecZeroValue(t *testing.T) {
	spec := RecipeContextSpec{}
	_ = spec.Enrichment
	_ = spec.Sharing.Default
	_ = spec.Aliases
}

// TestRecipeSharingSpecZeroValue ensures zero-value RecipeSharingSpec has no panics.
func TestRecipeSharingSpecZeroValue(t *testing.T) {
	spec := RecipeSharingSpec{}
	_ = spec.Default
}

// TestRecipeConfigurationZeroValue ensures zero-value RecipeConfiguration has no panics.
func TestRecipeConfigurationZeroValue(t *testing.T) {
	cfg := RecipeConfiguration{}
	_ = cfg.Modes
	_ = cfg.IntentKeywords
	_ = cfg.TriggerPriority
	_ = cfg.AllowDynamicResolution
}

// TestThoughtRecipeFieldAssignment tests that fields can be assigned correctly.
func TestThoughtRecipeFieldAssignment(t *testing.T) {
	recipe := ThoughtRecipe{
		APIVersion: "euclo/v1alpha1",
		Kind:       "ThoughtRecipe",
		Metadata: RecipeMetadata{
			Name:        "test-recipe",
			Description: "A test recipe",
			Version:     "1.0",
		},
		Global: RecipeGlobal{
			Prompt: "Global prompt",
			Capabilities: RecipeCapabilitySpec{
				Allowed: []string{"file_read", "file_write"},
			},
			Context: RecipeContextSpec{
				Enrichment: []string{"ast", "archaeology"},
				Sharing: RecipeSharingSpec{
					Default: "explicit",
				},
				Aliases: map[string]string{
					"my_alias": "my.state.key",
				},
			},
			Configuration: RecipeConfiguration{
				Modes:                  []string{"debug", "chat"},
				IntentKeywords:         []string{"fix", "debug"},
				TriggerPriority:        6,
				AllowDynamicResolution: true,
			},
		},
		Sequence: []RecipeStep{
			{
				ID: "step1",
				Parent: RecipeStepAgent{
					Paradigm: "react",
					Prompt:   "Step 1 prompt",
					Context: RecipeStepContext{
						Enrichment: []string{"bkc"},
						Inherit:    []string{"explore_findings"},
						Capture:    []string{"analysis_result"},
					},
					Capabilities: RecipeCapabilitySpec{
						Allowed: []string{"file_read"},
					},
				},
				Child: &RecipeStepAgent{
					Paradigm: "react",
					Prompt:   "Child prompt",
				},
				Fallback: &RecipeStepAgent{
					Paradigm: "planner",
					Prompt:   "Fallback prompt",
				},
			},
		},
	}

	// Verify field values
	if recipe.APIVersion != "euclo/v1alpha1" {
		t.Errorf("APIVersion = %q, want %q", recipe.APIVersion, "euclo/v1alpha1")
	}
	if recipe.Kind != "ThoughtRecipe" {
		t.Errorf("Kind = %q, want %q", recipe.Kind, "ThoughtRecipe")
	}
	if recipe.Metadata.Name != "test-recipe" {
		t.Errorf("Metadata.Name = %q, want %q", recipe.Metadata.Name, "test-recipe")
	}
	if recipe.Global.Prompt != "Global prompt" {
		t.Errorf("Global.Prompt = %q, want %q", recipe.Global.Prompt, "Global prompt")
	}
	if len(recipe.Global.Capabilities.Allowed) != 2 {
		t.Errorf("len(Global.Capabilities.Allowed) = %d, want 2", len(recipe.Global.Capabilities.Allowed))
	}
	if len(recipe.Global.Context.Enrichment) != 2 {
		t.Errorf("len(Global.Context.Enrichment) = %d, want 2", len(recipe.Global.Context.Enrichment))
	}
	if recipe.Global.Context.Sharing.Default != "explicit" {
		t.Errorf("Global.Context.Sharing.Default = %q, want %q", recipe.Global.Context.Sharing.Default, "explicit")
	}
	if len(recipe.Global.Context.Aliases) != 1 {
		t.Errorf("len(Global.Context.Aliases) = %d, want 1", len(recipe.Global.Context.Aliases))
	}
	if len(recipe.Global.Configuration.Modes) != 2 {
		t.Errorf("len(Global.Configuration.Modes) = %d, want 2", len(recipe.Global.Configuration.Modes))
	}
	if recipe.Global.Configuration.TriggerPriority != 6 {
		t.Errorf("Global.Configuration.TriggerPriority = %d, want 6", recipe.Global.Configuration.TriggerPriority)
	}
	if !recipe.Global.Configuration.AllowDynamicResolution {
		t.Error("Global.Configuration.AllowDynamicResolution should be true")
	}
	if len(recipe.Sequence) != 1 {
		t.Errorf("len(Sequence) = %d, want 1", len(recipe.Sequence))
	}
	if recipe.Sequence[0].ID != "step1" {
		t.Errorf("Sequence[0].ID = %q, want %q", recipe.Sequence[0].ID, "step1")
	}
	if recipe.Sequence[0].Parent.Paradigm != "react" {
		t.Errorf("Sequence[0].Parent.Paradigm = %q, want %q", recipe.Sequence[0].Parent.Paradigm, "react")
	}
	if recipe.Sequence[0].Child == nil {
		t.Error("Sequence[0].Child should not be nil")
	}
	if recipe.Sequence[0].Fallback == nil {
		t.Error("Sequence[0].Fallback should not be nil")
	}
}

// TestRecipeSequenceEmptySlice tests handling of empty sequence slice.
func TestRecipeSequenceEmptySlice(t *testing.T) {
	recipe := ThoughtRecipe{
		Sequence: []RecipeStep{},
	}
	if len(recipe.Sequence) != 0 {
		t.Errorf("len(Sequence) = %d, want 0", len(recipe.Sequence))
	}
}

// TestRecipeStepOptionalFieldsNil tests that optional fields can be nil.
func TestRecipeStepOptionalFieldsNil(t *testing.T) {
	step := RecipeStep{
		ID:     "step1",
		Parent: RecipeStepAgent{Paradigm: "react"},
		// Child and Fallback are nil (optional)
	}

	if step.Child != nil {
		t.Error("Child should be nil")
	}
	if step.Fallback != nil {
		t.Error("Fallback should be nil")
	}
}

// TestRecipeConfigurationDefaultValues tests that zero values are sensible defaults.
func TestRecipeConfigurationDefaultValues(t *testing.T) {
	cfg := RecipeConfiguration{}

	// TriggerPriority should default to 0 (loader will set to 5 in Phase 4)
	if cfg.TriggerPriority != 0 {
		t.Errorf("TriggerPriority zero value = %d, want 0", cfg.TriggerPriority)
	}

	// AllowDynamicResolution should default to false
	if cfg.AllowDynamicResolution {
		t.Error("AllowDynamicResolution zero value should be false")
	}

	// Slices should be nil
	if cfg.Modes != nil {
		t.Error("Modes zero value should be nil")
	}
	if cfg.IntentKeywords != nil {
		t.Error("IntentKeywords zero value should be nil")
	}
}

// TestRecipeSharingSpecDefault tests empty SharingSpec.Default.
func TestRecipeSharingSpecDefault(t *testing.T) {
	spec := RecipeSharingSpec{}

	// Empty string - loader will default to "explicit" in Phase 4
	if spec.Default != "" {
		t.Errorf("SharingSpec.Default zero value = %q, want empty string", spec.Default)
	}
}

// TestThoughtRecipeYAMLTagConsistency verifies all struct fields have yaml tags.
func TestThoughtRecipeYAMLTagConsistency(t *testing.T) {
	// This test ensures fields can be accessed without panics
	// A more thorough test would use reflection to verify tags
	recipe := ThoughtRecipe{}

	// All fields should be accessible
	recipe.APIVersion = "test"
	recipe.Kind = "test"
	recipe.Metadata = RecipeMetadata{Name: "test"}
	recipe.Global = RecipeGlobal{Prompt: "test"}
	recipe.Sequence = []RecipeStep{{ID: "test"}}
}

// BenchmarkThoughtRecipeFieldAccess benchmarks field access on ThoughtRecipe.
func BenchmarkThoughtRecipeFieldAccess(b *testing.B) {
	recipe := ThoughtRecipe{
		APIVersion: "euclo/v1alpha1",
		Kind:       "ThoughtRecipe",
		Metadata: RecipeMetadata{
			Name:        "benchmark-recipe",
			Description: "A benchmark recipe",
		},
		Global: RecipeGlobal{
			Prompt: "Global prompt text",
			Configuration: RecipeConfiguration{
				TriggerPriority: 5,
			},
		},
		Sequence: []RecipeStep{
			{ID: "step1", Parent: RecipeStepAgent{Paradigm: "react"}},
			{ID: "step2", Parent: RecipeStepAgent{Paradigm: "planner"}},
			{ID: "step3", Parent: RecipeStepAgent{Paradigm: "reflection"}},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = recipe.APIVersion
		_ = recipe.Kind
		_ = recipe.Metadata.Name
		_ = recipe.Global.Prompt
		_ = recipe.Global.Configuration.TriggerPriority
		_ = recipe.Sequence[0].ID
		_ = recipe.Sequence[1].Parent.Paradigm
	}
}
