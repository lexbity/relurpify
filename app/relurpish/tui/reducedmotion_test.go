package tui

import (
	"testing"
)

func TestReduceMotionReduced(t *testing.T) {
	r := NewReduceMotion(true)
	if !r.Reduced() {
		t.Error("expected reduced")
	}
}

func TestReduceMotionNotReduced(t *testing.T) {
	r := NewReduceMotion(false)
	if r.Reduced() {
		t.Error("expected not reduced")
	}
}

func TestReduceMotionNilSafe(t *testing.T) {
	var r *ReduceMotion
	if r.Reduced() {
		t.Error("nil ReduceMotion should not report reduced")
	}
	fr := r.Collapse(func() AnimationFrame { return AnimationFrame{Text: "test", Done: true} })
	if fr != "test" {
		t.Errorf("nil ReduceMotion Collapse = %q, want %q", fr, "test")
	}
}

func TestReduceMotionCollapseReduced(t *testing.T) {
	restore := preserveEnv("CI", "SSH_TTY", "SSH_CONNECTION", "TERM")
	defer restore()
	os.Setenv("CI", "true")

	r := NewReduceMotion()
	if !r.Reduced() {
		t.Skip("not reduced — can't test collapse")
	}

	calls := 0
	fn := func() AnimationFrame {
		calls++
		if calls >= 3 {
			return AnimationFrame{Text: "final", Done: true}
		}
		return AnimationFrame{Text: "step", Done: false}
	}

	result := r.Collapse(fn)
	if result != "final" {
		t.Errorf("Collapse = %q, want %q", result, "final")
	}
}

func TestReduceMotionCollapseNotReduced(t *testing.T) {
	restore := preserveEnv("CI", "SSH_TTY", "SSH_CONNECTION", "TERM")
	defer restore()
	os.Unsetenv("CI")
	os.Unsetenv("SSH_TTY")
	os.Unsetenv("SSH_CONNECTION")
	os.Setenv("TERM", "xterm-256color")

	r := NewReduceMotion()
	if r.Reduced() {
		t.Skip("reduced — can't test non-reduced collapse")
	}

	result := r.Collapse(func() AnimationFrame {
		return AnimationFrame{Text: "first", Done: true}
	})
	// When not reduced, Collapse returns the first frame.
	if result != "first" {
		t.Errorf("Collapse = %q, want %q", result, "first")
	}
}

// preserveEnv saves and restores environment variables.
func preserveEnv(keys ...string) func() {
	saved := make(map[string]string)
	for _, k := range keys {
		saved[k] = os.Getenv(k)
	}
	return func() {
		for k, v := range saved {
			if v == "" {
				os.Unsetenv(k)
			} else {
				os.Setenv(k, v)
			}
		}
	}
}
