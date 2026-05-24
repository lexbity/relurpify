package tui

import (
	"testing"
)

func TestEditorSupervisorNilSafe(t *testing.T) {
	var s *EditorSupervisor
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

func TestEditorSupervisorDefaultsInactive(t *testing.T) {
	s := &EditorSupervisor{}
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

func TestEditorSupervisorOpenEditorWithEmptyPath(t *testing.T) {
	s := &EditorSupervisor{}
	cmd := s.OpenEditor("")
	if cmd != nil {
		t.Fatal("expected nil cmd for empty path")
	}
}

func TestEditorSupervisorOpenEditorReturnsCmd(t *testing.T) {
	s := &EditorSupervisor{}
	cmd := s.OpenEditor("/tmp/test.go")
	if cmd == nil {
		t.Fatal("expected non-nil cmd for valid path")
	}
}

func TestEditorSupervisorActivePathAfterOpen(t *testing.T) {
	s := &EditorSupervisor{}
	_ = s.OpenEditor("/tmp/test.go")
	if s.ActivePath() != "/tmp/test.go" {
		t.Fatalf("expected /tmp/test.go, got %q", s.ActivePath())
	}
}

func TestEditorSupervisorMultipleOpenUpdates(t *testing.T) {
	s := &EditorSupervisor{}
	_ = s.OpenEditor("/tmp/a.go")
	if s.ActivePath() != "/tmp/a.go" {
		t.Fatalf("expected /tmp/a.go, got %q", s.ActivePath())
	}
	_ = s.OpenEditor("/tmp/b.go")
	if s.ActivePath() != "/tmp/b.go" {
		t.Fatalf("expected /tmp/b.go after second open, got %q", s.ActivePath())
	}
}

func TestEditFileCmdWithEmptyPath(t *testing.T) {
	cmd := editFileCmd("")
	if cmd != nil {
		t.Fatal("expected nil cmd for empty path")
	}
}

func TestEditFileCmdWithValidPath(t *testing.T) {
	cmd := editFileCmd("/tmp/test.go")
	if cmd == nil {
		t.Fatal("expected non-nil cmd for valid path")
	}
}

func TestEditorBasename(t *testing.T) {
	got := editorBasename("/some/dir/file.go")
	want := "file.go"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestEditorBasenameEmpty(t *testing.T) {
	got := editorBasename("")
	// filepath.Base("") returns ".", so we accept both empty and "."
	if got != "" && got != "." {
		t.Fatalf("expected empty or '.', got %q", got)
	}
}
