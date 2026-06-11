package tui

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"codeburg.org/lexbit/relurpify/userconfig/config"
)

type keybindingPaneRuntimeFake struct {
	workspace string
}

func (f *keybindingPaneRuntimeFake) SessionInfo() SessionInfo {
	return SessionInfo{Workspace: f.workspace}
}

func TestKeybindingConflictEngine(t *testing.T) {
	originalTab1 := append([]string(nil), GlobalKeys.Tab1.Keys()...)
	originalTab2 := append([]string(nil), GlobalKeys.Tab2.Keys()...)
	defer func() {
		GlobalKeys.Tab1.SetKeys(originalTab1...)
		GlobalKeys.Tab2.SetKeys(originalTab2...)
	}()

	dir := t.TempDir()
	workspace := dir
	path := filepath.Join(dir, ".relurpify_state", "keybindings.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir keybinding dir: %v", err)
	}
	initial := config.RuntimeKeybindingConfig{Bindings: make([]config.RuntimeKeybindingEntry, 0)}
	for _, target := range buildKeybindingTargets() {
		initial.Bindings = append(initial.Bindings, config.RuntimeKeybindingEntry{
			Action:      target.Action,
			Keys:        append([]string(nil), target.DefaultKeys...),
			Scope:       target.Scope,
			Source:      target.Source,
			Description: target.Description,
			DefaultKeys: append([]string(nil), target.DefaultKeys...),
		})
	}
	if err := config.SaveYAML(path, initial); err != nil {
		t.Fatalf("seed keybindings: %v", err)
	}

	pane := NewKeybindingPane(&keybindingPaneRuntimeFake{workspace: workspace})
	pane.SetSize(120, 24)
	if pane.currentTarget().Action != "switch surface 1" {
		t.Fatalf("selected action = %q, want switch surface 1", pane.currentTarget().Action)
	}

	pane, _ = pane.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	if !pane.waitingForKey {
		t.Fatal("expected key capture mode")
	}
	pane, _ = pane.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	if pane.confirm == nil {
		t.Fatal("expected conflict warning modal")
	}
	if pane.confirm.Other.Action != "switch surface 2" {
		t.Fatalf("conflict action = %q, want switch surface 2", pane.confirm.Other.Action)
	}

	pane, cmd := pane.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if pane.confirm != nil {
		t.Fatal("expected conflict modal to close")
	}
	if cmd == nil {
		t.Fatal("expected persist command after confirmation")
	}
	msg := cmd()
	if got, ok := msg.(chatSystemMsg); !ok || got.Text == "" {
		t.Fatalf("persist message = %#v, want chatSystemMsg", msg)
	}
	backups, err := filepath.Glob(filepath.Join(dir, ".relurpify_state", "backups", "keybindings.yaml.*.bak"))
	if err != nil {
		t.Fatalf("glob backups: %v", err)
	}
	if len(backups) == 0 {
		t.Fatal("expected keybinding backup to be created")
	}
	if got := GlobalKeys.Tab1.Keys(); len(got) != 1 || got[0] != "2" {
		t.Fatalf("tab1 keys = %#v, want [2]", got)
	}
	if got := GlobalKeys.Tab2.Keys(); len(got) != 0 {
		t.Fatalf("tab2 keys = %#v, want unbound", got)
	}
	written, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatalf("read keybinding file: %v", err)
	}
	if string(written) == "" || string(written) == "{}" {
		t.Fatalf("expected persisted keybinding config, got %q", string(written))
	}
}
