package tui

import (
	"context"
	"path"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"codeburg.org/lexbit/relurpify/app/relurpish/ports"
	tea "github.com/charmbracelet/bubbletea"
)

type recipeAction uint8

const (
	recipeCloseAction recipeAction = iota
	recipeSourceAction
	recipeValidateAction
	recipeUndoAction
	recipeRedoAction
	recipeAddAction
	recipeSaveAction
)

// registerRecipeCommands exposes only operations supplied by this session.
// All authoring uses Workshop's single source document, parser and review flow.
func registerRecipeCommands(parent context.Context, registry *CommandRegistry, library ports.RecipeLibrary) {
	if library == nil {
		return
	}
	commands := []Command{
		{Name: "recipe open", Usage: "/recipe open [path]", Description: "Open one saved recipe, or choose from the library", Handler: recipeOpenCommand, Arguments: &ArgumentCount{Max: 1}},
		{Name: "recipe close", Usage: "/recipe close", Description: "Close the recipe after reviewing unsaved edits", Handler: recipeEditCommand(parent, recipeCloseAction), Arguments: &ArgumentCount{}},
		{Name: "recipe source", Usage: "/recipe source", Description: "Edit the complete local source draft", Handler: recipeEditCommand(parent, recipeSourceAction), Arguments: &ArgumentCount{}},
		{Name: "recipe validate", Usage: "/recipe validate", Description: "Check source with the parser and installed strategy contracts", Handler: recipeEditCommand(parent, recipeValidateAction), Arguments: &ArgumentCount{}},
		{Name: "recipe undo", Usage: "/recipe undo", Description: "Undo a local source transaction", Handler: recipeEditCommand(parent, recipeUndoAction), Arguments: &ArgumentCount{}},
		{Name: "recipe redo", Usage: "/recipe redo", Description: "Redo a local source transaction", Handler: recipeEditCommand(parent, recipeRedoAction), Arguments: &ArgumentCount{}},
		{Name: "step add", Usage: "/step add <agent-name>", Description: "Add a source run with an installed strategy", Handler: recipeEditCommand(parent, recipeAddAction), Arguments: &ArgumentCount{Min: 1, Max: 1}, TabFilter: []TabID{TabWorkshop}},
	}
	if _, ok := library.(ports.RecipeWriter); ok {
		commands = append(commands,
			Command{Name: "recipe save", Usage: "/recipe save", Description: "Review saving the validated local source", Handler: recipeEditCommand(parent, recipeSaveAction), Arguments: &ArgumentCount{}},
			Command{Name: "recipe new", Usage: "/recipe new <path>", Description: "Start a new local recipe; saving requires review", Handler: recipeNewCommand, Arguments: &ArgumentCount{Min: 1, Max: 1}})
	}
	if _, ok := library.(ports.RecipeRunner); ok {
		commands = append(commands, Command{Name: "recipe run", Usage: "/recipe run <path>", Description: "Review an exact saved recipe and supply its task input", Handler: recipeRunCommand, Arguments: &ArgumentCount{Min: 1, Max: 1}})
	}
	for _, command := range commands {
		registry.Register(command)
	}
}

func recipeOpenCommand(m *RootModel, args []string) (*RootModel, tea.Cmd) {
	p := m.workshop
	m.setActiveTab(TabWorkshop)
	m.setFocus(FocusRegionRegion1)
	if p.working {
		m.rejectCommand("Wait for the pending recipe operation")
		return m, nil
	}
	if p.Dirty() {
		m.rejectCommand("Local recipe draft retained; save or close it before opening another recipe")
		return m, nil
	}
	id := ""
	if len(args) == 1 {
		id = args[0]
	}
	return m, p.load(m.parentCtx, id)
}

func recipeRunCommand(m *RootModel, args []string) (*RootModel, tea.Cmd) {
	p := m.workshop
	m.setActiveTab(TabWorkshop)
	m.setFocus(FocusRegionRegion1)
	if p.working || p.Dirty() {
		m.rejectCommand("Save or close the local draft and wait for pending work before reviewing a saved-file run")
		return m, nil
	}
	if p.file.Path == args[0] && p.file.Revision != "" {
		p.reviewRun()
		return m, nil
	}
	p.runAfterLoad = true
	return m, p.load(m.parentCtx, args[0])
}

func recipeNewCommand(m *RootModel, args []string) (*RootModel, tea.Cmd) {
	p := m.workshop
	if p.working || p.Dirty() {
		m.rejectCommand("Save or close the current draft before creating a recipe")
		return m, nil
	}
	filename := args[0]
	if !strings.HasSuffix(filename, ".erpe") || !filepath.IsLocal(filename) || path.Clean(filename) != filename || strings.ContainsRune(filename, '\\') || strings.ContainsFunc(filename, unicode.IsControl) {
		m.rejectCommand("Use a workspace-relative .erpe path without traversal or control characters")
		return m, nil
	}
	id := strings.TrimSuffix(path.Base(filename), ".erpe")
	if !validRecipeIdentifier(id) {
		m.rejectCommand("Recipe filename must use letters, digits and underscore")
		return m, nil
	}
	p.epoch++
	epoch, port, parent := p.epoch, p.port, p.parent
	p.working = true
	m.setActiveTab(TabWorkshop)
	m.setFocus(FocusRegionRegion1)
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 5*time.Second)
		defer cancel()
		catalog, err := port.RecipeCatalog(ctx)
		return workshopResultMsg{Epoch: epoch, Operation: workshopCreate, Catalog: catalog, File: ports.RecipeFile{ID: id, Path: filename}, Err: err}
	}
}

func recipeEditCommand(parent context.Context, operation recipeAction) CommandHandler {
	return func(m *RootModel, args []string) (*RootModel, tea.Cmd) {
		p := m.workshop
		if p.working {
			m.rejectCommand("Wait for the pending recipe operation")
			return m, nil
		}
		if p.file.ID == "" {
			m.rejectCommand("Open a recipe before authoring it")
			return m, nil
		}
		if p.Editing() {
			// Source typing is a draft transaction. Leaving the field keeps its
			// bytes, including invalid source, rather than saving behind the user.
			if !p.keepField() {
				m.rejectCommand(p.status)
				return m, nil
			}
		}
		switch operation {
		case recipeSourceAction:
			p.openField(workshopSource, p.source)
		case recipeSaveAction:
			p.reviewSave()
		case recipeValidateAction:
			p.parse()
			p.mode = workshopDiagnostics
		case recipeUndoAction:
			p.history(false)
		case recipeRedoAction:
			p.history(true)
		case recipeCloseAction:
			if p.Dirty() {
				p.mode = workshopClose
			} else {
				return m, p.load(parent, "")
			}
		case recipeAddAction:
			if !validRecipeIdentifier(args[0]) {
				m.rejectCommand("Agent name must use letters, digits and underscore")
				return m, nil
			}
			if p.document == nil {
				m.rejectCommand("Repair source diagnostics before adding a step")
				return m, nil
			}
			for _, step := range p.steps {
				if step.name == args[0] {
					m.rejectCommand("Agent name already appears in this recipe")
					return m, nil
				}
			}
			strategy := p.defaultStrategy()
			if strategy == "" {
				m.rejectCommand("No installed strategy is available")
				return m, nil
			}
			p.structuredChange(strings.TrimRight(p.source, "\n") + "\n\nagent " + args[0] + " uses " + strategy + "\n\nrun " + args[0] + ":\n  from input.prompt\n  goal \"Complete the task\"\n  capture result -> state." + args[0] + "\n")
		}
		m.setActiveTab(TabWorkshop)
		m.setFocus(FocusRegionRegion1)
		return m, nil
	}
}
