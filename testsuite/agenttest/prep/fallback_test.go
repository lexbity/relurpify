package prep

import (
	"context"
	"testing"

	"go.uber.org/goleak"

	"codeburg.org/lexbit/relurpify/testsuite/testhelper"
)

// TestBuiltinDefaultFallbackDispatches proves the built-in default execution
// route: an authored registry whose recipes match nothing (an instruction with
// no family-classifiable or keyword token) falls back to the programmatic
// `euclo.thoughtrecipe.default`, and the react paradigm runs. (A workspace
// with no recipes at all cannot even boot — Initialize fails, by design.)
func TestBuiltinDefaultFallbackDispatches(t *testing.T) {
	defer goleak.VerifyNone(t)
	report, err := Run(context.Background(), DryRunConfig{
		Name:           "fallback-default",
		WorkspaceFiles: map[string]string{"relurpify_cfg/euclo/stream_step.erpe": readFixture(t, "stream_step.erpe")},
		Instruction:    "explain summary.md",
		Turns: []testhelper.ModelTurn{
			{Text: `{"thought":"done","action":"complete","complete":true,"summary":"default route answer"}`},
		},
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !report.Success {
		t.Fatalf("run errors: %v", report.Errors)
	}
	if report.Dispatched != "euclo.thoughtrecipe.default" {
		t.Errorf("dispatched %q, want the built-in default recipe", report.Dispatched)
	}
	if !report.FallbackTaken {
		t.Errorf("expected the fallback path to be taken")
	}
	if len(report.ParadigmRuns) == 0 || len(report.ModelMessages) == 0 {
		t.Errorf("default fallback did not execute a paradigm (paradigms=%v messages=%d)", report.ParadigmRuns, len(report.ModelMessages))
	}
}
