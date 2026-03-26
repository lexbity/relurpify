package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// Messages for populating planner subtabs from the runtime / agent events.
// ---------------------------------------------------------------------------

// PlannerPatternsMsg delivers a fresh pattern list to the explore subtab.
type PlannerPatternsMsg struct {
	Records   []PatternRecordInfo
	Proposals []PatternProposalInfo
}

// PlannerTensionsMsg delivers tension and gap data to the analyze subtab.
type PlannerTensionsMsg struct {
	Tensions []TensionInfo
	Gaps     []IntentGapInfo
}

// PlannerPlanMsg delivers the current living plan to the finalize subtab.
type PlannerPlanMsg struct {
	Plan LivePlanInfo
}

// plannerNoteAddedMsg is an internal message emitted after a plan note is accepted.
type plannerNoteAddedMsg struct {
	stepID string
	note   string
}

// ---------------------------------------------------------------------------
// PlannerPane
// ---------------------------------------------------------------------------

// PlannerPane renders the three planning subtabs: explore, analyze, finalize.
// It is always held by pointer so mutations survive tea.Model value copies.
type PlannerPane struct {
	activeSubTab SubTabID

	// explore subtab state
	patterns      []PatternRecordInfo
	proposals     []PatternProposalInfo
	exploreFilter string
	exploreSel    int

	// analyze subtab state
	tensions   []TensionInfo
	gaps       []IntentGapInfo
	analyzeSel int

	// finalize subtab state
	plan        LivePlanInfo
	finalizeSel int

	// feedback line for the current subtab (cleared on next input)
	statusMsg string

	width, height int
}

// NewPlannerPane creates an empty PlannerPane with the explore subtab active.
func NewPlannerPane() *PlannerPane {
	return &PlannerPane{activeSubTab: SubTabPlannerExplore}
}

// SetSubTab switches the visible subtab.
func (p *PlannerPane) SetSubTab(id SubTabID) {
	p.activeSubTab = id
	p.statusMsg = ""
}

// SetSize resizes the pane.
func (p *PlannerPane) SetSize(w, h int) {
	p.width = w
	p.height = h
}

// HandleInputSubmit routes user text to the active subtab:
//   - explore  → update the pattern search filter
//   - analyze  → (no-op; analysis is triggered from the agent)
//   - finalize → append a note to the selected plan step
func (p *PlannerPane) HandleInputSubmit(value string) tea.Cmd {
	value = strings.TrimSpace(value)
	switch p.activeSubTab {
	case SubTabPlannerExplore:
		p.exploreFilter = value
		p.exploreSel = 0
		p.statusMsg = fmt.Sprintf("filter: %q", value)
	case SubTabPlannerAnalyze:
		// Trigger analysis annotation — reflected as a system message.
		if value != "" {
			return func() tea.Msg {
				return chatSystemMsg{Text: fmt.Sprintf("[planner/analyze] note: %s", value)}
			}
		}
	case SubTabPlannerFinalize:
		rows := p.finalizeRows()
		if value != "" && p.finalizeSel < len(rows) {
			stepID := rows[p.finalizeSel].id
			note := value
			p.statusMsg = fmt.Sprintf("note added to step %s", stepID)
			return func() tea.Msg {
				return plannerNoteAddedMsg{stepID: stepID, note: note}
			}
		}
	}
	return nil
}

// Update handles key events and async data messages.
func (p *PlannerPane) Update(msg tea.Msg) (PlannerPaner, tea.Cmd) {
	switch msg := msg.(type) {
	case PlannerPatternsMsg:
		p.patterns = msg.Records
		p.proposals = msg.Proposals
		p.exploreSel = 0

	case PlannerTensionsMsg:
		p.tensions = msg.Tensions
		p.gaps = msg.Gaps
		p.analyzeSel = 0

	case PlannerPlanMsg:
		p.plan = msg.Plan
		p.finalizeSel = 0

	case plannerNoteAddedMsg:
		// Append the note to the matching plan step.
		for i := range p.plan.Steps {
			if p.plan.Steps[i].ID == msg.stepID {
				p.plan.Steps[i].Notes = append(p.plan.Steps[i].Notes, msg.note)
				break
			}
		}

	case tea.KeyMsg:
		return p.handleKey(msg)
	}
	return p, nil
}

func (p *PlannerPane) handleKey(msg tea.KeyMsg) (PlannerPaner, tea.Cmd) {
	p.statusMsg = ""
	switch p.activeSubTab {
	case SubTabPlannerExplore:
		rows := p.exploreRows()
		switch msg.String() {
		case "up":
			if p.exploreSel > 0 {
				p.exploreSel--
			}
		case "down":
			if p.exploreSel < len(rows)-1 {
				p.exploreSel++
			}
		case "backspace":
			if len(p.exploreFilter) > 0 {
				runes := []rune(p.exploreFilter)
				p.exploreFilter = string(runes[:len(runes)-1])
				p.exploreSel = 0
			}
		}

	case SubTabPlannerAnalyze:
		rows := p.analyzeRows()
		switch msg.String() {
		case "up":
			if p.analyzeSel > 0 {
				p.analyzeSel--
			}
		case "down":
			if p.analyzeSel < len(rows)-1 {
				p.analyzeSel++
			}
		}

	case SubTabPlannerFinalize:
		rows := p.finalizeRows()
		switch msg.String() {
		case "up":
			if p.finalizeSel > 0 {
				p.finalizeSel--
			}
		case "down":
			if p.finalizeSel < len(rows)-1 {
				p.finalizeSel++
			}
		case "c":
			// Mark selected step as confirmed (done).
			if p.finalizeSel < len(rows) {
				id := rows[p.finalizeSel].id
				for i := range p.plan.Steps {
					if p.plan.Steps[i].ID == id && p.plan.Steps[i].Status != "done" {
						p.plan.Steps[i].Status = "done"
						p.statusMsg = fmt.Sprintf("step %q confirmed", p.plan.Steps[i].Title)
						break
					}
				}
			}
		case "r":
			// Reset selected failed/done step back to ready.
			if p.finalizeSel < len(rows) {
				id := rows[p.finalizeSel].id
				for i := range p.plan.Steps {
					if p.plan.Steps[i].ID == id {
						p.plan.Steps[i].Status = "ready"
						p.statusMsg = fmt.Sprintf("step %q reset to ready", p.plan.Steps[i].Title)
						break
					}
				}
			}
		}
	}
	return p, nil
}

// ---------------------------------------------------------------------------
// Row helpers (used by both Update and View).
// ---------------------------------------------------------------------------

type plannerRow struct {
	id    string
	label string
	kind  string // "record" | "proposal" | "tension" | "gap" | "step"
}

func (p *PlannerPane) exploreRows() []plannerRow {
	var rows []plannerRow
	filter := strings.ToLower(p.exploreFilter)
	for _, rec := range p.patterns {
		if filter != "" && !strings.Contains(strings.ToLower(rec.Title), filter) &&
			!strings.Contains(strings.ToLower(rec.Scope), filter) {
			continue
		}
		rows = append(rows, plannerRow{
			id:    rec.ID,
			label: fmt.Sprintf("[%s]  %s  %s", rec.IntentType, rec.Title, dimStyle.Render(rec.Scope)),
			kind:  "record",
		})
	}
	for _, prop := range p.proposals {
		if filter != "" && !strings.Contains(strings.ToLower(prop.Title), filter) &&
			!strings.Contains(strings.ToLower(prop.Scope), filter) {
			continue
		}
		rows = append(rows, plannerRow{
			id:    prop.ID,
			label: fmt.Sprintf("[proposal]  %s  %s  %s", prop.Title, dimStyle.Render(prop.Scope), dimStyle.Render(fmt.Sprintf("%.0f%%", prop.Confidence*100))),
			kind:  "proposal",
		})
	}
	return rows
}

func (p *PlannerPane) analyzeRows() []plannerRow {
	var rows []plannerRow
	for _, t := range p.tensions {
		rows = append(rows, plannerRow{
			id:    t.ID,
			label: fmt.Sprintf("tension  %s ↔ %s  (%d sites)", t.TitleA, t.TitleB, len(t.Sites)),
			kind:  "tension",
		})
	}
	for i, g := range p.gaps {
		rows = append(rows, plannerRow{
			id:    fmt.Sprintf("gap-%d", i),
			label: fmt.Sprintf("gap  %s:%d  %s  %s", g.FilePath, g.Line, g.AnchorName, dimStyle.Render(g.Severity)),
			kind:  "gap",
		})
	}
	return rows
}

func (p *PlannerPane) finalizeRows() []plannerRow {
	var rows []plannerRow
	for _, s := range p.plan.Steps {
		statusStyle := dimStyle
		switch s.Status {
		case "done":
			statusStyle = completedStyle
		case "running":
			statusStyle = inProgressStyle
		case "failed":
			statusStyle = diffRemoveStyle
		case "blocked":
			statusStyle = pendingStyle
		}
		label := statusStyle.Render(stepStatusGlyph(s.Status)) + "  " + s.Title
		if len(s.Notes) > 0 {
			label += dimStyle.Render(fmt.Sprintf("  [%d note(s)]", len(s.Notes)))
		}
		if len(s.Anchors) > 0 {
			label += dimStyle.Render(fmt.Sprintf("  ⚓%d", len(s.Anchors)))
		}
		rows = append(rows, plannerRow{id: s.ID, label: label, kind: "step"})
	}
	return rows
}

// stepStatusGlyph returns a compact indicator for a step status.
func stepStatusGlyph(status string) string {
	switch status {
	case "done":
		return "✓"
	case "running":
		return "▶"
	case "failed":
		return "✗"
	case "blocked":
		return "⊘"
	case "ready":
		return "○"
	default:
		return "·"
	}
}

func (p *PlannerPane) splitWidths(parts ...int) []int {
	available := p.width
	if available <= 0 {
		available = 120
	}
	gap := 2 * (len(parts) - 1)
	available -= gap
	if available < len(parts)*16 {
		available = len(parts) * 16
	}
	total := 0
	for _, part := range parts {
		total += part
	}
	widths := make([]int, len(parts))
	used := 0
	for i, part := range parts {
		if i == len(parts)-1 {
			widths[i] = available - used
			continue
		}
		widths[i] = available * part / total
		if widths[i] < 16 {
			widths[i] = 16
		}
		used += widths[i]
	}
	return widths
}

func plannerPanel(title string, width int, lines ...string) string {
	body := strings.TrimRight(strings.Join(lines, "\n"), "\n")
	content := panelHeaderStyle.Render(title)
	if body != "" {
		content += "\n" + body
	}
	return panelStyle.Width(width).Render(content)
}

func plannerList(lines []string, selected int, maxVisible int) string {
	if len(lines) == 0 {
		return dimStyle.Render("no items")
	}
	if maxVisible < 1 {
		maxVisible = len(lines)
	}
	start := 0
	if selected >= maxVisible {
		start = selected - maxVisible + 1
	}
	end := start + maxVisible
	if end > len(lines) {
		end = len(lines)
	}
	visible := make([]string, 0, end-start+1)
	for i := start; i < end; i++ {
		line := "  " + lines[i]
		if i == selected {
			line = panelItemActiveStyle.Render(line)
		}
		visible = append(visible, line)
	}
	if len(lines) > maxVisible {
		visible = append(visible, dimStyle.Render(fmt.Sprintf("(%d/%d)", selected+1, len(lines))))
	}
	return strings.Join(visible, "\n")
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

// View renders the active subtab.
func (p *PlannerPane) View() string {
	switch p.activeSubTab {
	case SubTabPlannerExplore:
		return p.viewExplore()
	case SubTabPlannerAnalyze:
		return p.viewAnalyze()
	case SubTabPlannerFinalize:
		return p.viewFinalize()
	default:
		return p.viewExplore()
	}
}

func (p *PlannerPane) viewExplore() string {
	rows := p.exploreRows()
	widths := p.splitWidths(4, 5, 6)
	scopeLines := []string{
		dimStyle.Render("Filter"),
		func() string {
			if p.exploreFilter == "" {
				return "all patterns"
			}
			return p.exploreFilter
		}(),
		"",
		dimStyle.Render("Inventory"),
		fmt.Sprintf("confirmed: %d", len(p.patterns)),
		fmt.Sprintf("proposed:  %d", len(p.proposals)),
	}
	if p.statusMsg != "" {
		scopeLines = append(scopeLines, "", inProgressStyle.Render(p.statusMsg))
	}
	listLines := make([]string, 0, len(rows))
	for _, row := range rows {
		line := row.label
		if row.kind == "proposal" {
			line = inProgressStyle.Render("·") + " " + line
		}
		listLines = append(listLines, line)
	}
	detailLines := []string{}
	if len(rows) == 0 {
		if len(p.patterns) == 0 && len(p.proposals) == 0 {
			detailLines = append(detailLines, dimStyle.Render("No patterns loaded. The agent will populate this as patterns are discovered."))
		} else {
			detailLines = append(detailLines, dimStyle.Render("No patterns match the current filter."))
		}
	} else if p.exploreSel < len(rows) {
		selected := rows[p.exploreSel]
		switch selected.kind {
		case "record":
			for _, rec := range p.patterns {
				if rec.ID != selected.id {
					continue
				}
				detailLines = append(detailLines,
					rec.Title,
					"",
					dimStyle.Render("Type")+"  confirmed pattern",
					dimStyle.Render("Intent")+"  "+rec.IntentType,
					dimStyle.Render("Scope")+"  "+rec.Scope,
				)
				if rec.Description != "" {
					detailLines = append(detailLines, "", rec.Description)
				}
				break
			}
		case "proposal":
			for _, prop := range p.proposals {
				if prop.ID != selected.id {
					continue
				}
				detailLines = append(detailLines,
					prop.Title,
					"",
					dimStyle.Render("Type")+"  proposal",
					dimStyle.Render("Confidence")+fmt.Sprintf("  %.0f%%", prop.Confidence*100),
					dimStyle.Render("Scope")+"  "+prop.Scope,
				)
				if prop.Description != "" {
					detailLines = append(detailLines, "", prop.Description)
				}
				break
			}
		}
	}
	return strings.Join([]string{
		sectionHeaderStyle.Render("Explore"),
		lipgloss.JoinHorizontal(lipgloss.Top,
			plannerPanel("Scope", widths[0], scopeLines...),
			plannerPanel("Candidates", widths[1], plannerList(listLines, p.exploreSel, p.height-10)),
			plannerPanel("Detail", widths[2], detailLines...),
		),
		dimStyle.Render("↑↓ navigate  type in input bar to filter"),
	}, "\n")
}

func (p *PlannerPane) viewAnalyze() string {
	rows := p.analyzeRows()
	widths := p.splitWidths(4, 5, 5)
	summary := []string{
		fmt.Sprintf("tensions: %d", len(p.tensions)),
		fmt.Sprintf("gaps:     %d", len(p.gaps)),
	}
	if p.statusMsg != "" {
		summary = append(summary, "", inProgressStyle.Render(p.statusMsg))
	}
	listLines := make([]string, 0, len(rows))
	for _, row := range rows {
		listLines = append(listLines, row.label)
	}
	detail := []string{}
	if len(rows) == 0 {
		detail = append(detail,
			dimStyle.Render("No tensions or gaps detected yet."),
			dimStyle.Render("Submit a prompt on the chat pane to trigger analysis."),
		)
	} else if p.analyzeSel < len(rows) {
		row := rows[p.analyzeSel]
		switch row.kind {
		case "tension":
			if p.analyzeSel < len(p.tensions) {
				t := p.tensions[p.analyzeSel]
				detail = append(detail,
					fmt.Sprintf("%s ↔ %s", t.TitleA, t.TitleB),
					"",
					dimStyle.Render("Sites")+fmt.Sprintf("  %d", len(t.Sites)),
				)
				if len(t.ResolutionPatterns) > 0 {
					detail = append(detail, "", dimStyle.Render("Resolution patterns"))
					for _, rp := range t.ResolutionPatterns {
						detail = append(detail, rp.Title)
					}
				}
				if len(t.Sites) > 0 {
					detail = append(detail, "", dimStyle.Render("Conflict sites"))
					for _, site := range t.Sites {
						detail = append(detail, fmt.Sprintf("%s:%d", site.FilePath, site.Line))
					}
				}
			}
		case "gap":
			gapIdx := p.analyzeSel - len(p.tensions)
			if gapIdx >= 0 && gapIdx < len(p.gaps) {
				g := p.gaps[gapIdx]
				detail = append(detail,
					g.AnchorName,
					"",
					dimStyle.Render("Severity")+"  "+g.Severity,
					dimStyle.Render("Location")+fmt.Sprintf("  %s:%d", g.FilePath, g.Line),
					dimStyle.Render("Anchor class")+"  "+g.AnchorClass,
				)
				if g.Description != "" {
					detail = append(detail, "", g.Description)
				}
			}
		}
	}
	timeline := []string{}
	if len(p.gaps) == 0 {
		timeline = append(timeline, dimStyle.Render("No active contradiction drift."))
	} else {
		for _, gap := range p.gaps {
			timeline = append(timeline, fmt.Sprintf("%s  %s:%d  %s", gap.Severity, gap.FilePath, gap.Line, gap.AnchorName))
		}
	}
	return strings.Join([]string{
		sectionHeaderStyle.Render("Analyze"),
		lipgloss.JoinHorizontal(lipgloss.Top,
			plannerPanel("Summary", widths[0], summary...),
			plannerPanel("Signals", widths[1], plannerList(listLines, p.analyzeSel, p.height-12)),
			plannerPanel("Detail", widths[2], detail...),
		),
		plannerPanel("Drift Timeline", p.width, timeline...),
		dimStyle.Render("↑↓ navigate"),
	}, "\n")
}

func (p *PlannerPane) viewFinalize() string {
	widths := p.splitWidths(4, 5, 5)
	summary := []string{}
	if p.plan.WorkflowID != "" {
		conf := fmt.Sprintf("%.0f%%", p.plan.Confidence*100)
		summary = append(summary,
			dimStyle.Render("Workflow")+"  "+p.plan.WorkflowID,
			dimStyle.Render("Confidence")+"  "+conf,
		)
		if p.plan.Title != "" {
			summary = append(summary, dimStyle.Render("Title")+"  "+p.plan.Title)
		}
	}
	rows := p.finalizeRows()
	if p.statusMsg != "" {
		summary = append(summary, "", inProgressStyle.Render(p.statusMsg))
	}
	listLines := make([]string, 0, len(rows))
	for _, row := range rows {
		listLines = append(listLines, row.label)
	}
	detail := []string{}
	anchorLines := []string{}
	if len(rows) == 0 {
		detail = append(detail, dimStyle.Render("No plan steps yet. The agent will build a plan as it works."))
		anchorLines = append(anchorLines, dimStyle.Render("No anchor refs attached to this step."))
	} else if p.finalizeSel < len(p.plan.Steps) {
		step := p.plan.Steps[p.finalizeSel]
		detail = append(detail,
			step.Title,
			"",
			dimStyle.Render("Status")+"  "+step.Status,
			dimStyle.Render("Attempts")+fmt.Sprintf("  %d", step.Attempts),
		)
		if len(step.SymbolScope) > 0 {
			detail = append(detail, "", dimStyle.Render("Symbol scope"))
			for _, sym := range step.SymbolScope {
				detail = append(detail, sym)
			}
		}
		if len(step.Notes) > 0 {
			detail = append(detail, "", dimStyle.Render("Notes"))
			for _, note := range step.Notes {
				detail = append(detail, note)
			}
		}
		if len(step.Anchors) == 0 {
			anchorLines = append(anchorLines, dimStyle.Render("No anchor refs attached to this step."))
		} else {
			for _, anchor := range step.Anchors {
				anchorLines = append(anchorLines, fmt.Sprintf("%s  %s  %s", anchor.Name, dimStyle.Render(anchor.Class), dimStyle.Render(anchor.Status)))
			}
		}
	}
	title := "Finalize"
	if p.plan.Title != "" {
		title = "Finalize: " + p.plan.Title
	}
	return strings.Join([]string{
		sectionHeaderStyle.Render(title),
		lipgloss.JoinHorizontal(lipgloss.Top,
			plannerPanel("Plan", widths[0], summary...),
			plannerPanel("Steps", widths[1], plannerList(listLines, p.finalizeSel, p.height-12)),
			plannerPanel("Selected Step", widths[2], detail...),
		),
		plannerPanel("Evidence", p.width, anchorLines...),
		dimStyle.Render("[c] confirm  [r] reset  ↑↓ navigate  type note + enter"),
	}, "\n")
}

// Verify that *PlannerPane satisfies PlannerPaner at compile time.
var _ PlannerPaner = (*PlannerPane)(nil)
