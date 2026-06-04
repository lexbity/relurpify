package tui

import (
	"math"

	"github.com/charmbracelet/bubbles/progress"
)

// ProgressBar is a determinate progress indicator driven by the host animation
// manager. It uses spring easing toward the target value and honours the
// reduce-motion flag by jumping directly to the target.
type ProgressBar struct {
	model    progress.Model
	target   float64
	current  float64
	animID   AnimationID
	animMgr  *AnimationManager
	reduce   *ReduceMotion
	width    int
	dirty    bool
	started  bool
}

// NewProgressBar creates a progress bar at 0 %.
func NewProgressBar() *ProgressBar {
	return &ProgressBar{
		model:   progress.New(progress.WithDefaultGradient()),
		current: 0,
		width:   40,
	}
}

// SetAnimManager links the bar to the host animation manager so it can
// register/deregister its animation tick budget.
func (b *ProgressBar) SetAnimManager(m *AnimationManager) {
	b.animMgr = m
}

// SetReduceMotion sets the reduce-motion detector. When reduced the bar jumps
// to its target value immediately instead of animating.
func (b *ProgressBar) SetReduceMotion(r *ReduceMotion) {
	b.reduce = r
}

// SetWidth sets the rendered width of the bar.
func (b *ProgressBar) SetWidth(w int) {
	if w < 4 {
		w = 4
	}
	b.width = w
	b.model.Width = w - 2
}

// SetTarget sets the target progress value, clamped to [0, 1].
func (b *ProgressBar) SetTarget(v float64) {
	if b == nil {
		return
	}
	if v < 0 {
		v = 0
	}
	if v > 1 {
		v = 1
	}
	b.target = v
	b.dirty = true
	b.started = true

	// Under reduce-motion, jump immediately.
	if b.reduce != nil && b.reduce.Reduced() {
		b.current = v
		b.dirty = false
		b.deregister()
		return
	}
	b.register()
}

// Advance moves the current value toward the target by the spring step.
// Returns true when the bar has reached its target.
func (b *ProgressBar) Advance() bool {
	if b == nil || !b.dirty {
		return true
	}
	diff := b.target - b.current
	if math.Abs(diff) < 0.005 {
		b.current = b.target
		b.dirty = false
		b.deregister()
		return true
	}
	// Spring-like easing: move 25 % of remaining distance each tick.
	b.current += diff * 0.25
	return false
}

// Value returns the current progress value.
func (b *ProgressBar) Value() float64 {
	if b == nil {
		return 0
	}
	return b.current
}

// Done reports whether the bar has reached its target after having been set.
func (b *ProgressBar) Done() bool {
	if b == nil {
		return true
	}
	if !b.started {
		return false
	}
	if b.dirty {
		return false
	}
	return b.current >= b.target
}

// View renders the progress bar as a styled string.
func (b *ProgressBar) View() string {
	if b == nil {
		return ""
	}
	return b.model.ViewAs(b.current)
}

func (b *ProgressBar) register() {
	if b.animMgr == nil || b.animID != 0 {
		return
	}
	b.animID = b.animMgr.Register(func() AnimationFrame {
		done := b.Advance()
		return AnimationFrame{Text: "", Done: done}
	})
}

func (b *ProgressBar) deregister() {
	if b.animMgr == nil || b.animID == 0 {
		return
	}
	b.animMgr.Deregister(b.animID)
	b.animID = 0
}


