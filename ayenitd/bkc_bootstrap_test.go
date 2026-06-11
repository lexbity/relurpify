package ayenitd

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/context/knowledge/ast"
)

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
