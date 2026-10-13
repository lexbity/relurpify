package otlptest

import (
	"testing"

	"codeburg.org/lexbit/relurpify/telemetry"
)

func TestWriteSegmentsThenLoadRecordsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	WriteSegments(t, dir, []telemetry.Event{
		{Type: "tool_call", Message: "shell", Metadata: map[string]any{"tool": "shell", "exit_code": 0}},
		{Type: "scheduler.tick"},
		{Type: "tool_call", Message: "edit", Metadata: map[string]any{"tool": "edit"}},
	})

	records := LoadRecords(t, dir)
	if len(records) != 3 {
		t.Fatalf("records = %d, want 3", len(records))
	}
	calls := RecordsNamed(records, "tool_call")
	if len(calls) != 2 {
		t.Fatalf("tool_call records = %d, want 2", len(calls))
	}
	if got := calls[0].Attr("tool"); got != "shell" {
		t.Fatalf("tool attr = %q, want %q", got, "shell")
	}
	if code, ok := calls[0].Attributes["exit_code"].(int64); !ok || code != 0 {
		t.Fatalf("exit_code attr = %#v, want int64(0)", calls[0].Attributes["exit_code"])
	}
}
