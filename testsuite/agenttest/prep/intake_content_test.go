package prep

import (
	"context"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/testsuite/testhelper"
)

// TestIntakeDeliversSeededBodyIntoFirstParadigmCall is the D5 end-to-end gate:
// a recipe with NO stream clause gets its entire streamed context from the
// INTAKE compile, and the first paradigm call renders it. The section header
// carries intake's signature — the run-level max-token budget (8192), not any
// DSL stream budget — proving the producer is the intake compile and not a
// stream node. Epoch 0 is the honest cold value: no barrier has closed when
// the intake compile lands (D1).
func TestIntakeDeliversSeededBodyIntoFirstParadigmCall(t *testing.T) {
	const sentinel = "wombat-intake-row ALPHA seeded intake evidence"
	report, err := Run(context.Background(), DryRunConfig{
		Name:           "intake-first",
		WorkspaceFiles: map[string]string{"relurpify_cfg/euclo/intake_first.erpe": readFixture(t, "intake_first.erpe")},
		Instruction:    "investigate wombat-intake-row.md",
		Turns: []testhelper.ModelTurn{
			{Text: `{}`}, // route disambiguation call
			{Text: `{"thought":"done","action":"complete","complete":true,"summary":"intake evidence consumed"}`},
		},
		SeedChunks: []SeedChunk{{Body: sentinel}},
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !report.Success {
		t.Fatalf("run errors: %v", report.Errors)
	}
	if len(report.StreamedSections) == 0 {
		t.Fatalf("no streamed section rendered into any model call")
	}
	section := report.StreamedSections[0]
	if !strings.Contains(section, sentinel) {
		t.Fatalf("seeded body missing from the intake-delivered section:\n%s", section)
	}
	header := firstLine(section)
	if !strings.Contains(header, `chunks=1`) {
		t.Errorf("intake section header does not declare chunks=1: %s", header)
	}
	if !strings.Contains(header, `budget=8192`) {
		t.Errorf("intake section header does not carry the intake budget 8192 (a DSL stream node would declare its own): %s", header)
	}
	// Exactly one producer: the fixture has no stream clause, so the intake
	// slice is the run's only streamed context.
	if len(report.StreamedSections) != 1 {
		t.Errorf("streamed sections = %d, want exactly the intake-delivered one", len(report.StreamedSections))
	}
}
