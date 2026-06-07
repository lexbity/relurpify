package ayenitd

import (
	"context"
	"sync/atomic"
	"testing"

	"codeburg.org/lexbit/relurpify/execution/agentenv"
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
	sm := agentenv.NewServiceManager()
	svc := &runnerService{}
	sm.RegisterWithInfo("runner", svc, agentenv.ServiceRegistrationInfo{Source: "ayenitd/service_runner_test.go", Owner: "workspace", Notes: []string{"test registration"}})
	ws := &agentenv.Workspace{ServiceManager: sm}

	if err := StartWorkspaceServices(context.Background(), ws); err != nil {
		t.Fatalf("StartWorkspaceServices returned %v", err)
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
