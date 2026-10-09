package session

import (
	"context"
	"path/filepath"
	"testing"

	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

// TestWorkspaceCloseClosesTelemetryFileSink proves Workspace.Close releases the
// JSONL file sink owned by the telemetry chain rather than leaking the handle.
func TestWorkspaceCloseClosesTelemetryFileSink(t *testing.T) {
	sink, err := telemetry.NewJSONFileTelemetry(filepath.Join(t.TempDir(), "telemetry.jsonl"))
	if err != nil {
		t.Fatalf("NewJSONFileTelemetry: %v", err)
	}

	ws := DegradedWorkspace("close wiring test")
	ws.Telemetry = telemetry.MultiplexTelemetry{
		Sinks: []telemetry.Telemetry{telemetry.LoggerTelemetry{}, sink},
	}

	if err := ws.Close(context.Background()); err != nil {
		t.Fatalf("Workspace.Close: %v", err)
	}

	// A closed sink drops rather than writes; DroppedTotal proves the wiring.
	sink.Emit(telemetry.Event{Type: telemetry.EventGraphStart})
	if got := sink.DroppedTotal(); got != 1 {
		t.Fatalf("DroppedTotal after Close = %d, want 1 (file sink not closed)", got)
	}
}
