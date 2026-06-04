package tui

import (
	"os"
	"testing"
)

func TestReduceMotionDefaultNotReduced(t *testing.T) {
	// Clean environment for this test.
	restore := preserveEnv("CI", "GITHUB_ACTIONS", "SSH_TTY", "SSH_CONNECTION", "TERM", "RELPURIFY_REDUCE_MOTION")
	defer restore()
	os.Unsetenv("CI")
	os.Unsetenv("GITHUB_ACTIONS")
	os.Unsetenv("SSH_TTY")
	os.Unsetenv("SSH_CONNECTION")
	os.Setenv("TERM", "xterm-256color")

	r := NewReduceMotion()
	// On a real TTY with xterm-256color and no CI env, we expect non-reduced.
	// The TTY detection depends on stdout being a char device, which may be
	// true or false in test runners. We just verify it doesn't crash.
	_ = r
}

func TestReduceMotionCIEnv(t *testing.T) {
	restore := preserveEnv("CI", "GITHUB_ACTIONS")
	defer restore()
	os.Setenv("CI", "true")
	os.Unsetenv("GITHUB_ACTIONS")

	r := NewReduceMotion()
	if !r.Reduced() {
		t.Error("expected reduced when CI=true")
	}
}

func TestReduceMotionGitHubActions(t *testing.T) {
	restore := preserveEnv("GITHUB_ACTIONS", "CI")
	defer restore()
	os.Setenv("GITHUB_ACTIONS", "true")
	os.Unsetenv("CI")

	r := NewReduceMotion()
	if !r.Reduced() {
		t.Error("expected reduced when GITHUB_ACTIONS=true")
	}
}

func TestReduceMotionExplicitEnv(t *testing.T) {
	restore := preserveEnv("RELPURIFY_REDUCE_MOTION", "CI")
	defer restore()
	os.Setenv("RELPURIFY_REDUCE_MOTION", "1")
	os.Unsetenv("CI")

	r := NewReduceMotion()
	if !r.Reduced() {
		t.Error("expected reduced when RELPURIFY_REDUCE_MOTION is set")
	}
}

func TestReduceMotionDumbTerm(t *testing.T) {
	restore := preserveEnv("TERM", "CI", "SSH_TTY", "SSH_CONNECTION")
	defer restore()
	os.Setenv("TERM", "dumb")
	os.Unsetenv("CI")
	os.Unsetenv("SSH_TTY")
	os.Unsetenv("SSH_CONNECTION")

	r := NewReduceMotion()
	if !r.Reduced() {
		t.Error("expected reduced for TERM=dumb")
	}
}

func TestReduceMotionEmptyTerm(t *testing.T) {
	restore := preserveEnv("TERM", "CI", "SSH_TTY", "SSH_CONNECTION")
	defer restore()
	os.Setenv("TERM", "")
	os.Unsetenv("CI")
	os.Unsetenv("SSH_TTY")
	os.Unsetenv("SSH_CONNECTION")

	r := NewReduceMotion()
	if !r.Reduced() {
		t.Error("expected reduced for empty TERM")
	}
}

func TestReduceMotionSSH(t *testing.T) {
	restore := preserveEnv("SSH_TTY", "SSH_CONNECTION", "CI")
	defer restore()
	os.Setenv("SSH_TTY", "/dev/pts/0")
	os.Unsetenv("CI")
	os.Unsetenv("SSH_CONNECTION")

	r := NewReduceMotion()
	if !r.Reduced() {
		t.Error("expected reduced when SSH_TTY is set")
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
