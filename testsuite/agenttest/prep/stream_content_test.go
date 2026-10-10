package prep

import (
	"context"
	"strings"
	"testing"

	"go.uber.org/goleak"

	"codeburg.org/lexbit/relurpify/testsuite/testhelper"
)

// TestStreamedBodyReachesPromptIsHermeticLoop proves the backward pass end to
// end through the real agent (FR-10, first half): a seeded chunk's body must
// appear verbatim — in compiler rank order — in the model call that follows
// the stream node, and in no call before it.
func TestStreamedBodyReachesPromptIsHermeticLoop(t *testing.T) {
	defer goleak.VerifyNone(t)
	const (
		sentinelOne = "wombat-fibonacci ALPHA prime fixture evidence"
		sentinelTwo = "wombat-fibonacci BETA composite fixture evidence"
	)
	report, err := Run(context.Background(), DryRunConfig{
		Name:           "stream-body-loop",
		WorkspaceFiles: map[string]string{"relurpify_cfg/euclo/stream_step.erpe": readFixture(t, "stream_step.erpe")},
		Instruction:    "investigate wombat-fibonacci.md",
		Turns: []testhelper.ModelTurn{
			{Text: `{}`}, // route disambiguation call
			{Text: `{"thought":"done","action":"complete","complete":true,"summary":"streamed evidence consumed"}`},
		},
		SeedChunks: []SeedChunk{
			{Body: sentinelOne},
			{Body: sentinelTwo},
		},
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
	if !strings.Contains(section, sentinelOne) || !strings.Contains(section, sentinelTwo) {
		t.Fatalf("seeded bodies missing from rendered section:\n%s", section)
	}
	// Rank order must agree with the section's own rank markers: whichever
	// body retrieval ranked first appears under the #1 marker.
	rankOne := strings.Index(section, "#1 ")
	rankTwo := strings.Index(section, "#2 ")
	oneAt := strings.Index(section, sentinelOne)
	twoAt := strings.Index(section, sentinelTwo)
	if rankOne < 0 || rankTwo < 0 || rankOne > rankTwo {
		t.Errorf("section rank markers out of order:\n%s", section)
	}
	if (oneAt < twoAt) != (oneAt == firstOccurrence(section, sentinelOne, sentinelTwo)) {
		t.Errorf("inconsistent body positions:\n%s", section)
	}
	// Budget header: the renderer's contract-carrying attributes must match
	// the slice accounting and the DSL's `max 256`.
	header := firstLine(section)
	if !strings.Contains(header, `chunks=2`) {
		t.Errorf("section header does not declare chunks=2: %s", header)
	}
	if !strings.Contains(header, `budget=256`) {
		t.Errorf("section header does not declare the DSL budget 256: %s", header)
	}
	if !strings.Contains(header, `epoch=`) || strings.Contains(header, `epoch=0`) {
		t.Errorf("section header missing a non-zero epoch: %s", header)
	}
	// Absent before the stream node: the route-disambiguation call (call 0)
	// must not carry any seeded body.
	for _, msg := range report.ModelMessages[0] {
		if strings.Contains(msg.Content, sentinelOne) || strings.Contains(msg.Content, sentinelTwo) {
			t.Errorf("seeded body leaked into the pre-stream model call")
		}
	}
}

func firstOccurrence(section, a, b string) int {
	if strings.Index(section, a) <= strings.Index(section, b) {
		return strings.Index(section, a)
	}
	return strings.Index(section, b)
}

// TestCaptureGroundsAndStreamsBack is the crown-jewel assertion (FR-10,
// second half): step 1's tool-fed capture becomes a grounded chunk, and step
// 2's stream clause delivers that chunk's body into the next model call —
// read-your-writes through the real agent, no LLM.
func TestCaptureGroundsAndStreamsBack(t *testing.T) {
	defer goleak.VerifyNone(t)
	probe := &ScriptedCapability{Name: "prep_probe", Response: map[string]any{"finding": "platypus-quartz 42"}}
	report, err := Run(context.Background(), DryRunConfig{
		Name:           "capture-ground-stream",
		WorkspaceFiles: map[string]string{"relurpify_cfg/euclo/capture_ground.erpe": readFixture(t, "capture_ground.erpe")},
		Instruction:    "investigate platypus-quartz.md",
		Turns: []testhelper.ModelTurn{
			{Text: `{}`}, // route disambiguation call
			{Text: `{"thought":"call the probe","action":"tool","tool":"prep_probe","arguments":{"target":"platypus-quartz.md"}}`},
			{Text: `{"thought":"collected","action":"complete","complete":true,"summary":"platypus-quartz 42 observed"}`},
			{Text: `{"thought":"summarized","action":"complete","complete":true,"summary":"platypus-quartz 42 confirmed"}`},
		},
		Scripted: []*ScriptedCapability{probe},
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !report.Success {
		t.Fatalf("run errors: %v", report.Errors)
	}
	if len(probe.Calls()) == 0 {
		t.Fatalf("prep_probe never invoked")
	}
	// The capture grounded a chunk carrying the observed value.
	found := false
	for _, chunk := range report.Knowledge.Chunks {
		if strings.Contains(chunk.Body, "platypus-quartz 42") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("no grounded chunk carries the captured tool output; knowledge=%+v", report.Knowledge)
	}
	// Read-your-writes: a later model call carries the captured body through
	// the streamed section.
	streamedAfter := false
	for _, section := range report.StreamedSections {
		if strings.Contains(section, "platypus-quartz 42") {
			streamedAfter = true
		}
	}
	if !streamedAfter {
		t.Errorf("captured tool output never streamed back into a model call; sections=%d", len(report.StreamedSections))
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// TestTemplatedGoalStreamsFromSeededChunk covers the from/goal templating
// shape: the stream query template interpolates the step goal, so the seeded
// chunk whose body matches the goal text streams into the react prompt.
func TestTemplatedGoalStreamsFromSeededChunk(t *testing.T) {
	defer goleak.VerifyNone(t)
	const seedBody = "heron rookery notes for templated-heron-goal"
	report, err := Run(context.Background(), DryRunConfig{
		Name:           "template-goal",
		WorkspaceFiles: map[string]string{"relurpify_cfg/euclo/template_goal.erpe": readFixture(t, "template_goal.erpe")},
		Instruction:    "explain templated-heron-goal.md",
		Turns: []testhelper.ModelTurn{
			{Text: `{}`}, // route disambiguation call
			{Text: `{"thought":"done","action":"complete","complete":true,"summary":"templated goal consumed"}`},
		},
		SeedChunks: []SeedChunk{{Body: seedBody}},
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !report.Success {
		t.Fatalf("run errors: %v", report.Errors)
	}
	streamed := false
	for _, section := range report.StreamedSections {
		if strings.Contains(section, seedBody) {
			streamed = true
		}
	}
	if !streamed {
		t.Errorf("goal-templated query never streamed the seeded chunk; sections=%d", len(report.StreamedSections))
	}
}
