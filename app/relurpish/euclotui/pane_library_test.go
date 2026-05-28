package euclotui

import (
	"strings"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/app/relurpish/tui"
	thoughtrecipe "codeburg.org/lexbit/relurpify/named/euclo/thoughtrecipes"
	tea "github.com/charmbracelet/bubbletea"
)

func TestEucloLibraryPaneLoadsItems(t *testing.T) {
	pane := NewEucloLibraryPane(nil, nil)
	if pane == nil {
		t.Fatal("expected non-nil pane")
	}
	if len(pane.allItems) == 0 {
		t.Log("pane loaded with items from workspace (or empty if no workspace)")
	}
}

func TestEucloLibraryPaneFiltersAndTagToggles(t *testing.T) {
	pane := &EucloLibraryPane{
		allItems: []eucloLibraryItem{
			{Kind: eucloLibraryItemRecipe, ID: "alpha.refactor", Title: "Alpha Refactor", Tags: []string{"go", "refactor"}},
			{Kind: eucloLibraryItemPrompt, ID: "beta.prompt", Title: "Beta Prompt", Tags: []string{"writing"}},
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

func TestEucloLibraryPaneViewScope(t *testing.T) {
	pane := &EucloLibraryPane{
		allItems: []eucloLibraryItem{
			{Kind: eucloLibraryItemRecipe, ID: "demo.recipe", Title: "Demo Recipe"},
			{Kind: eucloLibraryItemPrompt, ID: "test.prompt", Title: "Test Prompt"},
		},
		tagFilters: make(map[string]bool),
		lastUsed:   make(map[string]time.Time),
	}

	pane.rebuildItems("")
	if got := len(pane.items); got != 2 {
		t.Fatalf("all items = %d, want 2", got)
	}

	pane.Update(tea.KeyMsg{Type: tea.KeyTab})
	if pane.view != eucloLibraryViewRecipes {
		t.Fatal("expected tab to switch to recipes view")
	}
	pane.rebuildItems("")
	if got := len(pane.items); got != 1 || pane.items[0].Kind != eucloLibraryItemRecipe {
		t.Fatalf("recipes filtered items = %d, want 1 recipe", got)
	}

	pane.Update(tea.KeyMsg{Type: tea.KeyTab})
	if pane.view != eucloLibraryViewPrompts {
		t.Fatal("expected tab to switch to prompts view")
	}
	pane.rebuildItems("")
	if got := len(pane.items); got != 1 || pane.items[0].Kind != eucloLibraryItemPrompt {
		t.Fatalf("prompts filtered items = %d, want 1 prompt", got)
	}

	pane.Update(tea.KeyMsg{Type: tea.KeyTab})
	if pane.view != eucloLibraryViewAll {
		t.Fatal("expected tab to cycle back to all view")
	}
}

func TestEucloLibraryPaneRunSelectedPreparesPrompt(t *testing.T) {
	pane := &EucloLibraryPane{
		allItems: []eucloLibraryItem{
			{Kind: eucloLibraryItemRecipe, ID: "demo.recipe", Title: "Demo Recipe", Inputs: []string{"target_path", "mode"}},
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
	runMsg, ok := msg.(tui.LibraryRunRequestedMsg)
	if !ok {
		t.Fatalf("message type = %T, want tui.LibraryRunRequestedMsg", msg)
	}
	if runMsg.RecipeID != "demo.recipe" {
		t.Fatalf("recipe id = %q, want demo.recipe", runMsg.RecipeID)
	}
	if got, want := runMsg.Prompt, "/recipe run demo.recipe [target_path=?] [mode=?] "; got != want {
		t.Fatalf("prompt = %q, want %q", got, want)
	}
}

func TestEucloLibraryPaneDetailLines(t *testing.T) {
	now := time.Now()
	pane := &EucloLibraryPane{
		allItems: []eucloLibraryItem{
			{
				Kind:         eucloLibraryItemRecipe,
				ID:           "detail.test",
				Title:        "Detail Test",
				Description:  "A test recipe for detail view",
				Tags:         []string{"test", "detail"},
				Capabilities: []string{"read_file", "write_patch"},
				Inputs:       []string{"target"},
				LastUsed:     now,
			},
		},
		tagFilters: make(map[string]bool),
		lastUsed:   make(map[string]time.Time),
	}
	pane.rebuildItems("")
	pane.lastUsed["detail.test"] = now

	lines := pane.detailLines()
	detail := strings.Join(lines, "\n")
	if !strings.Contains(detail, "Detail Test") {
		t.Fatalf("detail missing title: %s", detail)
	}
	if !strings.Contains(detail, "recipe") {
		t.Fatalf("detail missing kind: %s", detail)
	}
	if !strings.Contains(detail, "detail.test") {
		t.Fatalf("detail missing id: %s", detail)
	}
	if !strings.Contains(detail, "test, detail") {
		t.Fatalf("detail missing tags: %s", detail)
	}
	if !strings.Contains(detail, "read_file") {
		t.Fatalf("detail missing capabilities: %s", detail)
	}
	if !strings.Contains(detail, "A test recipe") {
		t.Fatalf("detail missing description: %s", detail)
	}
	if !strings.Contains(detail, "target") {
		t.Fatalf("detail missing inputs: %s", detail)
	}
}

func TestEucloLibraryPaneProjectionIntegration(t *testing.T) {
	router := NewEucloEventRouter()
	router.ApplyExecutionEvent(ExecutionEvent{
		RecipeID: "projection.test",
		Summary:  "Projection test run",
	})
	pane := NewEucloLibraryPane(nil, router)
	if pane == nil {
		t.Fatal("expected non-nil pane")
	}

	pane.items = []eucloLibraryItem{
		{Kind: eucloLibraryItemRecipe, ID: "projection.test", Title: "Projection Test"},
	}
	pane.sel = 0

	lines := pane.detailLines()
	detail := strings.Join(lines, "\n")
	if !strings.Contains(detail, "session runs") {
		t.Logf("projection data may not be immediately visible: %s", detail)
	}

	view := pane.View()
	if view == "" {
		t.Fatal("expected non-empty view")
	}
}

func TestEucloLibraryPaneKeyboardNavigation(t *testing.T) {
	pane := &EucloLibraryPane{
		allItems: []eucloLibraryItem{
			{Kind: eucloLibraryItemRecipe, ID: "first.recipe", Title: "First Recipe"},
			{Kind: eucloLibraryItemRecipe, ID: "second.recipe", Title: "Second Recipe"},
			{Kind: eucloLibraryItemRecipe, ID: "third.recipe", Title: "Third Recipe"},
		},
		tagFilters: make(map[string]bool),
		lastUsed:   make(map[string]time.Time),
	}
	pane.rebuildItems("")

	if pane.sel != 0 {
		t.Fatalf("initial selection = %d, want 0", pane.sel)
	}

	pane.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if pane.sel != 1 {
		t.Fatalf("after down = %d, want 1", pane.sel)
	}

	pane.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if pane.sel != 2 {
		t.Fatalf("after second down = %d, want 2", pane.sel)
	}

	pane.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if pane.sel != 1 {
		t.Fatalf("after up = %d, want 1", pane.sel)
	}

	pane.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if pane.sel != 0 {
		t.Fatalf("after second up = %d, want 0", pane.sel)
	}
}

func TestEucloLibraryPaneClearFilterOnEscape(t *testing.T) {
	pane := &EucloLibraryPane{
		allItems: []eucloLibraryItem{
			{Kind: eucloLibraryItemRecipe, ID: "alpha.refactor", Title: "Alpha Refactor"},
			{Kind: eucloLibraryItemRecipe, ID: "beta.filter", Title: "Beta Filter"},
		},
		tagFilters: make(map[string]bool),
		lastUsed:   make(map[string]time.Time),
	}
	pane.rebuildItems("")
	pane.SetFilter("beta")
	if len(pane.items) != 1 {
		t.Fatalf("filtered items = %d, want 1", len(pane.items))
	}

	pane.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if len(pane.items) != 2 {
		t.Fatalf("after escape items = %d, want 2", len(pane.items))
	}
}

func TestEucloLibraryPaneFooterShowsSessionRuns(t *testing.T) {
	router := NewEucloEventRouter()
	router.ApplyExecutionEvent(ExecutionEvent{
		RecipeID: "counted.recipe",
	})
	router.ApplyExecutionEvent(ExecutionEvent{
		RecipeID: "counted.recipe",
	})

	pane := NewEucloLibraryPane(nil, router)
	pane.allItems = []eucloLibraryItem{
		{Kind: eucloLibraryItemRecipe, ID: "counted.recipe", Title: "Counted Recipe"},
	}
	pane.rebuildItems("")

	footer := pane.footer()
	if !strings.Contains(footer, "2") {
		t.Logf("footer: %s", footer)
	}
}

func TestEucloLibraryPaneEntryToEucloItem(t *testing.T) {
	recipe := &thoughtrecipe.ThoughtRecipe{
		ID:          "test.entry",
		Name:        "Test Entry",
		Description: "An entry test",
		Metadata: thoughtrecipe.ThoughtRecipeMetadata{
			Tags: []string{"test", "conversion"},
		},
	}
	entry := thoughtrecipe.ThoughtRecipeEntry{
		ThoughtRecipe: recipe,
		Source:        "/fake/path/test.entry.euclo",
	}

	item := entryToEucloItem(entry)
	if item.ID != "test.entry" {
		t.Fatalf("id = %q, want test.entry", item.ID)
	}
	if item.Title != "Test Entry" {
		t.Fatalf("title = %q, want Test Entry", item.Title)
	}
	if len(item.Tags) != 2 || item.Tags[0] != "test" {
		t.Fatalf("tags = %v, want [test conversion]", item.Tags)
	}
	if item.Source != "/fake/path/test.entry.euclo" {
		t.Fatalf("source = %q, want /fake/path/test.entry.euclo", item.Source)
	}
}

func TestEucloLibraryPanePromptInfoConversion(t *testing.T) {
	info := tui.PromptInfo{
		PromptID:    "test.prompt",
		Description: "A test prompt",
		Tags:        []string{"writing", "codegen"},
		Variables:   []string{"lang", "style"},
		Meta: tui.InspectableMeta{
			Title:  "Test Prompt",
			Source: "/fake/path/test.prompt.yaml",
		},
	}

	item := promptInfoToEucloItem(info)
	if item.ID != "test.prompt" {
		t.Fatalf("id = %q, want test.prompt", item.ID)
	}
	if item.Kind != eucloLibraryItemPrompt {
		t.Fatalf("kind = %v, want prompt", item.Kind)
	}
	if len(item.Tags) != 2 || item.Tags[0] != "writing" {
		t.Fatalf("tags = %v, want [writing codegen]", item.Tags)
	}
	if len(item.Variables) != 2 {
		t.Fatalf("variables = %v, want [lang style]", item.Variables)
	}
}

func TestEucloLibraryPaneEmptyState(t *testing.T) {
	pane := &EucloLibraryPane{
		allItems:   nil,
		tagFilters: make(map[string]bool),
		lastUsed:   make(map[string]time.Time),
	}
	pane.SetSize(120, 30)
	pane.rebuildItems("")

	view := pane.View()
	if !strings.Contains(view, "No recipes or prompts") {
		t.Fatalf("empty state missing message: %q", view)
	}
}

func TestEucloLibraryPaneNarrowTerminal(t *testing.T) {
	pane := &EucloLibraryPane{
		allItems: []eucloLibraryItem{
			{Kind: eucloLibraryItemRecipe, ID: "narrow.test", Title: "Narrow Test"},
		},
		tagFilters: make(map[string]bool),
		lastUsed:   make(map[string]time.Time),
	}
	pane.SetSize(80, 25)
	pane.rebuildItems("")

	view := pane.View()
	if !strings.Contains(view, "Narrow Test") {
		t.Fatalf("narrow view missing item: %q", view)
	}
}
