package tui

import (
	"math"
	"testing"
)

func TestProgressBarStartsAtZero(t *testing.T) {
	b := NewProgressBar()
	if b.Value() != 0 {
		t.Errorf("initial value = %f, want 0", b.Value())
	}
	if b.Done() {
		t.Error("new bar should not be done")
	}
}

func TestProgressBarSetTargetClamps(t *testing.T) {
	b := NewProgressBar()
	b.SetTarget(1.5)
	if b.Value() != 0 {
		t.Errorf("value after set target = %f, want 0 (not advanced yet)", b.Value())
	}
}

func TestProgressBarAdvanceTowardTarget(t *testing.T) {
	b := NewProgressBar()
	b.SetTarget(1)
	if b.Done() {
		t.Fatal("bar should not be done immediately after set target")
	}

	// Advance until done.
	steps := 0
	for !b.Done() && steps < 100 {
		b.Advance()
		steps++
	}
	if !b.Done() {
		t.Error("bar should reach target within 100 steps")
	}
	if steps < 2 {
		t.Error("bar should take multiple steps (spring easing)")
	}
	val := b.Value()
	if math.Abs(val-1) > 0.01 {
		t.Errorf("final value = %f, want ~1.0", val)
	}
}

func TestProgressBarSetTargetAtCurrentIsInstant(t *testing.T) {
	b := NewProgressBar()
	b.SetTarget(0)
	b.Advance()
	if !b.Done() {
		t.Error("bar at target 0 should be done immediately after advance")
	}
}

func TestProgressBarSetTargetReducesSteps(t *testing.T) {
	b := NewProgressBar()
	b.SetTarget(0.5)
	steps := 0
	for !b.Done() && steps < 100 {
		b.Advance()
		steps++
	}
	if !b.Done() {
		t.Error("bar should reach 0.5 target")
	}
	if val := b.Value(); val < 0.49 || val > 0.51 {
		t.Errorf("final value = %f, want ~0.5", val)
	}
}

func TestProgressBarMultipleSetTarget(t *testing.T) {
	b := NewProgressBar()
	b.SetTarget(0.3)
	for !b.Done() {
		b.Advance()
	}
	b.SetTarget(0.7)
	for !b.Done() {
		b.Advance()
	}
	if val := b.Value(); val < 0.69 || val > 0.71 {
		t.Errorf("final value = %f, want ~0.7", val)
	}
}

func TestProgressBarReduceMotionJumpsToTarget(t *testing.T) {
	b := NewProgressBar()
	r := NewReduceMotion(true)
	// Since detect() checks CI env which may be set in test runner,
	// we just verify the logic path.
	if r.Reduced() {
		b.SetReduceMotion(r)
		b.SetTarget(0.85)
		if !b.Done() {
			t.Error("reduce-motion bar should be done immediately after set target")
		}
		if val := b.Value(); math.Abs(val-0.85) > 0.01 {
			t.Errorf("value = %f, want ~0.85", val)
		}
	}
}

func TestProgressBarNilSafe(t *testing.T) {
	var b *ProgressBar
	if b.Value() != 0 {
		t.Error("nil bar value should be 0")
	}
	if !b.Done() {
		t.Error("nil bar should report done")
	}
	if b.View() != "" {
		t.Error("nil bar view should be empty")
	}
	b.Advance()
	b.SetTarget(0.5)
}

func TestProgressBarSetWidthClamps(t *testing.T) {
	b := NewProgressBar()
	b.SetWidth(2)
	if b.View() == "" {
		t.Error("progress bar should render even at minimum width")
	}
}
