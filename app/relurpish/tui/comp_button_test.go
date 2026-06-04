package tui

import (
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/app/relurpish/theme"
	tea "github.com/charmbracelet/bubbletea"
)

func TestButtonInitialState(t *testing.T) {
	b := NewButton("Start")
	if b.Label() != "Start" {
		t.Errorf("label = %q, want Start", b.Label())
	}
	if b.IsFocused() {
		t.Error("button should not be focused initially")
	}
}

func TestButtonFocusStyle(t *testing.T) {
	b := NewButton("Click")
	b.SetTheme(theme.Default())
	b.Focus()
	view := b.View()
	if !strings.Contains(view, "Click") {
		t.Errorf("view missing label: %s", view)
	}
}

func TestButtonBlurStyle(t *testing.T) {
	b := NewButton("Click")
	b.SetTheme(theme.Default())
	view := b.View()
	if !strings.Contains(view, "Click") {
		t.Errorf("view missing label: %s", view)
	}
}

func TestButtonKeyboardEnter(t *testing.T) {
	b := NewButton("Go")
	b.SetTheme(theme.Default())
	b.Focus()
	cmd, handled := b.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !handled {
		t.Fatal("enter on focused button should be handled")
	}
	if cmd == nil {
		t.Fatal("enter should produce a command")
	}
	msg := cmd()
	click, ok := msg.(ButtonClickedMsg)
	if !ok {
		t.Fatalf("command produced %T, want ButtonClickedMsg", msg)
	}
	if click.Label != "Go" {
		t.Errorf("click label = %q, want Go", click.Label)
	}
}

func TestButtonKeyboardSpace(t *testing.T) {
	b := NewButton("Stop")
	b.Focus()
	cmd, handled := b.Update(tea.KeyMsg{Type: tea.KeySpace})
	if !handled {
		t.Fatal("space on focused button should be handled")
	}
	if cmd == nil {
		t.Fatal("space should produce a command")
	}
}

func TestButtonNotFocusedIgnoreKeys(t *testing.T) {
	b := NewButton("X")
	_, handled := b.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if handled {
		t.Error("unfocused button should not handle keys")
	}
}

func TestButtonMouseClick(t *testing.T) {
	b := NewButton("Action")
	b.SetRenderPos(10, 5)
	b.SetWidth(b.RenderWidth())

	// Click on button.
	cmd, handled := b.HandleClick(12, 5)
	if !handled {
		t.Fatal("click on button should be handled")
	}
	if cmd == nil {
		t.Fatal("click should produce a command")
	}
	msg := cmd()
	click, ok := msg.(ButtonClickedMsg)
	if !ok || click.Label != "Action" {
		t.Errorf("click = %+v, want {Action nil}", click)
	}
}

func TestButtonMouseMiss(t *testing.T) {
	b := NewButton("Action")
	b.SetRenderPos(10, 5)
	b.SetWidth(b.RenderWidth())

	// Click far away.
	cmd, handled := b.HandleClick(1, 1)
	if handled {
		t.Error("click outside should not be handled")
	}
	if cmd != nil {
		t.Error("miss should not produce command")
	}
}

func TestButtonSetData(t *testing.T) {
	b := NewButton("Save")
	b.SetData("session-123")
	b.Focus()
	cmd, _ := b.Update(tea.KeyMsg{Type: tea.KeyEnter})
	msg := cmd()
	click := msg.(ButtonClickedMsg)
	if click.Data != "session-123" {
		t.Errorf("data = %v, want session-123", click.Data)
	}
}

func TestButtonBlur(t *testing.T) {
	b := NewButton("X")
	b.Focus()
	if !b.IsFocused() {
		t.Fatal("expected focused after Focus()")
	}
	b.Blur()
	if b.IsFocused() {
		t.Error("expected not focused after Blur()")
	}
}
