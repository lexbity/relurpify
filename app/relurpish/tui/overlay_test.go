package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

type testOverlay struct {
	name      string
	rendered  string
	keys      []string
	handleErr bool
}

func (o *testOverlay) Render(width, height int) string {
	_ = width
	_ = height
	return o.rendered
}

func (o *testOverlay) HandleKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	o.keys = append(o.keys, msg.String())
	if o.handleErr {
		return nil, false
	}
	return nil, true
}

func TestOverlayStackRoutesOnlyToTop(t *testing.T) {
	stack := NewOverlayStack()
	under := &testOverlay{name: "under", rendered: "under"}
	top := &testOverlay{name: "top", rendered: "top"}

	stack.Push(under)
	stack.Push(top)

	if got := stack.Len(); got != 2 {
		t.Fatalf("stack len = %d, want 2", got)
	}
	if _, handled := stack.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}}); !handled {
		t.Fatal("expected top overlay to handle key")
	}
	if len(top.keys) != 1 || top.keys[0] != "x" {
		t.Fatalf("top keys = %#v, want [x]", top.keys)
	}
	if len(under.keys) != 0 {
		t.Fatalf("under keys = %#v, want empty", under.keys)
	}

	if _, ok := stack.Pop(); !ok {
		t.Fatal("expected pop to succeed")
	}
	if _, handled := stack.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}}); !handled {
		t.Fatal("expected remaining overlay to handle key")
	}
	if len(under.keys) != 1 || under.keys[0] != "y" {
		t.Fatalf("under keys = %#v, want [y]", under.keys)
	}
}

func TestOverlayStackRenderOrderIsDeterministic(t *testing.T) {
	stack := NewOverlayStack()
	stack.Push(&testOverlay{rendered: "first"})
	stack.Push(&testOverlay{rendered: "second"})

	got := stack.Render(80, 24)
	want := "first\nsecond"
	if got != want {
		t.Fatalf("render = %q, want %q", got, want)
	}
}

func TestOverlayStackPushExclusiveReplacesExistingEntries(t *testing.T) {
	stack := NewOverlayStack()
	first := &testOverlay{rendered: "first"}
	second := &testOverlay{rendered: "second"}

	stack.Push(first)
	stack.PushExclusive(second)

	if got := stack.Len(); got != 1 {
		t.Fatalf("stack len = %d, want 1", got)
	}
	if got := stack.Top(); got != second {
		t.Fatalf("top overlay = %#v, want second", got)
	}
	if got := stack.Render(80, 24); got != "second" {
		t.Fatalf("render = %q, want second", got)
	}
}
