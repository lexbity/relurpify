package main

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/testsuite/agenttest"
)

func TestResetPreparedRunServicesRestart(t *testing.T) {
	ws := &agentenv.Workspace{ServiceManager: agentenv.NewServiceManager()}
	svc := &countingService{}
	ws.ServiceManager.Register("alpha", svc)
	if err := resetPreparedRunServices(context.Background(), ws, agenttest.ServiceResetContract{Strategy: "restart"}); err != nil {
		t.Fatal(err)
	}
	if svc.startCount == 0 || svc.stopCount == 0 {
		t.Fatalf("expected restart to stop and start service, got start=%d stop=%d", svc.startCount, svc.stopCount)
	}
}

func TestResetPreparedRunServicesNone(t *testing.T) {
	ws := &agentenv.Workspace{ServiceManager: agentenv.NewServiceManager()}
	svc := &countingService{}
	ws.ServiceManager.Register("alpha", svc)
	if err := resetPreparedRunServices(context.Background(), ws, agenttest.ServiceResetContract{Strategy: "none"}); err != nil {
		t.Fatal(err)
	}
	if svc.startCount != 0 || svc.stopCount != 0 {
		t.Fatalf("expected none strategy to skip reset, got start=%d stop=%d", svc.startCount, svc.stopCount)
	}
}

func TestRestartPreparedRunService(t *testing.T) {
	ws := &agentenv.Workspace{ServiceManager: agentenv.NewServiceManager()}
	svc := &countingService{}
	ws.ServiceManager.Register("alpha", svc)
	if err := restartPreparedRunService(context.Background(), ws, "alpha"); err != nil {
		t.Fatal(err)
	}
	if svc.startCount == 0 || svc.stopCount == 0 {
		t.Fatalf("expected restart to stop and start service, got start=%d stop=%d", svc.startCount, svc.stopCount)
	}
}

type countingService struct {
	startCount int
	stopCount  int
}

func (s *countingService) Start(context.Context) error {
	s.startCount++
	return nil
}

func (s *countingService) Stop() error {
	s.stopCount++
	return nil
}
