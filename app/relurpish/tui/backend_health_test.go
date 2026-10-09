package tui

import (
	"context"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// slowHealthRuntime delegates everything to a real adapter but delays the
// backend health probe beyond the paint budget.
type slowHealthRuntime struct {
	RuntimeAdapter
	delay time.Duration
}

func (s *slowHealthRuntime) SessionInfo() SessionInfo {
	return SessionInfo{BackendState: "checking"}
}

func (s *slowHealthRuntime) AvailableAgents() []string { return nil }

func (s *slowHealthRuntime) ProbeBackendHealth(ctx context.Context) string {
	select {
	case <-time.After(s.delay):
		return "ready"
	case <-ctx.Done():
		return "unknown(probe-timeout)"
	}
}

// TestBootDoesNotBlockOnBackendHealth: the RootModel constructor returns
// without probing the backend; the initial session snapshot carries the
// "checking" placeholder (NFR-7, AC-7).
func TestBootDoesNotBlockOnBackendHealth(t *testing.T) {
	adapter := &slowHealthRuntime{delay: 10 * time.Second}
	start := time.Now()
	m := newRootModel(context.Background(), adapter, NewDefaultSurfaceFactory())
	elapsed := time.Since(start)

	if elapsed > 500*time.Millisecond {
		t.Fatalf("constructor took %v; boot must not probe the backend synchronously", elapsed)
	}
	if m.sharedSess != nil && m.sharedSess.BackendState == "ready" {
		t.Fatal("backend was probed during construction")
	}
}

// TestBackendHealthMsgUpdatesSession: the post-first-paint refresh lands the
// probed state on the shared session.
func TestBackendHealthMsgUpdatesSession(t *testing.T) {
	adapter := &slowHealthRuntime{delay: time.Millisecond}
	m := newRootModel(context.Background(), adapter, NewDefaultSurfaceFactory())
	if m.sharedSess == nil {
		t.Fatal("shared session missing")
	}
	before := m.sharedSess.BackendState

	next, _ := m.Update(backendHealthMsg{state: "ready"})
	m2, ok := next.(RootModel)
	if !ok {
		t.Fatalf("Update returned %T", next)
	}
	if m2.sharedSess.BackendState != "ready" {
		t.Fatalf("BackendState = %q (was %q), want ready", m2.sharedSess.BackendState, before)
	}
}

// TestRefreshBackendHealthCmdNilRuntime: degraded/headless construction
// returns no refresh command.
func TestRefreshBackendHealthCmdNilRuntime(t *testing.T) {
	m := newRootModel(context.Background(), nil, NewDefaultSurfaceFactory())
	if cmd := m.refreshBackendHealthCmd(); cmd != nil {
		if msg := cmd(); msg != nil {
			t.Fatalf("nil runtime produced a health msg: %#v", msg)
		}
	}
}

// guard: the message type is a tea.Msg by construction.
var _ tea.Msg = backendHealthMsg{}
