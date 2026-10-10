package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/telemetry"
)

// TestAgentTestReportRunPrintsSelectionSummary runs `agenttest report --run`
// against a synthetic run directory whose telemetry carries euclo.route.selected
// events and asserts the printed selection decision statistics (AC-11/FR-22).
func TestAgentTestReportRunPrintsSelectionSummary(t *testing.T) {
	runDir := t.TempDir()
	telemetryDir := filepath.Join(runDir, "execution", "telemetry")
	if err := os.MkdirAll(telemetryDir, 0o750); err != nil {
		t.Fatal(err)
	}
	writeSelectionTelemetry(t, filepath.Join(telemetryDir, "agenttest.jsonl"))

	cmd := NewRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"agenttest", "report", "--run", runDir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	printed := out.String()
	for _, want := range []string{
		"Selection Decisions:",
		"dispatches: 3",
		"euclo.thoughtrecipe.debug_tdd_repair: 2",
		"relurpic:debug_trace: 1",
		"lattice:family_affinity: 2",
		"lattice:route_id: 1",
		"fallback rate: 33.3% (1/3)",
		"tier2 invocation rate: 66.7% (2/3)",
		"tier2 rejection rate: 50.0% (1/2)",
	} {
		if !strings.Contains(printed, want) {
			t.Fatalf("selection summary missing %q in:\n%s", want, printed)
		}
	}
}

func TestBuildSelectionSummary_NoTelemetryIsEmpty(t *testing.T) {
	summary, err := buildSelectionSummary(t.TempDir())
	if err != nil {
		t.Fatalf("empty run dir must not error: %v", err)
	}
	if summary == nil {
		t.Fatal("expected empty summary")
	}
	if summary.Total != 0 {
		t.Fatalf("expected 0 dispatches, got %d", summary.Total)
	}
}

func writeSelectionTelemetry(t *testing.T, path string) {
	t.Helper()
	events := []telemetry.Event{
		{Type: "euclo.route.selected", Metadata: map[string]any{
			"route_id":       "euclo.thoughtrecipe.debug_tdd_repair",
			"decided_by":     "lattice:family_affinity",
			"fallback_taken": false,
			"tier2_used":     true,
			"tier2_outcome":  "applied",
		}},
		{Type: "euclo.route.selected", Metadata: map[string]any{
			"route_id":       "euclo.thoughtrecipe.debug_tdd_repair",
			"decided_by":     "lattice:family_affinity",
			"fallback_taken": false,
			"tier2_used":     true,
			"tier2_outcome":  "rejected",
		}},
		{Type: "euclo.route.selected", Metadata: map[string]any{
			"route_id":       "relurpic:debug_trace",
			"decided_by":     "lattice:route_id",
			"fallback_taken": true,
			"tier2_used":     false,
			"tier2_outcome":  "",
		}},
		{Type: "euclo.route.completed", Metadata: map[string]any{"route_id": "ignored"}},
	}
	var b strings.Builder
	for _, ev := range events {
		raw, err := json.Marshal(ev)
		if err != nil {
			t.Fatal(err)
		}
		b.Write(raw)
		b.WriteString("\n")
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
}
