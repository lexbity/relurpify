package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/userconfig/config"
	tea "github.com/charmbracelet/bubbletea"
)

func TestV2KeybindingConflictReviewSaveAndIsolation(t *testing.T) {
	workspace := t.TempDir()
	keys, other := NewKeyMap(), NewKeyMap()
	pane := NewKeybindingPane(keys, workspace, false)
	pane.SetSize(80, 19)
	pane.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	pane.Update(tea.KeyMsg{Type: tea.KeyCtrlA})
	if pane.proposal == nil || len(pane.proposal.conflicts) != 1 || !strings.Contains(pane.View(), "agent_picker loses ctrl+a") {
		t.Fatal("capture did not explain its conflicting action")
	}
	if !keyMatchesBinding(keys.Help, "f1") || !keyMatchesBinding(keys.AgentPicker, "ctrl+a") {
		t.Fatal("staging changed active bindings")
	}
	_, cmd := pane.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("confirmation did not persist the proposal")
	}
	pane.Update(cmd())
	if !keyMatchesBinding(keys.Help, "ctrl+a") || len(keys.AgentPicker.Keys()) != 0 || !keyMatchesBinding(other.Help, "f1") {
		t.Fatal("activation did not apply the reviewed map or leaked into another host")
	}
	loaded := NewKeyMap()
	NewKeybindingPane(loaded, workspace, false)
	if !keyMatchesBinding(loaded.Help, "ctrl+a") {
		t.Fatal("saved key map did not survive reload")
	}
	// Reapplying a change creates a real backup through the configuration owner.
	pane.propose("open_help", []string{"f2"})
	cmd = pane.persist()
	pane.Update(cmd())
	backups, err := filepath.Glob(filepath.Join(workspace, ".relurpify_state", "backups", "keybindings.yaml.*.bak"))
	if err != nil || len(backups) == 0 {
		t.Fatal("existing preferences were not backed up")
	}
}

func TestV2KeybindingSaveFailureRetainsActiveMap(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, ".relurpify_state"), []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	keys := NewKeyMap()
	pane := NewKeybindingPane(keys, workspace, false)
	pane.propose("open_help", []string{"f2"})
	cmd := pane.persist()
	pane.Update(cmd())
	if !keyMatchesBinding(keys.Help, "f1") || !strings.Contains(pane.status, "previous keys remain active") {
		t.Fatal("failed save activated a staged map or hid the failure")
	}
}

func TestV2KeybindingResetRequiresReviewAndCancelPreservesMap(t *testing.T) {
	keys := NewKeyMap()
	keys.Help.SetKeys("f2")
	pane := NewKeybindingPane(keys, "", true)
	pane.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	if pane.proposal == nil || !keyMatchesBinding(keys.Help, "f2") {
		t.Fatal("reset bypassed staging")
	}
	pane.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if pane.proposal != nil || !keyMatchesBinding(keys.Help, "f2") {
		t.Fatal("cancel reset changed keys")
	}
	pane.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	_, cmd := pane.Update(tea.KeyMsg{Type: tea.KeyEnter})
	pane.Update(cmd())
	if !keyMatchesBinding(keys.Help, "f1") || !strings.Contains(pane.status, "DEMO") {
		t.Fatal("confirmed reset was not applied/labeled")
	}
}

func TestV2KeybindingLoadRejectsUnknownActions(t *testing.T) {
	workspace := t.TempDir()
	path := config.New(workspace).RuntimeKeybindingsFile()
	_, err := config.SaveRuntimeKeybindingConfigWithBackup(path, config.RuntimeKeybindingConfig{Bindings: []config.RuntimeKeybindingEntry{{Action: "switch surface 1", Keys: []string{"1"}}}})
	if err != nil {
		t.Fatal(err)
	}
	keys := NewKeyMap()
	pane := NewKeybindingPane(keys, workspace, false)
	if !strings.Contains(pane.status, "unknown action") || !keyMatchesBinding(keys.Help, "f1") {
		t.Fatal("obsolete actions were silently accepted or changed the map")
	}
}
