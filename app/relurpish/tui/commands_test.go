package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

type recipeGuestSurface struct {
	fakeSurface
}

func (s *recipeGuestSurface) RegisterCommands(reg *CommandRegistry) {
	reg.Register(Command{Name: "surface-cmd", Usage: "/surface-cmd", Handler: func(m *RootModel, args []string) (*RootModel, tea.Cmd) {
		return m, nil
	}})
}

func TestParseInputDraftPrefixes(t *testing.T) {
	for _, raw := range []string{"> write docs", ":git status", "? why", "ordinary request"} {
		draft := parseInputDraft(raw, TabSandbox, false)
		if draft.commandMode || draft.filterMode || draft.prefix != "" || draft.current != raw {
			t.Fatalf("ordinary request reinterpreted: %#v", draft)
		}
	}
	draft := parseInputDraft("/commit fix", TabChat, false)
	if !draft.commandMode || draft.command != "commit" || len(draft.args) != 1 || draft.args[0] != "fix" {
		t.Fatalf("bad command: %#v", draft)
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
	if label != "command" {
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

func TestHostRegistrySeparatesGuestCommands(t *testing.T) {
	if _, ok := newHostCommandRegistry().Lookup("workspace"); !ok {
		t.Fatal("expected workspace to remain a host command")
	}

	guest := &recipeGuestSurface{
		fakeSurface: fakeSurface{name: "guest", chat: &fakeChatPane{}},
	}
	factory := &registryFactory{
		defaultSurface: &baseSurfaceFake{},
		surfaces: map[string]AgentSurface{
			"guest": guest,
		},
	}
	m := NewRootModel(context.Background(), HostOptions{SurfaceFactory: factory})
	if _, ok := m.cmdReg.Lookup("workspace"); !ok {
		t.Fatal("expected workspace to remain a host command")
	}
	if _, ok := m.cmdReg.Lookup("surface-cmd"); ok {
		t.Fatal("expected surface-cmd to stay off the host registry")
	}
	if err := m.switchActiveAgent("guest"); err != nil {
		t.Fatalf("switch to guest failed: %v", err)
	}
	if _, ok := m.cmdReg.Lookup("surface-cmd"); !ok {
		t.Fatal("expected guest registry to include surface commands")
	}
	if _, ok := m.cmdReg.Lookup("workspace"); !ok {
		t.Fatal("expected guest registry to include host commands")
	}
}

func TestV2ContextCommandsChangeFutureSelectionWithoutFakeRuntimeSuccess(t *testing.T) {
	m, _ := newFocusTestModel()
	m.inputBar.SetValue("retain request")
	rootHandleAdd(&m, []string{"src/../notes.txt"})
	if got := m.sharedCtx.List(); len(got) != 1 || got[0] != "notes.txt" {
		t.Fatalf("selected context = %v", got)
	}
	rootHandleAdd(&m, []string{"notes.txt"})
	if len(m.sharedCtx.List()) != 1 || !strings.Contains(m.notice, "already in context") {
		t.Fatal("duplicate was falsely reported as added")
	}
	rootHandleRemove(&m, []string{"notes.txt"})
	if len(m.sharedCtx.List()) != 0 || m.inputBar.Value() != "retain request" {
		t.Fatal("removal failed or changed the request")
	}
	rootHandleAdd(&m, []string{"one", "two"})
	if len(m.sharedCtx.List()) != 0 {
		t.Fatal("invalid arguments changed the selection")
	}
}
