package ayenitd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/ast"
	"codeburg.org/lexbit/relurpify/framework/knowledge"
	"github.com/stretchr/testify/require"
)

func TestWorkspaceBootstrapServiceEmitsBootstrapComplete(t *testing.T) {
	workspace := t.TempDir()
	writeTestFile(t, filepath.Join(workspace, "main.go"), "package main\nfunc main(){}\n")
	store, err := ast.NewSQLiteStore(filepath.Join(workspace, "index.db"))
	if err != nil {
		t.Fatalf("new sqlite store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	manager := ast.NewIndexManager(store, ast.IndexConfig{WorkspacePath: workspace, ParallelWorkers: 1})
	bus := &knowledge.EventBus{}
	ch, unsub := bus.Subscribe(1)
	defer unsub()
	svc := &WorkspaceBootstrapService{
		IndexManager:  manager,
		EventBus:      bus,
		WorkspaceRoot: workspace,
	}
	if err := svc.Start(context.Background()); err != nil {
		t.Fatalf("start bootstrap: %v", err)
	}
	select {
	case event := <-ch:
		if event.Kind != knowledge.EventBootstrapComplete {
			t.Fatalf("unexpected event kind: %s", event.Kind)
		}
		payload, ok := event.Payload.(knowledge.BootstrapCompletePayload)
		if !ok {
			t.Fatalf("unexpected payload type: %T", event.Payload)
		}
		if payload.WorkspaceRoot != workspace || payload.IndexedFiles == 0 {
			t.Fatalf("unexpected payload: %+v", payload)
		}
	case <-time.After(time.Second):
		t.Fatal("expected bootstrap event")
	}
}

func TestWorkspaceBootstrapServiceRespectsIndexPathFilter(t *testing.T) {
	workspace := t.TempDir()
	writeTestFile(t, filepath.Join(workspace, "keep.go"), "package main\nfunc keep(){}\n")
	writeTestFile(t, filepath.Join(workspace, "skip.go"), "package main\nfunc skip(){}\n")
	store, err := ast.NewSQLiteStore(filepath.Join(workspace, "index.db"))
	if err != nil {
		t.Fatalf("new sqlite store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	manager := ast.NewIndexManager(store, ast.IndexConfig{WorkspacePath: workspace, ParallelWorkers: 1})
	manager.SetPathFilter(func(path string, isDir bool) bool {
		if isDir {
			return true
		}
		return filepath.Base(path) != "skip.go"
	})
	svc := &WorkspaceBootstrapService{IndexManager: manager}
	if err := svc.Start(context.Background()); err != nil {
		t.Fatalf("start bootstrap: %v", err)
	}
	stats, err := manager.Stats()
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if stats.TotalFiles != 1 {
		t.Fatalf("expected one indexed file, got %d", stats.TotalFiles)
	}
}

func TestWorkspaceBootstrapServiceIdempotent(t *testing.T) {
	workspace := t.TempDir()
	writeTestFile(t, filepath.Join(workspace, "main.go"), "package main\nfunc main(){}\n")
	store, err := ast.NewSQLiteStore(filepath.Join(workspace, "index.db"))
	if err != nil {
		t.Fatalf("new sqlite store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	manager := ast.NewIndexManager(store, ast.IndexConfig{WorkspacePath: workspace, ParallelWorkers: 1})
	svc := &WorkspaceBootstrapService{IndexManager: manager}
	if err := svc.Start(context.Background()); err != nil {
		t.Fatalf("first start: %v", err)
	}
	first, err := manager.Stats()
	if err != nil {
		t.Fatalf("first stats: %v", err)
	}
	if err := svc.Start(context.Background()); err != nil {
		t.Fatalf("second start: %v", err)
	}
	second, err := manager.Stats()
	if err != nil {
		t.Fatalf("second stats: %v", err)
	}
	if first.TotalFiles != second.TotalFiles || first.TotalNodes != second.TotalNodes || first.TotalEdges != second.TotalEdges {
		t.Fatalf("expected idempotent stats, first=%+v second=%+v", first, second)
	}
}

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

func writeTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}
