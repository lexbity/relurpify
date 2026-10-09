package ayenitd

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/context/knowledge/ast"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

// bootstrapTelemetry records emitted framework events.
type bootstrapTelemetry struct {
	events []telemetry.Event
}

func (b *bootstrapTelemetry) Emit(ev telemetry.Event) { b.events = append(b.events, ev) }

func TestWorkspaceBootstrapServiceStopCancelsInProgressScan(t *testing.T) {
	svc := &WorkspaceBootstrapService{
		IndexManager: &ast.IndexManager{},
		IndexWorkspace: func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		},
	}
	done := make(chan error, 1)
	go func() {
		done <- svc.Start(context.Background())
	}()
	time.Sleep(20 * time.Millisecond)
	if err := svc.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context cancellation, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("expected bootstrap service to stop")
	}
}

func TestWorkspaceBootstrapServiceStopNoop(t *testing.T) {
	require.NoError(t, (&WorkspaceBootstrapService{}).Stop())
	require.NoError(t, (&WorkspaceBootstrapService{IndexManager: &ast.IndexManager{}}).Stop())
}

// TestWorkspaceBootstrapIndexFailureDegradesNotAborts proves an operational
// indexing failure is a boot.degraded warning, never a boot abort.
func TestWorkspaceBootstrapIndexFailureDegradesNotAborts(t *testing.T) {
	rec := &bootstrapTelemetry{}
	svc := &WorkspaceBootstrapService{
		IndexManager:   &ast.IndexManager{},
		Telemetry:      rec,
		IndexWorkspace: func(context.Context) error { return errors.New("scan failed") },
	}
	require.NoError(t, svc.Start(context.Background()))
	require.Len(t, rec.events, 1)
	require.Equal(t, telemetry.EventBootDegraded, rec.events[0].Type)
	require.Equal(t, "knowledge_services", rec.events[0].Metadata["reason"])
	require.Equal(t, "knowledge.bootstrap", rec.events[0].Metadata["service"])
}
