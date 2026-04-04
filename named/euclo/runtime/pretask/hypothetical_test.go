package pretask

import (
	"context"
	"testing"
)

func TestHypotheticalGenerator_Generate(t *testing.T) {
	generator := &HypotheticalGenerator{}
	
	stage1 := Stage1Result{
		CodeEvidence: []CodeEvidenceItem{
			{Path: "test.go", Summary: "Test file"},
		},
	}
	
	sketch, err := generator.Generate(context.Background(), "test query", stage1)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	
	// With nil model and embedder, should return not grounded
	if sketch.Grounded {
		t.Error("Expected not grounded with nil dependencies")
	}
}
