package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

type recipeLibrarySurfaceFake struct {
	selected     string
	promptByID   map[string]string
	refreshCount int
	lastFilter   string
	lastSelected string
}

func (s *recipeLibrarySurfaceFake) SetSize(int, int) {}
func (s *recipeLibrarySurfaceFake) SetFilter(filter string) {
	s.lastFilter = filter
}
func (s *recipeLibrarySurfaceFake) Refresh() { s.refreshCount++ }
func (s *recipeLibrarySurfaceFake) Update(msg tea.Msg) (LibrarySurface, tea.Cmd) {
	return s, nil
}
func (s *recipeLibrarySurfaceFake) View() string { return "library" }
func (s *recipeLibrarySurfaceFake) SelectedID() string {
	if s.selected != "" {
		return s.selected
	}
	return s.lastSelected
}
func (s *recipeLibrarySurfaceFake) RunPromptForID(id string) (string, bool) {
	if s.promptByID == nil {
		return "", false
	}
	prompt, ok := s.promptByID[id]
	return prompt, ok
}
func (s *recipeLibrarySurfaceFake) SelectByID(id string) bool {
	s.lastSelected = id
	if s.selected == "" {
		s.selected = id
	}
	return true
}
func (s *recipeLibrarySurfaceFake) OpenSelectedEditorCmd() tea.Cmd { return nil }
func (s *recipeLibrarySurfaceFake) ValidateSelected() tea.Cmd      { return nil }

type recipeGuestSurface struct {
	fakeSurface
	library *recipeLibrarySurfaceFake
}

func (s *recipeGuestSurface) RegisterCommands(reg *CommandRegistry) {
	RegisterEucloCommands(reg)
}

func (s *recipeGuestSurface) NewLibrary(RuntimeAdapter, *AgentContext, *Session) LibrarySurface {
	return s.library
}

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

func TestHostRegistrySeparatesGuestCommands(t *testing.T) {
	if _, ok := rootCommandRegistry.Lookup("recipe"); ok {
		t.Fatal("expected recipe to be guest-only")
	}
	if _, ok := rootCommandRegistry.Lookup("workspace"); !ok {
		t.Fatal("expected workspace to remain a host command")
	}

	guest := &recipeGuestSurface{
		fakeSurface: fakeSurface{name: "guest", chat: &fakeChatPane{}},
		library: &recipeLibrarySurfaceFake{
			selected:   "demo.recipe",
			promptByID: map[string]string{"demo.recipe": "/recipe run demo.recipe "},
		},
	}
	factory := &registryFactory{
		defaultSurface: &baseSurfaceFake{},
		surfaces: map[string]AgentSurface{
			"guest": guest,
		},
	}
	m := newRootModel(nil, factory)
	if _, ok := m.cmdReg.Lookup("recipe"); ok {
		t.Fatal("expected recipe to stay off the host registry")
	}
	if _, ok := m.cmdReg.Lookup("workspace"); !ok {
		t.Fatal("expected workspace to remain a host command")
	}
	if err := m.switchActiveAgent("guest"); err != nil {
		t.Fatalf("switch to guest failed: %v", err)
	}
	if _, ok := m.cmdReg.Lookup("recipe"); !ok {
		t.Fatal("expected guest registry to include recipe commands")
	}
	if _, ok := m.cmdReg.Lookup("workspace"); !ok {
		t.Fatal("expected guest registry to include host commands")
	}
}

func TestRecipeCommandsUseGenericGuestSurface(t *testing.T) {
	lib := &recipeLibrarySurfaceFake{
		selected:     "demo.recipe",
		promptByID:   map[string]string{"demo.recipe": "/recipe run demo.recipe "},
		lastFilter:   "",
		lastSelected: "",
	}
	guest := &recipeGuestSurface{
		fakeSurface: fakeSurface{name: "guest", chat: &fakeChatPane{}},
		library:     lib,
	}
	factory := &registryFactory{
		defaultSurface: &baseSurfaceFake{},
		surfaces: map[string]AgentSurface{
			"guest": guest,
		},
	}
	m := newRootModel(nil, factory)

	updated, _ := rootHandleRecipes(&m, nil)
	m = *updated
	if got := m.activeAgentName(); got != "guest" {
		t.Fatalf("active agent = %q, want guest", got)
	}
	if got := lib.refreshCount; got == 0 {
		t.Fatal("expected generic guest library to refresh")
	}

	updated, cmd := rootHandleRecipe(&m, []string{"run"})
	m = *updated
	if cmd != nil {
		t.Fatalf("expected nil command after staging recipe prompt, got %v", cmd)
	}
	if got := m.inputBar.Value(); got != "/recipe run demo.recipe " {
		t.Fatalf("input value = %q, want /recipe run demo.recipe ", got)
	}
	if got := m.activeAgentName(); got != "guest" {
		t.Fatalf("active agent after recipe run = %q, want guest", got)
	}
}
