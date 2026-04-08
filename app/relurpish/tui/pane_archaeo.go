package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/interaction"
)

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const (
	// planSidebarWidth is the fixed column width of the blob sidebar.
	planSidebarWidth = 28
	// planSidebarCollapseAt is the terminal width below which the sidebar is
	// hidden by default and toggled as an overlay instead.
	planSidebarCollapseAt = 90
	// planOutputMaxLines is the maximum number of euclo output stream lines
	// shown in the plan subtab below the step list.
	planOutputMaxLines = 5
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// ExploreEntryKind distinguishes plain text entries from blob proposal entries
// in the archaeo explore feed.
type ExploreEntryKind string

const (
	ExploreEntryText ExploreEntryKind = "text"
	ExploreEntryBlob ExploreEntryKind = "blob"
)

// ExploreEntry is a single item in the archaeo explore feed.
type ExploreEntry struct {
	Kind     ExploreEntryKind
	Text     string    // for Kind=ExploreEntryText
	Blob     BlobEntry // for Kind=ExploreEntryBlob
	IsStaged bool      // only meaningful for Kind=ExploreEntryBlob
}

// StagedBlobEntry is a blob promoted from the explore feed into the plan subtab.
type StagedBlobEntry struct {
	ID    string
	Kind  BlobKind
	Title string
}

// ---------------------------------------------------------------------------
// ArchaeoPane
// ---------------------------------------------------------------------------

// ArchaeoPane renders the archaeo tab: an explore subtab (full-width feed of
// blob proposals from the archaeology relurpic capability) and a plan subtab
// (live plan + blob sidebar with add/remove operations).
type ArchaeoPane struct {
	activeSubTab SubTabID
	runtime      RuntimeAdapter
	width        int
	height       int
	emojiEnabled bool

	// ── explore subtab ──────────────────────────────────────────────────────
	exploreEntries []ExploreEntry
	exploreSel     int // cursor index into exploreEntries

	// ── staged blobs (explore → plan) ───────────────────────────────────────
	// Blobs staged in the explore subtab but not yet committed to archaeo.
	stagedBlobs []StagedBlobEntry

	// ── plan subtab layout ──────────────────────────────────────────────────
	planFocused    bool // false = main area focused, true = sidebar focused
	sidebarVisible bool // explicit toggle; meaningful only when width < planSidebarCollapseAt

	// ── plan subtab main area ────────────────────────────────────────────────
	livePlan         *ActivePlanView
	planScrollOff    int
	planOutputLines  []string // euclo output stream (last N lines from archaeo capability)
	newlyAddedStepID string   // step briefly highlighted after AddBlobToPlan
	newStepPending   bool     // AddBlobToPlan completed; next PlanUpdatedMsg highlights new step

	// ── plan subtab sidebar (blob list) ─────────────────────────────────────
	blobList       []BlobEntry // loaded from runtime; sorted tensions → patterns → learning
	blobSel        int         // cursor in effectiveBlobList()
	blobScrollOff  int
	expandedBlobID string // blob whose detail is expanded inline (via 'e')

	// ── history subtab ───────────────────────────────────────────────────────
	historyVersions []PlanVersionInfo
	historySel      int // cursor in historyVersions
	historyDiffSel  int // version whose step list is shown in diff view (0 = none)
}

// NewArchaeoPane creates a new ArchaeoPane with the explore subtab active.
func NewArchaeoPane(rt RuntimeAdapter) *ArchaeoPane {
	return &ArchaeoPane{
		activeSubTab: SubTabArchaeoExplore,
		runtime:      rt,
		emojiEnabled: true,
	}
}

// Init satisfies tea.Model (no background command needed at startup).
func (p *ArchaeoPane) Init() tea.Cmd { return nil }

// Update handles incoming messages.
func (p *ArchaeoPane) Update(msg tea.Msg) (ArchaeoPaner, tea.Cmd) {
	switch msg := msg.(type) {
	case PlanUpdatedMsg:
		prevStepIDs := p.stepIDSet()
		p.livePlan = msg.Plan
		// If we're waiting to highlight a newly added step, find it.
		if p.newStepPending && msg.Plan != nil {
			for _, step := range msg.Plan.Steps {
				if !prevStepIDs[step.ID] {
					p.newlyAddedStepID = step.ID
					p.newStepPending = false
					// Clear the highlight on the next update cycle.
					return p, func() tea.Msg { return clearPlanHighlightMsg{} }
				}
			}
			p.newStepPending = false
		}
		return p, nil

	case BlobsUpdatedMsg:
		p.blobList = sortBlobEntries(msg.Blobs)
		return p, nil

	case ArchaeoExploreMsg:
		p.exploreEntries = append(p.exploreEntries, msg.Entries...)
		return p, nil

	case blobAddedMsg:
		if msg.err != nil {
			p.planOutputLines = appendOutputLine(p.planOutputLines,
				fmt.Sprintf("add failed: %v", msg.err))
		} else {
			p.newStepPending = true
			p.planOutputLines = appendOutputLine(p.planOutputLines, "blob added to plan")
		}
		return p, nil

	case blobRemovedMsg:
		if msg.err != nil {
			p.planOutputLines = appendOutputLine(p.planOutputLines,
				fmt.Sprintf("remove failed: %v", msg.err))
		} else {
			p.planOutputLines = appendOutputLine(p.planOutputLines, "blob removed from plan")
		}
		return p, nil

	case clearPlanHighlightMsg:
		p.newlyAddedStepID = ""
		return p, nil

	case PlanHistoryUpdatedMsg:
		p.historyVersions = msg.Versions
		// Clamp cursor.
		if p.historySel >= len(p.historyVersions) {
			p.historySel = 0
		}
		return p, nil

	case planVersionActivatedMsg:
		if msg.err != nil {
			p.planOutputLines = appendOutputLine(p.planOutputLines,
				fmt.Sprintf("activate v%d failed: %v", msg.version, msg.err))
		} else {
			p.planOutputLines = appendOutputLine(p.planOutputLines,
				fmt.Sprintf("v%d activated", msg.version))
		}
		return p, nil

	case EucloFrameMsg:
		if msg.Frame.Kind == interaction.FrameArchaeoFindings {
			if content, ok := msg.Frame.Content.(interaction.ArchaeoFindingsContent); ok {
				entries := make([]ExploreEntry, 0, len(content.Blobs))
				for _, b := range content.Blobs {
					entries = append(entries, ExploreEntry{
						Kind: ExploreEntryBlob,
						Blob: BlobEntry{
							ID:          b.ID,
							Kind:        BlobKind(b.Kind),
							Title:       b.Title,
							Description: b.Description,
							AnchorRefs:  b.AnchorRefs,
							Severity:    b.Severity,
						},
					})
				}
				if len(entries) > 0 {
					p.exploreEntries = append(p.exploreEntries, entries...)
				}
			}
		}
		// Capture status/result frame text for the plan output stream.
		if msg.Frame.Kind == interaction.FrameStatus || msg.Frame.Kind == interaction.FrameResult {
			if sc, ok := msg.Frame.Content.(interaction.StatusContent); ok && sc.Message != "" {
				p.planOutputLines = appendOutputLine(p.planOutputLines, sc.Message)
			}
		}
		return p, nil

	case archaeoResultMsg:
		if msg.err != nil {
			p.exploreEntries = append(p.exploreEntries, ExploreEntry{
				Kind: ExploreEntryText,
				Text: fmt.Sprintf("error: %v", msg.err),
			})
			return p, nil
		}
		entries := parseArchaeoResultEntries(msg.result)
		if len(entries) == 0 && msg.result != nil {
			if text, ok := msg.result.Data["text"].(string); ok && text != "" {
				entries = []ExploreEntry{{Kind: ExploreEntryText, Text: text}}
			}
		}
		p.exploreEntries = append(p.exploreEntries, entries...)
		return p, nil

	case tea.KeyMsg:
		if p.activeSubTab == SubTabArchaeoExplore {
			return p.handleExploreKey(msg)
		}
		if p.activeSubTab == SubTabArchaeoPlan {
			return p.handlePlanKey(msg)
		}
		if p.activeSubTab == SubTabArchaeoHistory {
			return p.handleHistoryKey(msg)
		}
	}
	return p, nil
}

// ---------------------------------------------------------------------------
// Key handlers
// ---------------------------------------------------------------------------

// handleExploreKey routes key events in the explore subtab.
func (p *ArchaeoPane) handleExploreKey(msg tea.KeyMsg) (ArchaeoPaner, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if p.exploreSel < len(p.exploreEntries)-1 {
			p.exploreSel++
		}
	case "k", "up":
		if p.exploreSel > 0 {
			p.exploreSel--
		}
	case "enter":
		if p.exploreSel >= 0 && p.exploreSel < len(p.exploreEntries) {
			entry := &p.exploreEntries[p.exploreSel]
			if entry.Kind == ExploreEntryBlob {
				if entry.IsStaged {
					p.unstageBlob(entry.Blob.ID)
					entry.IsStaged = false
				} else {
					p.stageBlob(entry.Blob)
					entry.IsStaged = true
				}
			}
		}
	case "x", "d":
		if p.exploreSel >= 0 && p.exploreSel < len(p.exploreEntries) {
			entry := &p.exploreEntries[p.exploreSel]
			if entry.Kind == ExploreEntryBlob && entry.IsStaged {
				p.unstageBlob(entry.Blob.ID)
				entry.IsStaged = false
			}
		}
	}
	return p, nil
}

// handlePlanKey routes key events in the plan subtab.
func (p *ArchaeoPane) handlePlanKey(msg tea.KeyMsg) (ArchaeoPaner, tea.Cmd) {
	switch msg.String() {
	case "tab":
		p.planFocused = !p.planFocused

	case "ctrl+]":
		// Toggle sidebar overlay in narrow-terminal mode.
		p.sidebarVisible = !p.sidebarVisible

	default:
		if p.planFocused {
			return p.handleSidebarKey(msg)
		}
		return p.handleMainAreaKey(msg)
	}
	return p, nil
}

// handleMainAreaKey handles keys when the plan main area has focus.
func (p *ArchaeoPane) handleMainAreaKey(msg tea.KeyMsg) (ArchaeoPaner, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		p.planScrollOff++
	case "k", "up":
		if p.planScrollOff > 0 {
			p.planScrollOff--
		}
	}
	return p, nil
}

// handleSidebarKey handles keys when the blob sidebar has focus.
func (p *ArchaeoPane) handleSidebarKey(msg tea.KeyMsg) (ArchaeoPaner, tea.Cmd) {
	blobs := p.effectiveBlobList()
	switch msg.String() {
	case "j", "down":
		if p.blobSel < len(blobs)-1 {
			p.blobSel++
			// Scroll down if needed.
			if p.blobSel >= p.blobScrollOff+visibleBlobRows(p.height) {
				p.blobScrollOff++
			}
		}
	case "k", "up":
		if p.blobSel > 0 {
			p.blobSel--
			if p.blobSel < p.blobScrollOff {
				p.blobScrollOff--
			}
		}
	case "enter":
		if p.blobSel >= 0 && p.blobSel < len(blobs) {
			blob := blobs[p.blobSel]
			if !blob.InPlan {
				return p, p.addBlobCmd(blob)
			}
		}
	case "x", "d":
		if p.blobSel >= 0 && p.blobSel < len(blobs) {
			blob := blobs[p.blobSel]
			if blob.InPlan {
				return p, p.removeBlobCmd(blob)
			}
		}
	case "e":
		if p.blobSel >= 0 && p.blobSel < len(blobs) {
			id := blobs[p.blobSel].ID
			if p.expandedBlobID == id {
				p.expandedBlobID = ""
			} else {
				p.expandedBlobID = id
			}
		}
	}
	return p, nil
}

// ---------------------------------------------------------------------------
// Add / Remove blob commands
// ---------------------------------------------------------------------------

func (p *ArchaeoPane) addBlobCmd(blob BlobEntry) tea.Cmd {
	if p.runtime == nil {
		return nil
	}
	rt := p.runtime
	blobID := blob.ID
	return func() tea.Msg {
		err := rt.AddBlobToPlan(context.Background(), "", blobID)
		return blobAddedMsg{blobID: blobID, err: err}
	}
}

func (p *ArchaeoPane) removeBlobCmd(blob BlobEntry) tea.Cmd {
	if p.runtime == nil {
		return nil
	}
	rt := p.runtime
	blobID := blob.ID
	return func() tea.Msg {
		err := rt.RemoveBlobFromPlan(context.Background(), "", blobID)
		return blobRemovedMsg{blobID: blobID, err: err}
	}
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

// View renders the active subtab.
func (p *ArchaeoPane) View() string {
	switch p.activeSubTab {
	case SubTabArchaeoPlan:
		return p.viewPlan()
	case SubTabArchaeoHistory:
		return p.viewHistory()
	default:
		return p.viewExplore()
	}
}

// viewExplore renders the full-width explore feed.
func (p *ArchaeoPane) viewExplore() string {
	var sb strings.Builder
	sb.WriteString("\n explore\n")
	sb.WriteString(strings.Repeat("─", p.width) + "\n")

	if len(p.exploreEntries) == 0 {
		sb.WriteString("\n Submit a prompt to explore the archaeology subsystem.\n")
		sb.WriteString(" e.g. \"find naming inconsistencies in framework/capability\"\n")
	} else {
		for i, entry := range p.exploreEntries {
			focused := i == p.exploreSel
			switch entry.Kind {
			case ExploreEntryText:
				if focused {
					sb.WriteString(lipgloss.NewStyle().Bold(true).Render(" "+entry.Text) + "\n")
				} else {
					sb.WriteString(" " + entry.Text + "\n")
				}
			case ExploreEntryBlob:
				badge := BlobKindBadge(entry.Blob.Kind, p.emojiEnabled)
				action := " [stage]"
				if entry.IsStaged {
					action = " [staged]"
				}
				title := entry.Blob.Title
				if len(title) > p.width-20 && p.width > 25 {
					title = title[:p.width-23] + "..."
				}
				line := fmt.Sprintf(" %s %-30s%s", badge, title, action)
				if focused {
					line = lipgloss.NewStyle().Bold(true).Render(line)
				}
				sb.WriteString(line + "\n")
				if entry.Blob.Description != "" {
					sb.WriteString("    " + entry.Blob.Description + "\n")
				}
				if len(entry.Blob.AnchorRefs) > 0 {
					sb.WriteString("    Anchors: " + strings.Join(entry.Blob.AnchorRefs, ", ") + "\n")
				}
				sb.WriteString("\n")
			}
		}
	}

	sb.WriteString("\n " + helpLine("[j/k] navigate  [enter] stage  [x] unstage"))
	return sb.String()
}

// viewPlan renders the plan subtab with an internal horizontal split.
// Wide terminals (≥ planSidebarCollapseAt) get a true side-by-side layout;
// narrower terminals show only the main area with the sidebar togglable as an
// overlay via ctrl+].
func (p *ArchaeoPane) viewPlan() string {
	wide := p.width >= planSidebarCollapseAt

	if wide {
		mainW := p.width - planSidebarWidth - 1 // 1 for "│" separator
		if mainW < 10 {
			mainW = 10
		}
		main := p.viewPlanMain(mainW)
		side := p.viewPlanSidebar(planSidebarWidth)
		joined := joinColumns(main, "│", side)
		return "\n" + joined + "\n " + planHelpLine(p.planFocused)
	}

	// Narrow: show main area by default; sidebar as overlay if sidebarVisible.
	if p.sidebarVisible {
		side := p.viewPlanSidebar(p.width)
		return "\n" + side + "\n " + helpLine("[ctrl+]] hide sidebar  [tab] focus")
	}
	main := p.viewPlanMain(p.width)
	return "\n" + main + "\n " + helpLine("[ctrl+]] show sidebar  [tab] focus  [j/k] scroll")
}

// viewPlanMain renders the main (left) area of the plan subtab.
func (p *ArchaeoPane) viewPlanMain(w int) string {
	var sb strings.Builder
	sb.WriteString(" live plan\n")
	sb.WriteString(" " + strings.Repeat("─", w-2) + "\n")

	if p.livePlan == nil {
		sb.WriteString("\n (no active plan)\n")
	} else {
		if p.livePlan.Title != "" {
			sb.WriteString(" " + truncateStr(p.livePlan.Title, w-2) + "\n\n")
		}
		steps := p.livePlan.Steps
		// Apply scroll offset.
		maxVisible := p.height - 10
		if maxVisible < 3 {
			maxVisible = 3
		}
		start := p.planScrollOff
		if start >= len(steps) {
			start = 0
		}
		for i := start; i < len(steps) && i-start < maxVisible; i++ {
			step := steps[i]
			icon := stepStatusIcon(step.Status)
			num := fmt.Sprintf("%d", i+1)
			blobRef := p.blobRefForStep(step.ID)
			line := fmt.Sprintf(" %s  %-2s  %s", icon, num, truncateStr(step.Title, w-18))
			if blobRef != "" {
				line += "  " + truncateStr(blobRef, 12)
			}
			if step.ID == p.newlyAddedStepID {
				line = lipgloss.NewStyle().Bold(true).Reverse(true).Render(line)
			}
			sb.WriteString(line + "\n")
		}
		if p.planScrollOff > 0 {
			sb.WriteString(" ↑ more above\n")
		}
		if len(steps) > 0 && start+maxVisible < len(steps) {
			sb.WriteString(" ↓ more below\n")
		}
	}

	// Euclo output stream.
	if len(p.planOutputLines) > 0 {
		sb.WriteString("\n " + strings.Repeat("─", w-2) + "\n")
		lines := p.planOutputLines
		if len(lines) > planOutputMaxLines {
			lines = lines[len(lines)-planOutputMaxLines:]
		}
		for _, l := range lines {
			sb.WriteString(" " + truncateStr(l, w-2) + "\n")
		}
	}
	return sb.String()
}

// viewPlanSidebar renders the right-hand blob list sidebar.
func (p *ArchaeoPane) viewPlanSidebar(w int) string {
	blobs := p.effectiveBlobList()
	var sb strings.Builder

	if len(blobs) == 0 {
		sb.WriteString(" (no blobs)\n")
		sb.WriteString(" Use explore to find blobs\n")
		return sb.String()
	}

	maxVisible := p.height - 6
	if maxVisible < 3 {
		maxVisible = 3
	}

	lastKind := BlobKind("")
	visIdx := 0
	for i, blob := range blobs {
		// Blank-line group separator.
		if lastKind != "" && lastKind != blob.Kind {
			if i >= p.blobScrollOff && visIdx < maxVisible {
				sb.WriteString("\n")
				visIdx++
			}
		}
		lastKind = blob.Kind

		if i < p.blobScrollOff {
			continue
		}
		if visIdx >= maxVisible {
			break
		}

		badge := BlobKindBadge(blob.Kind, p.emojiEnabled)
		action := " [+]"
		if blob.InPlan {
			action = " [in]"
		}
		maxTitle := w - 8
		if maxTitle < 6 {
			maxTitle = 6
		}
		title := truncateStr(blob.Title, maxTitle)

		focused := p.planFocused && i == p.blobSel
		line := fmt.Sprintf(" %s %-*s%s", badge, maxTitle, title, action)
		if focused {
			line = lipgloss.NewStyle().Bold(true).Render(line)
		}
		sb.WriteString(line + "\n")
		visIdx++

		// Inline expanded detail.
		if blob.ID == p.expandedBlobID {
			if blob.Description != "" {
				sb.WriteString("  " + truncateStr(blob.Description, w-4) + "\n")
				visIdx++
			}
			for _, ref := range blob.AnchorRefs {
				if visIdx >= maxVisible {
					break
				}
				sb.WriteString("  " + truncateStr(ref, w-4) + "\n")
				visIdx++
			}
		}
	}

	if p.blobScrollOff > 0 {
		sb.WriteString(" ↑\n")
	}
	return sb.String()
}

// viewHistory renders the plan history subtab: a list of plan versions with
// status badges and a diff view for the selected version.
func (p *ArchaeoPane) viewHistory() string {
	var sb strings.Builder
	sb.WriteString("\n plan history\n")
	sb.WriteString(strings.Repeat("─", p.width) + "\n")

	if len(p.historyVersions) == 0 {
		sb.WriteString("\n (no plan versions found)\n")
		sb.WriteString(" Blobs must be added to the plan before versions are created.\n")
		sb.WriteString("\n " + helpLine("[j/k] navigate  [enter] activate  [d] view diff"))
		return sb.String()
	}

	maxVisible := p.height - 8
	if maxVisible < 3 {
		maxVisible = 3
	}

	// List versions — most recent first (descending by version number).
	for i, v := range p.historyVersions {
		if i >= maxVisible {
			sb.WriteString(fmt.Sprintf(" … (%d more)\n", len(p.historyVersions)-i))
			break
		}
		focused := i == p.historySel
		statusBadge := historyStatusBadge(v.Status)
		explorationHint := ""
		if v.ExplorationRef != "" {
			explorationHint = "  [" + v.ExplorationRef + "]"
		}
		line := fmt.Sprintf(" v%-3d %s  %d steps%s",
			v.Version, statusBadge, v.StepCount, explorationHint)
		// Mark the diff-selected version.
		if p.historyDiffSel == v.Version {
			line += "  ← selected"
		}
		if focused {
			line = lipgloss.NewStyle().Bold(true).Render(line)
		}
		sb.WriteString(line + "\n")
	}

	// Diff view: show step list for historyDiffSel version if set.
	if p.historyDiffSel > 0 {
		for _, v := range p.historyVersions {
			if v.Version != p.historyDiffSel {
				continue
			}
			sb.WriteString("\n")
			sb.WriteString(strings.Repeat("─", p.width) + "\n")
			sb.WriteString(fmt.Sprintf(" v%d  (%d steps)\n", v.Version, v.StepCount))
			// Steps come from the BlobsUpdatedMsg/plan, not directly from PlanVersionInfo.
			// Show a placeholder when detail is not cached.
			sb.WriteString(" (step detail available after plan loads)\n")
		}
	}

	sb.WriteString("\n " + helpLine("[j/k] navigate  [enter] activate version  [d] view diff  [esc] clear diff"))
	return sb.String()
}

// handleHistoryKey routes key events in the history subtab.
func (p *ArchaeoPane) handleHistoryKey(msg tea.KeyMsg) (ArchaeoPaner, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if p.historySel < len(p.historyVersions)-1 {
			p.historySel++
		}
	case "k", "up":
		if p.historySel > 0 {
			p.historySel--
		}
	case "d":
		// Toggle diff view for focused version.
		if p.historySel >= 0 && p.historySel < len(p.historyVersions) {
			v := p.historyVersions[p.historySel]
			if p.historyDiffSel == v.Version {
				p.historyDiffSel = 0
			} else {
				p.historyDiffSel = v.Version
			}
		}
	case "enter":
		if p.historySel >= 0 && p.historySel < len(p.historyVersions) {
			v := p.historyVersions[p.historySel]
			if v.Status != "active" {
				return p, p.activatePlanVersionCmd(v.Version)
			}
		}
	case "esc":
		p.historyDiffSel = 0
	}
	return p, nil
}

// activatePlanVersionCmd dispatches an ActivatePlanVersion call to the runtime.
func (p *ArchaeoPane) activatePlanVersionCmd(version int) tea.Cmd {
	if p.runtime == nil {
		return nil
	}
	rt := p.runtime
	ver := version
	return func() tea.Msg {
		err := rt.ActivatePlanVersion(context.Background(), "", ver)
		return planVersionActivatedMsg{version: ver, err: err}
	}
}

// historyStatusBadge returns a display badge for a plan version status.
func historyStatusBadge(status string) string {
	switch status {
	case "active":
		return "[active  ]"
	case "draft":
		return "[draft   ]"
	case "archived":
		return "[archived]"
	case "superseded":
		return "[supersed]"
	default:
		return "[" + truncateStr(status, 8) + "]"
	}
}

// ---------------------------------------------------------------------------
// Setter methods
// ---------------------------------------------------------------------------

// SetSize updates the pane dimensions.
func (p *ArchaeoPane) SetSize(w, h int) {
	p.width = w
	p.height = h
}

// SetSubTab switches the active subtab.
func (p *ArchaeoPane) SetSubTab(id SubTabID) {
	p.activeSubTab = id
	if id == SubTabArchaeoPlan {
		// Clamp blob cursor.
		blobs := p.effectiveBlobList()
		if p.blobSel >= len(blobs) {
			p.blobSel = 0
		}
	}
}

// HandleInputSubmit handles a submitted value in the explore subtab. In the
// plan subtab input is routed to blob operations via keys, not text.
func (p *ArchaeoPane) HandleInputSubmit(value string) tea.Cmd {
	if p.activeSubTab != SubTabArchaeoExplore {
		return nil
	}
	if p.runtime == nil {
		return nil
	}
	p.exploreEntries = append(p.exploreEntries, ExploreEntry{
		Kind: ExploreEntryText,
		Text: "> " + value,
	})
	rt := p.runtime
	return func() tea.Msg {
		result, err := rt.ExecuteInstruction(
			context.Background(), value, core.TaskTypeAnalysis,
			map[string]any{"mode": "archaeology"},
		)
		return archaeoResultMsg{result: result, err: err}
	}
}

// SetBlobEmojiEnabled toggles emoji vs letter-badge rendering for blob kinds.
func (p *ArchaeoPane) SetBlobEmojiEnabled(enabled bool) {
	p.emojiEnabled = enabled
}

// StagedBlobs returns the current staged blob list.
func (p *ArchaeoPane) StagedBlobs() []StagedBlobEntry {
	out := make([]StagedBlobEntry, len(p.stagedBlobs))
	copy(out, p.stagedBlobs)
	return out
}

// PromoteAll stages all unstaged blob proposals in the current explore feed.
func (p *ArchaeoPane) PromoteAll() {
	for i := range p.exploreEntries {
		entry := &p.exploreEntries[i]
		if entry.Kind == ExploreEntryBlob && !entry.IsStaged {
			p.stageBlob(entry.Blob)
			entry.IsStaged = true
		}
	}
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// stageBlob adds a blob to stagedBlobs if not already present.
func (p *ArchaeoPane) stageBlob(blob BlobEntry) {
	for _, s := range p.stagedBlobs {
		if s.ID == blob.ID {
			return
		}
	}
	p.stagedBlobs = append(p.stagedBlobs, StagedBlobEntry{
		ID:    blob.ID,
		Kind:  blob.Kind,
		Title: blob.Title,
	})
}

// unstageBlob removes a blob from stagedBlobs by ID.
func (p *ArchaeoPane) unstageBlob(id string) {
	out := p.stagedBlobs[:0]
	for _, s := range p.stagedBlobs {
		if s.ID != id {
			out = append(out, s)
		}
	}
	p.stagedBlobs = out
}

// effectiveBlobList merges blobList (from backend) with stagedBlobs (local,
// not yet committed). Staged blobs not already in blobList are appended as
// non-plan entries. The merged list is sorted tensions → patterns → learning.
func (p *ArchaeoPane) effectiveBlobList() []BlobEntry {
	inList := make(map[string]bool, len(p.blobList))
	for _, b := range p.blobList {
		inList[b.ID] = true
	}
	result := append([]BlobEntry(nil), p.blobList...)
	for _, s := range p.stagedBlobs {
		if !inList[s.ID] {
			result = append(result, BlobEntry{
				ID:    s.ID,
				Kind:  s.Kind,
				Title: s.Title,
			})
		}
	}
	return sortBlobEntries(result)
}

// blobRefForStep returns the title of a blob linked to the given step ID, or "".
func (p *ArchaeoPane) blobRefForStep(stepID string) string {
	for _, b := range p.effectiveBlobList() {
		if b.InPlan && b.StepID == stepID {
			return b.Title
		}
	}
	return ""
}

// stepIDSet returns a set of step IDs in the current live plan.
func (p *ArchaeoPane) stepIDSet() map[string]bool {
	if p.livePlan == nil {
		return nil
	}
	out := make(map[string]bool, len(p.livePlan.Steps))
	for _, step := range p.livePlan.Steps {
		out[step.ID] = true
	}
	return out
}

// visibleBlobRows estimates how many blob rows fit in the sidebar given the
// current pane height.
func visibleBlobRows(height int) int {
	rows := height - 6
	if rows < 3 {
		return 3
	}
	return rows
}

// ---------------------------------------------------------------------------
// Internal message types
// ---------------------------------------------------------------------------

// archaeoResultMsg is returned by HandleInputSubmit's command.
type archaeoResultMsg struct {
	result *core.Result
	err    error
}

// blobAddedMsg is returned when AddBlobToPlan completes.
type blobAddedMsg struct {
	blobID string
	err    error
}

// blobRemovedMsg is returned when RemoveBlobFromPlan completes.
type blobRemovedMsg struct {
	blobID string
	err    error
}

// clearPlanHighlightMsg clears the newly-added step highlight after one cycle.
type clearPlanHighlightMsg struct{}

// planVersionActivatedMsg is returned when ActivatePlanVersion completes.
type planVersionActivatedMsg struct {
	version int
	err     error
}

// ---------------------------------------------------------------------------
// Utilities
// ---------------------------------------------------------------------------

// BlobKindBadge returns the display badge for a blob kind. When emojiEnabled is
// true it returns an emoji; when false it returns a short letter badge.
func BlobKindBadge(kind BlobKind, emojiEnabled bool) string {
	if emojiEnabled {
		switch kind {
		case BlobTension:
			return "⚡"
		case BlobPattern:
			return "🧩"
		case BlobLearning:
			return "💡"
		default:
			return "·"
		}
	}
	switch kind {
	case BlobTension:
		return "[T]"
	case BlobPattern:
		return "[P]"
	case BlobLearning:
		return "[L]"
	default:
		return "[ ]"
	}
}

// sortBlobEntries returns blobs sorted tensions-first, patterns-second,
// learning-third, preserving order within each group.
func sortBlobEntries(blobs []BlobEntry) []BlobEntry {
	out := make([]BlobEntry, 0, len(blobs))
	for _, b := range blobs {
		if b.Kind == BlobTension {
			out = append(out, b)
		}
	}
	for _, b := range blobs {
		if b.Kind == BlobPattern {
			out = append(out, b)
		}
	}
	for _, b := range blobs {
		if b.Kind == BlobLearning {
			out = append(out, b)
		}
	}
	for _, b := range blobs {
		if b.Kind != BlobTension && b.Kind != BlobPattern && b.Kind != BlobLearning {
			out = append(out, b)
		}
	}
	return out
}

// renderBlobList renders a sorted blob list for the plan subtab sidebar with
// blank-line group separators and correct badges.
func renderBlobList(blobs []BlobEntry, emojiEnabled bool, width int) string {
	if len(blobs) == 0 {
		return ""
	}
	sorted := sortBlobEntries(blobs)
	var sb strings.Builder
	lastKind := BlobKind("")
	for _, b := range sorted {
		if lastKind != "" && lastKind != b.Kind {
			sb.WriteString("\n")
		}
		badge := BlobKindBadge(b.Kind, emojiEnabled)
		inPlan := " [+]"
		if b.InPlan {
			inPlan = " [in]"
		}
		title := b.Title
		maxTitle := width - 12
		if maxTitle < 10 {
			maxTitle = 10
		}
		if len(title) > maxTitle {
			title = title[:maxTitle-3] + "..."
		}
		sb.WriteString(fmt.Sprintf(" %s %-*s%s\n", badge, maxTitle, title, inPlan))
		lastKind = b.Kind
	}
	return sb.String()
}

// stepStatusIcon maps a plan step status string to its display icon.
func stepStatusIcon(status string) string {
	switch status {
	case "done":
		return "✓"
	case "running":
		return "▶"
	case "failed", "blocked":
		return "✗"
	default: // "ready", "pending", ""
		return "·"
	}
}

// parseArchaeoResultEntries attempts to extract blob proposals from the raw
// result data returned by ExecuteInstruction.
func parseArchaeoResultEntries(result *core.Result) []ExploreEntry {
	if result == nil {
		return nil
	}
	// Future: parse structured blob data from result.Data["blobs"].
	return nil
}

// helpLine renders a dimmed help text string for the pane footer.
func helpLine(text string) string {
	return lipgloss.NewStyle().Faint(true).Render(text)
}

// planHelpLine returns context-sensitive help for the plan subtab.
func planHelpLine(sidebarFocused bool) string {
	if sidebarFocused {
		return helpLine("[tab] focus main  [j/k] navigate  [enter] add  [x] remove  [e] expand")
	}
	return helpLine("[tab] focus sidebar  [j/k] scroll  [ctrl+]] toggle sidebar")
}

// joinColumns joins two multi-line strings side by side with a separator.
// Lines in each column are padded to equal count with blank lines.
func joinColumns(left, sep, right string) string {
	lLines := strings.Split(strings.TrimRight(left, "\n"), "\n")
	rLines := strings.Split(strings.TrimRight(right, "\n"), "\n")
	maxLen := len(lLines)
	if len(rLines) > maxLen {
		maxLen = len(rLines)
	}
	for len(lLines) < maxLen {
		lLines = append(lLines, "")
	}
	for len(rLines) < maxLen {
		rLines = append(rLines, "")
	}
	var sb strings.Builder
	for i := range lLines {
		sb.WriteString(lLines[i] + sep + rLines[i] + "\n")
	}
	return sb.String()
}

// appendOutputLine appends a line to the output buffer, capped at
// planOutputMaxLines*2 to bound memory.
func appendOutputLine(lines []string, line string) []string {
	lines = append(lines, line)
	if len(lines) > planOutputMaxLines*2 {
		lines = lines[len(lines)-planOutputMaxLines*2:]
	}
	return lines
}

// countBlobsByKind returns the count of tensions, patterns, and learning blobs
// in the given list. Used to update titlebar blob count badges.
func countBlobsByKind(blobs []BlobEntry) (tensions, patterns, learning int) {
	for _, b := range blobs {
		switch b.Kind {
		case BlobTension:
			tensions++
		case BlobPattern:
			patterns++
		case BlobLearning:
			learning++
		}
	}
	return
}

// truncateStr shortens s to at most n runes, appending "..." if truncated.
func truncateStr(s string, n int) string {
	if n <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	if n <= 3 {
		return string(runes[:n])
	}
	return string(runes[:n-3]) + "..."
}
