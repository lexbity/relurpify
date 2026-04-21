package runtime

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
)

func TestApplyEditIntentArtifacts_SynthesizesExecutedEditsFromPipelineCodeFinalOutput(t *testing.T) {
	state := core.NewContext()
	state.Set("pipeline.code", map[string]any{
		"summary": "file_write applied the requested changes",
		"final_output": map[string]any{
			"result": map[string]any{
				"file_write": map[string]any{
					"success": true,
					"data": map[string]any{
						"path": "testsuite/fixtures/strings.go",
					},
				},
				"go_test": map[string]any{
					"success": true,
					"data": map[string]any{
						"summary": "ok",
					},
				},
			},
		},
	})

	record, err := ApplyEditIntentArtifacts(context.Background(), nil, state)
	if err != nil {
		t.Fatalf("ApplyEditIntentArtifacts returned error: %v", err)
	}
	if record == nil {
		t.Fatal("expected edit execution record to be synthesized")
	}
	if len(record.Executed) != 1 {
		t.Fatalf("expected one executed edit, got %d", len(record.Executed))
	}
	if record.Executed[0].Path != "testsuite/fixtures/strings.go" {
		t.Fatalf("unexpected path %q", record.Executed[0].Path)
	}
	if record.Executed[0].Tool != "file_write" {
		t.Fatalf("unexpected tool %q", record.Executed[0].Tool)
	}
	if record.Executed[0].Action != "update" {
		t.Fatalf("unexpected action %q", record.Executed[0].Action)
	}
	if _, ok := state.Get("euclo.edit_execution"); !ok {
		t.Fatal("expected synthesized edit execution to be stored in state")
	}
}
