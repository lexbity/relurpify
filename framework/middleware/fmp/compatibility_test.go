package fmp

import (
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
)

func TestValidateVersionSkewUsesNaturalVersionOrderingForRuntimeVersions(t *testing.T) {
	t.Parallel()

	window := CompatibilityWindow{
		ContextClass:      "workflow-runtime",
		MinRuntimeVersion: "1.2.0",
		MaxRuntimeVersion: "1.10.0",
	}
	if refusal := ValidateVersionSkew(window, "fmp.context.v1", "1.9.0"); refusal != nil {
		t.Fatalf("expected runtime version within range, got %+v", refusal)
	}
	if refusal := ValidateVersionSkew(window, "fmp.context.v1", "1.11.0"); refusal == nil || refusal.Code != core.RefusalIncompatibleRuntime {
		t.Fatalf("expected incompatible runtime refusal, got %+v", refusal)
	}
}

func TestValidateVersionSkewHandlesSchemaVersionSuffixes(t *testing.T) {
	t.Parallel()

	window := CompatibilityWindow{
		ContextClass:      "workflow-runtime",
		MinSchemaVersion:  "fmp.context.v2",
		MaxSchemaVersion:  "fmp.context.v12",
		MinRuntimeVersion: "1.0.0",
		MaxRuntimeVersion: "2.0.0",
	}
	if refusal := ValidateVersionSkew(window, "fmp.context.v10", "1.5.0"); refusal != nil {
		t.Fatalf("expected schema version within range, got %+v", refusal)
	}
	if refusal := ValidateVersionSkew(window, "fmp.context.v13", "1.5.0"); refusal == nil || refusal.Code != core.RefusalIncompatibleRuntime {
		t.Fatalf("expected schema max-version refusal, got %+v", refusal)
	}
}
