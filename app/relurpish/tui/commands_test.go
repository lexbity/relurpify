package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestParseInputDraftPrefixes(t *testing.T) {
	draft := parseInputDraft("> write docs", TabChat, false)
	if draft.prefix != ">" {
		t.Fatalf("prefix = %q, want %q", draft.prefix, ">")
	}
	if draft.promptLabel != "prompt" {
		t.Fatalf("promptLabel = %q, want prompt", draft.promptLabel)
	}
	if got := sanitizeSubmittedValue("> write docs", draft.prefix); got != "write docs" {
		t.Fatalf("submit value = %q, want %q", got, "write docs")
	}

	draft = parseInputDraft(":git status", TabChat, false)
	if draft.prefix != ":" {
		t.Fatalf("prefix = %q, want %q", draft.prefix, ":")
	}
	if !draft.commandMode {
		t.Fatal("expected shell mode to be command mode")
	}
	if draft.command != "git" {
		t.Fatalf("command = %q, want git", draft.command)
	}
	if len(draft.args) != 1 || draft.args[0] != "status" {
		t.Fatalf("args = %#v, want [status]", draft.args)
	}

	draft = parseInputDraft("/commit fix", TabChat, false)
	if draft.prefix != "/" {
		t.Fatalf("prefix = %q, want %q", draft.prefix, "/")
	}
	if draft.command != "commit" {
		t.Fatalf("command = %q, want commit", draft.command)
	}
	if len(draft.args) != 1 || draft.args[0] != "fix" {
		t.Fatalf("args = %#v, want [fix]", draft.args)
	}
}

func TestParseCommandLineRejectsPromptPrefixes(t *testing.T) {
	if _, _, _, ok := parseCommandLine("> hello"); ok {
		t.Fatal("expected prompt prefix to be rejected as a command")
	}
	if _, _, _, ok := parseCommandLine("? search"); ok {
		t.Fatal("expected search prefix to be rejected as a command")
	}
}

func TestCommandPaletteFilteringAndTabCompletion(t *testing.T) {
	reg := NewCommandRegistry()
	reg.Register(Command{Name: "commit", Description: "Commit modified files", Usage: "/commit [message]"})
	reg.Register(Command{Name: "help", Description: "Show help", Usage: "/help [command]"})

	b := NewInputBar()
	b.SetCommandRegistry(reg)
	b.input.SetValue("/commit")
	b.updatePalette(parseInputDraft(b.input.Value(), TabChat, false))

	open, items, _, label := b.PaletteState()
	if !open {
		t.Fatal("expected command palette to open")
	}
	if label != "slash" {
		t.Fatalf("label = %q, want slash", label)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	if items[0].Name != "commit" {
		t.Fatalf("filtered command = %q, want commit", items[0].Name)
	}

	ib, _ := b.Update(tea.KeyMsg{Type: tea.KeyTab}, TabChat)
	if got := ib.Value(); got != "/commit " {
		t.Fatalf("completed value = %q, want %q", got, "/commit ")
	}
	if open, _, _, _ := ib.PaletteState(); open {
		t.Fatal("expected palette to close after completion")
	}

	ib, _ = ib.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}}, TabChat)
	if got := ib.Value(); got != "/commit f" {
		t.Fatalf("value after typing argument = %q, want %q", got, "/commit f")
	}
	if open, _, _, _ := ib.PaletteState(); open {
		t.Fatal("expected palette to stay closed while typing arguments")
	}
}
