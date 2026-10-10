package prep

import (
	"context"
	"strings"
	"testing"

	"go.uber.org/goleak"

	"codeburg.org/lexbit/relurpify/testsuite/testhelper"
)

// TestGroundingDeduplicatesIdenticalContent pins the ContentHash upsert
// (FR-11): grounding the identical capture value twice must not create a
// second chunk — the store keeps one identity, and the second commit reports
// the pre-existing entry.
func TestGroundingDeduplicatesIdenticalContent(t *testing.T) {
	defer goleak.VerifyNone(t)
	const body = "identical-capybara dedupe probe body"
	report, err := Run(context.Background(), DryRunConfig{
		Name:           "dedupe",
		WorkspaceFiles: map[string]string{"relurpify_cfg/euclo/stream_step.erpe": readFixture(t, "stream_step.erpe")},
		Instruction:    "investigate wombat-fibonacci.md",
		Turns: []testhelper.ModelTurn{
			{Text: `{}`}, // route disambiguation call
			{Text: `{"thought":"done","action":"complete","complete":true,"summary":"ok"}`},
		},
		SeedChunks: []SeedChunk{
			{Body: body},
			{Body: body},
		},
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !report.Success {
		t.Fatalf("run errors: %v", report.Errors)
	}
	matches := 0
	var ids []string
	for _, chunk := range report.Knowledge.Chunks {
		if strings.Contains(chunk.Body, body) {
			matches++
			ids = append(ids, chunk.ID)
		}
	}
	if matches != 1 {
		t.Errorf("identical content grounded %d chunks, want 1 (ContentHash upsert); ids=%v", matches, ids)
	}
	if matches == 1 && ids[0] == "" {
		t.Errorf("grounded chunk carries no identity")
	}
}
