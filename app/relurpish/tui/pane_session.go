package tui

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"codeburg.org/lexbit/relurpify/app/relurpish/theme"
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

	context       *AgentContext
	session       *Session
	runtime       RuntimeAdapter
	frameworkMode bool
	width         int
	height        int
	// Theme is the active semantic style source.
	th *theme.Theme
}

// NewSessionPane creates a SessionPane.
func NewSessionPane(ctx *AgentContext, sess *Session, rt RuntimeAdapter) *SessionPane {
	return &SessionPane{
		th:      theme.Default(),
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

// SetFrameworkMode switches the pane into the base-framework provider dashboard.
func (p *SessionPane) SetFrameworkMode(on bool) { p.frameworkMode = on }

// SetSection switches the visible task section.
func (p *SessionPane) SetSection(section SessionSection) {
	p.section = section
}

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
	case ServicesUpdatedMsg:
		p.services = append([]ServiceInfo(nil), msg.Services...)
		if p.serviceSel >= len(p.services) {
			p.serviceSel = max(0, len(p.services)-1)
		}
		return p, nil

	case fileIndexMsg:
		p.loading = false
		if msg.err != nil {
			p.loadErr = msg.err
			return p, nil
		}
		p.allFiles = msg.files
		p.applyFilter()

	case tea.KeyMsg:
		if p.frameworkMode {
			switch msg.String() {
			case "tab", "right", "l":
				p.liveSection = (p.liveSection + 1) % 3
			case "shift+tab", "left", "h":
				p.liveSection = (p.liveSection + 2) % 3
			case "up", "k":
				p.moveLiveSelection(-1)
			case "down", "j":
				p.moveLiveSelection(1)
			case "r":
				if p.runtime != nil {
					// Refresh on demand so the provider dashboard stays live.
					workflows, _ := p.runtime.ListWorkflows(3)
					p.SetLiveSnapshot(p.runtime.Diagnostics(), workflows, p.runtime.ListLiveProviders(), p.runtime.ListApprovals())
				}
			}
			return p, nil
		}
		if p.activeSubTab == SubTabSessionLive {
			switch msg.String() {
			case "tab", "right", "l":
				p.liveSection = (p.liveSection + 1) % 3
			case "shift+tab", "left", "h":
				p.liveSection = (p.liveSection + 2) % 3
			case "up", "k":
				p.moveLiveSelection(-1)
			case "down", "j":
				p.moveLiveSelection(1)
			}
			return p, nil
		}
		if p.activeSubTab == SubTabSessionServices {
			switch msg.String() {
			case "up", "k":
				if p.serviceSel > 0 {
					p.serviceSel--
				}
			case "down", "j":
				if p.serviceSel < len(p.services)-1 {
					p.serviceSel++
				}
			case "s":
				if p.runtime != nil && p.serviceSel >= 0 && p.serviceSel < len(p.services) {
					serviceID := p.services[p.serviceSel].ID
					_ = p.runtime.StopService(serviceID)
				}
			case "r":
				if p.runtime != nil && p.serviceSel >= 0 && p.serviceSel < len(p.services) {
					serviceID := p.services[p.serviceSel].ID
					_ = p.runtime.RestartService(context.Background(), serviceID)
				}
			case "R":
				if p.runtime != nil {
					_ = p.runtime.RestartAllServices(context.Background())
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
				editor := EditorPath()
			editorPath, err := exec.LookPath(editor)
			if err != nil {
				editorPath = editor
			}
			return p, tea.ExecProcess(
				&exec.Cmd{
					Path: editorPath,
					Args: []string{editorPath, filepath.Clean(path)},
				}, func(err error) tea.Msg {
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
func (p *SessionPane) HandleFilterInput(query string) {
	p.SetFilter(query)
}

// SetFilter updates the live filter string for the active base shell panel or
// the task file browser.
func (p *SessionPane) SetFilter(query string) {
	p.filter = strings.TrimSpace(query)
	p.fileSel = 0
	if p.frameworkMode {
		return
	}
	if p.activeSubTab != "" && p.activeSubTab != SubTabSessionTasks {
		return
	}
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

// View renders the active session subtab.
func (p *SessionPane) View() string {
	if p.frameworkMode {
		return p.viewLive()
	}
	switch p.activeSubTab {
	case SubTabSessionLive:
		return p.viewLive()
	case SubTabSessionTasks:
		if p.section == SectionChanges {
			return p.viewChanges()
		}
		return p.viewFiles()
	case SubTabSessionServices:
		return p.viewServices()
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
		return p.th.Dim().Render("Indexing workspace files...")
	}
	if p.loadErr != nil {
		return p.th.Notif(theme.NotifError).Render(fmt.Sprintf("File index error: %v", p.loadErr))
	}
	var b strings.Builder
	header := "Workspace Files"
	if p.filter != "" {
		header += "  " + p.th.Dim().Render(fmt.Sprintf("filter: %q", p.filter))
	}
	b.WriteString(p.th.Subhead().Render(header))
	b.WriteString("\n\n")
	if len(p.queued) > 0 {
		b.WriteString(p.th.Subhead().Render("Queued Tasks") + "\n")
		for _, task := range p.queued {
			style := p.th.Pending()
			icon := "☐"
			switch task.Status {
			case TaskCompleted:
				style = p.th.Success()
				icon = "✓"
			case TaskInProgress:
				style = p.th.Warning()
				icon = "●"
			}
			fmt.Fprintf(&b, "%s  %s\n", style.Render(icon), style.Render(task.Description))
		}
		b.WriteString("\n")
	}
	if len(p.filtered) == 0 {
		b.WriteString(p.th.Dim().Render("No matching files"))
	} else {
		for i, e := range p.filtered {
			line := renderFileEntryLine(e)
			if i == p.fileSel {
				line = p.th.Active().Render(line)
			} else {
				line = p.th.Body().Render(line)
			}
			b.WriteString(line + "\n")
		}
	}
	if p.context != nil && len(p.context.Files) > 0 {
		b.WriteString("\n" + p.th.Subhead().Render("Context") + "\n")
		for _, f := range p.context.Files {
			b.WriteString(p.th.Dim().Render("  • ") + p.th.Subhead().Render(f) + "\n")
		}
	}
	b.WriteString("\n" + p.th.Dim().Render("enter=add to context  e=open in editor  tab=view changes"))
	return b.String()
}

func (p *SessionPane) viewChanges() string {
	var b strings.Builder
	b.WriteString(p.th.Subhead().Render("Session Changes"))
	b.WriteString("\n\n")
	if len(p.queued) > 0 {
		b.WriteString(p.th.Subhead().Render("Queued Tasks") + "\n")
		for _, task := range p.queued {
			style := p.th.Pending()
			icon := "☐"
			switch task.Status {
			case TaskCompleted:
				style = p.th.Success()
				icon = "✓"
			case TaskInProgress:
				style = p.th.Warning()
				icon = "●"
			}
			fmt.Fprintf(&b, "%s  %s\n", style.Render(icon), style.Render(task.Description))
		}
		b.WriteString("\n")
	}
	if len(p.changes) == 0 {
		b.WriteString(p.th.Dim().Render("No changes in this session yet"))
		b.WriteString("\n\n" + p.th.Dim().Render("tab=view files"))
		return b.String()
	}
	for i, c := range p.changes {
		statusIcon, statusRender := changeStatusDisplay(p.th, c.Status)
		changeType := string(c.Type)
		if changeType == "" {
			changeType = "modify"
		}
		line := statusRender(statusIcon) + "  " +
			p.th.Subhead().Render(c.Path) +
			"  " + p.th.Dim().Render("("+changeType+")")
		if c.LinesAdded > 0 || c.LinesRemoved > 0 {
			line += p.th.Dim().Render(fmt.Sprintf("  +%d/-%d", c.LinesAdded, c.LinesRemoved))
		}
		if i == p.changeSel {
			line = p.th.Active().Render(line)
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\n" + p.th.Dim().Render("y=approve  n=reject  e=open in editor  tab=view files"))
	return b.String()
}

func (p *SessionPane) viewLive() string {
	widths := splitWidths(p.width, 4, 4, 4)
	var b strings.Builder
	title := "Live Session"
	if p.frameworkMode {
		title = "AI Provider"
	}
	b.WriteString(p.th.Subhead().Render(title) + "\n\n")
	filter := strings.ToLower(strings.TrimSpace(p.filter))
	if filter != "" {
		b.WriteString(p.th.Dim().Render(fmt.Sprintf("filter: %q", p.filter)) + "\n\n")
	}

	d := p.diagnostics
	if p.session != nil {
		if filter == "" || strings.Contains(strings.ToLower(p.session.Workspace), filter) {
			b.WriteString(p.th.Dim().Render("workspace  ") + p.th.Subhead().Render(p.session.Workspace) + "\n")
		}
		if filter == "" || strings.Contains(strings.ToLower(p.session.Agent), filter) {
			b.WriteString(p.th.Dim().Render("agent      ") + p.th.Body().Render(p.session.Agent) + "\n")
		}
		if p.session.Provider != "" {
			label := p.session.Provider
			if p.session.BackendState != "" {
				label = fmt.Sprintf("%s [%s]", label, p.session.BackendState)
			}
			if filter == "" || strings.Contains(strings.ToLower(label), filter) {
				b.WriteString(p.th.Dim().Render("provider   ") + p.th.Body().Render(label) + "\n")
			}
		}
		if filter == "" || strings.Contains(strings.ToLower(p.session.Model), filter) {
			b.WriteString(p.th.Dim().Render("model      ") + p.th.Body().Render(p.session.Model) + "\n")
		}
		if p.session.Mode != "" && (filter == "" || strings.Contains(strings.ToLower(p.session.Mode), filter)) {
			b.WriteString(p.th.Dim().Render("mode       ") + p.th.Warning().Render(p.session.Mode) + "\n")
		}
		if p.session.Strategy != "" && (filter == "" || strings.Contains(strings.ToLower(p.session.Strategy), filter)) {
			b.WriteString(p.th.Dim().Render("strategy   ") + p.th.Body().Render(p.session.Strategy) + "\n")
		}
		dur := p.session.TotalDuration.Round(1e9)
		if filter == "" || strings.Contains(strings.ToLower(fmt.Sprintf("%d %s", p.session.TotalTokens, dur.String())), filter) {
			b.WriteString(p.th.Dim().Render("tokens     ") + fmt.Sprintf("%d  %s", p.session.TotalTokens, p.th.Dim().Render(dur.String())) + "\n")
		}
		b.WriteString("\n")
	}

	if d.ContextTokensMax > 0 {
		pct := 100 * d.ContextTokensUsed / d.ContextTokensMax
		bar := contextBar(p.th, pct, 20)
		line := p.th.Dim().Render("context    ") + bar + p.th.Dim().Render(fmt.Sprintf("  %d/%d", d.ContextTokensUsed, d.ContextTokensMax))
		if filter == "" || strings.Contains(strings.ToLower(line), filter) {
			b.WriteString(line + "\n")
		}
	}
	if d.ActiveWorkflows > 0 || d.PatternEntries > 0 {
		if filter == "" || strings.Contains(strings.ToLower(fmt.Sprintf("%d", d.ActiveWorkflows)), filter) {
			b.WriteString(p.th.Dim().Render("workflows  ") + fmt.Sprintf("%d", d.ActiveWorkflows) + "\n")
		}
		if filter == "" || strings.Contains(strings.ToLower(fmt.Sprintf("%d", d.PatternEntries)), filter) {
			b.WriteString(p.th.Dim().Render("patterns   ") + fmt.Sprintf("%d", d.PatternEntries) + "\n")
		}
	}
	if d.ActiveMode != "" {
		if filter == "" || strings.Contains(strings.ToLower(d.ActiveMode), filter) {
			b.WriteString(p.th.Dim().Render("exec mode  ") + p.th.Warning().Render(d.ActiveMode) + "\n")
		}
	}
	if d.ActivePhase != "" {
		if filter == "" || strings.Contains(strings.ToLower(d.ActivePhase), filter) {
			b.WriteString(p.th.Dim().Render("phase      ") + p.th.Body().Render(d.ActivePhase) + "\n")
		}
	}
	if d.ActiveProfile != "" {
		if filter == "" || strings.Contains(strings.ToLower(d.ActiveProfile), filter) {
			b.WriteString(p.th.Dim().Render("profile    ") + p.th.Body().Render(d.ActiveProfile) + "\n")
		}
	}
	if d.ProfileReason != "" {
		if filter == "" || strings.Contains(strings.ToLower(d.ProfileReason), filter) {
			b.WriteString(p.th.Dim().Render("reason     ") + p.th.Body().Render(d.ProfileReason) + "\n")
		}
	}
	if d.ManifestFingerprint != "" {
		if filter == "" || strings.Contains(strings.ToLower(d.ManifestFingerprint), filter) {
			b.WriteString(p.th.Dim().Render("fingerprint") + "  " + p.th.Body().Render(d.ManifestFingerprint) + "\n")
		}
	}
	if len(d.ProtectedPaths) > 0 {
		line := strings.Join(d.ProtectedPaths, ", ")
		if filter == "" || strings.Contains(strings.ToLower(line), filter) {
			b.WriteString(p.th.Dim().Render("sandbox    ") + p.th.Body().Render(line) + "\n")
		}
	}
	if d.ManifestPolicy != "" {
		if filter == "" || strings.Contains(strings.ToLower(d.ManifestPolicy), filter) {
			b.WriteString(p.th.Dim().Render("policy     ") + p.th.Body().Render(d.ManifestPolicy) + "\n")
		}
	}
	if d.DoomLoopState != "" && d.DoomLoopState != "idle" {
		if filter == "" || strings.Contains(strings.ToLower(d.DoomLoopState), filter) {
			b.WriteString(p.th.Dim().Render("doom loop  ") + p.th.Error().Render(d.DoomLoopState) + "\n")
		}
	}
	if d.ContextStrategy != "" {
		if filter == "" || strings.Contains(strings.ToLower(d.ContextStrategy), filter) {
			b.WriteString(p.th.Dim().Render("ctx strat  ") + p.th.Body().Render(d.ContextStrategy) + "\n")
		}
	}
	if d.PruningEvents > 0 {
		line := fmt.Sprintf("%d event(s)", d.PruningEvents)
		if filter == "" || strings.Contains(strings.ToLower(line), filter) {
			b.WriteString(p.th.Dim().Render("pruning    ") + p.th.Warning().Render(line) + "\n")
		}
	}
	if d.CapabilitiesTotal > 0 {
		if filter == "" || strings.Contains(strings.ToLower(fmt.Sprintf("%d", d.CapabilitiesTotal)), filter) {
			b.WriteString(p.th.Dim().Render("caps       ") + fmt.Sprintf("%d", d.CapabilitiesTotal) + "\n")
		}
	}
	if d.PendingApprovals > 0 {
		line := fmt.Sprintf("%d", d.PendingApprovals)
		if filter == "" || strings.Contains(strings.ToLower(line), filter) {
			b.WriteString(p.th.Dim().Render("pending ✓  ") + p.th.Warning().Render(line) + "\n")
		}
	}
	if d.LiveProviders > 0 {
		if filter == "" || strings.Contains(strings.ToLower(fmt.Sprintf("%d", d.LiveProviders)), filter) {
			b.WriteString(p.th.Dim().Render("providers  ") + fmt.Sprintf("%d", d.LiveProviders) + "\n")
		}
	}
	if len(d.DeprecationNotices) > 0 {
		if filter == "" || strings.Contains(strings.ToLower(fmt.Sprintf("%d", len(d.DeprecationNotices))), filter) {
			b.WriteString(p.th.Dim().Render("deprecate  ") + fmt.Sprintf("%d", len(d.DeprecationNotices)) + "\n")
		}
	}

	panels := []string{
		sectionPanel(p.th, "Summary", widths[0], strings.Split(strings.TrimRight(b.String(), "\n"), "\n")...),
		sectionPanel(p.th, "Workflows", widths[1], sectionList(p.th, p.liveWorkflowLines(), p.workflowSel, p.height-12)),
		sectionPanel(p.th, "Providers / Approvals", widths[2],
			p.th.Subhead().Render("Providers"),
			sectionList(p.th, p.liveProviderLines(), p.providerSel, 4),
			"",
			p.th.Subhead().Render("Approvals"),
			sectionList(p.th, p.liveApprovalLines(), p.approvalSel, 4),
		),
	}
	return strings.Join([]string{
		lipgloss.JoinHorizontal(lipgloss.Top, panels...),
		sectionPanel(p.th, "Detail", p.width, p.liveDetailLines()...),
		p.th.Dim().Render("tab/shift+tab switch focus  ↑↓ navigate"),
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
		return []string{p.th.Dim().Render("no workflows")}
	}
	filter := strings.ToLower(strings.TrimSpace(p.filter))
	lines := make([]string, 0, len(p.workflows))
	for i, wf := range p.workflows {
		if filter != "" && !strings.Contains(strings.ToLower(wf.WorkflowID+" "+wf.Status+" "+wf.Instruction), filter) {
			continue
		}
		line := fmt.Sprintf("%s  %s  %s", wf.WorkflowID, wf.Status, wf.Instruction)
		if p.liveSection == liveSectionWorkflows && i == p.workflowSel {
			line = p.th.Active().Render("  " + line)
		}
		lines = append(lines, line)
	}
	return lines
}

func (p *SessionPane) liveProviderLines() []string {
	if len(p.providers) == 0 {
		return []string{p.th.Dim().Render("no providers")}
	}
	filter := strings.ToLower(strings.TrimSpace(p.filter))
	lines := make([]string, 0, len(p.providers))
	for i, provider := range p.providers {
		if filter != "" && !strings.Contains(strings.ToLower(provider.ProviderID+" "+provider.Kind+" "+provider.Meta.State), filter) {
			continue
		}
		line := fmt.Sprintf("%s  %s  %s", provider.ProviderID, provider.Kind, provider.Meta.State)
		if p.liveSection == liveSectionProviders && i == p.providerSel {
			line = p.th.Active().Render("  " + line)
		}
		lines = append(lines, line)
	}
	return lines
}

func (p *SessionPane) liveApprovalLines() []string {
	if len(p.approvals) == 0 {
		return []string{p.th.Dim().Render("no approvals")}
	}
	filter := strings.ToLower(strings.TrimSpace(p.filter))
	lines := make([]string, 0, len(p.approvals))
	for i, approval := range p.approvals {
		if filter != "" && !strings.Contains(strings.ToLower(approval.ID+" "+approval.Kind+" "+approval.Action), filter) {
			continue
		}
		line := fmt.Sprintf("%s  %s  %s", approval.ID, approval.Kind, approval.Action)
		if p.liveSection == liveSectionApprovals && i == p.approvalSel {
			line = p.th.Active().Render("  " + line)
		}
		lines = append(lines, line)
	}
	return lines
}

func (p *SessionPane) liveDetailLines() []string {
	filter := strings.ToLower(strings.TrimSpace(p.filter))
	switch p.liveSection {
	case liveSectionWorkflows:
		if p.workflow != nil {
			lines := []string{
				p.workflow.Workflow.WorkflowID,
				"",
				p.th.Dim().Render("Status") + "  " + p.workflow.Workflow.Status,
				p.th.Dim().Render("Task") + "  " + fallback(p.workflow.Workflow.TaskID, "n/a"),
				p.th.Dim().Render("Cursor") + "  " + fallback(p.workflow.Workflow.CursorStepID, "n/a"),
				p.th.Dim().Render("Instruction") + "  " + p.workflow.Workflow.Instruction,
				p.th.Dim().Render("Delegations") + fmt.Sprintf("  %d", len(p.workflow.Delegations)),
				p.th.Dim().Render("Artifacts") + fmt.Sprintf("  %d", len(p.workflow.WorkflowArtifacts)),
			}
			if len(p.workflow.ResourceDetails) > 0 {
				lines = append(lines, "", p.th.Dim().Render("Linked resources"))
				for _, resource := range p.workflow.ResourceDetails {
					lines = append(lines, fallback(resource.Summary, resource.URI))
				}
			}
			if filter != "" {
				var filtered []string
				for _, line := range lines {
					if strings.Contains(strings.ToLower(line), filter) {
						filtered = append(filtered, line)
					}
				}
				if len(filtered) > 0 {
					return filtered
				}
				return []string{p.th.Dim().Render("No matching workflow detail")}
			}
			return lines
		}
		if len(p.workflows) == 0 {
			return []string{p.th.Dim().Render("No workflow selected.")}
		}
		wf := p.workflows[p.workflowSel]
		return []string{
			wf.WorkflowID,
			"",
			p.th.Dim().Render("Status") + "  " + wf.Status,
			p.th.Dim().Render("Task") + "  " + fallback(wf.TaskID, "n/a"),
			p.th.Dim().Render("Cursor") + "  " + fallback(wf.CursorStepID, "n/a"),
			p.th.Dim().Render("Instruction") + "  " + wf.Instruction,
		}
	case liveSectionProviders:
		if p.provider != nil {
			lines := []string{
				p.provider.ProviderID,
				"",
				p.th.Dim().Render("Kind") + "  " + p.provider.Kind,
				p.th.Dim().Render("State") + "  " + p.provider.Meta.State,
				p.th.Dim().Render("Trust") + "  " + fallback(p.provider.TrustBaseline, "n/a"),
				p.th.Dim().Render("Recoverability") + "  " + fallback(p.provider.Recoverability, "n/a"),
				p.th.Dim().Render("Configured from") + "  " + fallback(p.provider.ConfiguredFrom, "n/a"),
				p.th.Dim().Render("Capabilities") + "  " + joinOrNA(p.provider.CapabilityIDs),
				p.th.Dim().Render("Metadata") + "  " + joinOrNA(p.provider.Metadata),
			}
			if filter != "" {
				var filtered []string
				for _, line := range lines {
					if strings.Contains(strings.ToLower(line), filter) {
						filtered = append(filtered, line)
					}
				}
				if len(filtered) > 0 {
					return filtered
				}
				return []string{p.th.Dim().Render("No matching provider detail")}
			}
			return lines
		}
		if len(p.providers) == 0 {
			return []string{p.th.Dim().Render("No provider selected.")}
		}
		provider := p.providers[p.providerSel]
		return []string{
			provider.ProviderID,
			"",
			p.th.Dim().Render("Kind") + "  " + provider.Kind,
			p.th.Dim().Render("State") + "  " + provider.Meta.State,
			p.th.Dim().Render("Trust") + "  " + fallback(provider.TrustBaseline, "n/a"),
			p.th.Dim().Render("Recoverability") + "  " + fallback(provider.Recoverability, "n/a"),
			p.th.Dim().Render("Capabilities") + "  " + joinOrNA(provider.CapabilityIDs),
		}
	case liveSectionApprovals:
		if p.approval != nil {
			lines := []string{
				p.approval.ID,
				"",
				p.th.Dim().Render("Kind") + "  " + p.approval.Kind,
				p.th.Dim().Render("Action") + "  " + p.approval.Action,
				p.th.Dim().Render("Resource") + "  " + fallback(p.approval.Resource, "n/a"),
				p.th.Dim().Render("Scope") + "  " + fallback(p.approval.Scope, "n/a"),
				p.th.Dim().Render("Risk") + "  " + fallback(p.approval.Risk, "n/a"),
				p.th.Dim().Render("Justification") + "  " + fallback(p.approval.Justification, "n/a"),
			}
			if len(p.approval.Metadata) > 0 {
				lines = append(lines, p.th.Dim().Render("Metadata")+"  "+joinStringMap(p.approval.Metadata))
			}
			if filter != "" {
				var filtered []string
				for _, line := range lines {
					if strings.Contains(strings.ToLower(line), filter) {
						filtered = append(filtered, line)
					}
				}
				if len(filtered) > 0 {
					return filtered
				}
				return []string{p.th.Dim().Render("No matching approval detail")}
			}
			return lines
		}
		if len(p.approvals) == 0 {
			return []string{p.th.Dim().Render("No approval selected.")}
		}
		approval := p.approvals[p.approvalSel]
		return []string{
			approval.ID,
			"",
			p.th.Dim().Render("Kind") + "  " + approval.Kind,
			p.th.Dim().Render("Action") + "  " + approval.Action,
			p.th.Dim().Render("Resource") + "  " + fallback(approval.Resource, "n/a"),
			p.th.Dim().Render("Scope") + "  " + fallback(approval.Scope, "n/a"),
			p.th.Dim().Render("Justification") + "  " + fallback(approval.Justification, "n/a"),
		}
	}
	return nil
}

// contextBar renders a simple fill bar for token usage.
func contextBar(th *theme.Theme, pct, width int) string {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	filled := pct * width / 100
	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	style := th.Success()
	if pct > 80 {
		style = th.Error()
	} else if pct > 60 {
		style = th.Warning()
	}
	return style.Render(bar)
}

func (p *SessionPane) viewSessionSettings() string {
	var b strings.Builder
	b.WriteString(p.th.Subhead().Render("Session Config") + "\n\n")

	if p.session != nil {
		rows := []struct{ k, v string }{
			{"agent", p.session.Agent},
			{"model", p.session.Model},
			{"mode", p.session.Mode},
			{"strategy", p.session.Strategy},
			{"workspace", p.session.Workspace},
			{"profile", p.session.Profile},
			{"profile_reason", p.session.ProfileReason},
			{"profile_source", p.session.ProfileSource},
		}
		for _, r := range rows {
			if r.v == "" {
				continue
			}
			b.WriteString(p.th.Dim().Render(fmt.Sprintf("%-10s", r.k)) + "  " + p.th.Body().Render(r.v) + "\n")
		}
	}

	if p.context != nil {
		b.WriteString("\n" + p.th.Subhead().Render("Context Files") + "\n")
		if len(p.context.Files) == 0 {
			b.WriteString(p.th.Dim().Render("  (none)") + "\n")
		} else {
			for _, f := range p.context.Files {
				b.WriteString(p.th.Dim().Render("  • ") + p.th.Subhead().Render(f) + "\n")
			}
		}
		b.WriteString(p.th.Dim().Render(fmt.Sprintf("  budget: %d tokens", p.context.MaxTokens)) + "\n")
	}

	if p.diagnostics.ManifestFingerprint != "" || len(p.diagnostics.ProtectedPaths) > 0 || p.diagnostics.ManifestPolicy != "" {
		b.WriteString("\n" + p.th.Subhead().Render("Manifest / Permissions") + "\n")
		if p.diagnostics.ManifestFingerprint != "" {
			b.WriteString(p.th.Dim().Render("  fingerprint") + "  " + p.th.Body().Render(p.diagnostics.ManifestFingerprint) + "\n")
		}
		if p.diagnostics.ManifestPolicy != "" {
			b.WriteString(p.th.Dim().Render("  policy") + "  " + p.th.Body().Render(p.diagnostics.ManifestPolicy) + "\n")
		}
		if len(p.diagnostics.ProtectedPaths) > 0 {
			b.WriteString(p.th.Dim().Render("  sandbox") + "  " + p.th.Body().Render(strings.Join(p.diagnostics.ProtectedPaths, ", ")) + "\n")
		}
		if len(p.diagnostics.DeprecationNotices) > 0 {
			b.WriteString(p.th.Dim().Render("  deprecations") + "  " + p.th.Body().Render(strings.Join(p.diagnostics.DeprecationNotices, "; ")) + "\n")
		}
	}

	b.WriteString("\n" + p.th.Dim().Render("full policy config → config tab"))
	return b.String()
}

func (p *SessionPane) viewServices() string {
	var b strings.Builder
	b.WriteString(p.th.Subhead().Render("ayenitd services") + "\n\n")

	if len(p.services) == 0 {
		b.WriteString(p.th.Dim().Render("No services registered.") + "\n")
	} else {
		for i, svc := range p.services {
			var statusBadge string
			switch svc.Status {
			case ServiceStatusRunning:
				statusBadge = p.th.Success().Render("[running]")
			case ServiceStatusStopped:
				statusBadge = p.th.Dim().Render("[stopped]")
			case ServiceStatusError:
				statusBadge = p.th.Error().Render("[error]")
			default:
				statusBadge = p.th.Dim().Render("[unknown]")
			}
			line := fmt.Sprintf("%-24s %s", svc.ID, statusBadge)
			if i == p.serviceSel {
				line = p.th.Active().Render(line)
			} else {
				line = p.th.Body().Render(line)
			}
			b.WriteString(line + "\n")
			if i == p.serviceSel {
				if detail := serviceSummaryLines(p.th, svc); len(detail) > 0 {
					for _, line := range detail {
						b.WriteString(p.th.Dim().Render("  ") + line + "\n")
					}
					b.WriteString("\n")
				}
			}
		}
	}

	b.WriteString("\n" + p.th.Dim().Render("[s] stop  [r] restart  [R] restart-all  ↑↓ navigate"))
	return b.String()
}

func serviceSummaryLines(th *theme.Theme, svc ServiceInfo) []string {
	if strings.TrimSpace(svc.Source) == "" && strings.TrimSpace(svc.Owner) == "" && len(svc.Notes) == 0 {
		return nil
	}
	lines := make([]string, 0, 3+len(svc.Notes))
	if strings.TrimSpace(svc.Source) != "" {
		lines = append(lines, th.Dim().Render("source")+"  "+th.Body().Render(svc.Source))
	}
	if strings.TrimSpace(svc.Owner) != "" {
		lines = append(lines, th.Dim().Render("owner")+"  "+th.Body().Render(svc.Owner))
	}
	if len(svc.Notes) > 0 {
		lines = append(lines, th.Dim().Render("notes"))
		for _, note := range svc.Notes {
			lines = append(lines, "    "+th.Body().Render(note))
		}
	}
	return lines
}

func changeStatusDisplay(th *theme.Theme, s ChangeStatus) (string, func(string) string) {
	wrap := func(st lipgloss.Style) func(string) string {
		return func(v string) string { return st.Render(v) }
	}
	switch s {
	case StatusApproved:
		return "✓", wrap(th.Success())
	case StatusRejected:
		return "✗", wrap(th.Error())
	default:
		return "?", wrap(th.Dim())
	}
}

// SetTheme sets the active semantic style source.
func (p *SessionPane) SetTheme(th *theme.Theme) {
	if th != nil {
		p.th = th
	}
}
