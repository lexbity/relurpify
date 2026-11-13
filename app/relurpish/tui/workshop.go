package tui

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	thoughtrecipe "codeburg.org/lexbit/relurpify/named/euclo/thoughtrecipes"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

type workshopMode string

const (
	workshopLibrary     workshopMode = "library"
	workshopCanvas      workshopMode = "recipe"
	workshopMenu        workshopMode = "edit-step"
	workshopStrategy    workshopMode = "strategy"
	workshopConnection  workshopMode = "connection"
	workshopPrompt      workshopMode = "prompt"
	workshopGoal        workshopMode = "goal"
	workshopSource      workshopMode = "source"
	workshopDiagnostics workshopMode = "diagnostics"
	workshopSave        workshopMode = "save-review"
	workshopRun         workshopMode = "run-review"
	workshopClose       workshopMode = "dirty-close"
)

type workshopResultMsg struct {
	Epoch     uint64
	Operation string
	Files     []RecipeFile
	File      RecipeFile
	Catalog   RecipeCatalog
	RunResult RecipeRunResult
	Err       error
}

// Workshop edits one concrete source artifact deeply. Its local undo history
// never carries execution markers or claims that a draft was run.
type Workshop struct {
	parent          context.Context
	port            RecipeWorkspace
	mode            workshopMode
	files           []RecipeFile
	file            RecipeFile
	source          string
	document        *thoughtrecipe.Document
	catalog         RecipeCatalog
	diagnostics     []string
	steps           []recipeStepSource
	step, selection int
	undo, redo      []string
	field           textarea.Model
	fieldOriginal   string
	status          string
	working         bool
	epoch           uint64
	width, height   int
	promptPreview   bool
	demo            bool
	previewOffset   int
}

func NewWorkshop(parent context.Context, port RecipeWorkspace) *Workshop {
	field := textarea.New()
	field.ShowLineNumbers = false
	field.CharLimit = 262144
	field.Prompt = ""
	return &Workshop{parent: parent, port: port, mode: workshopLibrary, field: field}
}
func (p *Workshop) SetSize(width, height int) {
	p.width, p.height = width, height
	p.field.SetWidth(max(1, width-2))
	p.field.SetHeight(max(1, height-9))
}
func (p *Workshop) Dirty() bool {
	return p.source != p.file.Source || ((p.mode == workshopSource || p.mode == workshopGoal) && p.field.Value() != p.fieldOriginal)
}
func (p *Workshop) OwnsEscape() bool { return p.mode != workshopCanvas && p.mode != workshopLibrary }
func (p *Workshop) Editing() bool {
	return p.mode == workshopSource || p.mode == workshopGoal || p.mode == workshopRun
}

func (p *Workshop) load(id string) tea.Cmd {
	if p.port == nil {
		p.status = "Recipe authoring service unavailable in this session"
		return nil
	}
	p.epoch++
	epoch, port, parent := p.epoch, p.port, p.parent
	p.working = true
	return func() tea.Msg {
		catalog, err := port.RecipeCatalog(parent)
		if err != nil {
			return workshopResultMsg{Epoch: epoch, Err: err}
		}
		if id == "" {
			files, err := port.ListRecipes(parent)
			return workshopResultMsg{Epoch: epoch, Operation: "list", Files: files, Catalog: catalog, Err: err}
		}
		file, err := port.OpenRecipe(parent, id)
		return workshopResultMsg{Epoch: epoch, Operation: "open", File: file, Catalog: catalog, Err: err}
	}
}

func (p *Workshop) apply(msg workshopResultMsg) {
	if msg.Epoch != p.epoch {
		return
	}
	p.working = false
	if msg.Err != nil {
		p.status = msg.Err.Error()
		return
	}
	switch msg.Operation {
	case "list":
		p.files = msg.Files
		p.catalog = msg.Catalog
		p.mode = workshopLibrary
		p.selection = 0
	case "open":
		if len(msg.File.Source) > 262144 || !utf8.ValidString(msg.File.Source) {
			p.status = "Recipe source must be valid UTF-8 and at most 256 KiB"
			return
		}
		p.file = msg.File
		p.catalog = msg.Catalog
		p.source = msg.File.Source
		p.undo, p.redo = nil, nil
		p.parse()
		p.mode = workshopCanvas
		p.step = 0
		p.status = "Saved source opened; no execution was triggered"
	case "save":
		p.file = msg.File
		p.status = "Saved source revision " + msg.File.Revision
		if p.demo {
			p.status = "DEMO: source saved in memory @" + msg.File.Revision
		}
		p.mode = workshopCanvas
	case "trigger":
		p.status = "Saved file was explicitly triggered"
		if p.demo {
			p.status = "DEMO: saved in-memory recipe triggered; no live execution"
		}
		p.mode = workshopCanvas
	}
}

func (p *Workshop) parse() {
	p.diagnostics = nil
	document, err := thoughtrecipe.ParseSource(p.file.Path, p.source)
	if err != nil {
		p.document = nil
		p.steps = nil
		p.diagnostics = []string{err.Error()}
		return
	}
	p.document = document
	p.steps = recipeSteps(document)
	for _, err := range thoughtrecipe.ValidateAgainstContracts(document, nil) {
		p.diagnostics = append(p.diagnostics, err.Error())
	}
	p.step = min(p.step, max(0, len(p.steps)-1))
}

func (p *Workshop) change(source string) bool {
	if len(source) > 262144 {
		p.status = "Local source exceeds 256 KiB; the previous draft was retained"
		return false
	}
	if source == p.source {
		return true
	}
	p.undo = append(p.undo, p.source)
	p.redo = nil
	bytes := 0
	for i := len(p.undo) - 1; i >= 0; i-- {
		bytes += len(p.undo[i])
		if len(p.undo)-i > 100 || bytes > 8<<20 {
			p.undo = slices.Clone(p.undo[i+1:])
			break
		}
	}
	p.source = source
	p.parse()
	p.status = "Local draft changed; saved recipe is unchanged"
	return true
}

func (p *Workshop) history(redo bool) {
	if p.working {
		return
	}
	from, to := &p.undo, &p.redo
	if redo {
		from, to = &p.redo, &p.undo
	}
	if len(*from) == 0 {
		p.status = "No further local edit history"
		return
	}
	*to = append(*to, p.source)
	p.source = (*from)[len(*from)-1]
	*from = (*from)[:len(*from)-1]
	p.parse()
	p.status = "Local draft history restored"
}

func (p *Workshop) openField(mode workshopMode, text string) {
	if strings.ContainsAny(text, "\x1b\x00") {
		p.status = "Source contains terminal control characters; edit it outside the structured editor"
		return
	}
	p.mode = mode
	p.fieldOriginal = text
	p.field.SetValue(text)
	p.field.Focus()
}

func (p *Workshop) key(msg tea.KeyMsg) (tea.Cmd, bool) {
	if p.working {
		if msg.String() == "esc" {
			return nil, false
		}
		return nil, true
	}
	if p.Editing() {
		if msg.String() == "esc" {
			p.mode = workshopCanvas
			p.field.Blur()
			return nil, true
		}
		if msg.String() == "ctrl+s" {
			switch p.mode {
			case workshopSource:
				if p.change(p.field.Value()) {
					p.mode = workshopCanvas
				}
			case workshopGoal:
				if len(p.steps) > 0 {
					source, err := patchRecipeGoal(p.source, p.steps[p.step], p.field.Value())
					if err != nil {
						p.status = err.Error()
						return nil, true
					}
					if p.change(source) {
						p.mode = workshopCanvas
					}
				}
			case workshopRun:
				return p.trigger(p.field.Value()), true
			}
			return nil, true
		}
		if msg.Paste && strings.ContainsAny(string(msg.Runes), "\x1b\x00") {
			p.status = "Paste rejected: terminal control characters are not recipe source"
			return nil, true
		}
		var cmd tea.Cmd
		p.field, cmd = p.field.Update(msg)
		return cmd, true
	}
	if msg.String() == "esc" {
		if p.mode != workshopCanvas && p.mode != workshopLibrary {
			p.mode = workshopCanvas
			p.selection = 0
			return nil, true
		}
		return nil, false
	}
	if msg.String() == "tab" && p.mode == workshopPrompt {
		p.promptPreview = !p.promptPreview
		p.previewOffset = 0
		return nil, true
	}
	if p.mode == workshopPrompt && p.promptPreview && (msg.String() == "pgdown" || msg.String() == "pgup") {
		delta := max(1, p.height/3)
		if msg.String() == "pgup" {
			delta = -delta
		}
		p.previewOffset = max(0, p.previewOffset+delta)
		return nil, true
	}
	if msg.String() == "up" || msg.String() == "down" || msg.String() == "home" || msg.String() == "end" {
		delta := 1
		if msg.String() == "up" {
			delta = -1
		}
		if p.mode == workshopCanvas {
			p.step = min(max(0, p.step+delta), max(0, len(p.steps)-1))
			if msg.String() == "home" {
				p.step = 0
			}
			if msg.String() == "end" {
				p.step = max(0, len(p.steps)-1)
			}
		} else {
			p.previewOffset = 0
			p.selection = min(max(0, p.selection+delta), max(0, len(p.options())-1))
			if msg.String() == "home" {
				p.selection = 0
			}
			if msg.String() == "end" {
				p.selection = max(0, len(p.options())-1)
			}
		}
		return nil, true
	}
	if msg.String() != "enter" {
		return nil, false
	}
	switch p.mode {
	case workshopLibrary:
		if len(p.files) > 0 {
			return p.load(p.files[p.selection].ID), true
		}
	case workshopCanvas:
		if len(p.steps) > 0 {
			p.mode = workshopMenu
			p.selection = 0
		}
	case workshopMenu:
		switch p.selection {
		case 0:
			p.mode = workshopStrategy
		case 1:
			p.openField(workshopGoal, p.steps[p.step].goal)
		case 2:
			p.mode = workshopConnection
		case 3:
			p.mode = workshopPrompt
		}
		p.selection = 0
	case workshopStrategy, workshopConnection, workshopPrompt:
		options := p.options()
		if len(options) == 0 {
			return nil, true
		}
		choice := options[p.selection]
		var source string
		var err error
		switch p.mode {
		case workshopStrategy:
			source, err = patchRecipeStrategy(p.source, p.steps[p.step], choice)
		case workshopConnection:
			source, err = patchRecipeConnection(p.source, p.steps[p.step], choice)
		case workshopPrompt:
			source, err = patchRecipePrompt(p.source, p.document, p.steps[p.step], choice)
		}
		if err != nil {
			p.status = err.Error()
			return nil, true
		}
		p.change(source)
		p.mode = workshopCanvas
	case workshopSave:
		return p.save(), true
	case workshopClose:
		p.source = p.file.Source
		p.parse()
		p.undo, p.redo = nil, nil
		return p.load(""), true
	}
	return nil, true
}

func (p *Workshop) options() []string {
	var options []string
	switch p.mode {
	case workshopLibrary:
		for _, file := range p.files {
			options = append(options, file.ID)
		}
	case workshopMenu:
		options = []string{"Choose strategy from cognition zoo", "Edit goal", "Connect input/context data", "Bind an available prompt"}
	case workshopStrategy:
		for _, strategy := range p.catalog.Strategies {
			options = append(options, strategy.Name)
		}
	case workshopConnection:
		options = recipeConnections(p.document, p.step)
	case workshopPrompt:
		for _, prompt := range p.catalog.Prompts {
			options = append(options, prompt.ID)
		}
	}
	return options
}

func (p *Workshop) reviewSave() {
	if p.working {
		return
	}
	if len(p.diagnostics) > 0 {
		p.mode = workshopDiagnostics
		return
	}
	p.mode = workshopSave
}
func (p *Workshop) reviewRun() {
	if p.working {
		return
	}
	if p.Dirty() {
		p.status = "Save the local draft before triggering the saved file"
		return
	}
	if len(p.diagnostics) > 0 {
		p.mode = workshopDiagnostics
		return
	}
	p.openField(workshopRun, "Run the saved recipe against the current task")
}

func (p *Workshop) save() tea.Cmd {
	if len(p.diagnostics) > 0 {
		p.mode = workshopDiagnostics
		return nil
	}
	file := p.file
	file.Source = p.source
	epoch, port, parent := p.epoch, p.port, p.parent
	p.working = true
	return func() tea.Msg {
		saved, err := port.SaveRecipe(parent, file)
		return workshopResultMsg{Epoch: epoch, Operation: "save", File: saved, Err: err}
	}
}
func (p *Workshop) trigger(input string) tea.Cmd {
	if p.Dirty() || strings.TrimSpace(input) == "" {
		p.status = "Trigger requires a saved file and a request input"
		return nil
	}
	file, epoch, port, parent := p.file, p.epoch, p.port, p.parent
	p.working = true
	return func() tea.Msg {
		snapshot, err := port.TriggerRecipe(parent, file, input)
		return workshopResultMsg{Epoch: epoch, Operation: "trigger", RunResult: snapshot, Err: err}
	}
}

func (p *Workshop) View() string {
	title := "Workshop / " + terminalText(p.file.ID)
	if p.file.ID == "" {
		title = "Recipe library"
	}
	if p.Dirty() {
		title += " · draft"
	}
	lines := []string{title, ""}
	if p.Editing() {
		label := map[workshopMode]string{workshopSource: "Source draft", workshopGoal: "Goal draft", workshopRun: "Trigger saved file: " + p.file.Path}[p.mode]
		lines = append(lines, terminalText(label), "", p.field.View(), "", "Ctrl+S confirms · Escape cancels this field")
	} else {
		switch p.mode {
		case workshopCanvas:
			budget := max(1, p.height-7)
			start := max(0, p.step-budget+1)
			for i := start; i < min(len(p.steps), start+budget); i++ {
				step := p.steps[i]
				marker := " "
				if i == p.step {
					marker = ">"
				}
				lines = append(lines, fmt.Sprintf("%s [ ] %-18s %s", marker, terminalText(step.name), terminalText(step.strategy)))
			}
			if len(p.steps) == 0 {
				lines = append(lines, "No editable top-level runs. /source opens the complete source.")
			}
			lines = append(lines, "", "↑/↓ select · Enter edit step · /source · /save-recipe · /trigger")
		case workshopDiagnostics:
			lines = append(lines, "Diagnostics", "")
			if len(p.diagnostics) == 0 {
				lines = append(lines, "Syntax and installed strategy contracts checked.", "Prompt imports, tool resolution, and runtime behavior are not verified here.")
			} else {
				for _, diagnostic := range p.diagnostics {
					lines = append(lines, terminalText(diagnostic))
				}
			}
			lines = append(lines, "", "Escape returns · /source edits the full source")
		case workshopSave:
			lines = append(lines, "Review saved source change", "", terminalText(p.file.Path), "Expected saved revision: "+terminalText(p.file.Revision), fmt.Sprintf("Source: %d → %d bytes", len(p.file.Source), len(p.source)), "", "Existing source outside selected edits is preserved.", "Syntax/strategy checks do not prove runtime behavior.", "", "Enter saves this revision · Escape keeps the local draft")
		case workshopClose:
			lines = append(lines, "Discard unsaved local edits?", "", "Enter discards edits and returns to the recipe library.", "Escape keeps editing. /save-recipe reviews saving first.")
		default:
			lines = append(lines, strings.ReplaceAll(string(p.mode), "-", " "), "")
			options := p.options()
			budget := max(1, p.height-9)
			if p.mode == workshopPrompt && p.promptPreview {
				budget = max(1, p.height/4)
			}
			start := max(0, p.selection-budget+1)
			for i := start; i < min(len(options), start+budget); i++ {
				marker := " "
				if i == p.selection {
					marker = ">"
				}
				lines = append(lines, marker+" "+terminalText(options[i]))
			}
			if len(options) == 0 {
				lines = append(lines, "No compatible options available.")
			}
			if p.mode == workshopPrompt && len(options) > 0 {
				lines = append(lines, "", "Available/reference binding; this is not evidence of actual prompt use.")
				if p.promptPreview {
					prompt := p.catalog.Prompts[p.selection]
					lines = append(lines, "Preview · "+terminalText(prompt.Version+" · "+prompt.Origin))
					preview := strings.Split(ansi.Hardwrap(terminalText(prompt.Text), max(1, p.width), true), "\n")
					rows := max(1, p.height-len(lines)-5)
					start := min(p.previewOffset, max(0, len(preview)-rows))
					lines = append(lines, preview[start:min(len(preview), start+rows)]...)
				} else {
					lines = append(lines, "Tab previews the selected prompt")
				}
			}
			lines = append(lines, "", "↑/↓ select · Enter confirms · Escape returns")
		}
	}
	if p.working {
		lines = append(lines, "", "Waiting for application result…")
	}
	if p.status != "" {
		lines = append(lines, "", terminalText(p.status))
	}
	return boundedScreen(strings.Join(lines, "\n"), max(1, p.width), max(1, p.height))
}

func registerWorkshopCommands(registry *CommandRegistry) {
	registry.Register(Command{Name: "workshop", Usage: "/workshop [recipe-id]", Description: "Open a saved recipe for deliberate authoring", Handler: func(m *RootModel, args []string) (*RootModel, tea.Cmd) {
		if len(args) > 1 {
			m.addSystemMessage("Usage: /workshop [recipe-id]")
			return m, nil
		}
		if m.workshop.Dirty() {
			m.workshop.mode = workshopClose
			m.setActiveTab(TabWorkshop)
			m.setFocus(FocusRegionRegion1)
			return m, nil
		}
		id := ""
		if len(args) == 1 {
			id = args[0]
		}
		m.setActiveTab(TabWorkshop)
		m.setFocus(FocusRegionRegion1)
		return m, m.workshop.load(id)
	}})
	for _, name := range []string{"source", "save-recipe", "trigger", "diagnostics", "undo", "redo", "step-add"} {
		registry.Register(Command{Name: name, Usage: "/" + name, Description: map[string]string{"source": "Edit the full recipe source draft", "save-recipe": "Review saving the recipe source", "trigger": "Review triggering the saved recipe file", "diagnostics": "Inspect source and strategy diagnostics", "undo": "Undo a local recipe edit", "redo": "Redo a local recipe edit", "step-add": "Add a local run: /step-add <agent-name>"}[name], TabFilter: []TabID{TabWorkshop}, Handler: workshopCommand(name)})
	}
}

func workshopCommand(name string) CommandHandler {
	return func(m *RootModel, args []string) (*RootModel, tea.Cmd) {
		p := m.workshop
		if p.Editing() {
			m.addSystemMessage("Apply or cancel the active recipe field before another recipe operation")
			return m, nil
		}
		if p.working {
			m.addSystemMessage("Wait for the pending recipe operation")
			return m, nil
		}
		if p.file.ID == "" {
			m.addSystemMessage("Open a recipe before authoring it")
			return m, nil
		}
		switch name {
		case "source":
			p.openField(workshopSource, p.source)
		case "save-recipe":
			p.reviewSave()
		case "trigger":
			p.reviewRun()
		case "diagnostics":
			p.mode = workshopDiagnostics
		case "undo":
			p.history(false)
		case "redo":
			p.history(true)
		case "step-add":
			if len(args) != 1 || !validRecipeIdentifier(args[0]) {
				m.addSystemMessage("Usage: /step-add <agent-name> (letters, digits, underscore)")
				return m, nil
			}
			if p.document == nil {
				m.addSystemMessage("Repair source diagnostics before adding a step")
				return m, nil
			}
			for _, declaration := range p.document.Declarations {
				if agent, ok := declaration.(*thoughtrecipe.AgentDecl); ok && agent.Name.Value == args[0] {
					m.addSystemMessage("Agent name already appears in this recipe")
					return m, nil
				}
			}
			p.change(strings.TrimRight(p.source, "\n") + fmt.Sprintf("\n\nagent %s uses react\n\nrun %s:\n  from input.prompt\n  goal \"Complete the task\"\n  capture result -> state.%s\n", args[0], args[0], args[0]))
		}
		m.setFocus(FocusRegionRegion1)
		return m, nil
	}
}
