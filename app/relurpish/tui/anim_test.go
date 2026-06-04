package tui

import (
	"testing"
)

func TestAnimationManagerNoTickWhenIdle(t *testing.T) {
	m := NewAnimationManager()
	if m.Active() {
		t.Error("expected no active animations after creation")
	}
	cmd := m.TickCmd()
	if cmd != nil {
		t.Error("TickCmd should return nil when no animations are registered")
	}
}

func TestAnimationManagerTickWhenActive(t *testing.T) {
	m := NewAnimationManager()
	frameCount := 0
	m.Register(func() AnimationFrame {
		frameCount++
		return AnimationFrame{Text: "tick", Done: true}
	})
	if !m.Active() {
		t.Error("expected active animations after register")
	}
	cmd := m.TickCmd()
	if cmd == nil {
		t.Fatal("TickCmd should return non-nil when animations are active")
	}
	msg := cmd()
	if _, ok := msg.(AnimationTickMsg); !ok {
		t.Fatalf("TickCmd produced %T, want AnimationTickMsg", msg)
	}
}

func TestAnimationManagerAdvanceRemovesDone(t *testing.T) {
	m := NewAnimationManager()
	m.Register(func() AnimationFrame {
		return AnimationFrame{Text: "done", Done: true}
	})
	if !m.Active() {
		t.Fatal("expected active after register")
	}
	m.Advance()
	if m.Active() {
		t.Error("expected no active animations after advancing done frame")
	}
}

func TestAnimationManagerAdvanceKeepsRunning(t *testing.T) {
	m := NewAnimationManager()
	m.Register(func() AnimationFrame {
		return AnimationFrame{Text: "running", Done: false}
	})
	m.Advance()
	if !m.Active() {
		t.Error("expected animation still active after advancing non-done frame")
	}
}

func TestAnimationManagerDeregister(t *testing.T) {
	m := NewAnimationManager()
	id := m.Register(func() AnimationFrame {
		return AnimationFrame{Text: "x", Done: true}
	})
	if !m.Active() {
		t.Fatal("expected active after register")
	}
	m.Deregister(id)
	if m.Active() {
		t.Error("expected inactive after deregister")
	}
}

func TestAnimationManagerDeregisterUnknownID(t *testing.T) {
	m := NewAnimationManager()
	// Should not panic.
	m.Deregister(999)
	m.Deregister(-1)
}

func TestAnimationManagerNilSafe(t *testing.T) {
	var m *AnimationManager
	if m.Active() {
		t.Error("nil manager should not be active")
	}
	if cmd := m.TickCmd(); cmd != nil {
		t.Error("nil manager TickCmd should be nil")
	}
	m.Advance()
	m.Deregister(1)
	id := m.Register(func() AnimationFrame { return AnimationFrame{} })
	if id != 0 {
		t.Error("nil manager Register should return 0")
	}
}

func TestAnimationManagerDedupeRegister(t *testing.T) {
	m := NewAnimationManager()
	id1 := m.Register(func() AnimationFrame { return AnimationFrame{Text: "a", Done: false} })
	id2 := m.Register(func() AnimationFrame { return AnimationFrame{Text: "b", Done: false} })
	if id1 == id2 {
		t.Error("Register should return unique IDs")
	}
	if !m.Active() {
		t.Fatal("expected active after two registrations")
	}
	m.Deregister(id1)
	if !m.Active() {
		t.Error("expected still active after deregistering one of two")
	}
	m.Deregister(id2)
	if m.Active() {
		t.Error("expected inactive after deregistering both")
	}
}

func TestAnimationManagerMultipleFrames(t *testing.T) {
	m := NewAnimationManager()
	callCount := 0
	m.Register(func() AnimationFrame {
		callCount++
		return AnimationFrame{Text: "frame", Done: callCount >= 3}
	})

	// Advance three times; on the third call the animation should be done.
	m.Advance()
	if !m.Active() {
		t.Error("expected active after first advance (count 1/3)")
	}
	m.Advance()
	if !m.Active() {
		t.Error("expected active after second advance (count 2/3)")
	}
	m.Advance()
	if m.Active() {
		t.Error("expected done after third advance")
	}
}
