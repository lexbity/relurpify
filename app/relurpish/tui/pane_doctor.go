package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	runtimesvc "codeburg.org/lexbit/relurpify/app/relurpish/runtime"
	"codeburg.org/lexbit/relurpify/app/relurpish/theme"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

type doctorAction string

const (
	doctorActionRefresh doctorAction = "refresh"
	doctorActionHeal    doctorAction = "initialize"
)

type doctorStatusMsg struct {
	Request uint64
	Action  doctorAction
	Report  runtimesvc.DoctorReport
	Message string
	Err     error
}

// DoctorOperations is independent of the execution adapter. A bootstrap service
// can supply initialization without pretending to be an execution runtime.
type DoctorOperations interface {
	InitializeWorkspaceFromTemplates(bool) error
}

// DoctorPane is a sparse, read-only findings list with deliberate detail/review.
// It never queries files or providers from View or its constructor.
type DoctorPane struct {
	parent        context.Context
	reader        DoctorReader
	initializer   DoctorOperations
	report        runtimesvc.DoctorReport
	hasReport     bool
	demo          bool
	selected      string
	detail        bool
	offset        int
	filter        string
	working       bool
	review        bool
	request       uint64
	status        string
	width, height int
	th            *theme.Theme
}

func NewDoctorPane(parent context.Context, reader DoctorReader, initializer DoctorOperations) *DoctorPane {
	return &DoctorPane{parent: parent, reader: reader, initializer: initializer, th: theme.Default()}
}

func (p *DoctorPane) Report() runtimesvc.DoctorReport { return p.report }
func (p *DoctorPane) SetReport(report runtimesvc.DoctorReport) {
	p.report, p.hasReport = report, true
	if p.selected == "" {
		items := p.items()
		if len(items) > 0 {
			p.selected = items[0].Name
		}
	}
}
func (p *DoctorPane) SetReader(reader DoctorReader, initializer DoctorOperations) {
	p.reader, p.initializer = reader, initializer
}
func (p *DoctorPane) SetStatus(status string)   { p.status = status }
func (p *DoctorPane) SetSize(width, height int) { p.width, p.height = width, height }
func (p *DoctorPane) SetFilter(filter string)   { p.filter = strings.ToLower(strings.TrimSpace(filter)) }
func (p *DoctorPane) SetTheme(th *theme.Theme) {
	if th != nil {
		p.th = th
	}
}
func (p *DoctorPane) OwnsEscape() bool { return p.detail || p.review }

func (p *DoctorPane) Update(message tea.Msg) (*DoctorPane, tea.Cmd) {
	switch msg := message.(type) {
	case doctorStatusMsg:
		if msg.Request != p.request {
			return p, nil
		}
		p.working = false
		if msg.Err != nil {
			p.status = fmt.Sprintf("%s failed: %v", msg.Action, msg.Err)
			return p, nil
		}
		p.SetReport(msg.Report)
		p.status = msg.Message
		return p, func() tea.Msg {
			return DoctorStatusMsg{Action: string(msg.Action), Report: msg.Report, Message: msg.Message}
		}
	case tea.KeyMsg:
		if p.review {
			switch msg.String() {
			case "esc":
				p.review = false
			case "enter":
				p.review = false
				return p, p.run(doctorActionHeal)
			}
			return p, nil
		}
		switch msg.String() {
		case "esc":
			p.detail, p.offset = false, 0
		case "enter":
			p.detail, p.offset = true, 0
		case "up", "down", "pgup", "pgdown":
			delta := 1
			if msg.String() == "up" || msg.String() == "pgup" {
				delta = -1
			}
			if p.detail {
				p.offset = max(0, p.offset+delta)
				return p, nil
			}
			items := p.items()
			for i, item := range items {
				if item.Name == p.selected {
					p.selected = items[min(len(items)-1, max(0, i+delta))].Name
					return p, nil
				}
			}
		case "r":
			return p, p.run(doctorActionRefresh)
		case "h":
			if p.initializer == nil {
				p.status = "Initialization unavailable here. Run relurpish doctor --yes for this workspace."
			} else if !p.working {
				p.review = true
			}
		}
	}
	return p, nil
}

func (p *DoctorPane) items() []runtimesvc.DependencyStatus {
	if !p.hasReport {
		return nil
	}
	configDetail := p.report.ConfigRoot
	if p.report.ConfigError != "" {
		configDetail = p.report.ConfigError
	}
	items := []runtimesvc.DependencyStatus{
		{Name: "workspace configuration", Available: p.report.ConfigExists && p.report.ConfigError == "", Blocking: !p.report.ConfigExists || p.report.ConfigError != "", Details: configDetail},
		{Name: "model", Available: p.report.ModelReady, Blocking: !p.report.ModelReady, Details: strings.TrimSpace(p.report.Inference.Provider + " / " + p.report.Inference.SelectedModel + " · " + string(p.report.Inference.State) + " " + p.report.Inference.Error)},
	}
	items = append(items, p.report.Dependencies...)
	sort.SliceStable(items, func(i, j int) bool {
		rank := func(d runtimesvc.DependencyStatus) int {
			if d.Blocking {
				return 0
			}
			if d.Degraded || !d.Available {
				return 1
			}
			return 2
		}
		return rank(items[i]) < rank(items[j])
	})
	if p.filter == "" {
		return items
	}
	filtered := make([]runtimesvc.DependencyStatus, 0, len(items))
	for _, item := range items {
		if strings.Contains(strings.ToLower(item.Name+" "+item.Details), p.filter) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func (p *DoctorPane) View() string {
	if p.review {
		lines := []string{"Doctor / Review initialization", "", "Target: " + terminalText(p.report.Workspace), "", "Create missing bundled configuration and prompt files", "under relurpify_cfg, plus workspace state directories.", "Existing files are preserved. No model is downloaded."}
		if p.demo {
			lines = append(lines, "DEMO: this changes scenario memory only.")
		}
		return p.bounded(append(lines, "", "Enter confirms initialization · Escape cancels"))
	}
	if !p.hasReport {
		return p.bounded([]string{"Doctor", "", "Checking workspace readiness…", "", "Your request remains in the bar. Checks do not submit it."})
	}
	items := p.items()
	verdict := "Setup needed"
	if p.report.Ready() {
		verdict = "Ready"
	}
	if p.detail {
		for _, item := range items {
			if item.Name == p.selected {
				lines := []string{"Doctor / " + terminalText(item.Name), "", doctorFindingState(item), "", terminalText(item.Details), "", "Workspace: " + terminalText(p.report.Workspace), "Checked: " + p.report.CheckedAt.Format(time.RFC3339), "", "Escape returns to findings · ↑/↓ scroll"}
				wrapped := strings.Split(ansi.Hardwrap(strings.Join(lines, "\n"), max(1, p.width), true), "\n")
				start := min(p.offset, max(0, len(wrapped)-p.height))
				return p.bounded(wrapped[start:])
			}
		}
	}
	lines := []string{"Doctor / " + verdict, ""}
	if p.report.NeedsInitialization() {
		lines = append(lines, "Workspace configuration is missing. h reviews initialization.", "")
	}
	selected := 0
	for i, item := range items {
		if item.Name == p.selected {
			selected = i
		}
	}
	budget := max(1, p.height-len(lines)-4)
	start := max(0, selected-budget+1)
	for _, item := range items[start:min(len(items), start+budget)] {
		marker := " "
		if item.Name == p.selected {
			marker = ">"
		}
		lines = append(lines, fmt.Sprintf("%s %-28s %s", marker, terminalText(item.Name), doctorFindingState(item)))
	}
	lines = append(lines, "")
	if p.working {
		lines = append(lines, "Waiting for the observed result…")
	} else {
		lines = append(lines, "↑/↓ select · Enter detail · r recheck · h initialize")
	}
	if p.status != "" {
		lines = append(lines, terminalText(p.status))
	}
	return p.bounded(lines)
}

func doctorFindingState(item runtimesvc.DependencyStatus) string {
	if item.Blocking {
		return "[!] blocked"
	}
	if item.Degraded {
		return "[~] degraded"
	}
	if item.Available {
		return "[+] checked"
	}
	return "[?] unavailable"
}

func (p *DoctorPane) bounded(lines []string) string {
	return boundedScreen(strings.Join(lines, "\n"), max(1, p.width), max(1, p.height))
}

func (p *DoctorPane) run(action doctorAction) tea.Cmd {
	if p.working || p.reader == nil {
		p.status = "Doctor service unavailable"
		return nil
	}
	if action == doctorActionHeal && p.initializer == nil {
		p.status = "Initialization unavailable"
		return nil
	}
	p.working, p.request = true, p.request+1
	request, parent, reader, initializer := p.request, p.parent, p.reader, p.initializer
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 15*time.Second)
		defer cancel()
		if action == doctorActionHeal {
			if err := initializer.InitializeWorkspaceFromTemplates(false); err != nil {
				return doctorStatusMsg{Request: request, Action: action, Err: err}
			}
		}
		return doctorStatusMsg{Request: request, Action: action, Report: reader.BuildDoctorReport(ctx), Message: "Checks complete. Submit your request when execution is available."}
	}
}
