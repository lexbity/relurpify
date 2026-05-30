package framework

import (
	"errors"
	"testing"

	"codeburg.org/lexbit/relurpify/app/relurpish/tui"
)

func TestTUIPTYSafePassesNilError(t *testing.T) {
	err := tui.PTYSafe(func() error {
		return nil
	})
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestTUIPTYSafePassesThroughError(t *testing.T) {
	want := errors.New("test error")
	got := tui.PTYSafe(func() error {
		return want
	})
	if !errors.Is(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestTUIPTYSafeRecoversPanic(t *testing.T) {
	err := tui.PTYSafe(func() error {
		panic("integration test panic")
	})
	if err == nil {
		t.Fatal("expected error from recovered panic, got nil")
	}
}

func TestTUIEditorExitMsgType(t *testing.T) {
	msg := tui.EditorExitMsg{
		Path: "/tmp/test.go",
		PID:  1234,
		Err:  nil,
	}
	if msg.Path != "/tmp/test.go" {
		t.Fatalf("expected path /tmp/test.go, got %q", msg.Path)
	}
	if msg.PID != 1234 {
		t.Fatalf("expected PID 1234, got %d", msg.PID)
	}
}

func TestTUIEditorSupervisorNilSafe(t *testing.T) {
	var s *tui.EditorSupervisor
	if s.ActivePID() != 0 {
		t.Fatal("expected nil-safe ActivePID to return 0")
	}
	if s.ActivePath() != "" {
		t.Fatal("expected nil-safe ActivePath to return empty")
	}
	if s.IsActive() {
		t.Fatal("expected nil-safe IsActive to return false")
	}
}

func TestTUIEditorSupervisorDefaultsInactive(t *testing.T) {
	s := &tui.EditorSupervisor{}
	if s.ActivePID() != 0 {
		t.Fatal("expected default ActivePID to be 0")
	}
	if s.ActivePath() != "" {
		t.Fatal("expected default ActivePath to be empty")
	}
	if s.IsActive() {
		t.Fatal("expected default IsActive to return false")
	}
}

func TestTUIEditorSupervisorOpenEditorWithEmptyPath(t *testing.T) {
	s := &tui.EditorSupervisor{}
	cmd := s.OpenEditor("")
	if cmd != nil {
		t.Fatal("expected nil cmd for empty path")
	}
}

func TestTUIEditorSupervisorOpenEditorReturnsCmd(t *testing.T) {
	s := &tui.EditorSupervisor{}
	cmd := s.OpenEditor("/tmp/integration-test.go")
	if cmd == nil {
		t.Fatal("expected non-nil cmd for valid path")
	}
}

func TestTUIEditorSupervisorActivePathAfterOpen(t *testing.T) {
	s := &tui.EditorSupervisor{}
	_ = s.OpenEditor("/tmp/integration-test.go")
	if s.ActivePath() != "/tmp/integration-test.go" {
		t.Fatalf("expected /tmp/integration-test.go, got %q", s.ActivePath())
	}
}
