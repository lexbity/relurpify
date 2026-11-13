package tui

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"codeburg.org/lexbit/relurpify/app/relurpish/ports"
	thoughtrecipe "codeburg.org/lexbit/relurpify/named/euclo/thoughtrecipes"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

const recipeSourceLimit = 1 << 20
const workshopCreate = "create-recipe"

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
	Epoch      uint64
	Navigation uint64
	Operation  string
	Files      []ports.RecipeFile
	File       ports.RecipeFile
	Catalog    ports.RecipeCatalog
	RunResult  ports.RecipeRunResult
	Err        error
}

// Workshop edits one concrete source artifact deeply. Its local undo history
// never carries execution markers or claims that a draft was run.
type Workshop struct {
	parent          context.Context
	port            ports.RecipeLibrary
	mode            workshopMode
	files           []ports.RecipeFile
	file            ports.RecipeFile
	source          string
	document        *thoughtrecipe.Document
	catalog         ports.RecipeCatalog
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
	runAfterLoad    bool
	viewEpoch       uint64
	demo            bool
	previewOffset   int
}

// NewWorkshop constructs the single-document recipe authoring surface.
func NewWorkshop(parent context.Context, port ports.RecipeLibrary) *Workshop {
	field := textarea.New()
	field.ShowLineNumbers = false
	field.CharLimit = recipeSourceLimit
	field.Prompt = ""
	return &Workshop{parent: parent, port: port, mode: workshopLibrary, field: field}
}
func (p *Workshop) SetSize(width, height int) {
	p.width, p.height = width, height
	p.field.SetWidth(max(1, width-2))
	p.field.SetHeight(max(1, height-11))
}
func (p *Workshop) Dirty() bool {
	return p.source != p.file.Source || ((p.mode == workshopSource || p.mode == workshopGoal) && p.field.Value() != p.fieldOriginal)
}
func (p *Workshop) OwnsEscape() bool { return p.mode != workshopCanvas && p.mode != workshopLibrary }
func (p *Workshop) Editing() bool {
	return p.mode == workshopSource || p.mode == workshopGoal || p.mode == workshopRun
}

func (p *Workshop) load(parent context.Context, id string) tea.Cmd {
	if p.port == nil {
		p.status = "Recipe authoring service unavailable in this session"
		return nil
	}
	p.epoch++
	epoch, port := p.epoch, p.port
	p.working = true
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 5*time.Second)
		defer cancel()
		catalog, err := port.RecipeCatalog(ctx)
		if err != nil {
			return workshopResultMsg{Epoch: epoch, Err: err}
		}
		if id == "" {
			files, err := port.ListRecipes(ctx)
			return workshopResultMsg{Epoch: epoch, Operation: "list", Files: files, Catalog: catalog, Err: err}
		}
		file, err := port.OpenRecipe(ctx, id)
		return workshopResultMsg{Epoch: epoch, Operation: "open", File: file, Catalog: catalog, Err: err}
	}
}

func (p *Workshop) apply(msg workshopResultMsg) {
	if msg.Epoch != p.epoch {
		return
	}
	p.working = false
	if msg.Err != nil {
		p.runAfterLoad = false
		p.status = msg.Err.Error()
		return
	}
	switch msg.Operation {
	case workshopCreate:
		p.catalog = msg.Catalog
		p.newDraft(msg.File)
	case "list":
		p.file = ports.RecipeFile{}
		p.source, p.document, p.steps = "", nil, nil
		p.undo, p.redo = nil, nil
		p.files = msg.Files
		p.catalog = msg.Catalog
		p.mode = workshopLibrary
		p.selection = 0
	case "open":
		if len(msg.File.Source) > recipeSourceLimit || !utf8.ValidString(msg.File.Source) {
			p.status = "Recipe source must be valid UTF-8 and at most 1 MiB"
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
		if p.runAfterLoad {
			p.runAfterLoad = false
			p.reviewRun()
		}
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
	if len(source) > recipeSourceLimit {
		p.status = "Local source exceeds 1 MiB; the previous draft was retained"
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

func (p *Workshop) structuredChange(source string) bool {
	document, err := thoughtrecipe.ParseSource(p.file.Path, source)
	if err != nil {
		p.status = "Edit rejected: " + err.Error()
		return false
	}
	if diagnostics := thoughtrecipe.ValidateAgainstContracts(document, nil); len(diagnostics) > 0 {
		p.status = "Edit rejected: " + diagnostics[0].Error()
		return false
	}
	return p.change(source)
}

func (p *Workshop) newDraft(file ports.RecipeFile) {
	strategy := p.defaultStrategy()
	if strategy == "" {
		p.status = "No installed strategy is available; no draft was replaced"
		return
	}
	p.file = file
	p.source = "thoughtrecipe " + file.ID + "\n\"New recipe\"\n\ninput prompt: user.request\n\nagent worker uses " + strategy + "\n\nrun worker:\n  from input.prompt\n  goal \"Complete the task\"\n  capture result -> state.summary\n"
	p.undo, p.redo = nil, nil
	p.step = 0
	p.parse()
	p.mode = workshopCanvas
	p.status = "New local draft; no file has been created"
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
	p.field.SetValue(text)
	p.fieldOriginal = p.field.Value()
	p.field.Focus()
}

func (p *Workshop) defaultStrategy() string {
	for _, strategy := range p.catalog.Strategies {
		if strategy.Name == "react" {
			return strategy.Name
		}
	}
	if len(p.catalog.Strategies) > 0 {
		return p.catalog.Strategies[0].Name
	}
	return ""
}

// keepField closes a field without discarding its unsaved bytes. Invalid
// source remains a local recovery draft and cannot be saved or run.
func (p *Workshop) keepField() bool {
	switch p.mode {
	case workshopSource:
		if p.field.Value() != p.fieldOriginal && !p.change(p.field.Value()) {
			return false
		}
	case workshopGoal:
		if p.field.Value() != p.fieldOriginal && len(p.steps) > 0 {
			source, err := patchRecipeGoal(p.source, p.steps[p.step], p.field.Value())
			if err != nil {
				p.status = err.Error()
				return false
			}
			if !p.change(source) {
				return false
			}
		}
	}
	p.field.Blur()
	p.mode = workshopCanvas
	return true
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
			p.keepField()
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
			return p.load(p.parent, p.files[p.selection].Path), true
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
		if p.structuredChange(source) {
			p.mode = workshopCanvas
		}
	case workshopSave:
		return p.save(), true
	case workshopClose:
		p.source = p.file.Source
		p.parse()
		p.undo, p.redo = nil, nil
		return p.load(p.parent, ""), true
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
	if _, ok := p.port.(ports.RecipeWriter); !ok {
		p.status = "Live recipe saving is unavailable until confined source transactions are integrated; the local draft is retained"
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
	if _, ok := p.port.(ports.RecipeRunner); !ok {
		p.status = "Saved-file execution is unavailable in this session; validation does not execute a recipe"
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
	port, ok := p.port.(ports.RecipeWriter)
	if !ok {
		p.reviewSave()
		return nil
	}
	epoch, parent := p.epoch, p.parent
	p.working = true
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 5*time.Second)
		defer cancel()
		saved, err := port.SaveRecipe(ctx, file)
		return workshopResultMsg{Epoch: epoch, Operation: "save", File: saved, Err: err}
	}
}
func (p *Workshop) trigger(input string) tea.Cmd {
	if p.Dirty() || strings.TrimSpace(input) == "" {
		p.status = "Trigger requires a saved file and a request input"
		return nil
	}
	port, ok := p.port.(ports.RecipeRunner)
	if !ok {
		p.reviewRun()
		return nil
	}
	file, epoch, parent, navigation := p.file, p.epoch, p.parent, p.viewEpoch
	p.working = true
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 5*time.Second)
		defer cancel()
		snapshot, err := port.TriggerRecipe(ctx, file, input)
		return workshopResultMsg{Epoch: epoch, Navigation: navigation, Operation: "trigger", RunResult: snapshot, Err: err}
	}
}

func (p *Workshop) View() string {
	title := "Recipe / " + terminalText(p.file.Path)
	if p.file.ID == "" {
		title = "Recipe library"
	}
	if p.Dirty() {
		title += " · draft"
	}
	lines := []string{title, ""}
	if p.Editing() {
		label := map[workshopMode]string{workshopSource: "Source draft", workshopGoal: "Goal draft", workshopRun: "Trigger saved file: " + p.file.Path}[p.mode]
		lines = append(lines, terminalText(label), "", p.field.View(), "", "Ctrl+S confirms · Escape keeps a local draft")
	} else {
		switch p.mode {
		case workshopCanvas:
			var definitions []CircuitStep
			for i, step := range p.steps {
				definition := CircuitStep{ID: step.name, Strategy: step.strategy}
				for _, prior := range p.steps[:i] {
					if slices.Contains(prior.captures, step.from) {
						definition.Dependencies = append(definition.Dependencies, prior.name)
					}
				}
				definitions = append(definitions, definition)
			}
			selected := ""
			if p.step < len(p.steps) {
				selected = p.steps[p.step].name
			}
			diagram, graph := circuitDiagram(definitions, selected, p.width, true)
			if graph && p.height >= 16 {
				lines = append(lines, "", diagram, "", "Authoring · arrows show source bindings; no run outcomes")
			}
			budget := max(1, p.height-7)
			start := max(0, p.step-budget+1)
			for i := start; i < min(len(p.steps), start+budget); i++ {
				if graph && p.height >= 16 {
					break
				}
				step := p.steps[i]
				marker := " "
				if i == p.step {
					marker = ">"
				}
				lines = append(lines, fmt.Sprintf("%s [ ] %-18s %s", marker, terminalText(step.name), terminalText(step.strategy)))
			}
			if len(p.steps) == 0 {
				lines = append(lines, "No editable top-level runs. /recipe source opens the complete source.")
			}
			lines = append(lines, "", "↑/↓ select · Enter edit step · /recipe source")
			if _, ok := p.port.(ports.RecipeWriter); ok {
				lines = append(lines, "/recipe save reviews saving this draft")
			} else {
				lines = append(lines, "Local draft only · live save/run integration unavailable")
			}
		case workshopDiagnostics:
			lines = append(lines, "Diagnostics", "")
			if len(p.diagnostics) == 0 {
				lines = append(lines, "Syntax and installed strategy contracts checked.", "Prompt imports, tool resolution, and runtime behavior are not verified here.")
			} else {
				for _, diagnostic := range p.diagnostics {
					lines = append(lines, terminalText(diagnostic))
				}
			}
			lines = append(lines, "", "Escape returns · /recipe source edits the full source")
		case workshopSave:
			lines = append(lines, "Review saved source change", "", terminalText(p.file.Path), "Expected saved revision: "+terminalText(p.file.Revision), fmt.Sprintf("Source: %d → %d bytes", len(p.file.Source), len(p.source)), "", "Existing source outside selected edits is preserved.", "Syntax/strategy checks do not prove runtime behavior.", "", "Enter saves this revision · Escape keeps the local draft")
		case workshopClose:
			lines = append(lines, "Discard unsaved local edits?", "", "Enter discards edits and returns to the recipe library.", "Escape keeps editing. /recipe save reviews saving first.")
		default:
			lines = append(lines, strings.ReplaceAll(string(p.mode), "-", " "), "")
			options := p.options()
			budget := max(1, p.height-11)
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
