package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"codeburg.org/lexbit/relurpify/app/relurpish/theme"
	"codeburg.org/lexbit/relurpify/platform/llm"
	"codeburg.org/lexbit/relurpify/userconfig/config"
	"codeburg.org/lexbit/relurpify/userconfig/config/model"
)

type aiProviderRuntime interface {
	SessionInfo() SessionInfo
	ReloadWorkspace(context.Context, string) error
}

type providerField int

const (
	providerFieldProvider providerField = iota
	providerFieldEndpoint
	providerFieldModel
	providerFieldTimeout
	providerFieldNativeToolCalling
)

// catalogProviderInfo holds data for one provider from the catalog YAML.
type catalogProviderInfo struct {
	Name              string
	Kind              string
	Endpoint          string
	NativeToolCalling bool
	SetupHint         string
}

type AIProviderPane struct {
	runtime aiProviderRuntime

	profile config.RuntimeProviderConfig
	models  []llm.ModelInfo
	status  string

	catalog   []catalogProviderInfo
	catalogIdx int // index into catalog for the currently selected provider

	kindFocus int
	sel       int
	fieldSel  providerField
	editing   bool
	editBuf   string
	editLabel string
	setupHint string

	width  int
	height int
	// Theme is the active semantic style source.
	th *theme.Theme
}

func NewAIProviderPane(rt aiProviderRuntime) *AIProviderPane {
	p := &AIProviderPane{th: theme.Default(), runtime: rt}
	p.Refresh()
	return p
}

func (p *AIProviderPane) SetSize(w, h int) {
	p.width = w
	p.height = h
}

func (p *AIProviderPane) SetFilter(filter string) {
	_ = filter
}

func (p *AIProviderPane) Refresh() {
	p.loadCatalog()
	p.loadProfile()
	p.syncCatalogSelection()
	p.refreshModels()
}

func (p *AIProviderPane) Update(msg tea.Msg) (*AIProviderPane, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if p.editing {
			switch msg.String() {
			case "enter":
				p.commitEdit()
				return p, nil
			case "esc":
				p.editing = false
				p.editBuf = ""
				p.editLabel = ""
				return p, nil
			case "backspace", "delete":
				if len(p.editBuf) > 0 {
					p.editBuf = p.editBuf[:len(p.editBuf)-1]
				}
				return p, nil
			}
			if len(msg.Runes) > 0 {
				p.editBuf += string(msg.Runes)
			}
			return p, nil
		}
		switch msg.String() {
		case "tab":
			p.kindFocus = (p.kindFocus + 1) % 2
			if p.kindFocus == 0 {
				p.sel = clampIndex(p.sel, max(1, len(p.models)))
			}
		case "shift+tab":
			p.kindFocus = (p.kindFocus + 1) % 2
		case "left", "h":
			if len(p.catalog) > 1 {
				p.cycleProvider(-1)
			}
		case "right", "l":
			if len(p.catalog) > 1 {
				p.cycleProvider(1)
			}
		case "up", "k":
			if p.kindFocus == 0 {
				if p.sel > 0 {
					p.sel--
				}
			} else {
				p.fieldSel = providerField(max(0, int(p.fieldSel)-1))
			}
		case "down", "j":
			if p.kindFocus == 0 {
				if p.sel < len(p.models)-1 {
					p.sel++
				}
			} else {
				maxField := int(providerFieldNativeToolCalling)
				if int(p.fieldSel) < maxField {
					p.fieldSel++
				}
			}
		case "enter":
			if p.kindFocus == 0 {
				if p.sel >= 0 && p.sel < len(p.models) {
					p.profile.Model = p.models[p.sel].Name
					p.status = fmt.Sprintf("selected model %q", p.profile.Model)
				}
				return p, nil
			}
			p.beginEdit()
		case "e":
			p.beginEdit()
		case "t":
			return p, p.testProviderCmd()
		case "s":
			return p, p.saveProviderCmd()
		}
	}
	return p, nil
}

func (p *AIProviderPane) View() string {
	left := p.renderModelList()
	right := p.renderConfigPanel()
	footer := p.th.Dim().Render("tab switch focus  arrows navigate  enter select  e edit  t test  s save")
	if p.editing {
		footer = warningText(p.th, fmt.Sprintf("Edit %s: %s", p.editLabel, p.editBuf)) + "\n" + footer
	}
	if p.status != "" {
		footer = p.th.Dim().Render(p.status) + "\n" + footer
	}
	if p.setupHint != "" {
		hint := p.th.Dim().Render("hint: " + p.setupHint)
		footer = hint + "\n" + footer
	}
	return strings.Join([]string{
		lipgloss.JoinHorizontal(lipgloss.Top, left, right),
		footer,
	}, "\n\n")
}

// loadCatalog loads provider definitions from the workspace's catalog YAMLs.
func (p *AIProviderPane) loadCatalog() {
	workspace := ""
	if p.runtime != nil {
		workspace = p.runtime.SessionInfo().Workspace
	}
	if workspace == "" {
		p.catalog = nil
		p.catalogIdx = 0
		return
	}
	providerDir := filepath.Join(config.New(workspace).ConfigRoot(), "model", "provider")
	providers, err := model.LoadProviderDir(providerDir, config.StrictDecode)
	if err != nil {
		p.catalog = nil
		p.catalogIdx = 0
		return
	}
	out := make([]catalogProviderInfo, 0, len(providers))
	for _, rp := range providers {
		out = append(out, catalogProviderInfo{
			Name:              rp.Name,
			Kind:              rp.Kind,
			Endpoint:          rp.Endpoint,
			NativeToolCalling: rp.NativeToolCalling,
			SetupHint:         rp.SetupHint,
		})
	}
	p.catalog = out
}

// syncCatalogSelection aligns catalogIdx with the current profile.Provider.
func (p *AIProviderPane) syncCatalogSelection() {
	p.catalogIdx = 0
	for i, cp := range p.catalog {
		if strings.EqualFold(cp.Name, p.profile.Provider) {
			p.catalogIdx = i
			return
		}
	}
}

func (p *AIProviderPane) loadProfile() {
	workspace := ""
	if p.runtime != nil {
		workspace = p.runtime.SessionInfo().Workspace
	}
	path := config.New(workspace).RuntimeProvidersFile()
	loaded, err := config.LoadRuntimeProviderConfig(path)
	if err != nil {
		if os.IsNotExist(err) {
			p.profile = defaultProviderProfile()
			return
		}
		p.status = fmt.Sprintf("load failed: %v", err)
		p.profile = defaultProviderProfile()
		return
	}
	if loaded.Provider == "" {
		loaded.Provider = "ollama"
	}
	p.profile = loaded
}

func (p *AIProviderPane) refreshModels() {
	cfg := p.llmConfig()
	backend, err := llm.New(cfg, llm.ProviderSecrets{})
	if err != nil {
		p.status = fmt.Sprintf("backend error: %v", err)
		p.models = nil
		return
	}
	defer func() { _ = backend.Close() }()
	models, err := backend.ListModels(context.Background())
	if err != nil {
		p.status = fmt.Sprintf("model list failed: %v", err)
		p.models = nil
		return
	}
	p.models = models
	if p.profile.Model == "" && len(models) > 0 {
		p.profile.Model = models[0].Name
	}
	p.status = fmt.Sprintf("loaded %d models", len(models))
}

// cycleProvider switches to the next or previous catalog provider.
// Always fills endpoint and native_tool_calling from the catalog definition.
func (p *AIProviderPane) cycleProvider(delta int) {
	if len(p.catalog) == 0 {
		return
	}
	p.catalogIdx = (p.catalogIdx + delta) % len(p.catalog)
	if p.catalogIdx < 0 {
		p.catalogIdx += len(p.catalog)
	}
	cp := &p.catalog[p.catalogIdx]
	p.profile.Provider = cp.Name
	p.profile.Endpoint = cp.Endpoint
	p.profile.NativeToolCalling = cp.NativeToolCalling
	p.setupHint = cp.SetupHint
	p.refreshModels()
}

func (p *AIProviderPane) beginEdit() {
	switch p.fieldSel {
	case providerFieldProvider:
		p.editLabel = "provider"
		p.editBuf = p.profile.Provider
	case providerFieldEndpoint:
		p.editLabel = "endpoint"
		p.editBuf = p.profile.Endpoint
	case providerFieldModel:
		p.editLabel = "model"
		p.editBuf = p.profile.Model
	case providerFieldTimeout:
		p.editLabel = "timeout"
		p.editBuf = p.profile.Timeout
	case providerFieldNativeToolCalling:
		p.editLabel = "native tool calling"
		p.editBuf = fmt.Sprintf("%v", p.profile.NativeToolCalling)
	}
	p.editing = true
}

func (p *AIProviderPane) commitEdit() {
	value := strings.TrimSpace(p.editBuf)
	switch p.fieldSel {
	case providerFieldProvider:
		if value == "" {
			value = "ollama"
		}
		p.profile.Provider = strings.ToLower(value)
		p.syncCatalogSelection()
	case providerFieldEndpoint:
		p.profile.Endpoint = value
	case providerFieldModel:
		p.profile.Model = value
	case providerFieldTimeout:
		if value != "" {
			if _, err := time.ParseDuration(value); err != nil {
				p.status = fmt.Sprintf("invalid timeout: %v", err)
				p.editing = false
				return
			}
		}
		p.profile.Timeout = value
	case providerFieldNativeToolCalling:
		p.profile.NativeToolCalling = strings.EqualFold(value, "true") || strings.EqualFold(value, "yes") || value == "1"
	}
	p.editing = false
	p.editBuf = ""
	p.editLabel = ""
	p.refreshModels()
}

func (p *AIProviderPane) saveProviderCmd() tea.Cmd {
	profile := p.profile
	workspace := ""
	if p.runtime != nil {
		workspace = p.runtime.SessionInfo().Workspace
	}
	return func() tea.Msg {
		if workspace == "" {
			return chatSystemMsg{Text: "provider save failed: workspace unavailable"}
		}
		path := config.New(workspace).RuntimeProvidersFile()
		profile.LastUpdated = time.Now().Unix()
		backup, err := config.SaveRuntimeProviderConfigWithBackup(path, profile)
		if err != nil {
			return chatSystemMsg{Text: fmt.Sprintf("provider save failed: %v", err)}
		}
		if p.runtime != nil {
			if reloader, ok := p.runtime.(interface {
				ReloadWorkspace(context.Context, string) error
			}); ok && reloader != nil {
				if err := reloader.ReloadWorkspace(context.Background(), workspace); err != nil {
					return chatSystemMsg{Text: fmt.Sprintf("provider save failed after reload: %v", err)}
				}
			}
		}
		return chatSystemMsg{Text: fmt.Sprintf("provider saved (backup: %s)", backup)}
	}
}

func (p *AIProviderPane) testProviderCmd() tea.Cmd {
	cfg := p.llmConfig()
	return func() tea.Msg {
		backend, err := llm.New(cfg, llm.ProviderSecrets{})
		if err != nil {
			return chatSystemMsg{Text: fmt.Sprintf("provider test failed: %v", err)}
		}
		defer func() { _ = backend.Close() }()
		start := time.Now()
		report, err := backend.Health(context.Background())
		elapsed := time.Since(start)
		if err != nil {
			return chatSystemMsg{Text: fmt.Sprintf("provider test failed after %s: %v", elapsed.Truncate(time.Millisecond), err)}
		}
		if report != nil {
			return chatSystemMsg{Text: fmt.Sprintf("provider %s ready in %s", report.State, elapsed.Truncate(time.Millisecond))}
		}
		return chatSystemMsg{Text: fmt.Sprintf("provider responded in %s", elapsed.Truncate(time.Millisecond))}
	}
}

func (p *AIProviderPane) llmConfig() llm.ProviderConfig {
	timeout := time.Duration(0)
	if strings.TrimSpace(p.profile.Timeout) != "" {
		if parsed, err := time.ParseDuration(p.profile.Timeout); err == nil {
			timeout = parsed
		}
	}
	if p.profile.Provider == "" {
		p.profile.Provider = "ollama"
	}
	return llm.ProviderConfig{
		Provider:          p.profile.Provider,
		Endpoint:          p.profile.Endpoint,
		Model:             p.profile.Model,
		Timeout:           timeout,
		NativeToolCalling: p.profile.NativeToolCalling,
	}
}

func (p *AIProviderPane) renderModelList() string {
	marker := " "
	if len(p.catalog) > 1 && p.kindFocus == 0 {
		marker = "< >"
	}
	lines := []string{
		p.th.Subhead().Render("AI Provider"),
		fmt.Sprintf("Provider: %s  %s", p.profile.Provider, marker),
		fmt.Sprintf("Endpoint: %s", p.profile.Endpoint),
		"",
		p.th.Subhead().Render("Models"),
	}
	if len(p.models) == 0 {
		lines = append(lines, p.th.Dim().Render("(no models found)"))
	} else {
		for i, model := range p.models {
			line := model.Name
			if i == p.sel && p.kindFocus == 0 {
				line = p.th.Active().Render(line)
			} else if model.Name == p.profile.Model {
				line = p.th.Active().Render(line)
			}
			lines = append(lines, line)
		}
	}
	return sectionPanel(p.th, "Models", p.width/2, strings.Join(lines, "\n"))
}

func (p *AIProviderPane) renderConfigPanel() string {
	fields := []string{
		fmt.Sprintf("provider: %s", p.profile.Provider),
		fmt.Sprintf("endpoint: %s", p.profile.Endpoint),
		fmt.Sprintf("model: %s", p.profile.Model),
		fmt.Sprintf("timeout: %s", p.profile.Timeout),
		fmt.Sprintf("native_tool_calling: %t", p.profile.NativeToolCalling),
	}
	if p.kindFocus == 1 {
		for i := range fields {
			if i == int(p.fieldSel) {
				fields[i] = p.th.Active().Render(fields[i])
			}
		}
	}
	return sectionPanel(p.th, "Configurator", max(20, p.width/2), strings.Join(fields, "\n"))
}

func defaultProviderProfile() config.RuntimeProviderConfig {
	return config.RuntimeProviderConfig{
		Provider:          "ollama",
		Endpoint:          "http://localhost:11434",
		Model:             "",
		Timeout:           "30s",
		NativeToolCalling: true,
	}
}

// SetTheme sets the active semantic style source.
func (p *AIProviderPane) SetTheme(th *theme.Theme) {
	if th != nil {
		p.th = th
	}
}
