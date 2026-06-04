package tui

import (
	"codeburg.org/lexbit/relurpify/app/relurpish/theme"
)

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type doctorSection int
type doctorAction string

const (
	doctorActionRefresh doctorAction = "refresh"
	doctorActionHeal    doctorAction = "heal"
	doctorActionPull    doctorAction = "model-refresh"
)

const (
	doctorSectionChecks doctorSection = iota
	doctorSectionTemplates
	doctorSectionLogs
)

type doctorProgressMsg struct {
	Action   doctorAction
	Progress float64
}

type doctorStatusMsg struct {
	Action  doctorAction
	Report  DoctorReport
	Message string
	Err     error
}

type DoctorPane struct {
	runtime  RuntimeAdapter
	report   DoctorReport
	section  doctorSection
	filter   string
	working  bool
	action   doctorAction
	progress float64
	status   string
	width    int
	height   int
	// Theme is the active semantic style source.
	th *theme.Theme
	// anim is the host animation manager, set by the model via SetAnimManager.
	anim *AnimationManager
	// bar is a determinate progress indicator for operations with a known total.
	bar *ProgressBar
}

// Report returns the current doctor report.
func (p *DoctorPane) Report() DoctorReport { return p.report }

// SetReport replaces the current doctor report.
func (p *DoctorPane) SetReport(report DoctorReport) { p.report = report }

// SetStatus overrides the visible doctor status line.
func (p *DoctorPane) SetStatus(status string) { p.status = status }

func NewDoctorPane(rt RuntimeAdapter) *DoctorPane {
	p := &DoctorPane{
		th:      theme.Default(),
		runtime: rt,
		bar:     NewProgressBar(),
	}
	p.Refresh()
	return p
}

func (p *DoctorPane) SetSize(w, h int) {
	p.width = w
	p.height = h
}

func (p *DoctorPane) SetFilter(filter string) {
	p.filter = strings.ToLower(strings.TrimSpace(filter))
}

func (p *DoctorPane) Refresh() {
	if p.runtime == nil {
		p.report = DoctorReport{}
		return
	}
	p.report = p.runtime.BuildDoctorReport(context.Background())
}

func (p *DoctorPane) Update(msg tea.Msg) (*DoctorPane, tea.Cmd) {
	switch msg := msg.(type) {
	case doctorProgressMsg:
		if p.working && msg.Action == p.action {
			p.progress = min(0.95, p.progress+0.12)
			return p, p.progressCmd()
		}
		return p, nil
	case doctorStatusMsg:
		if msg.Action != "" && msg.Action == p.action {
			p.working = false
			p.progress = 1
		}
		if msg.Err != nil {
			p.status = fmt.Sprintf("%s failed: %v", msg.Action, msg.Err)
			return p, nil
		}
		if msg.Message != "" {
			p.status = msg.Message
		}
		p.report = msg.Report
		return p, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "tab":
			p.section = (p.section + 1) % 3
		case "shift+tab":
			p.section = (p.section + 2) % 3
		case "up", "k":
			// no-op: sections are content panels, but keep the key live.
		case "r":
			return p, p.refreshCmd()
		case "h":
			return p, p.healCmd()
		case "p":
			return p, p.modelRefreshCmd()
		}
	}
	return p, nil
}

func (p *DoctorPane) View() string {
	switch p.section {
	case doctorSectionTemplates:
		return p.viewTemplates()
	case doctorSectionLogs:
		return p.viewLogs()
	default:
		return p.viewChecks()
	}
}

func (p *DoctorPane) viewChecks() string {
	lines := make([]string, 0, len(p.report.Dependencies))
	for _, dep := range p.report.Dependencies {
		if p.filter != "" && !strings.Contains(strings.ToLower(dep.Name+" "+dep.Details), p.filter) {
			continue
		}
		state := "OK"
		if !dep.Available {
			state = "MISSING"
		}
		if dep.Blocking {
			state = "ERROR"
		}
		lines = append(lines, fmt.Sprintf("%-12s %-8s %s", dep.Name, state, dep.Details))
	}
	if len(lines) == 0 {
		lines = []string{p.th.Dim().Render("No matching checks")}
	}
	header := []string{
		p.th.Dim().Render("workspace") + "  " + fallback(p.report.Workspace, "(unset)"),
		p.th.Dim().Render("config") + "  " + fallback(p.report.ConfigRoot, "(unset)"),
		p.th.Dim().Render("manifest") + "  " + func() string {
			if p.report.ManifestExists {
				return "present"
			}
			return "missing"
		}(),
		p.th.Dim().Render("fingerprint") + "  " + fallback(p.report.ManifestFingerprint, "(none)"),
	}
	if len(p.report.ManifestWarnings) > 0 {
		header = append(header, p.th.Dim().Render("warnings")+"  "+strings.Join(p.report.ManifestWarnings, "; "))
	}
	return strings.Join([]string{
		sectionPanel(p.th, "Doctor / Checks", p.width, append(header, sectionList(p.th, lines, 0, p.height-8))...),
		p.footerLine(),
	}, "\n\n")
}

func (p *DoctorPane) viewTemplates() string {
	root := p.report.ConfigRoot
	if root == "" {
		root = "."
	}
	paths := []string{
		filepath.Join(root, "agents"),
		filepath.Join(root, "skills"),
		filepath.Join(root, "logs"),
		filepath.Join(root, "telemetry"),
	}
	var lines []string
	for _, path := range paths {
		info, err := os.Stat(path)
		state := "missing"
		if err == nil && info.IsDir() {
			state = "present"
		}
		if p.filter != "" && !strings.Contains(strings.ToLower(path+" "+state), p.filter) {
			continue
		}
		lines = append(lines, fmt.Sprintf("%-12s %s", state, path))
	}
	if len(lines) == 0 {
		lines = []string{p.th.Dim().Render("No matching template roots")}
	}
	return strings.Join([]string{
		sectionPanel(p.th, "Doctor / Templates", p.width, sectionList(p.th, lines, 0, p.height-6)),
		p.footerLine(),
	}, "\n\n")
}

func (p *DoctorPane) viewLogs() string {
	lines := []string{
		fmt.Sprintf("Checked at: %s", p.report.CheckedAt.Format(time.RFC3339)),
		fmt.Sprintf("Config error: %s", fallback(p.report.ConfigError, "(none)")),
		fmt.Sprintf("Manifest error: %s", fallback(p.report.ManifestError, "(none)")),
		fmt.Sprintf("Inference state: %s", fallback(string(p.report.Inference.State), "(unknown)")),
	}
	if len(p.report.DeprecationNotices) > 0 {
		lines = append(lines, "Deprecations:")
		for _, notice := range p.report.DeprecationNotices {
			lines = append(lines, "  - "+notice)
		}
	}
	if p.filter != "" {
		var filtered []string
		for _, line := range lines {
			if strings.Contains(strings.ToLower(line), p.filter) {
				filtered = append(filtered, line)
			}
		}
		lines = filtered
	}
	if len(lines) == 0 {
		lines = []string{p.th.Dim().Render("No matching logs")}
	}
	return strings.Join([]string{
		sectionPanel(p.th, "Doctor / Logs", p.width, lines...),
		p.footerLine(),
	}, "\n\n")
}

func (p *DoctorPane) footerLine() string {
	footer := p.th.Dim().Render("tab checks/templates/logs  r refresh  h heal  p models")
	if p.working {
		barView := renderPercentBar(p.progress, 18)
		if p.bar != nil && !p.bar.Done() {
			barView = p.bar.View()
		}
		footer = p.th.Dim().Render(fmt.Sprintf("%s  %s", p.action, barView))
	}
	if p.status != "" {
		footer = p.th.Dim().Render(p.status) + "\n" + footer
	}
	return footer
}

func (p *DoctorPane) refreshCmd() tea.Cmd {
	return p.runAsync(doctorActionRefresh)
}

func (p *DoctorPane) healCmd() tea.Cmd {
	return p.runAsync(doctorActionHeal)
}

func (p *DoctorPane) modelRefreshCmd() tea.Cmd {
	return p.runAsync(doctorActionPull)
}

func (p *DoctorPane) runAsync(action doctorAction) tea.Cmd {
	if p.runtime == nil {
		p.status = "runtime unavailable"
		return nil
	}
	p.working = true
	p.action = action
	p.progress = 0
	p.status = ""
	if p.bar != nil {
		p.bar.SetTarget(1)
	}
	return tea.Batch(p.progressCmd(), func() tea.Msg {
		ctx := context.Background()
		switch action {
		case doctorActionRefresh:
			report := p.runtime.BuildDoctorReport(ctx)
			return doctorStatusMsg{
				Action:  action,
				Report:  report,
				Message: "dependency check complete",
			}
		case doctorActionHeal:
			if err := p.runtime.InitializeWorkspaceFromTemplates(false); err != nil {
				return doctorStatusMsg{Action: action, Err: err}
			}
			report := p.runtime.BuildDoctorReport(ctx)
			return doctorStatusMsg{
				Action:  action,
				Report:  report,
				Message: "starter templates copied",
			}
		case doctorActionPull:
			models, err := p.runtime.InferenceModels(ctx)
			if err != nil {
				return doctorStatusMsg{Action: action, Err: err}
			}
			report := p.runtime.BuildDoctorReport(ctx)
			if len(models) > 0 {
				report.Inference.Models = append([]string(nil), models...)
			}
			return doctorStatusMsg{
				Action:  action,
				Report:  report,
				Message: fmt.Sprintf("refreshed %d models", len(models)),
			}
		default:
			return doctorStatusMsg{Action: action, Err: fmt.Errorf("unknown doctor action %q", action)}
		}
	})
}

func (p *DoctorPane) progressCmd() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg {
		return doctorProgressMsg{Action: p.action, Progress: p.progress}
	})
}

// SetTheme sets the active semantic style source.
func (p *DoctorPane) SetTheme(th *theme.Theme) {
	if th != nil {
		p.th = th
	}
}

// SetAnimManager links the host animation manager and wires it to the
// progress bar so determinate animations consume the shared tick budget.
func (p *DoctorPane) SetAnimManager(m *AnimationManager) {
	p.anim = m
	if p.bar != nil {
		p.bar.SetAnimManager(m)
	}
}

// SetReduceMotion passes the reduce-motion detector to internal components.
func (p *DoctorPane) SetReduceMotion(r *ReduceMotion) {
	if p.bar != nil {
		p.bar.SetReduceMotion(r)
	}
}
