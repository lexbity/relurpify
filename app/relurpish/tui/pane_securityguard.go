package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/cfgload"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/manifest"
	"codeburg.org/lexbit/relurpify/framework/sandbox"
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
	guardRules []core.PolicyRule

	activePanel securityPanel
	shellSel    int
	guardSel    int

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
}

func NewSecurityGuardPane(rt securityGuardRuntime) *SecurityGuardPane {
	p := &SecurityGuardPane{runtime: rt}
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
	p.loadGuardRules()
}

func (p *SecurityGuardPane) Update(msg tea.Msg) (*SecurityGuardPane, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
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
					p.testBuffer = p.testBuffer[:len(p.testBuffer)-1]
				}
				return p, nil
			}
			if len(msg.Runes) > 0 {
				p.testBuffer += string(msg.Runes)
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
					p.editBuffer = p.editBuffer[:len(p.editBuffer)-1]
				}
				return p, nil
			}
			if len(msg.Runes) > 0 {
				p.editBuffer += string(msg.Runes)
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

func (p *SecurityGuardPane) View() string {
	shell := p.renderShellPanel()
	ingest := p.renderIngestionPanel()
	footer := dimStyle.Render("tab switch panel  arrows navigate  e edit  n new  d delete  t test  space toggle")
	if p.testMode {
		footer = warningText(fmt.Sprintf("Test %s: %s", p.testLabel, p.testBuffer)) + "\n" + footer
	}
	if p.editing {
		footer = warningText(fmt.Sprintf("Edit %s: %s", p.editLabel, p.editBuffer)) + "\n" + footer
	}
	if p.confirmDrop {
		footer = warningText("Delete selected rule? press y to confirm or n to cancel") + "\n" + footer
	}
	if p.status != "" {
		footer = dimStyle.Render(p.status) + "\n" + footer
	}
	return strings.Join([]string{shell, ingest, footer}, "\n\n")
}

func (p *SecurityGuardPane) loadShellRules() {
	workspace := ""
	if p.runtime != nil {
		workspace = p.runtime.SessionInfo().Workspace
	}
	path := filepath.Join(manifest.New(workspace).ConfigRoot(), "security", "shell.policy.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			p.shellRules = nil
			return
		}
		p.status = fmt.Sprintf("shell blacklist load failed: %v", err)
		p.shellRules = nil
		return
	}
	if err := cfgload.RejectForbiddenSecretFields(path, data); err != nil {
		p.status = fmt.Sprintf("shell blacklist secret field rejected: %v", err)
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

func (p *SecurityGuardPane) loadGuardRules() {
	workspace := ""
	if p.runtime != nil {
		workspace = p.runtime.SessionInfo().Workspace
	}
	path := filepath.Join(manifest.New(workspace).ConfigRoot(), "policy_rules.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			p.guardRules = nil
			return
		}
		p.status = fmt.Sprintf("policy rules load failed: %v", err)
		p.guardRules = nil
		return
	}
	var rules []core.PolicyRule
	if err := json.Unmarshal(data, &rules); err != nil {
		p.status = fmt.Sprintf("policy rules parse failed: %v", err)
		p.guardRules = nil
		return
	}
	p.guardRules = append([]core.PolicyRule(nil), rules...)
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
	case securityPanelIngestion:
		rule := p.selectedGuardRule()
		if rule == nil {
			return
		}
		p.editLabel = "rule name"
		p.editBuffer = rule.Name
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
	case securityPanelIngestion:
		if rule := p.selectedGuardRule(); rule != nil {
			if value != "" {
				rule.Name = value
			}
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
	case securityPanelIngestion:
		p.guardRules = append(p.guardRules, core.PolicyRule{
			ID:       fmt.Sprintf("policy-%d", time.Now().UnixNano()),
			Name:     "new guardrail",
			Priority: len(p.guardRules) + 100,
			Enabled:  true,
			Effect: core.PolicyEffect{
				Action: "deny",
				Reason: "created from SecurityGuard",
			},
		})
		p.guardSel = len(p.guardRules) - 1
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
	case securityPanelIngestion:
		if len(p.guardRules) == 0 {
			return nil
		}
		idx := clampIndex(p.guardSel, len(p.guardRules))
		p.guardRules = append(p.guardRules[:idx], p.guardRules[idx+1:]...)
		p.guardSel = clampIndex(p.guardSel, len(p.guardRules))
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
	case securityPanelIngestion:
		if len(p.guardRules) == 0 {
			p.guardSel = 0
			return
		}
		p.guardSel = clampIndex(p.guardSel+delta, len(p.guardRules))
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
	if rule := p.selectedGuardRule(); rule != nil {
		rule.Enabled = !rule.Enabled
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
	case securityPanelIngestion:
		rule := p.selectedGuardRule()
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
		re, err := regexp.Compile(rule.Raw)
		if err != nil {
			p.testResult = fmt.Sprintf("invalid regex: %v", err)
		} else if re.MatchString(sample) {
			p.testResult = fmt.Sprintf("%q matches %s", sample, rule.ID)
		} else {
			p.testResult = fmt.Sprintf("%q does not match %s", sample, rule.ID)
		}
	case securityPanelIngestion:
		rule := p.selectedGuardRule()
		if rule == nil {
			return
		}
		matched := false
		for _, value := range append(append([]string{}, rule.Conditions.Capabilities...), rule.Conditions.ExportNames...) {
			if strings.TrimSpace(value) != "" && strings.Contains(strings.ToLower(sample), strings.ToLower(value)) {
				matched = true
				break
			}
		}
		if matched {
			p.testResult = fmt.Sprintf("%q hits %s", sample, rule.ID)
		} else {
			p.testResult = fmt.Sprintf("%q does not hit %s", sample, rule.ID)
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
	guardRules := append([]core.PolicyRule(nil), p.guardRules...)
	return func() tea.Msg {
		if workspace == "" {
			return sandboxPersistedMsg{Err: fmt.Errorf("workspace unavailable")}
		}
		paths := manifest.New(workspace)
		shellPath := filepath.Join(paths.ConfigRoot(), "security", "shell.policy.yaml")
		guardPath := filepath.Join(paths.ConfigRoot(), "policy_rules.yaml")
		shellBackup, err := cfgload.CreateTimestampedBackup(shellPath)
		if err != nil {
			return sandboxPersistedMsg{Err: err}
		}
		guardBackup, err := cfgload.CreateTimestampedBackup(guardPath)
		if err != nil {
			return sandboxPersistedMsg{Err: err}
		}
		if err := cfgload.SaveYAML(shellPath, securityGuardFile{Version: "1", Rules: shellRules}); err != nil {
			return sandboxPersistedMsg{Err: err}
		}
		if err := cfgload.SaveJSON(guardPath, guardRules); err != nil {
			return sandboxPersistedMsg{Err: err}
		}
		if p.runtime != nil {
			if reloader, ok := p.runtime.(interface {
				ReloadWorkspace(context.Context, string) error
			}); ok && reloader != nil {
				if err := reloader.ReloadWorkspace(context.Background(), workspace); err != nil {
					return chatSystemMsg{Text: fmt.Sprintf("security save failed after reload: %v", err)}
				}
			}
		}
		backup := shellBackup
		if backup == "" {
			backup = guardBackup
		}
		return chatSystemMsg{Text: fmt.Sprintf("security rules saved (backup: %s)", backup)}
	}
}

func (p *SecurityGuardPane) selectedShellRule() *sandbox.BlacklistRule {
	if len(p.shellRules) == 0 {
		return nil
	}
	idx := clampIndex(p.shellSel, len(p.shellRules))
	return &p.shellRules[idx]
}

func (p *SecurityGuardPane) selectedGuardRule() *core.PolicyRule {
	if len(p.guardRules) == 0 {
		return nil
	}
	idx := clampIndex(p.guardSel, len(p.guardRules))
	return &p.guardRules[idx]
}

func (p *SecurityGuardPane) renderShellPanel() string {
	lines := []string{sectionHeaderStyle.Render("Shell Command Blacklist")}
	if len(p.shellRules) == 0 {
		lines = append(lines, dimStyle.Render("(no blacklist rules)"))
	} else {
		for i, rule := range p.shellRules {
			line := fmt.Sprintf("%s  %s", strings.ToUpper(string(rule.Action)), rule.Raw)
			if i == p.shellSel && p.activePanel == securityPanelShell {
				line = panelItemActiveStyle.Render(line)
			}
			lines = append(lines, line)
			if rule.Reason != "" {
				lines = append(lines, dimStyle.Render("  "+rule.Reason))
			}
		}
	}
	if p.activePanel == securityPanelShell && p.testResult != "" {
		lines = append(lines, "", dimStyle.Render("test: "+p.testResult))
	}
	return sectionPanel("SecurityGuard", p.width, strings.Join(lines, "\n"))
}

func (p *SecurityGuardPane) renderIngestionPanel() string {
	lines := []string{sectionHeaderStyle.Render("Ingestion Guardrails")}
	if len(p.guardRules) == 0 {
		lines = append(lines, dimStyle.Render("(no guardrails configured)"))
	} else {
		for i, rule := range p.guardRules {
			prefix := "off"
			if rule.Enabled {
				prefix = "on"
			}
			line := fmt.Sprintf("%s  %s", prefix, rule.Name)
			if i == p.guardSel && p.activePanel == securityPanelIngestion {
				line = panelItemActiveStyle.Render(line)
			}
			lines = append(lines, line)
			if rule.Effect.Reason != "" {
				lines = append(lines, dimStyle.Render("  "+rule.Effect.Reason))
			}
		}
	}
	if p.activePanel == securityPanelIngestion && p.testResult != "" {
		lines = append(lines, "", dimStyle.Render("test: "+p.testResult))
	}
	return sectionPanel("Ingestion Guardrails", p.width, strings.Join(lines, "\n"))
}
