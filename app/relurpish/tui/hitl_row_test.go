package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestHITLRowDefaultsToInactive(t *testing.T) {
	h := &HITLRow{}
	if h.Active() {
		t.Fatal("expected row to be inactive by default")
	}
}

func TestHITLRowOpensAndCloses(t *testing.T) {
	h := &HITLRow{}
	h.Open("frame-1", "Which target?", []string{"target.go", "types.go"}, []string{"Target Go", "Types Go"})
	if !h.Active() {
		t.Fatal("expected row to be active after Open")
	}
	if got := h.FrameID(); got != "frame-1" {
		t.Fatalf("frame id = %q, want frame-1", got)
	}
	h.Close()
	if h.Active() {
		t.Fatal("expected row to be inactive after Close")
	}
}

func TestHITLRowRendersQuestionAndSlots(t *testing.T) {
	h := &HITLRow{}
	h.SetWidth(80)
	h.Open("frame-1", "Which target?", []string{"target.go", "types.go"}, []string{"Target Go", "Types Go"})
	v := h.View()
	if !strings.Contains(v, "Which target?") {
		t.Fatalf("view missing question: %s", v)
	}
	if !strings.Contains(v, "[1] Target Go") {
		t.Fatalf("view missing slot 1: %s", v)
	}
	if !strings.Contains(v, "[2] Types Go") {
		t.Fatalf("view missing slot 2: %s", v)
	}
	if !strings.Contains(v, "[esc] dismiss") {
		t.Fatalf("view missing dismiss hint: %s", v)
	}
}

func TestHITLRowCloseView(t *testing.T) {
	h := &HITLRow{}
	h.Open("frame-1", "question", []string{"a"}, []string{"A"})
	h.Close()
	if v := h.View(); v != "" {
		t.Fatalf("expected empty view after close, got %q", v)
	}
}

func TestHITLRowNilSafe(t *testing.T) {
	var h *HITLRow
	if h.Active() {
		t.Fatal("nil-safe Active should return false")
	}
	if v := h.View(); v != "" {
		t.Fatalf("nil-safe View should return empty, got %q", v)
	}
	if _, handled := h.HandleKey(tea.KeyMsg{}); handled {
		t.Fatal("nil-safe HandleKey should return handled=false")
	}
	h.SetWidth(80)
	h.Close()
	h.Open("f", "q", nil, nil)
}

func TestHITLRowNumberKeyEmitsAnswerMsg(t *testing.T) {
	h := &HITLRow{}
	h.Open("frame-1", "Pick one", []string{"review", "implement"}, []string{"Review", "Implement"})

	cmd, handled := h.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
	if !handled {
		t.Fatal("expected key 2 to be handled")
	}
	if cmd == nil {
		t.Fatal("expected non-nil command")
	}
	if h.Active() {
		t.Fatal("expected row to close after answer")
	}
	msg := cmd()
	answer, ok := msg.(HITLRowAnswerMsg)
	if !ok {
		t.Fatalf("expected HITLRowAnswerMsg, got %T", msg)
	}
	if answer.FrameID != "frame-1" {
		t.Fatalf("frame id = %q", answer.FrameID)
	}
	if answer.SlotID != "implement" {
		t.Fatalf("slot id = %q", answer.SlotID)
	}
	if answer.SlotName != "Implement" {
		t.Fatalf("slot name = %q", answer.SlotName)
	}
}

func TestHITLRowEscEmitsDismissMsg(t *testing.T) {
	h := &HITLRow{}
	h.Open("frame-1", "Pick one", []string{"review"}, []string{"Review"})

	cmd, handled := h.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("esc")})
	if !handled {
		t.Fatal("expected esc to be handled")
	}
	if cmd == nil {
		t.Fatal("expected non-nil command")
	}
	if h.Active() {
		t.Fatal("expected row to close after dismiss")
	}
	msg := cmd()
	dismiss, ok := msg.(HITLRowDismissMsg)
	if !ok {
		t.Fatalf("expected HITLRowDismissMsg, got %T", msg)
	}
	if dismiss.FrameID != "frame-1" {
		t.Fatalf("frame id = %q", dismiss.FrameID)
	}
}

func TestHITLRowNumberKeyOutOfRangeIgnored(t *testing.T) {
	h := &HITLRow{}
	h.Open("frame-1", "Pick one", []string{"only"}, []string{"Only"})

	cmd, handled := h.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("9")})
	if handled {
		t.Fatal("expected key 9 to not be handled (only 1 slot)")
	}
	if cmd != nil {
		t.Fatal("expected nil command for out-of-range key")
	}
	if !h.Active() {
		t.Fatal("expected row to remain active after unhandled key")
	}
}

func TestHITLRowCloseWhileInactiveIsNoop(t *testing.T) {
	h := &HITLRow{}
	h.Close()
	if h.Active() {
		t.Fatal("expected inactive after close")
	}
}

func TestHITLRowSetsWidth(t *testing.T) {
	h := &HITLRow{}
	h.SetWidth(100)
	h.Open("f", "q", nil, nil)
	v := h.View()
	if v == "" {
		t.Fatal("expected non-empty view after open with width")
	}
}
