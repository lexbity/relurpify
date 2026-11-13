package tui

import (
	"context"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/app/relurpish/ports"
	tea "github.com/charmbracelet/bubbletea"
)

func TestV2ComposerAcknowledgementRejectsRecreatedDraft(t *testing.T) {
	b := NewInputBar()
	b.SetValue("same text")
	_, cmd := b.Update(tea.KeyMsg{Type: tea.KeyEnter}, TabChat)
	submitted := cmd().(InputSubmittedMsg)
	b.SetValue("changed")
	b.SetValue("same text")
	b.Accept(submitted.Value, submitted.Sequence)
	if b.Value() != "same text" {
		t.Fatal("late acknowledgement erased a newer edit with identical bytes")
	}
	b.Accept(b.Value(), b.editSequence)
	if b.Value() != "" {
		t.Fatal("current acknowledgement did not clear its own draft")
	}
}

func TestV2ComposerHistoryAndPalettePreserveDraft(t *testing.T) {
	b := NewInputBar()
	b.history.Push("earlier request")
	b.SetValue("unfinished request")
	b.Update(tea.KeyMsg{Type: tea.KeyDown}, TabChat)
	if b.Value() != "unfinished request" {
		t.Fatal("Down with no recall erased the draft")
	}
	b.Update(tea.KeyMsg{Type: tea.KeyUp}, TabChat)
	if b.Value() != "earlier request" {
		t.Fatal("history did not recall")
	}
	b.Update(tea.KeyMsg{Type: tea.KeyDown}, TabChat)
	if b.Value() != "unfinished request" {
		t.Fatal("recall did not restore the unsent draft")
	}
	b.SetValue("first\nsecond")
	b.Update(tea.KeyMsg{Type: tea.KeyUp}, TabChat)
	if b.Value() != "first\nsecond" {
		t.Fatal("multiline navigation recalled over the draft")
	}
	b.SetValue("/he")
	b.updatePalette(parseInputDraft(b.Value(), TabChat, false))
	b.Update(tea.KeyMsg{Type: tea.KeyUp}, TabChat)
	b.Update(tea.KeyMsg{Type: tea.KeyDown}, TabChat)
	if b.Value() != "/he" {
		t.Fatal("palette boundary keys escaped into input history")
	}
}

func TestV2QuitRequiresFocusedReviewAndPreservesDraft(t *testing.T) {
	m, _ := newFocusTestModel()
	m.inputBar.SetValue("unsent")
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlQ})
	m = next.(RootModel)
	if cmd != nil || !m.quitReview {
		t.Fatal("reserved quit skipped review")
	}
	if !strings.Contains(m.quitView(), "unsent request") {
		t.Fatal("quit review omitted unsent work")
	}
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(RootModel)
	if cmd != nil || m.quitReview || m.inputBar.Value() != "unsent" {
		t.Fatal("default review action discarded work")
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = next.(RootModel)
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("quit"), Paste: true})
	m = next.(RootModel)
	if cmd != nil || m.inputBar.Value() != "unsent" || !m.quitReview {
		t.Fatal("paste escaped the quit review")
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(RootModel)
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("deliberate Quit had no operation")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("confirmed quit did not exit")
	}
}

func TestV2HelpOwnsPrintableAndPasteKeys(t *testing.T) {
	m, _ := newFocusTestModel()
	m.inputBar.SetValue("keep")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyF1})
	m = next.(RootModel)
	for _, msg := range []tea.KeyMsg{{Type: tea.KeyRunes, Runes: []rune("hidden")}, {Type: tea.KeyRunes, Runes: []rune("paste"), Paste: true}, {Type: tea.KeyEnter}} {
		next, _ = m.Update(msg)
		m = next.(RootModel)
	}
	if m.inputBar.Value() != "keep" || !m.showHelp {
		t.Fatal("help leaked input into the underlying composer")
	}
}

func TestV2BackRestoresCanvasReadingAndFocus(t *testing.T) {
	m, _ := newFocusTestModel()
	m.setActiveTab(TabChat)
	m.setFocus(FocusRegionRegion1)
	m.canvasOffset = 17
	m.inputBar.SetValue("draft")
	m.setActiveTab(TabSettings)
	m.canvasOffset = 3
	m.setActiveTab(TabDoctor)
	rootHandleBack(&m, nil)
	if m.activeTab != TabSettings || m.canvasOffset != 3 {
		t.Fatal("back lost settings reading position")
	}
	rootHandleBack(&m, nil)
	if m.activeTab != TabChat || m.canvasOffset != 17 || !m.focus.State().InRegion1() || m.inputBar.Value() != "draft" {
		t.Fatal("back lost chat reading/focus/draft state")
	}
}

type reviewRecipeLibrary struct{}

func (reviewRecipeLibrary) ListRecipes(context.Context) ([]ports.RecipeFile, error) {
	return []ports.RecipeFile{{ID: "example", Path: "recipes/example.erpe"}}, nil
}
func (reviewRecipeLibrary) OpenRecipe(context.Context, string) (ports.RecipeFile, error) {
	return ports.RecipeFile{ID: "example", Path: "recipes/example.erpe", Source: workshopSourceFixture, Revision: "r1"}, nil
}
func (reviewRecipeLibrary) RecipeCatalog(context.Context) (ports.RecipeCatalog, error) {
	return ports.RecipeCatalog{Strategies: []ports.RecipeStrategy{{Name: "react"}}}, nil
}

func TestV2CanonicalRecipeCommandsUseOneAsyncWorkshop(t *testing.T) {
	m, _ := newFocusTestModel()
	m.workshop = NewWorkshop(t.Context(), reviewRecipeLibrary{})
	registerRecipeCommands(t.Context(), m.cmdReg, m.workshop.port)
	m.inputBar.SetValue("\t/recipe open \"recipes/example.erpe\"")
	_, cmd := m.inputBar.Update(tea.KeyMsg{Type: tea.KeyEnter}, TabChat)
	if cmd == nil {
		t.Fatal("canonical command did not submit")
	}
	next, cmd := m.Update(cmd())
	m = next.(RootModel)
	if cmd == nil || !m.workshop.working {
		t.Fatal("open performed I/O in Update or failed dispatch")
	}
	next, _ = m.Update(cmd())
	m = next.(RootModel)
	if m.activeTab != TabWorkshop || len(m.workshop.steps) != 2 {
		t.Fatal("real parsed recipe did not reach the shared workshop")
	}
	for _, name := range []string{"recipe save", "recipe run", "recipe new", "workshop", "source", "trigger", "save-recipe"} {
		if _, ok := m.cmdReg.Lookup(name); ok {
			t.Fatalf("read-only library advertises unsupported or obsolete command %s", name)
		}
	}
	m.inputBar.SetValue("/recipe source extra")
	_, cmd = m.inputBar.Update(tea.KeyMsg{Type: tea.KeyEnter}, TabWorkshop)
	next, _ = m.Update(cmd())
	m = next.(RootModel)
	if m.inputBar.Value() != "/recipe source extra" || m.workshop.Editing() {
		t.Fatal("invalid arguments dispatched or cleared the command draft")
	}
	// Completing a multiword path must not duplicate its partially typed suffix.
	m.inputBar.SetValue("/recipe so")
	m.inputBar.updatePalette(parseInputDraft(m.inputBar.Value(), TabWorkshop, false))
	m.inputBar.Update(tea.KeyMsg{Type: tea.KeyTab}, TabWorkshop)
	if m.inputBar.Value() != "/recipe source " {
		t.Fatalf("multiword completion: %q", m.inputBar.Value())
	}
}

func TestV2RecipeEscapeRetainsInvalidSourceDraft(t *testing.T) {
	p := NewWorkshop(t.Context(), reviewRecipeLibrary{})
	cmd := p.load(t.Context(), "recipes/example.erpe")
	p.apply(cmd().(workshopResultMsg))
	p.openField(workshopSource, p.source)
	p.field.SetValue("invalid source")
	p.key(tea.KeyMsg{Type: tea.KeyEsc})
	if !p.Dirty() || p.source != "invalid source" || len(p.diagnostics) == 0 {
		t.Fatal("Escape discarded invalid source without review")
	}
	p.reviewSave()
	if p.mode == workshopSave {
		t.Fatal("read-only live source offered a save")
	}
	p.openField(workshopSource, p.source)
	if p.field.Value() != "invalid source" {
		t.Fatal("reopening source lost the recovery draft")
	}
}

func TestV2CircuitDiagramKeepsGeometryAndHonestEdges(t *testing.T) {
	steps := []CircuitStep{{ID: "read", Status: "done"}, {ID: "edit", Dependencies: []string{"read"}, Status: "active"}, {ID: "verify", Dependencies: []string{"edit"}, Status: "pending"}}
	before, ok := circuitDiagram(steps, "edit", 80, false)
	if !ok || strings.Count(before, "-->") != 2 {
		t.Fatalf("declared chain was not rendered: %s", before)
	}
	steps[1].Status = "failed"
	steps[2].Status = "done"
	after, ok := circuitDiagram(steps, "edit", 80, false)
	if !ok {
		t.Fatal("activity changed graph eligibility")
	}
	for _, label := range []string{"read", "> edit", "verify"} {
		if strings.Index(before, label) != strings.Index(after, label) {
			t.Fatal("outcome moved a source definition")
		}
	}
	steps[2].Dependencies = []string{"read"}
	if _, ok := circuitDiagram(steps, "edit", 80, false); ok {
		t.Fatal("nonadjacent edge was rendered as an invented chain")
	}
	steps[2].Dependencies = nil
	steps[1].Dependencies = nil
	unconnected, ok := circuitDiagram(steps, "", 80, true)
	if !ok || strings.Contains(unconnected, "-->") || strings.Contains(unconnected, "completed") {
		t.Fatal("draft diagram invented dependencies or execution outcomes")
	}
}

func TestV2AgentChooserConsumesKeysBeforeSettings(t *testing.T) {
	m, _ := newFocusTestModel()
	m.setActiveTab(TabSettings)
	m.setFocus(FocusRegionRegion1)
	m.inputBar.SetValue("request")
	m.openAgentPicker()
	if !m.agentPicker.IsOpen() {
		t.Fatal("chooser did not open")
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(RootModel)
	if m.activeTab == TabDoctor || m.inputBar.Value() != "request" || m.agentPicker.IsOpen() {
		t.Fatal("chooser Enter escaped into the settings canvas")
	}
}

func TestV2AttentionPasteCannotEditUnderlyingRequest(t *testing.T) {
	m, _ := newFocusTestModel()
	m.inputBar.SetValue("request")
	m.notifQ.Push(NotificationItem{ID: "permission", Kind: NotifKindHITL, Msg: "one-time write approval"})
	m.decisionFocused = true
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("yes"), Paste: true})
	m = next.(RootModel)
	if m.inputBar.Value() != "request" || cmd != nil {
		t.Fatal("paste escaped the focused decision or approved it")
	}
}

func TestV2LateRecipeRunCannotReplaceChosenCanvas(t *testing.T) {
	m, _ := newFocusTestModel()
	m.setActiveTab(TabWorkshop)
	epoch := m.navigationEpoch
	m.setActiveTab(TabChat)
	next, _ := m.Update(workshopResultMsg{Epoch: m.workshop.epoch, Navigation: epoch, Operation: "trigger", RunResult: ports.RecipeRunResult{Status: "accepted"}})
	m = next.(RootModel)
	if m.activeTab != TabChat {
		t.Fatal("late recipe acknowledgement replaced a canvas selected after submission")
	}
}

func TestV2HelpSupportsScopedDrilldown(t *testing.T) {
	m, _ := newFocusTestModel()
	m.setActiveTab(TabDoctor)
	m.inputBar.SetValue("draft")
	rootHandleHelp(&m, nil)
	if !strings.Contains(m.helpView(), "> /doctor") {
		t.Fatal("help did not prioritize the selected canvas")
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(RootModel)
	if m.helpCommand != "doctor" {
		t.Fatal("Enter did not open selected command detail")
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(RootModel)
	if !m.showHelp || m.helpCommand != "" {
		t.Fatal("first Escape did not return from detail")
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(RootModel)
	if m.showHelp || m.inputBar.Value() != "draft" {
		t.Fatal("closing help lost the underlying draft")
	}
}
