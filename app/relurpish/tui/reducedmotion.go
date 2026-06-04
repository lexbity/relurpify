package tui

import (
	"os"

	"github.com/muesli/termenv"
)

// ReduceMotion holds the cached result of motion-preference detection.
// Query it via Reduced() before deciding whether to skip animations.
type ReduceMotion struct {
	reduced bool
}

// NewReduceMotion auto-detects whether animations should be disabled by
// checking OS environment variables and terminal capabilities. It returns a
// cached detector safe for concurrent reads.
func NewReduceMotion() *ReduceMotion {
	r := &ReduceMotion{}
	r.detect()
	return r
}

// Reduced returns true when animations should be skipped.
func (r *ReduceMotion) Reduced() bool {
	if r == nil {
		return false
	}
	return r.reduced
}

// detect populates the reduced flag based on heuristics.
func (r *ReduceMotion) detect() {
	if r == nil {
		return
	}

	// 1. Explicit env var opt-out.
	if v := os.Getenv("RELPURIFY_REDUCE_MOTION"); v != "" {
		r.reduced = true
		return
	}

	// 2. CI environments — always reduce.
	if v := os.Getenv("CI"); v != "" {
		r.reduced = true
		return
	}
	if v := os.Getenv("GITHUB_ACTIONS"); v != "" {
		r.reduced = true
		return
	}

	// 3. Non-interactive / pipe — no terminal to animate on.
	stat, _ := os.Stdout.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		r.reduced = true
		return
	}

	// 4. SSH or remote session — local motion may not render smoothly.
	if v := os.Getenv("SSH_TTY"); v != "" {
		r.reduced = true
		return
	}
	if v := os.Getenv("SSH_CONNECTION"); v != "" {
		r.reduced = true
		return
	}

	// 5. Dumb terminal or very limited colour support.
	profile := termenv.EnvColorProfile()
	if profile == termenv.Ascii {
		r.reduced = true
		return
	}
	term := os.Getenv("TERM")
	if term == "dumb" || term == "" {
		r.reduced = true
		return
	}
}

// Collapse returns the final frame of an animation. For an animation
// producing N frames, Collapse returns the Nth frame as if the animation ran
// to completion instantly. When ReduceMotion is not active, Collapse returns
// the first frame so callers can use it as a static initial value.
func (r *ReduceMotion) Collapse(fn func() AnimationFrame) string {
	if fn == nil {
		return ""
	}
	if r == nil || !r.reduced {
		// Not reducing — return the first frame as a static placeholder.
		fr := fn()
		return fr.Text
	}
	// Reducing — run the animation to completion and return the last frame.
	var last AnimationFrame
	for {
		fr := fn()
		if fr.Done {
			last = fr
			break
		}
		last = fr
	}
	return last.Text
}
