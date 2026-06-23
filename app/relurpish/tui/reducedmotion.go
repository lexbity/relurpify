package tui

// ReduceMotion holds the cached result of motion-preference detection.
// Query it via Reduced() before deciding whether to skip animations.
type ReduceMotion struct {
	reduced bool
}

// NewReduceMotion creates a ReduceMotion with the given motion preference.
// Callers should detect motion preference (e.g. from config, env vars) and
// pass the result here. The detection must happen in the app init path, not
// in this constructor, so config-read paths stay centralized.
func NewReduceMotion(reduced bool) *ReduceMotion {
	return &ReduceMotion{reduced: reduced}
}

// Reduced returns true when animations should be skipped.
func (r *ReduceMotion) Reduced() bool {
	if r == nil {
		return false
	}
	return r.reduced
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
		// Not reducing; return the first frame as a static frame.
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
	}
	return last.Text
}
