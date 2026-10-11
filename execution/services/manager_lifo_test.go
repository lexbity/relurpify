package services

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

// lifoService records its stop order and can block in Stop until released,
// proving stop-outside-lock and the LIFO sequence.
type lifoService struct {
	id      string
	stopped chan<- string
	gate    chan struct{}
	inStop  chan<- struct{}
}

func (s *lifoService) Start(context.Context) error { return nil }

func (s *lifoService) Stop() error {
	if s.inStop != nil {
		s.inStop <- struct{}{}
	}
	if s.gate != nil {
		<-s.gate
	}
	s.stopped <- s.id
	return nil
}

// TestStopAllStopsInReverseRegistrationOrder proves the LIFO contract (D7):
// services register spool-watcher → executor → status-writer and stop
// status-writer → executor → spool-watcher, so the status writer's final
// drain flush runs after the executor has drained.
func TestStopAllStopsInReverseRegistrationOrder(t *testing.T) {
	defer goleak.VerifyNone(t)
	sm := NewServiceManager()
	stopped := make(chan string, 3)
	a := &lifoService{id: "first", stopped: stopped}
	b := &lifoService{id: "second", stopped: stopped}
	c := &lifoService{id: "third", stopped: stopped}
	sm.Register("first", a)
	sm.Register("second", b)
	sm.Register("third", c)

	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, sm.StartAll(ctx))
	cancel()
	require.NoError(t, sm.StopAll())

	require.Equal(t, "third", <-stopped)
	require.Equal(t, "second", <-stopped)
	require.Equal(t, "first", <-stopped)
}

// TestDeregisterDoesNotHoldLockAcrossStop proves D7's lock contract: a Stop
// that runs while Deregister is in flight can still take the manager lock
// (Snapshot) — a Stop joining workers must never stall observers.
func TestDeregisterDoesNotHoldLockAcrossStop(t *testing.T) {
	defer goleak.VerifyNone(t)
	sm := NewServiceManager()
	inStop := make(chan struct{})
	gate := make(chan struct{})
	stopped := make(chan string, 1)
	sm.Register("slow", &lifoService{id: "slow", stopped: stopped, gate: gate, inStop: inStop})

	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, sm.StartAll(ctx))
	cancel()

	done := make(chan struct{})
	go func() {
		sm.Deregister("slow")
		close(done)
	}()
	<-inStop // Deregister is inside the service's Stop

	// While Stop blocks, the manager lock is free: Snapshot returns promptly.
	snapshotDone := make(chan []ServiceSnapshot, 1)
	go func() { snapshotDone <- sm.Snapshots() }()
	select {
	case <-snapshotDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Deregister held the manager lock across Stop")
	}

	close(gate)
	<-stopped
	<-done
}
