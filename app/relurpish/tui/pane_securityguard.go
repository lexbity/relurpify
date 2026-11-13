package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"codeburg.org/lexbit/relurpify/app/relurpish/theme"
	"codeburg.org/lexbit/relurpify/capability/sandbox"
	"codeburg.org/lexbit/relurpify/userconfig/config"

	tea "github.com/charmbracelet/bubbletea"
	"gopkg.in/yaml.v3"
)

type securityGuardRuntime interface {
	SessionInfo() SessionInfo
}

type securityPanel int

const (
	securityPanelShell securityPanel = iota
	securityPanelIngestion
)

type securityGuardFile struct {
	Version string                  `yaml:"version,omitempty" json:"version,omitempty"`
	Rules   []sandbox.BlacklistRule `yaml:"rules" json:"rules"`
}

type SecurityGuardPane struct {
	runtime securityGuardRuntime

	shellRules []sandbox.BlacklistRule

	activePanel securityPanel
	shellSel    int

	editing    bool
	editBuffer string
	editLabel  string

	testMode    bool
	testBuffer  string
	testLabel   string
	testResult  string
	confirmDrop bool

	width  int
	height int
	status string
	// Theme is the active semantic style source.
	th *theme.Theme
}

func NewSecurityGuardPane(rt securityGuardRuntime) *SecurityGuardPane {
	p := &SecurityGuardPane{th: theme.Default(), runtime: rt}
	p.Refresh()
	return p
}

func (p *SecurityGuardPane) SetSize(w, h int) {
	p.width = w
	p.height = h
}

func (p *SecurityGuardPane) SetFilter(filter string) {
	_ = filter
}

func (p *SecurityGuardPane) Refresh() {
	p.loadShellRules()
}

func (p *SecurityGuardPane) Update(msg tea.Msg) (*SecurityGuardPane, tea.Cmd) {
	if msg, ok := msg.(tea.KeyMsg); ok {
		if p.activePanel == securityPanelIngestion {
			if msg.String() == "tab" || msg.String() == "shift+tab" {
				p.activePanel = securityPanelShell
			}
			return p, nil
		}
		if p.confirmDrop {
			switch msg.String() {
			case "y", "enter":
				p.confirmDrop = false
				return p, p.dropSelectedCmd()
			case "n", "esc":
				p.confirmDrop = false
				return p, nil
			}
			return p, nil
		}
		if p.testMode {
			switch msg.String() {
			case "enter":
				p.runTest()
				return p, nil
			case "esc":
				p.testMode = false
				p.testBuffer = ""
				p.testLabel = ""
				return p, nil
			case "backspace", "delete":
				if len(p.testBuffer) > 0 {
					_, size := utf8.DecodeLastRuneInString(p.testBuffer)
					p.testBuffer = p.testBuffer[:len(p.testBuffer)-size]
				}
				return p, nil
			}
			if len(msg.Runes) > 0 {
				if len(p.testBuffer)+len(string(msg.Runes)) <= 65536 {
					p.testBuffer += string(msg.Runes)
				} else {
					p.status = "Test input exceeds 64 KiB; previous text retained"
				}
			}
			return p, nil
		}
		if p.editing {
			switch msg.String() {
			case "enter":
				p.commitEdit()
				return p, nil
			case "esc":
				p.editing = false
				p.editBuffer = ""
				p.editLabel = ""
				return p, nil
			case "backspace", "delete":
				if len(p.editBuffer) > 0 {
					_, size := utf8.DecodeLastRuneInString(p.editBuffer)
					p.editBuffer = p.editBuffer[:len(p.editBuffer)-size]
				}
				return p, nil
			}
			if len(msg.Runes) > 0 {
				if len(p.editBuffer)+len(string(msg.Runes)) <= 8192 {
					p.editBuffer += string(msg.Runes)
				} else {
					p.status = "Rule pattern exceeds 8 KiB; previous text retained"
				}
			}
			return p, nil
		}
		switch msg.String() {
		case "tab":
			p.activePanel = (p.activePanel + 1) % 2
		case "shift+tab":
			p.activePanel = (p.activePanel + 1) % 2
		case "up", "k":
			p.moveSelection(-1)
		case "down", "j":
			p.moveSelection(1)
		case "e":
			p.beginEdit()
		case "n":
			p.addNewRule()
		case "d":
			p.confirmDrop = true
			p.status = "press y to delete selected rule"
		case "t":
			p.beginTest()
		case "space":
			p.toggleSelectedState()
		}
	}
	return p, nil
}

// OwnsEscape keeps edit, deletion review and pattern testing inside their focus frame.
func (p *SecurityGuardPane) OwnsEscape() bool { return p.editing || p.confirmDrop || p.testMode }

func (p *SecurityGuardPane) View() string {
	panel := p.renderShellPanel()
	footer := p.th.Dim().Render("tab switch panel  arrows navigate  e edit  n new  d delete  t test  space toggle")
	if p.activePanel == securityPanelIngestion {
		panel = p.renderIngestionPanel()
		footer = p.th.Dim().Render("Tab returns to shell rules · ingestion mutation is unavailable")
	}
	switch {
	case p.testMode:
		footer = warningText(p.th, fmt.Sprintf("Test %s: %s", p.testLabel, p.testBuffer)) + "\n" + footer
	case p.editing:
		footer = warningText(p.th, fmt.Sprintf("Edit %s: %s", p.editLabel, p.editBuffer)) + "\n" + footer
	case p.confirmDrop:
		footer = warningText(p.th, "Delete selected rule? press y to confirm or n to cancel") + "\n" + footer
	}
	if p.status != "" {
		footer = p.th.Dim().Render(p.status) + "\n" + footer
	}
	return strings.Join([]string{panel, footer}, "\n\n")
}

func (p *SecurityGuardPane) loadShellRules() {
	workspace := ""
	if p.runtime != nil {
		workspace = p.runtime.SessionInfo().Workspace
	}
	path := filepath.Join(config.New(workspace).ConfigRoot(), "security", "shell.policy.yaml")
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		if os.IsNotExist(err) {
			p.shellRules = nil
			return
		}
		p.status = fmt.Sprintf("shell blacklist load failed: %v", err)
		p.shellRules = nil
		return
	}
	var file securityGuardFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		p.status = fmt.Sprintf("shell blacklist parse failed: %v", err)
		p.shellRules = nil
		return
	}
	p.shellRules = append([]sandbox.BlacklistRule(nil), file.Rules...)
}

func (p *SecurityGuardPane) beginEdit() {
	switch p.activePanel {
	case securityPanelShell:
		rule := p.selectedShellRule()
		if rule == nil {
			return
		}
		p.editLabel = "pattern"
		p.editBuffer = rule.Raw
		p.editing = true
	}
}

func (p *SecurityGuardPane) commitEdit() {
	value := strings.TrimSpace(p.editBuffer)
	switch p.activePanel {
	case securityPanelShell:
		if rule := p.selectedShellRule(); rule != nil {
			rule.Raw = value
			rule.Pattern = nil
		}
	}
	p.editing = false
	p.editBuffer = ""
	p.editLabel = ""
}

func (p *SecurityGuardPane) addNewRule() {
	switch p.activePanel {
	case securityPanelShell:
		p.shellRules = append(p.shellRules, sandbox.BlacklistRule{
			ID:     fmt.Sprintf("rule-%d", time.Now().UnixNano()),
			Raw:    "",
			Reason: "created from SecurityGuard",
			Action: sandbox.BlacklistActionBlock,
		})
		p.shellSel = len(p.shellRules) - 1
		p.beginEdit()
	}
}

func (p *SecurityGuardPane) dropSelectedCmd() tea.Cmd {
	switch p.activePanel {
	case securityPanelShell:
		if len(p.shellRules) == 0 {
			return nil
		}
		idx := clampIndex(p.shellSel, len(p.shellRules))
		p.shellRules = append(p.shellRules[:idx], p.shellRules[idx+1:]...)
		p.shellSel = clampIndex(p.shellSel, len(p.shellRules))
	}
	return p.saveCmd()
}

func (p *SecurityGuardPane) moveSelection(delta int) {
	switch p.activePanel {
	case securityPanelShell:
		if len(p.shellRules) == 0 {
			p.shellSel = 0
			return
		}
		p.shellSel = clampIndex(p.shellSel+delta, len(p.shellRules))
	}
}

func (p *SecurityGuardPane) toggleSelectedState() {
	if p.activePanel != securityPanelIngestion {
		if rule := p.selectedShellRule(); rule != nil {
			if rule.Action == sandbox.BlacklistActionBlock {
				rule.Action = sandbox.BlacklistActionHITL
			} else {
				rule.Action = sandbox.BlacklistActionBlock
			}
		}
		return
	}
}

func (p *SecurityGuardPane) beginTest() {
	switch p.activePanel {
	case securityPanelShell:
		rule := p.selectedShellRule()
		if rule == nil {
			return
		}
		p.testLabel = rule.ID
		p.testBuffer = ""
		p.testMode = true
	}
}

func (p *SecurityGuardPane) runTest() {
	sample := p.testBuffer
	switch p.activePanel {
	case securityPanelShell:
		rule := p.selectedShellRule()
		if rule == nil {
			return
		}
		matcher, err := sandbox.NewShellBlacklist([]sandbox.BlacklistRule{*rule})
		switch {
		case err != nil:
			p.testResult = "Invalid shell rule: " + err.Error()
		case matcher.Check(sample) != nil:
			p.testResult = "Matches selected rule: " + string(rule.Action) + " · pattern only; command not executed"
		default:
			p.testResult = "No match · pattern only; this does not establish authorization"
		}
	}
	p.status = p.testResult
	p.testMode = false
	p.testBuffer = ""
	p.testLabel = ""
}

func (p *SecurityGuardPane) saveCmd() tea.Cmd {
	workspace := ""
	if p.runtime != nil {
		workspace = p.runtime.SessionInfo().Workspace
	}
	shellRules := append([]sandbox.BlacklistRule(nil), p.shellRules...)
	return func() tea.Msg {
		if workspace == "" {
			return sandboxPersistedMsg{Err: fmt.Errorf("workspace unavailable")}
		}
		shellPath := filepath.Join(config.New(workspace).ConfigRoot(), "security", "shell.policy.yaml")
		backup, err := config.CreateTimestampedBackup(shellPath)
		if err != nil {
			return sandboxPersistedMsg{Err: err}
		}
		if err := config.SaveYAML(shellPath, securityGuardFile{Version: "1", Rules: shellRules}); err != nil {
			return sandboxPersistedMsg{Err: err}
		}
		if p.runtime != nil {
			if reloader, ok := p.runtime.(interface {
				ReloadWorkspace(context.Context, string) error
			}); ok && reloader != nil {
				if err := reloader.ReloadWorkspace(context.Background(), workspace); err != nil {
					return ChatSystemMsg{Text: fmt.Sprintf("security save failed after reload: %v", err)}
				}
			}
		}
		return ChatSystemMsg{Text: fmt.Sprintf("security rules saved (backup: %s)", backup)}
	}
}

func (p *SecurityGuardPane) selectedShellRule() *sandbox.BlacklistRule {
	if len(p.shellRules) == 0 {
		return nil
	}
	idx := clampIndex(p.shellSel, len(p.shellRules))
	return &p.shellRules[idx]
}

func (p *SecurityGuardPane) renderShellPanel() string {
	lines := []string{p.th.Subhead().Render("Shell Command Blacklist")}
	if len(p.shellRules) == 0 {
		lines = append(lines, p.th.Dim().Render("(no blacklist rules)"))
	} else {
		for i, rule := range p.shellRules {
			line := fmt.Sprintf("%s  %s", strings.ToUpper(string(rule.Action)), rule.Raw)
			if i == p.shellSel && p.activePanel == securityPanelShell {
				line = p.th.Active().Render(line)
			}
			lines = append(lines, line)
			if rule.Reason != "" {
				lines = append(lines, p.th.Dim().Render("  "+rule.Reason))
			}
		}
	}
	if p.activePanel == securityPanelShell && p.testResult != "" {
		lines = append(lines, "", p.th.Dim().Render("test: "+p.testResult))
	}
	return sectionPanel(p.th, "SecurityGuard", p.width, strings.Join(lines, "\n"))
}

func (p *SecurityGuardPane) renderIngestionPanel() string {
	return sectionPanel(p.th, "Ingestion policy", p.width,
		"Ingestion policy owner is not connected.\nInspection and editing are unavailable.\nShell pattern tests do not establish authorization.")
}

// SetTheme sets the active semantic style source.
func (p *SecurityGuardPane) SetTheme(th *theme.Theme) {
	if th != nil {
		p.th = th
	}
}
