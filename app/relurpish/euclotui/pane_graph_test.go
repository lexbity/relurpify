package euclotui

import (
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/named/euclo/reporting"
	tea "github.com/charmbracelet/bubbletea"
)

func TestGraphPaneRendersTimelineAndDetails(t *testing.T) {
	router := NewEucloEventRouter()
	router.ApplyExecutionEvent(ExecutionEvent{
		NodeID:    "parse",
		Summary:   "Parse input",
		Milestone: "Parse input",
		Type:      reporting.EventTypeStepCompletedEuclo,
		RouteScores: map[string]float64{
			"main": 0.71,
		},
	})
	router.ApplyExecutionEvent(ExecutionEvent{
		NodeID:    "dispatch",
		Summary:   "Dispatch work",
		Milestone: "Dispatch work",
		Type:      reporting.EventTypeRouteSelected,
		RouteScores: map[string]float64{
			"dispatch": 0.91,
			"fallback": 0.03,
		},
	})
	router.ApplyExecutionEvent(ExecutionEvent{
		NodeID:    "finalize",
		Summary:   "Finalize",
		Milestone: "Finalize",
		Type:      reporting.EventTypeExecutionComplete,
	})
	router.graph.Nodes["parse"].Status = "completed"
	router.graph.Nodes["dispatch"].Status = "running"
	router.graph.Nodes["finalize"].Status = "failed"
	router.graph.Nodes["skip"] = &GraphNodeProjection{ID: "skip", Label: "Skipped branch", Status: "skipped"}
	router.graph.Order = append(router.graph.Order, "skip")

	pane := NewGraphPane(router)
	pane.SetSize(120, 30)

	view := pane.View()
	if !strings.Contains(view, "Execution DAG") {
		t.Fatalf("view missing DAG header: %q", view)
	}
	if !strings.Contains(view, "Parse input") || !strings.Contains(view, "Dispatch work") {
		t.Fatalf("view missing graph nodes: %q", view)
	}
	if !strings.Contains(view, "0.71") {
		t.Fatalf("view missing route scores: %q", view)
	}
	if !strings.Contains(view, "Node Detail") {
		t.Fatalf("view missing detail panel: %q", view)
	}
	if !strings.Contains(view, "○") {
		t.Fatalf("view missing skipped node icon: %q", view)
	}

	pane.Update(tea.KeyMsg{Type: tea.KeyDown})
	view = pane.View()
	if !strings.Contains(view, "Dispatch work") {
		t.Fatalf("detail panel did not advance to second node: %q", view)
	}
	if !strings.Contains(view, "dispatch: 0.91") {
		t.Fatalf("detail panel missing active node scores: %q", view)
	}
}
