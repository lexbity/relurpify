package euclotui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/app/relurpish/tui"
	"codeburg.org/lexbit/relurpify/framework/prompt"
	thoughtrecipe "codeburg.org/lexbit/relurpify/named/euclo/thoughtrecipes"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type eucloLibraryView int

const (
	eucloLibraryViewAll eucloLibraryView = iota
	eucloLibraryViewRecipes
	eucloLibraryViewPrompts
)

type eucloLibraryItemKind string

const (
	eucloLibraryItemRecipe eucloLibraryItemKind = "recipe"
	eucloLibraryItemPrompt eucloLibraryItemKind = "prompt"
)

type eucloLibraryItem struct {
	Kind         eucloLibraryItemKind
	ID           string
	Title        string
	Description  string
	Source       string
	Tags         []string
	Variables    []string
	Capabilities []string
	Inputs       []string
	Warnings     []string
	LastUsed     time.Time
}

type EucloLibraryPane struct {
	runtime    tui.RuntimeAdapter
	router     *EucloEventRouter
	allItems   []eucloLibraryItem
	items      []eucloLibraryItem
	sel        int
	filter     string
	view       eucloLibraryView
	tagFilters map[string]bool
	lastUsed   map[string]time.Time
	selected   eucloLibraryItem
	status     string
	width      int
	height     int
	projection LibraryProjection
}

func NewEucloLibraryPane(rt tui.RuntimeAdapter, router *EucloEventRouter) *EucloLibraryPane {
	p := &EucloLibraryPane{
		runtime:    rt,
		router:     router,
		tagFilters: make(map[string]bool),
		lastUsed:   make(map[string]time.Time),
	}
	if router != nil {
		p.projection = router.Snapshot().Library
	}
	p.Refresh()
	return p
}

func (p *EucloLibraryPane) SetSize(w, h int) {
	p.width = w
	p.height = h
}

func (p *EucloLibraryPane) SetFilter(filter string) {
	p.filter = strings.ToLower(strings.TrimSpace(filter))
	p.rebuildItems("")
}

func (p *EucloLibraryPane) SetProjection(snapshot EucloProjectionSnapshot) {
	p.projection = snapshot.Library
}

func (p *EucloLibraryPane) Refresh() {
	items, err := loadEucloLibraryItems(p.runtime)
	if err != nil {
		p.allItems = nil
		p.items = nil
		p.selected = eucloLibraryItem{}
		p.sel = 0
		p.status = fmt.Sprintf("library reload failed: %v", err)
		return
	}
	p.allItems = items
	p.rebuildItems(p.selectedID())
	p.status = fmt.Sprintf("loaded %d recipes and %d prompts",
		countEucloItems(items, eucloLibraryItemRecipe),
		countEucloItems(items, eucloLibraryItemPrompt))
}

func (p *EucloLibraryPane) Update(msg tea.Msg) (*EucloLibraryPane, tea.Cmd) {
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

func (p *EucloLibraryPane) View() string {
	title := p.sectionTabs()
	if len(p.items) == 0 {
		emptyW := maxEuclo(1, p.width-2)
		return strings.Join([]string{
			title,
			p.sectionPanel("Library", emptyW, dimStyle.Render("No recipes or prompts match the current filter.")),
			dimStyle.Render("type to filter  tab scope  up/down navigate  r run  e open  v validate  t tag filter  esc clear"),
		}, "\n\n")
	}

	if p.width < 88 {
		catalogW := maxEuclo(1, p.width-2)
		listH := maxEuclo(1, p.height-12)
		return strings.Join([]string{
			title,
			p.sectionPanel("Catalog", catalogW, p.sectionList(p.leftLines(), p.sel, listH)),
			p.sectionPanel("Detail", catalogW, p.detailLines()...),
			p.footer(),
		}, "\n\n")
	}

	leftW, rightW := splitEucloWidths(p.width, 5, 7)
	listH := maxEuclo(1, p.height-12)
	left := p.sectionPanel("Catalog", leftW, p.sectionList(p.leftLines(), p.sel, listH))
	right := p.sectionPanel("Detail", rightW, p.detailLines()...)
	return strings.Join([]string{
		title,
		lipgloss.JoinHorizontal(lipgloss.Top, left, right),
		p.footer(),
	}, "\n\n")
}

func (p *EucloLibraryPane) footer() string {
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
	if len(p.projection.Recipes) > 0 {
		total := 0
		for _, count := range p.projection.Recipes {
			total += count
		}
		parts = append(parts, fmt.Sprintf("session runs: %d", total))
	}
	return dimStyle.Render(strings.Join(parts, "  "))
}

func (p *EucloLibraryPane) sectionTabs() string {
	type tabLabel struct {
		view  eucloLibraryView
		label string
	}
	labels := []tabLabel{
		{eucloLibraryViewAll, "all"},
		{eucloLibraryViewRecipes, "recipes"},
		{eucloLibraryViewPrompts, "prompts"},
	}
	var parts []string
	for _, l := range labels {
		if l.view == p.view {
			parts = append(parts, panelItemActiveStyle.Render(l.label))
		} else {
			parts = append(parts, dimStyle.Render(l.label))
		}
	}
	return strings.Join(parts, "  ")
}

func (p *EucloLibraryPane) leftLines() []string {
	lines := make([]string, 0, len(p.items))
	for _, item := range p.items {
		lines = append(lines, p.catalogLine(item))
	}
	return lines
}

func (p *EucloLibraryPane) catalogLine(item eucloLibraryItem) string {
	parts := []string{headerStyle.Render(item.Title)}
	kindStr := dimStyle.Render(string(item.Kind))
	parts = append(parts, kindStr)
	if len(item.Tags) > 0 {
		tagStr := dimStyle.Render(strings.Join(item.Tags, ","))
		parts = append(parts, tagStr)
	}
	if item.Source != "" {
		srcStr := dimStyle.Render(filepath.Base(item.Source))
		parts = append(parts, srcStr)
	}
	return strings.Join(parts, "  ")
}

func (p *EucloLibraryPane) detailLines() []string {
	if p.sel < 0 || p.sel >= len(p.items) {
		return []string{dimStyle.Render("No selection")}
	}
	item := p.items[p.sel]
	var lines []string
	lines = append(lines, headerStyle.Render(item.Title))
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("%s  %s", dimStyle.Render("kind"), string(item.Kind)))
	lines = append(lines, fmt.Sprintf("%s  %s", dimStyle.Render("id"), item.ID))
	if item.Source != "" {
		lines = append(lines, fmt.Sprintf("%s  %s", dimStyle.Render("source"), item.Source))
	}
	if len(item.Tags) > 0 {
		lines = append(lines, fmt.Sprintf("%s  %s", dimStyle.Render("tags"), strings.Join(item.Tags, ", ")))
	}
	if len(item.Variables) > 0 {
		lines = append(lines, fmt.Sprintf("%s  %s", dimStyle.Render("inputs"), strings.Join(item.Variables, ", ")))
	}
	if len(item.Capabilities) > 0 {
		lines = append(lines, fmt.Sprintf("%s  %s", dimStyle.Render("capabilities"), strings.Join(item.Capabilities, ", ")))
	}
	if len(item.Inputs) > 0 {
		lines = append(lines, fmt.Sprintf("%s  %s", dimStyle.Render("run inputs"), strings.Join(item.Inputs, ", ")))
	}
	if item.Description != "" {
		lines = append(lines, "", item.Description)
	}
	if len(item.Warnings) > 0 {
		lines = append(lines, "", diffRemoveStyle.Render("Validation"))
		for _, warn := range item.Warnings {
			lines = append(lines, "  "+warn)
		}
	}
	if !item.LastUsed.IsZero() {
		lines = append(lines, "", fmt.Sprintf("%s  %s", dimStyle.Render("last used"), formatEucloAge(item.LastUsed)))
	}
	if count, ok := p.projection.Recipes[item.ID]; ok && count > 0 {
		lines = append(lines, "", fmt.Sprintf("%s  %d %s", dimStyle.Render("session runs"), count, "this session"))
	}
	return lines
}

func (p *EucloLibraryPane) rebuildItems(preferredID string) {
	p.items = p.filteredItems()
	sort.SliceStable(p.items, func(i, j int) bool {
		li := p.lastUsed[p.items[i].ID]
		lj := p.lastUsed[p.items[j].ID]
		if !li.Equal(lj) {
			return li.After(lj)
		}
		if p.items[i].Kind != p.items[j].Kind {
			return p.items[i].Kind == eucloLibraryItemRecipe
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

func (p *EucloLibraryPane) filteredItems() []eucloLibraryItem {
	var out []eucloLibraryItem
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

func (p *EucloLibraryPane) matchesView(item eucloLibraryItem) bool {
	switch p.view {
	case eucloLibraryViewRecipes:
		return item.Kind == eucloLibraryItemRecipe
	case eucloLibraryViewPrompts:
		return item.Kind == eucloLibraryItemPrompt
	default:
		return true
	}
}

func (p *EucloLibraryPane) matchesTagFilter(item eucloLibraryItem) bool {
	if len(p.tagFilters) == 0 {
		return true
	}
	for _, tag := range item.allEucloTags() {
		if p.tagFilters[strings.ToLower(tag)] {
			return true
		}
	}
	return false
}

func (p *EucloLibraryPane) matchesFilter(item eucloLibraryItem) bool {
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
		strings.Join(item.Capabilities, " "),
		strings.Join(item.Inputs, " "),
	}, " "))
	ok, _ := eucloFuzzyMatch(p.filter, corpus)
	return ok
}

func (p *EucloLibraryPane) updateSelected() {
	if p.sel < 0 || p.sel >= len(p.items) {
		p.selected = eucloLibraryItem{}
		return
	}
	p.selected = p.items[p.sel]
}

func (p *EucloLibraryPane) selectedID() string {
	if p.selected.ID != "" {
		return p.selected.ID
	}
	if p.sel >= 0 && p.sel < len(p.items) {
		return p.items[p.sel].ID
	}
	return ""
}

func (p *EucloLibraryPane) runSelectedCmd() (*EucloLibraryPane, tea.Cmd) {
	if p.sel < 0 || p.sel >= len(p.items) {
		p.status = "nothing selected"
		return p, nil
	}
	item := p.items[p.sel]
	if item.Kind != eucloLibraryItemRecipe {
		p.status = "run is only available for recipes"
		return p, nil
	}
	p.lastUsed[item.ID] = time.Now()
	p.selected = item
	p.status = fmt.Sprintf("prepared run prompt for %s", item.ID)
	return p, p.emitRunMsgCmd(item)
}

func (p *EucloLibraryPane) emitRunMsgCmd(item eucloLibraryItem) tea.Cmd {
	return func() tea.Msg {
		parts := []string{"/recipe", "run", item.ID}
		for _, input := range item.Inputs {
			parts = append(parts, fmt.Sprintf("[%s=?]", input))
		}
		return tui.LibraryRunRequestedMsg{
			RecipeID: item.ID,
			Prompt:   strings.Join(parts, " ") + " ",
		}
	}
}

func (p *EucloLibraryPane) openSelectedEditorCmd() (*EucloLibraryPane, tea.Cmd) {
	if p.sel < 0 || p.sel >= len(p.items) {
		p.status = "nothing selected"
		return p, nil
	}
	item := p.items[p.sel]
	path := strings.TrimSpace(item.Source)
	if path == "" {
		p.status = "selected item has no source path"
		return p, nil
	}
	editor := strings.TrimSpace(os.Getenv("EDITOR"))
	if editor == "" {
		editor = "vi"
	}
	p.lastUsed[item.ID] = time.Now()
	p.selected = item
	p.status = fmt.Sprintf("opening editor: %s", filepath.Base(path))
	return p, tea.ExecProcess(exec.Command(editor, path), func(err error) tea.Msg {
		if err != nil {
			return tui.ChatSystemMsg{Text: fmt.Sprintf("Editor error: %v", err)}
		}
		return nil
	})
}

func (p *EucloLibraryPane) validateSelected() (*EucloLibraryPane, tea.Cmd) {
	if p.sel < 0 || p.sel >= len(p.items) {
		p.status = "nothing selected"
		return p, nil
	}
	item := p.items[p.sel]
	if item.Kind != eucloLibraryItemRecipe {
		p.status = "prompt templates are validated on load"
		return p, nil
	}
	if p.runtime == nil {
		p.status = "runtime unavailable for validation"
		return p, nil
	}
	workspace := p.runtime.SessionInfo().Workspace
	source := strings.TrimSpace(item.Source)
	if source == "" || workspace == "" {
		p.status = "cannot validate: source or workspace missing"
		return p, nil
	}
	loader := thoughtrecipe.NewLoader()
	reg := eucloPromptRegistry(p.runtime)
	if reg != nil {
		loader.WithPromptRegistry(reg)
	}
	result, err := loader.LoadWorkspace(workspace)
	if err != nil {
		p.status = fmt.Sprintf("validation failed: %v", err)
		return p, nil
	}
	var found bool
	for _, entry := range result.Registry.Entries() {
		if entry.ThoughtRecipe == nil || strings.TrimSpace(entry.ThoughtRecipe.ID) != item.ID {
			continue
		}
		newItem := entryToEucloItem(entry)
		for i := range p.allItems {
			if p.allItems[i].ID == newItem.ID {
				p.allItems[i] = newItem
				break
			}
		}
		p.rebuildItems(newItem.ID)
		p.selected = newItem
		if len(newItem.Warnings) == 0 {
			p.status = fmt.Sprintf("recipe %q validated cleanly", newItem.ID)
		} else {
			p.status = fmt.Sprintf("recipe %q validated with %d warning(s)", newItem.ID, len(newItem.Warnings))
		}
		found = true
		break
	}
	if !found {
		p.status = fmt.Sprintf("recipe %s not found after reload", item.ID)
	}
	return p, nil
}

func (p *EucloLibraryPane) toggleSelectedTagFilter() {
	if p.sel < 0 || p.sel >= len(p.items) {
		return
	}
	item := p.items[p.sel]
	tags := item.allEucloTags()
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

func (item eucloLibraryItem) allEucloTags() []string {
	var tags []string
	tags = append(tags, item.Tags...)
	if item.Kind != "" {
		tags = append(tags, string(item.Kind))
	}
	return tags
}

func (p *EucloLibraryPane) sectionPanel(title string, width int, lines ...string) string {
	if width < 4 {
		width = 4
	}
	content := strings.Join(lines, "\n")
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorDim).
		Padding(0, 1).
		Width(width - 2)
	header := sectionHeaderStyle.Render(title)
	return style.Render(header + "\n" + content)
}

func (p *EucloLibraryPane) sectionList(lines []string, sel, maxHeight int) string {
	if len(lines) == 0 {
		return dimStyle.Render("(empty)")
	}
	start := 0
	if sel >= maxHeight {
		start = sel - maxHeight + 1
	}
	end := start + maxHeight
	if end > len(lines) {
		end = len(lines)
	}
	visible := lines[start:end]
	var b strings.Builder
	for i, line := range visible {
		idx := start + i
		if idx == sel {
			b.WriteString(panelItemActiveStyle.Render("▸ " + line))
		} else {
			b.WriteString("  " + line)
		}
		if i < len(visible)-1 {
			b.WriteString("\n")
		}
	}
	if end < len(lines) {
		b.WriteString(fmt.Sprintf("\n%s", dimStyle.Render(fmt.Sprintf("▼ %d more", len(lines)-end))))
	}
	if start > 0 {
		b.WriteString(fmt.Sprintf("\n%s", dimStyle.Render(fmt.Sprintf("▲ %d above", start))))
	}
	return b.String()
}

func loadEucloLibraryItems(rt tui.RuntimeAdapter) ([]eucloLibraryItem, error) {
	workspace := ""
	if rt != nil {
		workspace = rt.SessionInfo().Workspace
	}
	if workspace == "" {
		var err error
		workspace, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	}
	loader := thoughtrecipe.NewLoader()
	reg := eucloPromptRegistry(rt)
	if reg != nil {
		loader.WithPromptRegistry(reg)
	}
	result, err := loader.LoadWorkspace(workspace)
	if err != nil {
		return nil, err
	}
	var items []eucloLibraryItem
	for _, entry := range result.Registry.Entries() {
		if entry.ThoughtRecipe == nil {
			continue
		}
		items = append(items, entryToEucloItem(entry))
	}
	if rt != nil {
		for _, promptInfo := range rt.ListPrompts() {
			items = append(items, promptInfoToEucloItem(promptInfo))
		}
	}
	return items, nil
}

func entryToEucloItem(entry thoughtrecipe.ThoughtRecipeEntry) eucloLibraryItem {
	item := eucloLibraryItem{
		Kind:  eucloLibraryItemRecipe,
		ID:    strings.TrimSpace(entry.ThoughtRecipe.ID),
		Title: strings.TrimSpace(entry.ThoughtRecipe.EffectiveName()),
		Description: strings.TrimSpace(entry.ThoughtRecipe.Description),
		Source: strings.TrimSpace(entry.Source),
		Tags:   append([]string(nil), entry.ThoughtRecipe.Metadata.Tags...),
	}
	if item.Title == "" {
		item.Title = item.ID
	}
	if entry.Plan != nil {
		for _, step := range entry.Plan.Steps {
			if step.CapabilityID != "" {
				item.Capabilities = eucloAppendUnique(item.Capabilities, step.CapabilityID)
			}
		}
		for _, warn := range entry.Plan.Warnings {
			msg := strings.TrimSpace(warn.Message)
			if msg != "" {
				item.Warnings = append(item.Warnings, msg)
			}
		}
	}
	if item.Source != "" {
		item.Inputs = eucloRecipeInputsFromSource(item.Source)
	}
	return item
}

func promptInfoToEucloItem(info tui.PromptInfo) eucloLibraryItem {
	item := eucloLibraryItem{
		Kind:        eucloLibraryItemPrompt,
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

func eucloPromptRegistry(rt tui.RuntimeAdapter) prompt.Registry {
	if rt == nil {
		return nil
	}
	if accessor, ok := rt.(interface{ PromptRegistry() prompt.Registry }); ok {
		return accessor.PromptRegistry()
	}
	return nil
}

func eucloRecipeInputsFromSource(path string) []string {
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

func eucloAppendUnique(values []string, value string) []string {
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

func countEucloItems(items []eucloLibraryItem, kind eucloLibraryItemKind) int {
	n := 0
	for _, item := range items {
		if item.Kind == kind {
			n++
		}
	}
	return n
}

func eucloFuzzyMatch(query, target string) (bool, int) {
	if query == "" {
		return true, 0
	}
	q := strings.ToLower(query)
	t := strings.ToLower(target)
	qRunes := []rune(q)
	tRunes := []rune(t)
	qIndex := 0
	score := 0
	consecutive := 0
	start := -1
	for i := 0; i < len(tRunes) && qIndex < len(qRunes); i++ {
		if tRunes[i] == qRunes[qIndex] {
			if start == -1 {
				start = i
			}
			if qIndex > 0 && i > 0 && tRunes[i-1] == qRunes[qIndex-1] {
				consecutive++
				score += 6
			} else {
				consecutive = 0
				score += 2
			}
			qIndex++
		} else if consecutive > 0 {
			consecutive = 0
		}
	}
	if qIndex != len(qRunes) {
		return false, 0
	}
	if start >= 0 {
		score += maxEuclo(0, 20-start)
	}
	score += maxEuclo(0, 10-(len(tRunes)-len(qRunes)))
	return true, score
}

func formatEucloAge(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

func maxEuclo(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func splitEucloWidths(totalWidth int, leftParts, rightParts int) (int, int) {
	divider := 1
	usable := totalWidth - divider
	left := usable * leftParts / (leftParts + rightParts)
	right := usable - left
	return left, right
}
