package knowledge

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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

func TestBootstrapServiceStopCancelsInProgressScan(t *testing.T) {
	svc := &BootstrapService{
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

func TestBootstrapServiceStopNoop(t *testing.T) {
	require.NoError(t, (&BootstrapService{}).Stop())
	require.NoError(t, (&BootstrapService{IndexManager: &ast.IndexManager{}}).Stop())
}

// TestWorkspaceBootstrapIndexFailureDegradesNotAborts proves an operational
// indexing failure is a boot.degraded warning, never a boot abort.
func TestWorkspaceBootstrapIndexFailureDegradesNotAborts(t *testing.T) {
	rec := &bootstrapTelemetry{}
	svc := &BootstrapService{
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

// TestWorkspaceBootstrapSuccessIndexesAndEmitsComplete proves the in-process
// fallback's happy path: the index pass runs (the seed file lands in the
// index) and BootstrapComplete is emitted with the workspace root and the
// indexed-file count.
func TestWorkspaceBootstrapSuccessIndexesAndEmitsComplete(t *testing.T) {
	store, err := ast.NewTestStore(filepath.Join(t.TempDir(), "index.db"))
	require.NoError(t, err)
	defer func() { _ = store.Close() }()

	workspace := t.TempDir()
	seed := filepath.Join(workspace, "main.go")
	require.NoError(t, os.WriteFile(seed, []byte("package main\n\nfunc main() {}\n"), 0o600))

	manager := ast.NewIndexManager(store, ast.IndexConfig{WorkspacePath: workspace, ParallelWorkers: 1})
	bus := &EventBus{}
	svc := &BootstrapService{
		IndexManager:  manager,
		EventBus:      bus,
		WorkspaceRoot: workspace,
	}

	events, unsubscribe := bus.Subscribe(10)
	defer unsubscribe()

	require.NoError(t, svc.Start(context.Background()))

	// The index pass ran: the seed file is indexed.
	meta, err := store.GetFileByPath(seed)
	require.NoError(t, err)
	require.NotNil(t, meta)

	// BootstrapComplete was emitted with the stats.
	select {
	case ev := <-events:
		require.Equal(t, EventBootstrapComplete, ev.Kind)
		payload, ok := ev.Payload.(BootstrapCompletePayload)
		require.True(t, ok)
		require.Equal(t, workspace, payload.WorkspaceRoot)
		require.Equal(t, 1, payload.IndexedFiles)
	case <-time.After(time.Second):
		t.Fatal("expected bootstrap_complete event")
	}
}
