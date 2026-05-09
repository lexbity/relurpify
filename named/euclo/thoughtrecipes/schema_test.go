package thoughtrecipe

import "testing"

func TestThoughtRecipeSchemaValidation(t *testing.T) {
	thoughtrecipe := &ThoughtRecipe{
		Metadata: ThoughtRecipeMetadata{
			Name:           "Test ThoughtRecipe",
			Families:       []string{"debug"},
			Keywords:       []string{"panic"},
			HandoffTargets: []string{"debug_followup"},
		},
	}

	if err := thoughtrecipe.Validate(); err != nil {
		t.Fatalf("expected valid thoughtrecipe to pass validation, got error: %v", err)
	}
}

func TestThoughtRecipeSchemaMissingName(t *testing.T) {
	thoughtrecipe := &ThoughtRecipe{
		Metadata: ThoughtRecipeMetadata{},
	}

	if err := thoughtrecipe.Validate(); err == nil {
		t.Fatal("expected validation error for missing name")
	}
}

func TestThoughtRecipeSchemaRejectsUnsupportedRouteKind(t *testing.T) {
	thoughtrecipe := &ThoughtRecipe{
		RouteKind: TriggerRouteKind("bootstrap"),
		Metadata: ThoughtRecipeMetadata{
			Name: "Bad Route ThoughtRecipe",
		},
	}

	if err := thoughtrecipe.Validate(); err == nil {
		t.Fatal("expected route kind validation error")
	}
}

func TestThoughtRecipeSchemaClarificationStepValidation(t *testing.T) {
	step := ThoughtRecipeStep{
		ID:   "extract",
		Type: string(ClarificationStepTypeExtract),
		Config: map[string]any{
			"output_schema_id": "clarification.answer.v1",
			"validation_mode":  "strict",
		},
	}

	cfg, err := DecodeClarificationStepConfig(step)
	if err != nil {
		t.Fatalf("DecodeClarificationStepConfig failed: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected clarification config")
	}
	if cfg.OutputSchemaID != "clarification.answer.v1" {
		t.Fatalf("unexpected output schema id: %q", cfg.OutputSchemaID)
	}
	if cfg.ValidationMode != "strict" {
		t.Fatalf("unexpected validation mode: %q", cfg.ValidationMode)
	}
}
