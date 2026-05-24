package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/prompt"
	thoughtrecipe "codeburg.org/lexbit/relurpify/named/euclo/thoughtrecipes"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type libraryView int

const (
	libraryViewAll libraryView = iota
	libraryViewRecipes
	libraryViewPrompts
)

type libraryItemKind string

const (
	libraryItemRecipe libraryItemKind = "recipe"
	libraryItemPrompt libraryItemKind = "prompt"
)

type libraryItem struct {
	Kind        libraryItemKind
	ID          string
	Title       string
	Description string
	Source      string
	Tags        []string
	Variables   []string
	Families    []string
	Keywords    []string
	Capabilities []string
	Inputs      []string
	Warnings    []string
	LastUsed    time.Time
}

type LibraryRunRequestedMsg struct {
	RecipeID string
	Prompt   string
}

type libraryRecipeValidationMsg struct {
	RecipeID string
	Result   string
}

type LibraryPane struct {
	runtime RuntimeAdapter

	allItems []libraryItem
	items    []libraryItem
	sel      int
	filter   string
	view     libraryView

	tagFilters map[string]bool
	lastUsed   map[string]time.Time

	selected libraryItem
	status   string
	width    int
	height   int
}

func NewLibraryPane(rt RuntimeAdapter) *LibraryPane {
	p := &LibraryPane{
		runtime:    rt,
		tagFilters: make(map[string]bool),
		lastUsed:   make(map[string]time.Time),
	}
	p.Refresh()
	return p
}

func (p *LibraryPane) SetSize(w, h int) {
	p.width = w
	p.height = h
}

func (p *LibraryPane) SetFilter(filter string) {
	p.filter = strings.ToLower(strings.TrimSpace(filter))
	p.rebuildItems("")
}

func (p *LibraryPane) Refresh() {
	items, err := loadLibraryItems(p.runtime)
	if err != nil {
		p.allItems = nil
		p.items = nil
		p.selected = libraryItem{}
		p.sel = 0
		p.status = fmt.Sprintf("library reload failed: %v", err)
		return
	}
	p.allItems = items
	p.rebuildItems(p.selectedID())
	p.status = fmt.Sprintf("loaded %d recipes and %d prompts", countLibraryItems(items, libraryItemRecipe), countLibraryItems(items, libraryItemPrompt))
}

func (p *LibraryPane) Update(msg tea.Msg) (*LibraryPane, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if p.sel > 0 {
				p.sel--
				p.updateSelected()
			}
		case "down", "j":
			if p.sel < len(p.items)-1 {
				p.sel++
				p.updateSelected()
			}
		case "tab":
			p.view = (p.view + 1) % 3
			p.rebuildItems(p.selectedID())
		case "shift+tab":
			p.view = (p.view + 2) % 3
			p.rebuildItems(p.selectedID())
		case "r", "enter":
			return p.runSelectedCmd()
		case "e":
			return p.openSelectedEditorCmd()
		case "v":
			return p.validateSelected()
		case "t":
			p.toggleSelectedTagFilter()
			p.rebuildItems(p.selectedID())
		case "esc":
			if len(p.tagFilters) > 0 {
				p.tagFilters = make(map[string]bool)
				p.rebuildItems(p.selectedID())
			} else if p.filter != "" {
				p.filter = ""
				p.rebuildItems("")
			}
		}
	}
	return p, nil
}

func (p *LibraryPane) View() string {
	title := p.sectionTabs()
	if len(p.items) == 0 {
		return strings.Join([]string{
			title,
			sectionPanel("Library", max(1, p.width-2), dimStyle.Render("No recipes or prompts match the current filter.")),
			dimStyle.Render("type to filter  tab scope  up/down navigate  r run  e open  v validate  t tag filter  esc clear"),
		}, "\n\n")
	}

	if p.width < 88 {
		return strings.Join([]string{
			title,
			sectionPanel("Catalog", max(1, p.width-2), sectionList(p.leftLines(), p.sel, max(1, p.height-12))),
			sectionPanel("Detail", max(1, p.width-2), p.detailLines()...),
			p.footer(),
		}, "\n\n")
	}

	widths := splitWidths(p.width, 5, 7)
	left := sectionPanel("Catalog", widths[0], sectionList(p.leftLines(), p.sel, max(1, p.height-12)))
	right := sectionPanel("Detail", widths[1], p.detailLines()...)
	return strings.Join([]string{
		title,
		lipgloss.JoinHorizontal(lipgloss.Top, left, right),
		p.footer(),
	}, "\n\n")
}

func (p *LibraryPane) footer() string {
	var parts []string
	parts = append(parts, "type to filter")
	parts = append(parts, "tab scope")
	parts = append(parts, "up/down navigate")
	parts = append(parts, "r run")
	parts = append(parts, "e open")
	parts = append(parts, "v validate")
	parts = append(parts, "t tag filter")
	parts = append(parts, "esc clear")
	if len(p.tagFilters) > 0 {
		var tags []string
		for tag := range p.tagFilters {
			tags = append(tags, tag)
		}
		sort.Strings(tags)
		parts = append(parts, "tags="+strings.Join(tags, ","))
	}
	if p.status != "" {
		parts = append(parts, p.status)
	}
	return dimStyle.Render(strings.Join(parts, "  "))
}

func (p *LibraryPane) sectionTabs() string {
	labels := []struct {
		view  libraryView
		label string
	}{
		{libraryViewAll, "all"},
		{libraryViewRecipes, "recipes"},
		{libraryViewPrompts, "prompts"},
	}
	var parts []string
	for _, l := range labels {
		if l.view == p.view {
			parts = append(parts, subtabActiveStyle.Render(l.label))
		} else {
			parts = append(parts, subtabInactiveStyle.Render(l.label))
		}
	}
	return strings.Join(parts, "  ")
}

func (p *LibraryPane) leftLines() []string {
	lines := make([]string, 0, len(p.items))
	for _, item := range p.items {
		lines = append(lines, p.catalogLine(item))
	}
	return lines
}

func (p *LibraryPane) catalogLine(item libraryItem) string {
	parts := []string{item.Title, string(item.Kind)}
	if len(item.Tags) > 0 {
		parts = append(parts, strings.Join(item.Tags, ","))
	}
	if item.Source != "" {
		parts = append(parts, filepath.Base(item.Source))
	}
	return strings.Join(parts, "  ")
}

func (p *LibraryPane) detailLines() []string {
	item, ok := p.selectedItem()
	if !ok {
		return []string{dimStyle.Render("No selection")}
	}
	var lines []string
	lines = append(lines, headerStyle.Render(item.Title))
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("kind")+"  "+string(item.Kind))
	lines = append(lines, dimStyle.Render("id")+"  "+item.ID)
	if item.Source != "" {
		lines = append(lines, dimStyle.Render("source")+"  "+item.Source)
	}
	if len(item.Tags) > 0 {
		lines = append(lines, dimStyle.Render("tags")+"  "+strings.Join(item.Tags, ", "))
	}
	if len(item.Variables) > 0 {
		lines = append(lines, dimStyle.Render("inputs")+"  "+strings.Join(item.Variables, ", "))
	}
	if len(item.Families) > 0 {
		lines = append(lines, dimStyle.Render("families")+"  "+strings.Join(item.Families, ", "))
	}
	if len(item.Keywords) > 0 {
		lines = append(lines, dimStyle.Render("keywords")+"  "+strings.Join(item.Keywords, ", "))
	}
	if len(item.Capabilities) > 0 {
		lines = append(lines, dimStyle.Render("capabilities")+"  "+strings.Join(item.Capabilities, ", "))
	}
	if len(item.Inputs) > 0 {
		lines = append(lines, dimStyle.Render("run inputs")+"  "+strings.Join(item.Inputs, ", "))
	}
	if item.Description != "" {
		lines = append(lines, "", item.Description)
	}
	if len(item.Warnings) > 0 {
		lines = append(lines, "", warningText("Validation"))
		for _, warn := range item.Warnings {
			lines = append(lines, "  "+warn)
		}
	}
	if !item.LastUsed.IsZero() {
		lines = append(lines, "", dimStyle.Render("last used")+"  "+formatAge(item.LastUsed))
	}
	return lines
}

func (p *LibraryPane) rebuildItems(preferredID string) {
	p.items = p.filteredItems()
	sort.SliceStable(p.items, func(i, j int) bool {
		li := p.lastUsed[p.items[i].ID]
		lj := p.lastUsed[p.items[j].ID]
		if !li.Equal(lj) {
			return li.After(lj)
		}
		if p.items[i].Kind != p.items[j].Kind {
			return p.items[i].Kind == libraryItemRecipe
		}
		if p.items[i].Title != p.items[j].Title {
			return p.items[i].Title < p.items[j].Title
		}
		return p.items[i].ID < p.items[j].ID
	})
	p.sel = 0
	if preferredID != "" {
		for i, item := range p.items {
			if item.ID == preferredID {
				p.sel = i
				break
			}
		}
	}
	p.updateSelected()
}

func (p *LibraryPane) filteredItems() []libraryItem {
	var out []libraryItem
	for _, item := range p.allItems {
		if !p.matchesView(item) {
			continue
		}
		if !p.matchesTagFilter(item) {
			continue
		}
		if !p.matchesFilter(item) {
			continue
		}
		out = append(out, item)
	}
	return out
}

func (p *LibraryPane) matchesView(item libraryItem) bool {
	switch p.view {
	case libraryViewRecipes:
		return item.Kind == libraryItemRecipe
	case libraryViewPrompts:
		return item.Kind == libraryItemPrompt
	default:
		return true
	}
}

func (p *LibraryPane) matchesTagFilter(item libraryItem) bool {
	if len(p.tagFilters) == 0 {
		return true
	}
	for _, tag := range item.allTags() {
		if p.tagFilters[strings.ToLower(tag)] {
			return true
		}
	}
	return false
}

func (p *LibraryPane) matchesFilter(item libraryItem) bool {
	if p.filter == "" {
		return true
	}
	corpus := strings.ToLower(strings.Join([]string{
		item.ID,
		item.Title,
		item.Description,
		item.Source,
		strings.Join(item.Tags, " "),
		strings.Join(item.Variables, " "),
		strings.Join(item.Families, " "),
		strings.Join(item.Keywords, " "),
		strings.Join(item.Capabilities, " "),
		strings.Join(item.Inputs, " "),
	}, " "))
	ok, _ := fuzzyMatchScore(p.filter, corpus)
	return ok
}

func (p *LibraryPane) updateSelected() {
	item, ok := p.selectedItem()
	if !ok {
		p.selected = libraryItem{}
		return
	}
	p.selected = item
}

func (p *LibraryPane) selectedItem() (libraryItem, bool) {
	if p.sel < 0 || p.sel >= len(p.items) {
		return libraryItem{}, false
	}
	return p.items[p.sel], true
}

func (p *LibraryPane) selectedID() string {
	if p.selected.ID != "" {
		return p.selected.ID
	}
	if item, ok := p.selectedItem(); ok {
		return item.ID
	}
	return ""
}

func (p *LibraryPane) selectByID(id string) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	for i, item := range p.items {
		if item.ID == id {
			p.sel = i
			p.updateSelected()
			return true
		}
	}
	for _, item := range p.allItems {
		if item.ID != id {
			continue
		}
		p.view = libraryViewAll
		p.rebuildItems(id)
		return true
	}
	return false
}

func (p *LibraryPane) runPromptForID(id string) (string, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", false
	}
	for _, item := range p.allItems {
		if item.ID == id && item.Kind == libraryItemRecipe {
			return buildRecipeRunPrompt(item), true
		}
	}
	return "", false
}

func (p *LibraryPane) touchSelected() {
	if item, ok := p.selectedItem(); ok {
		p.lastUsed[item.ID] = time.Now()
		p.selected = item
	}
}

func (p *LibraryPane) toggleSelectedTagFilter() {
	item, ok := p.selectedItem()
	if !ok {
		return
	}
	tags := item.allTags()
	if len(tags) == 0 {
		p.status = "no tags on selected item"
		return
	}
	tag := strings.ToLower(tags[0])
	if p.tagFilters[tag] {
		delete(p.tagFilters, tag)
		p.status = fmt.Sprintf("tag filter cleared: %s", tag)
		return
	}
	p.tagFilters[tag] = true
	p.status = fmt.Sprintf("tag filter added: %s", tag)
}

func (p *LibraryPane) runSelectedCmd() (*LibraryPane, tea.Cmd) {
	item, ok := p.selectedItem()
	if !ok {
		p.status = "nothing selected"
		return p, nil
	}
	if item.Kind != libraryItemRecipe {
		p.status = "run is only available for recipes"
		return p, nil
	}
	p.touchSelected()
	p.status = fmt.Sprintf("prepared run prompt for %s", item.ID)
	return p, func() tea.Msg {
		return LibraryRunRequestedMsg{
			RecipeID: item.ID,
			Prompt:   buildRecipeRunPrompt(item),
		}
	}
}

func (p *LibraryPane) validateSelected() (*LibraryPane, tea.Cmd) {
	item, ok := p.selectedItem()
	if !ok {
		p.status = "nothing selected"
		return p, nil
	}
	if item.Kind != libraryItemRecipe {
		p.status = "prompt templates are validated on load"
		return p, nil
	}
	p.touchSelected()
	revalidated, err := revalidateRecipe(item, p.runtime)
	if err != nil {
		p.status = fmt.Sprintf("validation failed: %v", err)
		return p, nil
	}
	for i := range p.allItems {
		if p.allItems[i].ID == revalidated.ID {
			p.allItems[i] = revalidated
			break
		}
	}
	p.rebuildItems(revalidated.ID)
	p.status = revalidated.validationStatus()
	return p, nil
}

func (p *LibraryPane) openSelectedEditorCmd() (*LibraryPane, tea.Cmd) {
	item, ok := p.selectedItem()
	if !ok {
		p.status = "nothing selected"
		return p, nil
	}
	path := strings.TrimSpace(item.Source)
	if path == "" {
		p.status = "selected item has no source path"
		return p, nil
	}
	editor := strings.TrimSpace(os.Getenv("EDITOR"))
	if editor == "" {
		editor = "vi"
	}
	p.touchSelected()
	p.status = fmt.Sprintf("opening editor: %s", filepath.Base(path))
	return p, tea.ExecProcess(exec.Command(editor, path), func(err error) tea.Msg {
		if err != nil {
			return chatSystemMsg{Text: fmt.Sprintf("Editor error: %v", err)}
		}
		return nil
	})
}

func (i libraryItem) allTags() []string {
	var tags []string
	tags = append(tags, i.Tags...)
	tags = append(tags, i.Families...)
	tags = append(tags, i.Keywords...)
	if i.Kind != "" {
		tags = append(tags, string(i.Kind))
	}
	return tags
}

func (i libraryItem) validationStatus() string {
	if len(i.Warnings) == 0 {
		return fmt.Sprintf("%s %q validated cleanly", i.Kind, i.ID)
	}
	return fmt.Sprintf("%s %q validated with %d warning(s)", i.Kind, i.ID, len(i.Warnings))
}

func buildRecipeRunPrompt(item libraryItem) string {
	parts := []string{"/recipe", "run", item.ID}
	for _, input := range item.Inputs {
		parts = append(parts, fmt.Sprintf("[%s=?]", input))
	}
	return strings.Join(parts, " ") + " "
}

func revalidateRecipe(item libraryItem, rt RuntimeAdapter) (libraryItem, error) {
	workspace := ""
	if rt != nil {
		workspace = rt.SessionInfo().Workspace
	}
	source := strings.TrimSpace(item.Source)
	if source == "" {
		return libraryItem{}, fmt.Errorf("source path required")
	}
	if workspace == "" {
		workspace = workspaceRootFromRecipeSource(source)
	}
	if workspace == "" {
		workspace = discoverRepoRoot()
	}
	loader := thoughtrecipe.NewLoader()
	if reg := promptRegistryFromRuntime(rt); reg != nil {
		loader.WithPromptRegistry(reg)
	}
	result, err := loader.LoadWorkspace(workspace)
	if err != nil {
		return libraryItem{}, err
	}
	for _, entry := range result.Registry.Entries() {
		if entry.ThoughtRecipe == nil || strings.TrimSpace(entry.ThoughtRecipe.ID) != item.ID {
			continue
		}
		return recipeItemFromEntry(entry), nil
	}
	return libraryItem{}, fmt.Errorf("recipe %s not found after reload", item.ID)
}

func loadLibraryItems(rt RuntimeAdapter) ([]libraryItem, error) {
	workspace := ""
	if rt != nil {
		workspace = rt.SessionInfo().Workspace
	}
	if workspace == "" {
		workspace = discoverRepoRoot()
	}
	loader := thoughtrecipe.NewLoader()
	if reg := promptRegistryFromRuntime(rt); reg != nil {
		loader.WithPromptRegistry(reg)
	}
	result, err := loader.LoadWorkspace(workspace)
	if err != nil {
		return nil, err
	}
	var items []libraryItem
	for _, entry := range result.Registry.Entries() {
		if entry.ThoughtRecipe == nil {
			continue
		}
		items = append(items, recipeItemFromEntry(entry))
	}
	if rt != nil {
		for _, promptInfo := range rt.ListPrompts() {
			items = append(items, promptItemFromInfo(promptInfo))
		}
	}
	return items, nil
}

func recipeItemFromEntry(entry thoughtrecipe.ThoughtRecipeEntry) libraryItem {
	item := libraryItem{
		Kind:        libraryItemRecipe,
		ID:          strings.TrimSpace(entry.ThoughtRecipe.ID),
		Title:       strings.TrimSpace(entry.ThoughtRecipe.EffectiveName()),
		Description: strings.TrimSpace(entry.ThoughtRecipe.Description),
		Source:      strings.TrimSpace(entry.Source),
		Tags:        append([]string(nil), entry.ThoughtRecipe.Metadata.Tags...),
		Families:    append([]string(nil), entry.ThoughtRecipe.Metadata.Families...),
		Keywords:    append([]string(nil), entry.ThoughtRecipe.Metadata.Keywords...),
	}
	if item.Title == "" {
		item.Title = item.ID
	}
	if entry.Plan != nil {
		for _, step := range entry.Plan.Steps {
			if step.CapabilityID != "" {
				item.Capabilities = appendUnique(item.Capabilities, step.CapabilityID)
			}
		}
		for _, warn := range entry.Plan.Warnings {
			msg := strings.TrimSpace(warn.Message)
			if msg != "" {
				item.Warnings = append(item.Warnings, msg)
			}
		}
	}
	item.Inputs = recipeInputsFromSource(item.Source)
	return item
}

func promptItemFromInfo(info PromptInfo) libraryItem {
	item := libraryItem{
		Kind:        libraryItemPrompt,
		ID:          strings.TrimSpace(info.PromptID),
		Title:       strings.TrimSpace(info.Meta.Title),
		Description: strings.TrimSpace(info.Description),
		Source:      strings.TrimSpace(info.Meta.Source),
		Tags:        append([]string(nil), info.Tags...),
		Variables:   append([]string(nil), info.Variables...),
	}
	if item.Title == "" {
		item.Title = item.ID
	}
	if item.Description == "" {
		item.Description = strings.TrimSpace(info.Meta.Source)
	}
	return item
}

func promptRegistryFromRuntime(rt RuntimeAdapter) prompt.Registry {
	if rt == nil {
		return nil
	}
	if accessor, ok := rt.(interface{ PromptRegistry() prompt.Registry }); ok {
		return accessor.PromptRegistry()
	}
	return nil
}

func recipeInputsFromSource(path string) []string {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	doc, err := thoughtrecipe.ParseSource(path, string(data))
	if err != nil {
		return nil
	}
	seen := make(map[string]struct{})
	var out []string
	for _, decl := range doc.Declarations {
		input, ok := decl.(*thoughtrecipe.InputDecl)
		if !ok || input == nil {
			continue
		}
		name := strings.TrimSpace(input.Name.Value)
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func appendUnique(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if strings.EqualFold(existing, value) {
			return values
		}
	}
	return append(values, value)
}

func countLibraryItems(items []libraryItem, kind libraryItemKind) int {
	n := 0
	for _, item := range items {
		if item.Kind == kind {
			n++
		}
	}
	return n
}

func workspaceRootFromRecipeSource(source string) string {
	source = strings.TrimSpace(source)
	if source == "" {
		return ""
	}
	dir := filepath.Dir(source)
	if filepath.Base(dir) != "euclo" {
		return ""
	}
	cfgDir := filepath.Dir(dir)
	if filepath.Base(cfgDir) != "relurpify_cfg" {
		return ""
	}
	return filepath.Dir(cfgDir)
}

func discoverRepoRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		candidate := filepath.Join(dir, "relurpify_cfg", "euclo")
		if info, statErr := os.Stat(candidate); statErr == nil && info.IsDir() {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}
