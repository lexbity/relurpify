package ayenitd

import (
	"context"
	"sync/atomic"
	"testing"

	"codeburg.org/lexbit/relurpify/execution/session"
)

type runnerService struct {
	startCount atomic.Int32
	stopCount  atomic.Int32
}

func (s *runnerService) Start(context.Context) error {
	s.startCount.Add(1)
	return nil
}

func (s *runnerService) Stop() error {
	s.stopCount.Add(1)
	return nil
}

func TestStartWorkspaceServicesStartsRegisteredServices(t *testing.T) {
	sm := session.NewServiceManager()
	svc := &runnerService{}
	sm.RegisterWithInfo("runner", svc, session.ServiceRegistrationInfo{Source: "test", Owner: "workspace", Notes: []string{"test registration"}})

	if err := sm.StartAll(context.Background()); err != nil {
		t.Fatalf("StartAll returned %v", err)
	}
	if svc.startCount.Load() != 1 {
		t.Fatalf("startCount = %d, want 1", svc.startCount.Load())
	}
	snapshots := sm.Snapshot()
	if len(snapshots) != 1 {
		t.Fatalf("Snapshot len = %d, want 1", len(snapshots))
	}
	if snapshots[0].Status != "running" {
		t.Fatalf("Snapshot status = %q, want %q", snapshots[0].Status, "running")
	}
	if snapshots[0].Source == "" || snapshots[0].Owner == "" {
		t.Fatalf("expected snapshot provenance, got %#v", snapshots[0])
	}
}
