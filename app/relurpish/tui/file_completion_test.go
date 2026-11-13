package tui

import (
	"context"
	"strings"
	"testing"

	capabilityports "codeburg.org/lexbit/relurpify/capability/ports"
	tea "github.com/charmbracelet/bubbletea"
)

type completionRuntime struct {
	RuntimeAdapter
	calls int
	args  []string
}

func (r *completionRuntime) InvokeCapability(ctx context.Context, name string, arguments map[string]any) (*capabilityports.ToolResult, error) {
	if _, ok := ctx.Deadline(); !ok {
		panic("unbounded completion")
	}
	if name != "cli_find" {
		panic("unexpected capability")
	}
	r.calls++
	r.args = arguments["args"].([]string)
	return &capabilityports.ToolResult{Data: map[string]any{"stdout": "/workspace/界 file.go\x00/workspace-other/escape.go\x00"}}, nil
}

func TestV2FileCompletionIsExplicitBoundedAndSpanChecked(t *testing.T) {
	rt := &completionRuntime{}
	b := NewInputBar()
	b.SetWorkspace("/workspace")
	b.SetRuntime(t.Context(), rt)
	b.SetValue("prior 界 @fi trailing text")
	b.input.SetCursor(len([]rune("prior 界 @fi")))
	b.Update(tea.KeyMsg{Type: tea.KeyLeft}, TabChat)
	if rt.calls != 0 {
		t.Fatal("typing/navigation launched a file query")
	}
	b.input.SetCursor(len([]rune("prior 界 @fi")))
	_, cmd := b.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}, Alt: true}, TabChat)
	if cmd == nil || rt.calls != 0 {
		t.Fatal("explicit completion did not defer its query")
	}
	msg := cmd().(filePickerResultMsg)
	if rt.calls != 1 || rt.args[len(rt.args)-1] != "-print0" || len(msg.Results) != 1 {
		t.Fatalf("query did not use bounded governed NUL-delimited results: %+v", msg)
	}
	b.Update(msg, TabChat)
	if !b.IsFilePickerActive() {
		t.Fatal("actual query result did not reach the chooser")
	}
	b.Update(tea.KeyMsg{Type: tea.KeyEnter}, TabChat)
	if b.Value() != "prior 界 @界 file.go trailing text" {
		t.Fatalf("Unicode/token span replacement lost trailing text: %q", b.Value())
	}
	b.SetValue("new request")
	b.Update(msg, TabChat)
	if b.IsFilePickerActive() || b.Value() != "new request" {
		t.Fatal("late file completion changed a newer draft")
	}
}

func TestV2FileCompletionTokenBytesAndCancellation(t *testing.T) {
	input := "界 @recipe suffix"
	query, start, end, ok := filePickerTokenAt(input, len("界 @rec"))
	if !ok || query != "@rec" || input[:start] != "界 " || input[end:] != " suffix" {
		t.Fatal("completion span did not use UTF-8 bytes")
	}
	for _, literal := range []string{"email@example.com", "ordinary text"} {
		if _, _, _, ok := filePickerTokenAt(literal, len(literal)); ok {
			t.Fatal("nonreference text became a token")
		}
	}
	b := NewInputBar()
	b.SetValue("@fi")
	b.SetWorkspace("/workspace")
	b.SetRuntime(t.Context(), &completionRuntime{})
	_, cmd := b.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}, Alt: true}, TabChat)
	b.Update(tea.KeyMsg{Type: tea.KeyEsc}, TabChat)
	b.Update(cmd(), TabChat)
	if b.IsFilePickerActive() || !strings.Contains(b.Value(), "@fi") {
		t.Fatal("canceled completion reopened or changed the draft")
	}
}
