package session

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/telemetry"
	"codeburg.org/lexbit/relurpify/telemetry/otelleg"
)

// TestWorkspaceCloseClosesTelemetryChain proves Workspace.Close releases the OTel
// export leg it owns (flushing the providers and the segment store) and is safe
// to repeat.
func TestWorkspaceCloseClosesTelemetryChain(t *testing.T) {
	chain, err := otelleg.New(context.Background(), otelleg.Settings{
		Enabled:     true,
		FileEnabled: true,
		FileDir:     t.TempDir(),
	})
	if err != nil {
		t.Fatalf("otelleg.New: %v", err)
	}

	ws := DegradedWorkspace("close wiring test")
	ws.Telemetry = chain
	ws.TelemetryChain = chain

	if err := ws.Close(context.Background()); err != nil {
		t.Fatalf("Workspace.Close: %v", err)
	}

	// The chain is closed: a subsequent Emit is counted as a drop.
	chain.Emit(telemetry.Event{Type: telemetry.EventStateChange})
	if got := chain.SelfMetrics().Dropped; got == 0 {
		t.Fatal("Dropped = 0 after Close; the chain was not closed")
	}

	// A second Close is idempotent.
	if err := ws.Close(context.Background()); err != nil {
		t.Fatalf("second Workspace.Close: %v", err)
	}
}
