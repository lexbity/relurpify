package euclotui

import (
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/named/euclo/reporting"
)

func TestEucloEventRouterProjectsWorkflowStream(t *testing.T) {
	router := NewEucloEventRouter()

	router.ApplyExecutionEvent(ExecutionEvent{
		Header:    reporting.EventHeader{TaskID: "task-1", SessionID: "session-1"},
		Type:      reporting.EventTypeStepCompletedEuclo,
		TaskID:    "task-1",
		NodeID:    "step.inspect_parser",
		Summary:   "Step 1/3: Inspect parser package",
		Milestone: "Step 1/3: Inspect parser package",
		RecipeID:  "test.generate_unit_tests",
	})
	router.ApplyExecutionEvent(ExecutionEvent{
		Header:  reporting.EventHeader{TaskID: "task-1", SessionID: "session-1"},
		Type:    reporting.EventTypeRouteSelected,
		TaskID:  "task-1",
		NodeID:  "dispatch",
		Summary: "dispatch",
		RouteScores: map[string]float64{
			"test.generate_unit_tests": 0.91,
			"parser.cover_edge_cases":  0.77,
			"generic.implement_tests":  0.62,
		},
	})
	router.ApplyExecutionEvent(ExecutionEvent{
		Header:   reporting.EventHeader{TaskID: "task-1", SessionID: "session-1"},
		Type:     reporting.EventTypeProjectionCompleted,
		TaskID:   "task-1",
		RecipeID: "test.generate_unit_tests",
		PatchHunks: []PatchHunk{
			{
				File:         "thoughtrecipes/parser_test.go",
				Summary:      "Add parser tests",
				Body:         "+func TestParse() {}\n",
				StepID:       "step.write_patch",
				Origin:       "capability.write_patch",
				LinesAdded:   24,
				LinesRemoved: 0,
			},
		},
	})

	snap := router.Snapshot()
	if len(snap.Chat.Milestones) == 0 || !strings.Contains(snap.Chat.Milestones[0], "Inspect parser package") {
		t.Fatalf("chat milestones = %#v", snap.Chat.Milestones)
	}
	if got := len(snap.Diff.Hunks); got != 1 {
		t.Fatalf("diff hunks = %d, want 1", got)
	}
	if got := snap.Diff.Hunks[0].File; got != "thoughtrecipes/parser_test.go" {
		t.Fatalf("diff file = %q, want parser test", got)
	}
	if got := RenderChatProjection(snap.Chat); !strings.Contains(got, "Inspect parser package") {
		t.Fatalf("chat render missing milestone: %q", got)
	}
	diffPane := NewDiffPane(router, "")
	diffPane.SetSize(120, 30)
	if got := diffPane.View(); !strings.Contains(got, "thoughtrecipes/parser_test.go") {
		t.Fatalf("diff render missing file name: %q", got)
	}
}
