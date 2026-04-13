package agenttest

import (
	"strings"
	"testing"
)

func TestEvaluateLatencyExpectations(t *testing.T) {
	// Test with valid constraints
	transcript := &ToolTranscriptArtifact{
		Entries: []ToolTranscriptEntry{
			{Index: 0, Tool: "file_read", DurationMS: 30},
			{Index: 1, Tool: "file_read", DurationMS: 40},
			{Index: 2, Tool: "go_test", DurationMS: 1000},
		},
	}

	expect := ExpectSpec{
		ToolCallLatencyMs: map[string]string{
			"file_read": "<50",
			"go_test":   "<2000",
		},
		MaxTotalToolTimeMs: 2000,
	}

	failures := EvaluateLatencyExpectations(expect, transcript)
	if len(failures) > 0 {
		t.Errorf("Expected no failures for valid latencies, got: %v", failures)
	}
}

func TestEvaluateLatencyExpectationsExceeds(t *testing.T) {
	// Test with exceeded constraints
	transcript := &ToolTranscriptArtifact{
		Entries: []ToolTranscriptEntry{
			{Index: 0, Tool: "file_read", DurationMS: 100},
			{Index: 1, Tool: "go_test", DurationMS: 3000},
		},
	}

	expect := ExpectSpec{
		ToolCallLatencyMs: map[string]string{
			"file_read": "<50",
			"go_test":   "<2000",
		},
		MaxTotalToolTimeMs: 100,
	}

	failures := EvaluateLatencyExpectations(expect, transcript)
	if len(failures) != 3 {
		t.Errorf("Expected 3 failures, got %d: %v", len(failures), failures)
	}
}

func TestEvaluateLatencyExpectationsNil(t *testing.T) {
	// Test with nil transcript
	expect := ExpectSpec{
		ToolCallLatencyMs: map[string]string{
			"file_read": "<50",
		},
	}

	failures := EvaluateLatencyExpectations(expect, nil)
	if failures != nil {
		t.Error("Expected nil for nil transcript")
	}
}

func TestParseLatencyConstraint(t *testing.T) {
	tests := []struct {
		constraint string
		want       int64
		wantErr    bool
	}{
		{"<50", 50, false},
		{"100", 100, false},
		{">200", 200, false},
		{"<=500", 500, false},
		{">=1000", 1000, false},
		{"  75  ", 75, false},
		{"", 0, true},
		{"invalid", 0, true},
	}

	for _, tt := range tests {
		got, err := parseLatencyConstraint(tt.constraint)
		if (err != nil) != tt.wantErr {
			t.Errorf("parseLatencyConstraint(%q) error = %v, wantErr %v", tt.constraint, err, tt.wantErr)
			continue
		}
		if got != tt.want {
			t.Errorf("parseLatencyConstraint(%q) = %d, want %d", tt.constraint, got, tt.want)
		}
	}
}

func TestComputeMaxLatency(t *testing.T) {
	transcript := &ToolTranscriptArtifact{
		Entries: []ToolTranscriptEntry{
			{Index: 0, Tool: "file_read", DurationMS: 30},
			{Index: 1, Tool: "file_read", DurationMS: 50},
			{Index: 2, Tool: "file_read", DurationMS: 40},
		},
	}

	max := computeMaxLatency(transcript, "file_read")
	if max != 50 {
		t.Errorf("Expected max 50ms, got %d", max)
	}

	// Non-existent tool
	max = computeMaxLatency(transcript, "nonexistent")
	if max != 0 {
		t.Errorf("Expected 0 for nonexistent tool, got %d", max)
	}

	// Nil transcript
	max = computeMaxLatency(nil, "file_read")
	if max != 0 {
		t.Errorf("Expected 0 for nil transcript, got %d", max)
	}
}

func TestComputeTotalToolTime(t *testing.T) {
	transcript := &ToolTranscriptArtifact{
		Entries: []ToolTranscriptEntry{
			{Index: 0, Tool: "file_read", DurationMS: 30},
			{Index: 1, Tool: "file_write", DurationMS: 50},
			{Index: 2, Tool: "go_test", DurationMS: 1000},
		},
	}

	total := computeTotalToolTime(transcript)
	if total != 1080 {
		t.Errorf("Expected total 1080ms, got %d", total)
	}

	// Nil transcript
	total = computeTotalToolTime(nil)
	if total != 0 {
		t.Errorf("Expected 0 for nil transcript, got %d", total)
	}
}

func TestBuildLatencyReport(t *testing.T) {
	transcript := &ToolTranscriptArtifact{
		Entries: []ToolTranscriptEntry{
			{Index: 0, Tool: "file_read", DurationMS: 30},
			{Index: 1, Tool: "file_read", DurationMS: 50},
			{Index: 2, Tool: "file_read", DurationMS: 40},
			{Index: 3, Tool: "go_test", DurationMS: 1000},
		},
	}

	report := BuildLatencyReport(transcript)
	if report == nil {
		t.Fatal("Expected non-nil report")
	}

	// Check total tool time
	if report.TotalToolTimeMs != 1120 {
		t.Errorf("Expected total 1120ms, got %d", report.TotalToolTimeMs)
	}

	// Check file_read stats
	fileReadStats, ok := report.ToolLatencies["file_read"]
	if !ok {
		t.Fatal("Expected file_read in report")
	}
	if fileReadStats.MinMs != 30 {
		t.Errorf("Expected min 30ms, got %d", fileReadStats.MinMs)
	}
	if fileReadStats.MaxMs != 50 {
		t.Errorf("Expected max 50ms, got %d", fileReadStats.MaxMs)
	}
	if fileReadStats.AvgMs != 40 {
		t.Errorf("Expected avg 40ms, got %d", fileReadStats.AvgMs)
	}

	// Check go_test stats
	goTestStats, ok := report.ToolLatencies["go_test"]
	if !ok {
		t.Fatal("Expected go_test in report")
	}
	if goTestStats.MinMs != 1000 || goTestStats.MaxMs != 1000 || goTestStats.AvgMs != 1000 {
		t.Errorf("Expected go_test min/max/avg 1000ms, got %d/%d/%d", goTestStats.MinMs, goTestStats.MaxMs, goTestStats.AvgMs)
	}
}

func TestBuildLatencyReportNil(t *testing.T) {
	report := BuildLatencyReport(nil)
	if report != nil {
		t.Error("Expected nil for nil transcript")
	}
}

func TestComputeLatencyStats(t *testing.T) {
	latencies := []int64{10, 20, 30, 40, 50}
	stats := computeLatencyStats(latencies)

	if stats.MinMs != 10 {
		t.Errorf("Expected min 10, got %d", stats.MinMs)
	}
	if stats.MaxMs != 50 {
		t.Errorf("Expected max 50, got %d", stats.MaxMs)
	}
	if stats.AvgMs != 30 {
		t.Errorf("Expected avg 30, got %d", stats.AvgMs)
	}
	// P95 should be the 95th percentile (roughly index 4.75, so index 4 = 50)
	if stats.P95Ms != 50 {
		t.Errorf("Expected p95 50, got %d", stats.P95Ms)
	}

	// Empty slice
	emptyStats := computeLatencyStats([]int64{})
	if emptyStats.MinMs != 0 || emptyStats.MaxMs != 0 {
		t.Error("Expected zero stats for empty slice")
	}
}

func TestComputePercentile(t *testing.T) {
	sorted := []int64{10, 20, 30, 40, 50}

	tests := []struct {
		percentile float64
		want       int64
	}{
		{0, 10},
		{50, 30},
		{95, 40}, // Index 3 (95% of 4 = 3.8, truncated to 3)
		{100, 50},
	}

	for _, tt := range tests {
		got := computePercentile(sorted, tt.percentile)
		if got != tt.want {
			t.Errorf("computePercentile(%v) = %d, want %d", tt.percentile, got, tt.want)
		}
	}

	// Empty slice
	got := computePercentile([]int64{}, 50)
	if got != 0 {
		t.Errorf("computePercentile(empty, 50) = %d, want 0", got)
	}
}

func TestFormatLatencyReport(t *testing.T) {
	report := &ToolLatencyReport{
		ToolLatencies: map[string]LatencyStats{
			"file_read": {MinMs: 10, MaxMs: 50, AvgMs: 30, P95Ms: 45},
			"go_test":   {MinMs: 1000, MaxMs: 2000, AvgMs: 1500, P95Ms: 1900},
		},
		TotalToolTimeMs: 3000,
	}

	formatted := FormatLatencyReport(report)

	expectedStrings := []string{
		"Latency Report",
		"Total Tool Time: 3000ms",
		"file_read",
		"go_test",
	}

	for _, expected := range expectedStrings {
		if !containsSubstring(formatted, expected) {
			t.Errorf("Expected report to contain %q", expected)
		}
	}
}

func TestFormatLatencyReportNil(t *testing.T) {
	formatted := FormatLatencyReport(nil)
	if formatted != "Latency Report: nil" {
		t.Errorf("Unexpected nil report format: %s", formatted)
	}
}

func TestCheckLatencyAgainstBaseline(t *testing.T) {
	// Exceeds threshold: current 101 > baseline 50 * 2.0 = 100
	current := LatencyStats{MaxMs: 101}
	baseline := LatencyStats{MaxMs: 50}

	warnings := CheckLatencyAgainstBaseline(current, baseline, "file_read", 2.0)
	if len(warnings) != 1 {
		t.Errorf("Expected 1 warning, got %d", len(warnings))
	}

	// Exactly at threshold (no warning since we use > not >=)
	current = LatencyStats{MaxMs: 100}
	warnings = CheckLatencyAgainstBaseline(current, baseline, "file_read", 2.0)
	if len(warnings) != 0 {
		t.Errorf("Expected 0 warnings at threshold, got %d", len(warnings))
	}

	// Within threshold
	current = LatencyStats{MaxMs: 90}
	warnings = CheckLatencyAgainstBaseline(current, baseline, "file_read", 2.0)
	if len(warnings) != 0 {
		t.Errorf("Expected 0 warnings, got %d", len(warnings))
	}

	// Zero baseline (should not warn)
	baseline = LatencyStats{MaxMs: 0}
	warnings = CheckLatencyAgainstBaseline(current, baseline, "file_read", 2.0)
	if len(warnings) != 0 {
		t.Errorf("Expected 0 warnings for zero baseline, got %d", len(warnings))
	}
}

func TestGetQueueTimeStats(t *testing.T) {
	transcript := &ToolTranscriptArtifact{
		Entries: []ToolTranscriptEntry{
			{Index: 0, Tool: "file_read", QueueTimeMS: 10},
			{Index: 1, Tool: "file_read", QueueTimeMS: 20},
			{Index: 2, Tool: "go_test", QueueTimeMS: 100},
		},
	}

	stats := GetQueueTimeStats(transcript, "file_read")
	if stats.MinMs != 10 || stats.MaxMs != 20 || stats.AvgMs != 15 {
		t.Errorf("Expected file_read queue min/max/avg 10/20/15, got %d/%d/%d", stats.MinMs, stats.MaxMs, stats.AvgMs)
	}

	// Non-existent tool
	stats = GetQueueTimeStats(transcript, "nonexistent")
	if stats.MinMs != 0 || stats.MaxMs != 0 {
		t.Error("Expected zero stats for nonexistent tool")
	}

	// Nil transcript
	stats = GetQueueTimeStats(nil, "file_read")
	if stats.MinMs != 0 || stats.MaxMs != 0 {
		t.Error("Expected zero stats for nil transcript")
	}
}

func containsSubstring(s, substr string) bool {
	return strings.Contains(s, substr)
}
