package demo_test

import (
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/app/relurpish/tui"
	tea "github.com/charmbracelet/bubbletea"
)

const defaultRecipe = "euclo.thoughtrecipe.default"

func command(m tui.RootModel, name string, args ...string) tui.RootModel {
	m, cmd := update(m, tui.CommandInvokedMsg{Name: name, Args: args})
	if cmd != nil {
		m, _ = update(m, cmd())
	}
	return m
}
func press(m tui.RootModel, keys ...tea.KeyType) tui.RootModel {
	for _, kind := range keys {
		var cmd tea.Cmd
		m, cmd = key(m, kind)
		if cmd != nil {
			m, _ = update(m, cmd())
		}
	}
	return m
}

func TestV2WorkshopCompleteAuthorConnectBindSaveTrigger(t *testing.T) {
	m, session := model(t, "quiet")
	m = command(m, "recipe open", defaultRecipe)
	if !strings.Contains(m.View(), "worker") {
		t.Fatal("saved recipe did not open")
	}
	m = command(m, "step add", "verifier")
	m = press(m, tea.KeyDown, tea.KeyEnter, tea.KeyDown, tea.KeyEnter)
	if !strings.Contains(m.View(), "Goal draft") {
		t.Fatal("selected step goal field did not open")
	}
	m = press(m, tea.KeyCtrlA, tea.KeyCtrlK)
	m, _ = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Run focused verification")})
	m = press(m, tea.KeyCtrlS)
	// Open the selected step's data connection chooser, then choose the previous capture.
	m = press(m, tea.KeyEnter, tea.KeyDown, tea.KeyDown, tea.KeyEnter, tea.KeyEnd, tea.KeyEnter)
	// Bind/preview an available prompt. This remains reference/preview data.
	m = press(m, tea.KeyEnter, tea.KeyDown, tea.KeyDown, tea.KeyDown, tea.KeyEnter, tea.KeyTab)
	if !strings.Contains(m.View(), "not evidence of actual prompt use") {
		t.Fatalf("prompt preview did not expose its evidence label:\n%s", m.View())
	}
	m = press(m, tea.KeyEnter)
	m = command(m, "recipe undo")
	m = command(m, "recipe redo")
	savedBefore, err := session.OpenRecipe(t.Context(), defaultRecipe)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(savedBefore.Source, "agent verifier") {
		t.Fatal("local authoring changed saved source before review")
	}
	m = command(m, "recipe save")
	if !strings.Contains(m.View(), "Review saved source change") {
		t.Fatal("save bypassed review")
	}
	m = press(m, tea.KeyEnter)
	saved, err := session.OpenRecipe(t.Context(), defaultRecipe)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Revision == savedBefore.Revision || !strings.Contains(saved.Source, "from state.summary") || !strings.Contains(saved.Source, "import prompt demo.prompt.repair") {
		t.Fatalf("confirmed save did not retain authoring effects: %s", saved.Source)
	}
	m = command(m, "recipe run", "relurpify_cfg/euclo/default.erpe")
	if !strings.Contains(m.View(), "Trigger saved file") {
		t.Fatal("saved-file trigger bypassed its separate input/review")
	}
	m = press(m, tea.KeyCtrlS)
	if m.InteractionState().Canvas != tui.TabRun || !session.Current().Running || len(session.Current().Circuit.Steps) != 2 {
		t.Fatal("saved file did not produce its actual parsed demo run graph")
	}
}

func TestV2WorkshopInvalidSourceCannotSaveAndUndoRecovers(t *testing.T) {
	m, session := model(t, "quiet")
	m = command(m, "recipe open", defaultRecipe)
	before, err := session.OpenRecipe(t.Context(), defaultRecipe)
	if err != nil {
		t.Fatal(err)
	}
	m = command(m, "recipe source")
	m = press(m, tea.KeyCtrlHome)
	m, _ = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("invalid ")})
	m = press(m, tea.KeyCtrlS)
	m = command(m, "recipe save")
	if !strings.Contains(m.View(), "Diagnostics") {
		t.Fatal("invalid source did not block save with diagnostics")
	}
	after, err := session.OpenRecipe(t.Context(), defaultRecipe)
	if err != nil || after.Source != before.Source {
		t.Fatal("invalid draft changed the saved artifact")
	}
	m = command(m, "recipe undo")
	m = command(m, "recipe validate")
	if !strings.Contains(m.View(), "Syntax and installed strategy contracts checked") {
		t.Fatal("undo did not recover the original valid source")
	}
}

func TestV2WorkshopSaveConflictDoesNotOverwrite(t *testing.T) {
	_, session := model(t, "quiet")
	a, err := session.OpenRecipe(t.Context(), defaultRecipe)
	if err != nil {
		t.Fatal(err)
	}
	b := a
	a.Source += "\n# first writer\n"
	first, err := session.SaveRecipe(t.Context(), a)
	if err != nil {
		t.Fatal(err)
	}
	b.Source += "\n# stale writer\n"
	if _, err := session.SaveRecipe(t.Context(), b); err == nil {
		t.Fatal("stale save overwrote a newer revision")
	}
	current, err := session.OpenRecipe(t.Context(), defaultRecipe)
	if err != nil || current != first {
		t.Fatal("conflicting save changed stored source")
	}
}

func TestV2RecipeNewSaveReopenAndConflict(t *testing.T) {
	m, session := model(t, "quiet")
	m = command(m, "recipe new", "recipes/new_flow.erpe")
	if !strings.Contains(m.View(), "new_flow") || !strings.Contains(m.View(), "New local draft") {
		t.Fatalf("creation did not produce a draft: %s", m.View())
	}
	if _, err := session.OpenRecipe(t.Context(), "new_flow"); err == nil {
		t.Fatal("new recipe was saved without review")
	}
	m = command(m, "recipe save")
	m = press(m, tea.KeyEnter)
	saved, err := session.OpenRecipe(t.Context(), "recipes/new_flow.erpe")
	if err != nil || saved.ID != "new_flow" || saved.Revision != "1" {
		t.Fatalf("reviewed new recipe was not saved: %+v %v", saved, err)
	}
	m = command(m, "recipe close")
	m = command(m, "recipe open", "recipes/new_flow.erpe")
	if !strings.Contains(m.View(), "worker") {
		t.Fatal("saved new recipe could not reopen")
	}
	// Creating the same destination is a local draft until the compare-and-swap
	// rejects its absent revision. The original saved bytes survive.
	m = command(m, "recipe new", "recipes/new_flow.erpe")
	m = command(m, "recipe save")
	m = press(m, tea.KeyEnter)
	if !strings.Contains(m.View(), "saved source changed") {
		t.Fatalf("creation conflict did not remain visible: %s", m.View())
	}
	current, err := session.OpenRecipe(t.Context(), "new_flow")
	if err != nil || current != saved {
		t.Fatal("conflicting creation overwrote saved source")
	}
}
