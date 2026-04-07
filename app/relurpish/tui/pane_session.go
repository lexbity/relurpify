package tui

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// SessionSection selects which view is active in the session pane.
type SessionSection int

const (
	SectionFiles SessionSection = iota
	SectionChanges
)

type liveSection int

const (
	liveSectionWorkflows liveSection = iota
	liveSectionProviders
	liveSectionApprovals
	liveSectionServices
)

// SessionPane displays workspace files, session changes, and live diagnostics.
type SessionPane struct {
	activeSubTab SubTabID
	section      SessionSection

	// Files section (session/tasks subtab)
	allFiles []FileEntry
	filtered []FileEntry
	fileSel  int
	filter   string
	loading  bool
	loadErr  error

	// Changes and queued work (session/tasks subtab)
	changes   []FileChange
	changeSel int
	queued    []TaskItem

	// Live subtab diagnostics snapshot
	diagnostics DiagnosticsInfo
	workflows   []WorkflowInfo
	providers   []LiveProviderInfo
	approvals   []ApprovalInfo
	services    []ServiceInfo
	liveSection liveSection
	workflowSel int
	providerSel int
	approvalSel int
	serviceSel  int
	workflow    *WorkflowDetails
	provider    *LiveProviderDetail
	approval    *ApprovalDetail

	context *AgentContext
	session *Session
	runtime RuntimeAdapter
	width   int
	height  int
}

// NewSessionPane creates a SessionPane.
func NewSessionPane(ctx *AgentContext, sess *Session, rt RuntimeAdapter) *SessionPane {
	return &SessionPane{
		context: ctx,
		session: sess,
		runtime: rt,
	}
}

// Init starts the file index build.
func (p *SessionPane) Init() tea.Cmd {
	root := "."
	if p.session != nil && p.session.Workspace != "" {
		root = p.session.Workspace
	}
	p.loading = true
	return fileIndexCmd(root)
}

// SetSize resizes the pane.
func (p *SessionPane) SetSize(w, h int) { p.width = w; p.height = h }

// SetSubTab switches the active subtab.
func (p *SessionPane) SetSubTab(id SubTabID) { p.activeSubTab = id }

// SetDiagnostics refreshes the live diagnostics snapshot.
func (p *SessionPane) SetDiagnostics(d DiagnosticsInfo) { p.diagnostics = d }

// SetLiveSnapshot refreshes the live diagnostics and runtime summaries.
func (p *SessionPane) SetLiveSnapshot(d DiagnosticsInfo, workflows []WorkflowInfo, providers []LiveProviderInfo, approvals []ApprovalInfo) {
	p.diagnostics = d
	p.workflows = append([]WorkflowInfo(nil), workflows...)
	p.providers = append([]LiveProviderInfo(nil), providers...)
	p.approvals = append([]ApprovalInfo(nil), approvals...)
	
	// Load services
	if p.runtime != nil {
		p.services = p.runtime.ListServices()
	}
	
	if p.workflowSel >= len(p.workflows) {
		p.workflowSel = max(0, len(p.workflows)-1)
	}
	if p.providerSel >= len(p.providers) {
		p.providerSel = max(0, len(p.providers)-1)
	}
	if p.approvalSel >= len(p.approvals) {
		p.approvalSel = max(0, len(p.approvals)-1)
	}
	if p.serviceSel >= len(p.services) {
		p.serviceSel = max(0, len(p.services)-1)
	}
	p.refreshLiveDetails()
}

// SyncChanges updates the changes list, called by root after each run.
func (p *SessionPane) SyncChanges(changes []FileChange) {
	p.changes = append([]FileChange(nil), changes...)
	if p.changeSel >= len(p.changes) {
		p.changeSel = 0
	}
}

// SyncQueuedTasks updates the queued task snapshot used by the tasks subtab.
func (p *SessionPane) SyncQueuedTasks(tasks []TaskItem) {
	p.queued = append([]TaskItem(nil), tasks...)
}

// SyncContext re-syncs the context reference (no-op if pointer unchanged).
func (p *SessionPane) SyncContext(ctx *AgentContext) {
	if ctx != nil {
		p.context = ctx
	}
}

// Update handles navigation and async messages.
func (p *SessionPane) Update(msg tea.Msg) (*SessionPane, tea.Cmd) {
	switch msg := msg.(type) {
	case fileIndexMsg:
		p.loading = false
		if msg.err != nil {
			p.loadErr = msg.err
			return p, nil
		}
		p.allFiles = msg.files
		p.applyFilter()

	case tea.KeyMsg:
		if p.activeSubTab == SubTabSessionLive {
			switch msg.String() {
			case "tab", "right", "l":
				p.liveSection = (p.liveSection + 1) % 4
			case "shift+tab", "left", "h":
				p.liveSection = (p.liveSection + 3) % 4
			case "up", "k":
				p.moveLiveSelection(-1)
			case "down", "j":
				p.moveLiveSelection(1)
			case "s":
				if p.liveSection == liveSectionServices && p.runtime != nil {
					if p.serviceSel >= 0 && p.serviceSel < len(p.services) {
						serviceID := p.services[p.serviceSel].ID
						if err := p.runtime.StopService(serviceID); err != nil {
							// Handle error
						}
					}
				}
			case "r":
				if p.liveSection == liveSectionServices && p.runtime != nil {
					if p.serviceSel >= 0 && p.serviceSel < len(p.services) {
						serviceID := p.services[p.serviceSel].ID
						if err := p.runtime.RestartService(context.Background(), serviceID); err != nil {
							// Handle error
						}
					}
				}
			case "R":
				if p.liveSection == liveSectionServices && p.runtime != nil {
					if err := p.runtime.RestartAllServices(context.Background()); err != nil {
						// Handle error
					}
				}
			}
			return p, nil
		}
		switch msg.String() {
		case "tab":
			if p.section == SectionFiles {
				p.section = SectionChanges
			} else {
				p.section = SectionFiles
			}

		case "up":
			if p.section == SectionChanges {
				if p.changeSel > 0 {
					p.changeSel--
				}
			} else {
				if p.fileSel > 0 {
					p.fileSel--
				}
			}

		case "down":
			if p.section == SectionChanges {
				if p.changeSel < len(p.changes)-1 {
					p.changeSel++
				}
			} else {
				if p.fileSel < len(p.filtered)-1 {
					p.fileSel++
				}
			}

		case "enter":
			if p.section == SectionFiles && p.fileSel < len(p.filtered) {
				e := p.filtered[p.fileSel]
				if p.context != nil {
					if err := p.context.AddFile(e.Path); err == nil {
						return p, func() tea.Msg {
							return chatSystemMsg{Text: fmt.Sprintf("Added: %s", e.DisplayPath)}
						}
					}
				}
			}

		case "y", "Y":
			if p.section == SectionChanges && p.changeSel < len(p.changes) {
				p.changes[p.changeSel].Status = StatusApproved
			}

		case "n", "N":
			if p.section == SectionChanges && p.changeSel < len(p.changes) {
				p.changes[p.changeSel].Status = StatusRejected
			}

		case "e":
			path := ""
			if p.section == SectionFiles && p.fileSel < len(p.filtered) {
				path = p.filtered[p.fileSel].Path
			} else if p.section == SectionChanges && p.changeSel < len(p.changes) {
				path = p.changes[p.changeSel].Path
			}
			if path != "" {
				editor := os.Getenv("EDITOR")
				if editor == "" {
					editor = "vi"
				}
				return p, tea.ExecProcess(exec.Command(editor, path), func(err error) tea.Msg {
					if err != nil {
						return chatSystemMsg{Text: fmt.Sprintf("Editor error: %v", err)}
					}
					return nil
				})
			}
		}
	}
	return p, nil
}

// HandleFilterInput updates the file filter from the input bar.
// Only has effect when the tasks subtab (or legacy no-subtab mode) is active.
func (p *SessionPane) HandleFilterInput(query string) {
	if p.activeSubTab != "" && p.activeSubTab != SubTabSessionTasks {
		return
	}
	p.filter = strings.TrimSpace(query)
	p.fileSel = 0
	p.applyFilter()
}

func (p *SessionPane) applyFilter() {
	const maxRows = 20
	p.filtered = filterFileEntries(p.allFiles, p.filter, maxRows)
	sort.Slice(p.filtered, func(i, j int) bool {
		if p.filtered[i].Score != p.filtered[j].Score {
			return p.filtered[i].Score > p.filtered[j].Score
		}
		return p.filtered[i].DisplayPath < p.filtered[j].DisplayPath
	})
	if p.fileSel >= len(p.filtered) {
		p.fileSel = 0
	}
}

// View renders the active subtab (or section for backward compat).
func (p *SessionPane) View() string {
	switch p.activeSubTab {
	case SubTabSessionLive:
		return p.viewLive()
	case SubTabSessionTasks:
		if p.section == SectionChanges {
			return p.viewChanges()
		}
		return p.viewFiles()
	case SubTabSessionSettings:
		return p.viewSessionSettings()
	default:
		// Legacy: no subtab set — use old section toggle.
		if p.section == SectionChanges {
			return p.viewChanges()
		}
		return p.viewFiles()
	}
}

func (p *SessionPane) viewFiles() string {
	if p.loading {
		return dimStyle.Render("Indexing workspace files...")
	}
	if p.loadErr != nil {
		return notifErrorStyle.Render(fmt.Sprintf("File index error: %v", p.loadErr))
	}
	var b strings.Builder
	header := "Workspace Files"
	if p.filter != "" {
		header += "  " + dimStyle.Render(fmt.Sprintf("filter: %q", p.filter))
	}
	b.WriteString(sectionHeaderStyle.Render(header))
	b.WriteString("\n\n")
	if len(p.queued) > 0 {
		b.WriteString(sectionHeaderStyle.Render("Queued Tasks") + "\n")
		for _, task := range p.queued {
			style := taskPendingStyle
			icon := "☐"
			switch task.Status {
			case TaskCompleted:
				style = taskDoneStyle
				icon = "✓"
			case TaskInProgress:
				style = taskRunningStyle
				icon = "●"
			}
			b.WriteString(fmt.Sprintf("%s  %s\n", style.Render(icon), style.Render(task.Description)))
		}
		b.WriteString("\n")
	}
	if len(p.filtered) == 0 {
		b.WriteString(dimStyle.Render("No matching files"))
	} else {
		for i, e := range p.filtered {
			line := renderFileEntryLine(e)
			if i == p.fileSel {
				line = panelItemActiveStyle.Render(line)
			} else {
				line = panelItemStyle.Render(line)
			}
			b.WriteString(line + "\n")
		}
	}
	if p.context != nil && len(p.context.Files) > 0 {
		b.WriteString("\n" + sectionHeaderStyle.Render("Context") + "\n")
		for _, f := range p.context.Files {
			b.WriteString(dimStyle.Render("  • ") + filePathStyle.Render(f) + "\n")
		}
	}
	b.WriteString("\n" + dimStyle.Render("enter=add to context  e=open in editor  tab=view changes"))
	return b.String()
}

func (p *SessionPane) viewChanges() string {
	var b strings.Builder
	b.WriteString(sectionHeaderStyle.Render("Session Changes"))
	b.WriteString("\n\n")
	if len(p.queued) > 0 {
		b.WriteString(sectionHeaderStyle.Render("Queued Tasks") + "\n")
		for _, task := range p.queued {
			style := taskPendingStyle
			icon := "☐"
			switch task.Status {
			case TaskCompleted:
				style = taskDoneStyle
				icon = "✓"
			case TaskInProgress:
				style = taskRunningStyle
				icon = "●"
			}
			b.WriteString(fmt.Sprintf("%s  %s\n", style.Render(icon), style.Render(task.Description)))
		}
		b.WriteString("\n")
	}
	if len(p.changes) == 0 {
		b.WriteString(dimStyle.Render("No changes in this session yet"))
		b.WriteString("\n\n" + dimStyle.Render("tab=view files"))
		return b.String()
	}
	for i, c := range p.changes {
		statusIcon, statusRender := changeStatusDisplay(c.Status)
		changeType := string(c.Type)
		if changeType == "" {
			changeType = "modify"
		}
		line := statusRender(statusIcon) + "  " +
			filePathStyle.Render(c.Path) +
			"  " + dimStyle.Render("("+changeType+")")
		if c.LinesAdded > 0 || c.LinesRemoved > 0 {
			line += dimStyle.Render(fmt.Sprintf("  +%d/-%d", c.LinesAdded, c.LinesRemoved))
		}
		if i == p.changeSel {
			line = panelItemActiveStyle.Render(line)
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\n" + dimStyle.Render("y=approve  n=reject  e=open in editor  tab=view files"))
	return b.String()
}

func (p *SessionPane) viewLive() string {
	widths := (&PlannerPane{width: p.width}).splitWidths(4, 4, 4)
	var b strings.Builder
	b.WriteString(sectionHeaderStyle.Render("Live Session") + "\n\n")

	d := p.diagnostics
	if p.session != nil {
		b.WriteString(dimStyle.Render("workspace  ") + filePathStyle.Render(p.session.Workspace) + "\n")
		b.WriteString(dimStyle.Render("agent      ") + textStyle.Render(p.session.Agent) + "\n")
		b.WriteString(dimStyle.Render("model      ") + textStyle.Render(p.session.Model) + "\n")
		if p.session.Mode != "" {
			b.WriteString(dimStyle.Render("mode       ") + inProgressStyle.Render(p.session.Mode) + "\n")
		}
		if p.session.Strategy != "" {
			b.WriteString(dimStyle.Render("strategy   ") + textStyle.Render(p.session.Strategy) + "\n")
		}
		dur := p.session.TotalDuration.Round(1e9)
		b.WriteString(dimStyle.Render("tokens     ") + fmt.Sprintf("%d  %s", p.session.TotalTokens, dimStyle.Render(dur.String())) + "\n")
		b.WriteString("\n")
	}

	if d.ContextTokensMax > 0 {
		pct := 100 * d.ContextTokensUsed / d.ContextTokensMax
		bar := contextBar(pct, 20)
		b.WriteString(dimStyle.Render("context    ") + bar +
			dimStyle.Render(fmt.Sprintf("  %d/%d", d.ContextTokensUsed, d.ContextTokensMax)) + "\n")
	}
	if d.ActiveWorkflows > 0 || d.PatternEntries > 0 {
		b.WriteString(dimStyle.Render("workflows  ") + fmt.Sprintf("%d", d.ActiveWorkflows) + "\n")
		b.WriteString(dimStyle.Render("patterns   ") + fmt.Sprintf("%d", d.PatternEntries) + "\n")
	}
	if d.ActiveMode != "" {
		b.WriteString(dimStyle.Render("exec mode  ") + inProgressStyle.Render(d.ActiveMode) + "\n")
	}
	if d.ActivePhase != "" {
		b.WriteString(dimStyle.Render("phase      ") + textStyle.Render(d.ActivePhase) + "\n")
	}
	if d.DoomLoopState != "" && d.DoomLoopState != "idle" {
		b.WriteString(dimStyle.Render("doom loop  ") + diffRemoveStyle.Render(d.DoomLoopState) + "\n")
	}
	if d.ContextStrategy != "" {
		b.WriteString(dimStyle.Render("ctx strat  ") + textStyle.Render(d.ContextStrategy) + "\n")
	}
	if d.PruningEvents > 0 {
		b.WriteString(dimStyle.Render("pruning    ") + inProgressStyle.Render(fmt.Sprintf("%d event(s)", d.PruningEvents)) + "\n")
	}
	if d.CapabilitiesTotal > 0 {
		b.WriteString(dimStyle.Render("caps       ") + fmt.Sprintf("%d", d.CapabilitiesTotal) + "\n")
	}
	if d.PendingApprovals > 0 {
		b.WriteString(dimStyle.Render("pending ✓  ") + inProgressStyle.Render(fmt.Sprintf("%d", d.PendingApprovals)) + "\n")
	}
	if d.LiveProviders > 0 {
		b.WriteString(dimStyle.Render("providers  ") + fmt.Sprintf("%d", d.LiveProviders) + "\n")
	}
	return strings.Join([]string{
		lipgloss.JoinHorizontal(lipgloss.Top,
			plannerPanel("Summary", widths[0], strings.Split(strings.TrimRight(b.String(), "\n"), "\n")...),
			plannerPanel("Workflows", widths[1], plannerList(p.liveWorkflowLines(), p.workflowSel, p.height-12)),
			plannerPanel("Providers / Approvals", widths[2],
				sectionHeaderStyle.Render("Providers"),
				plannerList(p.liveProviderLines(), p.providerSel, 4),
				"",
				sectionHeaderStyle.Render("Approvals"),
				plannerList(p.liveApprovalLines(), p.approvalSel, 4),
			),
		),
		plannerPanel("Detail", p.width, p.liveDetailLines()...),
		dimStyle.Render("tab/shift+tab switch focus  ↑↓ navigate"),
	}, "\n")
}

func (p *SessionPane) moveLiveSelection(delta int) {
	switch p.liveSection {
	case liveSectionWorkflows:
		if len(p.workflows) == 0 {
			return
		}
		p.workflowSel += delta
		if p.workflowSel < 0 {
			p.workflowSel = 0
		}
		if p.workflowSel >= len(p.workflows) {
			p.workflowSel = len(p.workflows) - 1
		}
		p.refreshLiveDetails()
	case liveSectionProviders:
		if len(p.providers) == 0 {
			return
		}
		p.providerSel += delta
		if p.providerSel < 0 {
			p.providerSel = 0
		}
		if p.providerSel >= len(p.providers) {
			p.providerSel = len(p.providers) - 1
		}
		p.refreshLiveDetails()
	case liveSectionApprovals:
		if len(p.approvals) == 0 {
			return
		}
		p.approvalSel += delta
		if p.approvalSel < 0 {
			p.approvalSel = 0
		}
		if p.approvalSel >= len(p.approvals) {
			p.approvalSel = len(p.approvals) - 1
		}
		p.refreshLiveDetails()
	case liveSectionServices:
		if len(p.services) == 0 {
			return
		}
		p.serviceSel += delta
		if p.serviceSel < 0 {
			p.serviceSel = 0
		}
		if p.serviceSel >= len(p.services) {
			p.serviceSel = len(p.services) - 1
		}
		p.refreshLiveDetails()
	}
}

func (p *SessionPane) refreshLiveDetails() {
	if p.runtime == nil {
		return
	}
	p.workflow = nil
	p.provider = nil
	p.approval = nil
	switch p.liveSection {
	case liveSectionWorkflows:
		if p.workflowSel >= 0 && p.workflowSel < len(p.workflows) {
			detail, err := p.runtime.GetWorkflow(p.workflows[p.workflowSel].WorkflowID)
			if err == nil {
				p.workflow = detail
			}
		}
	case liveSectionProviders:
		if p.providerSel >= 0 && p.providerSel < len(p.providers) {
			detail, err := p.runtime.GetLiveProviderDetail(p.providers[p.providerSel].ProviderID)
			if err == nil {
				p.provider = detail
			}
		}
	case liveSectionApprovals:
		if p.approvalSel >= 0 && p.approvalSel < len(p.approvals) {
			detail, err := p.runtime.GetApprovalDetail(p.approvals[p.approvalSel].ID)
			if err == nil {
				p.approval = detail
			}
		}
	}
}

func (p *SessionPane) liveWorkflowLines() []string {
	if len(p.workflows) == 0 {
		return []string{dimStyle.Render("no workflows")}
	}
	lines := make([]string, 0, len(p.workflows))
	for i, wf := range p.workflows {
		line := fmt.Sprintf("%s  %s  %s", wf.WorkflowID, wf.Status, wf.Instruction)
		if p.liveSection == liveSectionWorkflows && i == p.workflowSel {
			line = panelItemActiveStyle.Render("  " + line)
		}
		lines = append(lines, line)
	}
	return lines
}

func (p *SessionPane) liveProviderLines() []string {
	if len(p.providers) == 0 {
		return []string{dimStyle.Render("no providers")}
	}
	lines := make([]string, 0, len(p.providers))
	for i, provider := range p.providers {
		line := fmt.Sprintf("%s  %s  %s", provider.ProviderID, provider.Kind, provider.Meta.State)
		if p.liveSection == liveSectionProviders && i == p.providerSel {
			line = panelItemActiveStyle.Render("  " + line)
		}
		lines = append(lines, line)
	}
	return lines
}

func (p *SessionPane) liveApprovalLines() []string {
	if len(p.approvals) == 0 {
		return []string{dimStyle.Render("no approvals")}
	}
	lines := make([]string, 0, len(p.approvals))
	for i, approval := range p.approvals {
		line := fmt.Sprintf("%s  %s  %s", approval.ID, approval.Kind, approval.Action)
		if p.liveSection == liveSectionApprovals && i == p.approvalSel {
			line = panelItemActiveStyle.Render("  " + line)
		}
		lines = append(lines, line)
	}
	return lines
}

func (p *SessionPane) liveDetailLines() []string {
	switch p.liveSection {
	case liveSectionWorkflows:
		if p.workflow != nil {
			lines := []string{
				p.workflow.Workflow.WorkflowID,
				"",
				dimStyle.Render("Status") + "  " + p.workflow.Workflow.Status,
				dimStyle.Render("Task") + "  " + fallback(p.workflow.Workflow.TaskID, "n/a"),
				dimStyle.Render("Cursor") + "  " + fallback(p.workflow.Workflow.CursorStepID, "n/a"),
				dimStyle.Render("Instruction") + "  " + p.workflow.Workflow.Instruction,
				dimStyle.Render("Delegations") + fmt.Sprintf("  %d", len(p.workflow.Delegations)),
				dimStyle.Render("Artifacts") + fmt.Sprintf("  %d", len(p.workflow.WorkflowArtifacts)),
			}
			if len(p.workflow.ResourceDetails) > 0 {
				lines = append(lines, "", dimStyle.Render("Linked resources"))
				for _, resource := range p.workflow.ResourceDetails {
					lines = append(lines, fallback(resource.Summary, resource.URI))
				}
			}
			return lines
		}
		if len(p.workflows) == 0 {
			return []string{dimStyle.Render("No workflow selected.")}
		}
		wf := p.workflows[p.workflowSel]
		return []string{
			wf.WorkflowID,
			"",
			dimStyle.Render("Status") + "  " + wf.Status,
			dimStyle.Render("Task") + "  " + fallback(wf.TaskID, "n/a"),
			dimStyle.Render("Cursor") + "  " + fallback(wf.CursorStepID, "n/a"),
			dimStyle.Render("Instruction") + "  " + wf.Instruction,
		}
	case liveSectionProviders:
		if p.provider != nil {
			return []string{
				p.provider.ProviderID,
				"",
				dimStyle.Render("Kind") + "  " + p.provider.Kind,
				dimStyle.Render("State") + "  " + p.provider.Meta.State,
				dimStyle.Render("Trust") + "  " + fallback(p.provider.TrustBaseline, "n/a"),
				dimStyle.Render("Recoverability") + "  " + fallback(p.provider.Recoverability, "n/a"),
				dimStyle.Render("Configured from") + "  " + fallback(p.provider.ConfiguredFrom, "n/a"),
				dimStyle.Render("Capabilities") + "  " + joinOrNA(p.provider.CapabilityIDs),
				dimStyle.Render("Metadata") + "  " + joinOrNA(p.provider.Metadata),
			}
		}
		if len(p.providers) == 0 {
			return []string{dimStyle.Render("No provider selected.")}
		}
		provider := p.providers[p.providerSel]
		return []string{
			provider.ProviderID,
			"",
			dimStyle.Render("Kind") + "  " + provider.Kind,
			dimStyle.Render("State") + "  " + provider.Meta.State,
			dimStyle.Render("Trust") + "  " + fallback(provider.TrustBaseline, "n/a"),
			dimStyle.Render("Recoverability") + "  " + fallback(provider.Recoverability, "n/a"),
			dimStyle.Render("Capabilities") + "  " + joinOrNA(provider.CapabilityIDs),
		}
	default:
		if p.approval != nil {
			lines := []string{
				p.approval.ID,
				"",
				dimStyle.Render("Kind") + "  " + p.approval.Kind,
				dimStyle.Render("Action") + "  " + p.approval.Action,
				dimStyle.Render("Resource") + "  " + fallback(p.approval.Resource, "n/a"),
				dimStyle.Render("Scope") + "  " + fallback(p.approval.Scope, "n/a"),
				dimStyle.Render("Risk") + "  " + fallback(p.approval.Risk, "n/a"),
				dimStyle.Render("Justification") + "  " + fallback(p.approval.Justification, "n/a"),
			}
			if len(p.approval.Metadata) > 0 {
				lines = append(lines, dimStyle.Render("Metadata")+"  "+joinStringMap(p.approval.Metadata))
			}
			return lines
		}
		if len(p.approvals) == 0 {
			return []string{dimStyle.Render("No approval selected.")}
		}
		approval := p.approvals[p.approvalSel]
		return []string{
			approval.ID,
			"",
			dimStyle.Render("Kind") + "  " + approval.Kind,
			dimStyle.Render("Action") + "  " + approval.Action,
			dimStyle.Render("Resource") + "  " + fallback(approval.Resource, "n/a"),
			dimStyle.Render("Scope") + "  " + fallback(approval.Scope, "n/a"),
			dimStyle.Render("Justification") + "  " + fallback(approval.Justification, "n/a"),
		}
	}
}

// contextBar renders a simple fill bar for token usage.
func contextBar(pct, width int) string {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	filled := pct * width / 100
	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	style := completedStyle
	if pct > 80 {
		style = diffRemoveStyle
	} else if pct > 60 {
		style = inProgressStyle
	}
	return style.Render(bar)
}

func (p *SessionPane) viewSessionSettings() string {
	var b strings.Builder
	b.WriteString(sectionHeaderStyle.Render("Session Config") + "\n\n")

	if p.session != nil {
		rows := []struct{ k, v string }{
			{"agent", p.session.Agent},
			{"model", p.session.Model},
			{"mode", p.session.Mode},
			{"strategy", p.session.Strategy},
			{"workspace", p.session.Workspace},
		}
		for _, r := range rows {
			if r.v == "" {
				continue
			}
			b.WriteString(dimStyle.Render(fmt.Sprintf("%-10s", r.k)) + "  " + textStyle.Render(r.v) + "\n")
		}
	}

	if p.context != nil {
		b.WriteString("\n" + sectionHeaderStyle.Render("Context Files") + "\n")
		if len(p.context.Files) == 0 {
			b.WriteString(dimStyle.Render("  (none)") + "\n")
		} else {
			for _, f := range p.context.Files {
				b.WriteString(dimStyle.Render("  • ") + filePathStyle.Render(f) + "\n")
			}
		}
		b.WriteString(dimStyle.Render(fmt.Sprintf("  budget: %d tokens", p.context.MaxTokens)) + "\n")
	}

	b.WriteString("\n" + dimStyle.Render("full policy config → config tab"))
	return b.String()
}

func changeStatusDisplay(s ChangeStatus) (string, func(string) string) {
	wrap := func(st lipgloss.Style) func(string) string {
		return func(v string) string { return st.Render(v) }
	}
	switch s {
	case StatusApproved:
		return "✓", wrap(taskDoneStyle)
	case StatusRejected:
		return "✗", wrap(lipgloss.NewStyle().Foreground(lipgloss.Color("1")))
	default:
		return "?", wrap(dimStyle)
	}
}
