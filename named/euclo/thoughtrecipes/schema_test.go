package thoughtrecipe

import (
	"testing"

	"codeburg.org/lexbit/relurpify/named/euclo/surface"
)

func TestThoughtRecipeSchemaClarificationStepValidation(t *testing.T) {
	step := surface.ThoughtRecipeStep{
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
