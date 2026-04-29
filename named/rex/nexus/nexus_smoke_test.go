package nexus

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	rexmanifest "codeburg.org/lexbit/relurpify/named/rex/config"
	"codeburg.org/lexbit/relurpify/named/rex/proof"
	"codeburg.org/lexbit/relurpify/named/rex/runtime"
	"codeburg.org/lexbit/relurpify/named/rex/store"
)

type stubManagedRuntime struct {
	projection   Projection
	capabilities []string
}

func (s *stubManagedRuntime) Execute(context.Context, *core.Task, *contextdata.Envelope) (*core.Result, error) {
	return &core.Result{Success: true}, nil
}

func (s *stubManagedRuntime) RuntimeProjection() Projection { return s.projection }
func (s *stubManagedRuntime) Capabilities() []string {
	return append([]string{}, s.capabilities...)
}

func TestBuildProjectionAndAdapter(t *testing.T) {
	memStore := storeMust(t)
	manager := runtime.New(rexmanifest.Default(), memStore)
	finish := manager.BeginExecution("wf-1", "run-1")
	finish(nil)
	projection := BuildProjection(manager, proof.ProofSurface{RouteFamily: "react"})
	if projection.WorkflowID != "wf-1" || projection.RunID != "run-1" {
		t.Fatalf("unexpected projection: %+v", projection)
	}

	adapter := NewAdapter("rex", &stubManagedRuntime{projection: projection, capabilities: []string{"plan"}}, memStore)
	if adapter.Registration().Name != "rex" {
		t.Fatalf("unexpected registration")
	}
}

func storeMust(t *testing.T) *store.SQLiteWorkflowStore {
	t.Helper()
	s, err := store.NewSQLiteWorkflowStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewSQLiteWorkflowStore: %v", err)
	}
	return s
}
