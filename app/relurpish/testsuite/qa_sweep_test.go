package testsuite_test

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"codeburg.org/lexbit/relurpify/app/relurpish/euclotui"
	"codeburg.org/lexbit/relurpify/app/relurpish/tui"
)

// drive applies a key to the model and returns the updated model, failing on a
// non-RootModel result (which would indicate the model type changed mid-flight).
func drive(t *testing.T, m tui.RootModel, key tea.KeyMsg) tui.RootModel {
	t.Helper()
	updated, _ := m.Update(key)
	rm, ok := updated.(tui.RootModel)
	if !ok {
		t.Fatalf("Update returned non-RootModel: %T", updated)
	}
	return rm
}

// allTabs is the user-reachable tab set.
var allTabs = []tui.TabID{
	tui.TabWelcome,
	tui.TabSandbox,
	tui.TabSecurityGuard,
	tui.TabAIProvider,
	tui.TabKeybindings,
	tui.TabDoctor,
	tui.TabChat,
	tui.TabDiff,
}

// keySoup is a barrage of navigation / editing keys that any pane might receive.
var keySoup = []tea.KeyMsg{
	{Type: tea.KeyUp}, {Type: tea.KeyDown}, {Type: tea.KeyLeft}, {Type: tea.KeyRight},
	{Type: tea.KeyEnter}, {Type: tea.KeyEsc}, {Type: tea.KeyTab}, {Type: tea.KeyShiftTab},
	{Type: tea.KeyBackspace}, {Type: tea.KeyDelete}, {Type: tea.KeyHome}, {Type: tea.KeyEnd},
	{Type: tea.KeyPgUp}, {Type: tea.KeyPgDown}, {Type: tea.KeySpace},
	{Type: tea.KeyRunes, Runes: []rune("e")},
	{Type: tea.KeyRunes, Runes: []rune("t")},
	{Type: tea.KeyRunes, Runes: []rune("s")},
	{Type: tea.KeyRunes, Runes: []rune("j")},
	{Type: tea.KeyRunes, Runes: []rune("k")},
	{Type: tea.KeyRunes, Runes: []rune("h")},
	{Type: tea.KeyRunes, Runes: []rune("l")},
}

// TestQASweep_AllTabsSurviveKeySoup drives every tab through the key barrage at
// a normal size and asserts the model never panics and always renders a frame.
func TestQASweep_AllTabsSurviveKeySoup(t *testing.T) {
	m := tui.NewTestRootModel(nil, euclotui.NewSurfaceFactory())
	m.SetWidthHeightForTest(120, 40)

	for _, tab := range allTabs {
		m.SetActiveTabForTest(tab)
		for _, key := range keySoup {
			m = drive(t, m, key)
			view := m.View()
			if strings.TrimSpace(view) == "" {
				t.Fatalf("tab %q produced empty view after key %v", tab, key)
			}
		}
	}
}

// TestQASweep_ExtremeResize stresses layout math at degenerate sizes.
func TestQASweep_ExtremeResize(t *testing.T) {
	sizes := [][2]int{
		{0, 0}, {1, 1}, {2, 2}, {1, 80}, {80, 1}, {10, 10},
		{40, 12}, {80, 24}, {200, 60}, {500, 200}, {3, 3},
	}
	for _, tab := range allTabs {
		m := tui.NewTestRootModel(nil, euclotui.NewSurfaceFactory())
		m.SetActiveTabForTest(tab)
		for _, s := range sizes {
			m.SetWidthHeightForTest(s[0], s[1])
			// Render must not panic at any size.
			_ = m.View()
		}
	}
}

// TestQASweep_AIProviderEditAdversarial exercises the AI provider edit fields
// with hostile values: empty, whitespace, unicode, very long, control-ish.
func TestQASweep_AIProviderEditAdversarial(t *testing.T) {
	m := tui.NewTestRootModel(nil, euclotui.NewSurfaceFactory())
	m.SetWidthHeightForTest(120, 40)
	m.SetActiveTabForTest(tui.TabAIProvider)

	// Move focus into the configurator column.
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyTab})

	inputs := []string{
		"",
		"   ",
		strings.Repeat("x", 4096),
		"日本語のモデル名",
		"http://[::1]:11434",
		"not a url at all",
		"-1",
		"99999999999999999999",
		"\t\t",
	}
	for field := 0; field < 5; field++ {
		// enter edit mode on the focused field
		m = drive(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
		for _, in := range inputs {
			// clear then type
			for i := 0; i < 20; i++ {
				m = drive(t, m, tea.KeyMsg{Type: tea.KeyBackspace})
			}
			if in != "" {
				m = drive(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(in)})
			}
			_ = m.View()
		}
		m = drive(t, m, tea.KeyMsg{Type: tea.KeyEnter}) // commit
		m = drive(t, m, tea.KeyMsg{Type: tea.KeyDown})  // next field
	}
}

// TestQASweep_RapidTabSwitching flips between tabs while mid-edit / mid-nav to
// surface state bleed or stale-focus panics.
func TestQASweep_RapidTabSwitching(t *testing.T) {
	m := tui.NewTestRootModel(nil, euclotui.NewSurfaceFactory())
	m.SetWidthHeightForTest(100, 30)
	for round := 0; round < 3; round++ {
		for _, tab := range allTabs {
			m.SetActiveTabForTest(tab)
			m = drive(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
			m = drive(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("zzz")})
			// switch away mid-edit
		}
	}
	_ = m.View()
}
