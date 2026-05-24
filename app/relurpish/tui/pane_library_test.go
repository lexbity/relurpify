package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	thoughtrecipe "codeburg.org/lexbit/relurpify/named/euclo/thoughtrecipes"
	tea "github.com/charmbracelet/bubbletea"
)

func TestLibraryPaneFiltersAndTagToggles(t *testing.T) {
	pane := &LibraryPane{
		allItems: []libraryItem{
			{Kind: libraryItemRecipe, ID: "alpha.refactor", Title: "Alpha Refactor", Tags: []string{"go", "refactor"}},
			{Kind: libraryItemPrompt, ID: "beta.prompt", Title: "Beta Prompt", Tags: []string{"writing"}},
		},
		tagFilters: make(map[string]bool),
		lastUsed:   make(map[string]time.Time),
	}

	pane.rebuildItems("")
	if got := len(pane.items); got != 2 {
		t.Fatalf("items = %d, want 2", got)
	}

	pane.SetFilter("alp")
	if got := len(pane.items); got != 1 {
		t.Fatalf("filtered items = %d, want 1", got)
	}
	if got := pane.items[0].ID; got != "alpha.refactor" {
		t.Fatalf("filtered item = %q, want alpha.refactor", got)
	}

	pane.SetFilter("")
	pane.rebuildItems("")
	pane.toggleSelectedTagFilter()
	if !pane.tagFilters["go"] {
		t.Fatal("expected selected tag filter to toggle on")
	}
	pane.rebuildItems("")
	if got := len(pane.items); got != 1 {
		t.Fatalf("tag filtered items = %d, want 1", got)
	}
	if got := pane.items[0].ID; got != "alpha.refactor" {
		t.Fatalf("tag filtered item = %q, want alpha.refactor", got)
	}
}

func TestLibraryPaneRunSelectedPreparesPrompt(t *testing.T) {
	pane := &LibraryPane{
		allItems: []libraryItem{
			{Kind: libraryItemRecipe, ID: "demo.recipe", Title: "Demo Recipe", Inputs: []string{"target_path", "mode"}},
		},
		tagFilters: make(map[string]bool),
		lastUsed:   make(map[string]time.Time),
	}
	pane.rebuildItems("")

	_, cmd := pane.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if cmd == nil {
		t.Fatal("expected run command")
	}
	msg := cmd()
	runMsg, ok := msg.(LibraryRunRequestedMsg)
	if !ok {
		t.Fatalf("message type = %T, want LibraryRunRequestedMsg", msg)
	}
	if runMsg.RecipeID != "demo.recipe" {
		t.Fatalf("recipe id = %q, want demo.recipe", runMsg.RecipeID)
	}
	if got, want := runMsg.Prompt, "/recipe run demo.recipe [target_path=?] [mode=?] "; got != want {
		t.Fatalf("prompt = %q, want %q", got, want)
	}
}

func TestLibraryPaneValidatesRecipeSource(t *testing.T) {
	root := t.TempDir()
	sourceRoot := filepath.Join(root, "relurpify_cfg", "euclo")
	if err := os.MkdirAll(sourceRoot, 0o755); err != nil {
		t.Fatalf("mkdir source root: %v", err)
	}
	source := filepath.Join(sourceRoot, "validate.euclo")
	if err := os.WriteFile(source, []byte(`thoughtrecipe validate_demo
"Demo."

trigger as capability:
  may read workspace

input target_path: "**/*.go"

agent reviewer uses react

run reviewer:
  from input.target_path
  goal "Review the target."
`), 0o644); err != nil {
		t.Fatalf("write recipe: %v", err)
	}

	result, err := thoughtrecipe.NewLoader().LoadWorkspace(root)
	if err != nil {
		t.Fatalf("LoadWorkspace: %v", err)
	}
	entry, ok := result.Registry.Get("validate_demo")
	if !ok || entry == nil {
		t.Fatal("expected validate_demo recipe to load")
	}

	pane := &LibraryPane{
		allItems: []libraryItem{recipeItemFromEntry(result.Registry.Entries()[0])},
		tagFilters: make(map[string]bool),
		lastUsed:   make(map[string]time.Time),
	}
	pane.rebuildItems("")
	if _, cmd := pane.validateSelected(); cmd != nil {
		t.Fatal("expected validation to be synchronous")
	}
	if !strings.Contains(pane.status, "validated cleanly") {
		t.Fatalf("status = %q, want clean validation", pane.status)
	}
	detail := strings.Join(pane.detailLines(), "\n")
	if !strings.Contains(detail, "target_path") {
		t.Fatalf("detail view missing input name: %s", detail)
	}
}
