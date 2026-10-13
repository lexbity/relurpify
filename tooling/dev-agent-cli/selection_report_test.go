package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/telemetry"
	oteltest "codeburg.org/lexbit/relurpify/telemetry/otelleg/otlptest"
)

// goldenSelectionEvents is the recorded route-decision fixture. The expected
// summary below is frozen from the pre-cutover NDJSON parser's assertions —
// the OTLP-JSON parser must derive identical statistics from the same events
// (FR-12/AC-4 parity).
//
//nolint:gochecknoglobals // the golden fixture is deliberately shared by every parity test
var goldenSelectionEvents = []telemetry.Event{
	{Type: eventRouteSelected, Metadata: map[string]any{
		attrRouteID:       "euclo.thoughtrecipe.debug_tdd_repair",
		attrDecidedBy:     "lattice:family_affinity",
		attrFallbackTaken: false,
		attrTier2Used:     true,
		attrTier2Outcome:  "applied",
	}},
	{Type: eventRouteSelected, Metadata: map[string]any{
		attrRouteID:       "euclo.thoughtrecipe.debug_tdd_repair",
		attrDecidedBy:     "lattice:family_affinity",
		attrFallbackTaken: false,
		attrTier2Used:     true,
		attrTier2Outcome:  "rejected",
	}},
	{Type: eventRouteSelected, Metadata: map[string]any{
		attrRouteID:       "relurpic:debug_trace",
		attrDecidedBy:     "lattice:route_id",
		attrFallbackTaken: true,
		attrTier2Used:     false,
		attrTier2Outcome:  "",
	}},
	{Type: "euclo.route.completed", Metadata: map[string]any{attrRouteID: "ignored"}},
}

// goldenSelectionWant is the frozen pre-cutover summary of
// goldenSelectionEvents; every selection test asserts against it.
//
//nolint:gochecknoglobals // the frozen pre-cutover expectations are deliberately shared
var goldenSelectionWant = []string{
	"Selection Decisions:",
	"dispatches: 3",
	"euclo.thoughtrecipe.debug_tdd_repair: 2",
	"relurpic:debug_trace: 1",
	"lattice:family_affinity: 2",
	"lattice:route_id: 1",
	"fallback rate: 33.3% (1/3)",
	"tier2 invocation rate: 66.7% (2/3)",
	"tier2 rejection rate: 50.0% (1/2)",
}

// TestAgentTestReportRunPrintsSelectionSummary runs `agenttest report --run`
// against a synthetic run directory whose OTLP-JSON segments carry
// euclo.route.selected records and asserts the printed selection decision
// statistics (AC-11/FR-22) against the frozen golden values.
func TestAgentTestReportRunPrintsSelectionSummary(t *testing.T) {
	runDir := t.TempDir()
	telemetryDir := filepath.Join(runDir, "execution", "telemetry")
	if err := os.MkdirAll(telemetryDir, 0o750); err != nil {
		t.Fatal(err)
	}
	oteltest.WriteSegments(t, telemetryDir, goldenSelectionEvents)

	cmd := NewRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"agenttest", "report", "--run", runDir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	printed := out.String()
	for _, want := range goldenSelectionWant {
		if !strings.Contains(printed, want) {
			t.Fatalf("selection summary missing %q in:\n%s", want, printed)
		}
	}
}

// TestSelectionReportParity proves the golden fixture through the summary API
// directly: the new OTLP-JSON parser yields the exact statistics the old
// NDJSON parser was frozen to assert (FR-12/AC-4).
func TestSelectionReportParity(t *testing.T) {
	runDir := t.TempDir()
	telemetryDir := filepath.Join(runDir, "telemetry")
	if err := os.MkdirAll(telemetryDir, 0o750); err != nil {
		t.Fatal(err)
	}
	oteltest.WriteSegments(t, telemetryDir, goldenSelectionEvents)

	summary, err := buildSelectionSummary(runDir)
	if err != nil {
		t.Fatalf("buildSelectionSummary: %v", err)
	}
	if summary.FilesScanned != 1 || summary.Total != 3 {
		t.Fatalf("files=%d total=%d, want 1/3", summary.FilesScanned, summary.Total)
	}
	if summary.RouteHistogram["euclo.thoughtrecipe.debug_tdd_repair"] != 2 ||
		summary.RouteHistogram["relurpic:debug_trace"] != 1 {
		t.Fatalf("route histogram drifted: %v", summary.RouteHistogram)
	}
	if summary.DecidedByHistogram["lattice:family_affinity"] != 2 ||
		summary.DecidedByHistogram["lattice:route_id"] != 1 {
		t.Fatalf("decided-by histogram drifted: %v", summary.DecidedByHistogram)
	}
	if summary.FallbackCount != 1 || summary.Tier2UsedCount != 2 || summary.Tier2RejectionCount != 1 {
		t.Fatalf("counts drifted: fallback=%d tier2=%d rejections=%d",
			summary.FallbackCount, summary.Tier2UsedCount, summary.Tier2RejectionCount)
	}
	if summary.DegradationCount != 0 {
		t.Fatalf("degradation count = %d, want 0", summary.DegradationCount)
	}
}

// TestSelectionReportSkipsGarbage interleaves span and metric batches, a
// truncated line, and foreign JSON between valid segments: the summary is
// unaffected (D11 leniency).
func TestSelectionReportSkipsGarbage(t *testing.T) {
	runDir := t.TempDir()
	telemetryDir := filepath.Join(runDir, "telemetry")
	if err := os.MkdirAll(telemetryDir, 0o750); err != nil {
		t.Fatal(err)
	}
	oteltest.WriteSegments(t, telemetryDir, goldenSelectionEvents)

	segment := filepath.Join(telemetryDir, oteltest.SegmentName+".jsonl")
	raw, err := os.ReadFile(segment) //nolint:gosec // segment path is the test's own temp dir
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	garbage := []string{
		`{"resourceSpans":{"scopeSpans":[{"spans":[{"name":"tool shell"}]}]}}`,
		`{"resourceMetrics":{"scopeMetrics":[{"metrics":[{"name":"relurpify.llm.tokens"}]}]}}`,
		`{"resourceLogs":{"scopeLogs":[`, // truncated write
		`this is not json at all`,
	}
	mixed := append([]string{}, lines[:1]...)
	mixed = append(mixed, garbage...)
	mixed = append(mixed, lines[1:]...)
	if err := os.WriteFile(segment, []byte(strings.Join(mixed, "\n")+"\n"), 0o600); err != nil { //nolint:gosec,G703 // segment path is the test's own temp dir
		t.Fatal(err)
	}

	summary, err := buildSelectionSummary(runDir)
	if err != nil {
		t.Fatalf("buildSelectionSummary: %v", err)
	}
	if summary.Total != 3 {
		t.Fatalf("dispatches = %d, want 3 (garbage must be skipped)", summary.Total)
	}
	if summary.RouteHistogram["euclo.thoughtrecipe.debug_tdd_repair"] != 2 {
		t.Fatalf("route histogram drifted under garbage: %v", summary.RouteHistogram)
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
