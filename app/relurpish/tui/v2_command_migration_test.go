package tui

import (
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestV2CommandMigrationInventory(t *testing.T) {
	registry := newHostCommandRegistry()
	RegisterEucloCommands(registry)
	// Full 45-command inventory from 3dd026c2. A removal has an explicit reason;
	// existing real operations remain registered until their replacement ships.
	retained := strings.Fields(`workspace permissions securityguard filescopes doctor model model-provider keybindings help mode agent retry export workflows workflow rerun cancelwf resume hitl queue service save backend save-model restart-all add remove context clear diff parallel commit local-review checkpoint compact`)
	removed := map[string]string{
		"approve":        "message status mutation is not authorization; focused /attention handles actual decisions",
		"reject":         "message status mutation is not a governed denial; focused /attention handles actual decisions",
		"recipe":         "session pinning retired; canonical recipe paths use the shared workshop",
		"stop":           "replaced directly by /cancel",
		"test-latency":   "advertised handler had no implementation; provider panel retains its real probe",
		"test-rule":      "advertised handler had no implementation; guard panel retains its pattern test",
		"reset-bindings": "advertised handler had no implementation; focused keybinding panel retains reset",
		"diff-apply-all": "unsafe view filesystem mutation removed; governed tool execution remains",
		"diff-revert":    "unsafe view filesystem mutation removed; governed tool execution remains",
		"diff-view":      "unimplemented palette handler removed; read-only evidence remains",
	}
	if len(retained)+len(removed) != 45 {
		t.Fatal("baseline command inventory is incomplete")
	}
	for _, name := range retained {
		command, ok := registry.Lookup(name)
		if !ok || command.Handler == nil || command.Usage == "" {
			t.Errorf("existing operation /%s disappeared or became an empty catalog entry", name)
		}
	}
	for name, reason := range removed {
		if _, ok := registry.Lookup(name); ok || reason == "" {
			t.Errorf("removed command /%s reappeared without delivered replacement", name)
		}
	}
	for _, canvas := range []TabID{TabChat, TabDoctor, TabRun, TabSandbox, TabAIProvider} {
		eligible := registry.Match("", canvas, "")
		for _, command := range eligible {
			if command.Handler == nil || (len(command.TabFilter) > 0 && !slices.Contains(command.TabFilter, canvas)) {
				t.Fatalf("completion advertised an unavailable operation: %s in %s", command.Name, canvas)
			}
		}
	}
}

func TestV2HelpUsesCurrentCatalogWithoutRuntime(t *testing.T) {
	m, _ := newFocusTestModel()
	m.chat = nil
	m.setActiveTab(TabDoctor)
	m.inputBar.SetValue("keep request")
	next, _ := m.Update(CommandInvokedMsg{Name: "help"})
	m = next.(RootModel)
	if !m.showHelp || !strings.Contains(m.helpView(), "/doctor") || strings.Contains(m.helpView(), "/inspect") {
		t.Fatal("Doctor help did not open its actual contextual command catalog")
	}
	if m.inputBar.Value() != "keep request" {
		t.Fatal("opening help erased a request")
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(RootModel)
	if m.showHelp {
		t.Fatal("Escape did not close help")
	}
	m.setActiveTab(TabRun)
	rootHandleHelp(&m, []string{"inspect"})
	if !strings.Contains(m.helpView(), "/inspect [step-id]") {
		t.Fatal("per-command help used an unrelated global catalog")
	}
}
