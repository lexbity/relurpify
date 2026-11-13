package tui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/model"
	"codeburg.org/lexbit/relurpify/platform/llm"
	"codeburg.org/lexbit/relurpify/userconfig/config"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"
)

type providerFixture struct {
	state         ProviderEditorState
	saves, probes int
	saveError     error
}

func (p *providerFixture) ProviderSettings(context.Context) (ProviderEditorState, error) {
	return p.state, nil
}
func (p *providerFixture) SaveProviderSettings(_ context.Context, request ProviderSaveRequest) (ProviderEditorState, error) {
	p.saves++
	if p.saveError != nil {
		return ProviderEditorState{}, p.saveError
	}
	p.state.Saved = request.Profile
	return p.state, nil
}
func (p *providerFixture) ProbeProviderSettings(context.Context, config.RuntimeProviderConfig) (ProviderProbeResult, error) {
	p.probes++
	return ProviderProbeResult{State: "ready", Models: []string{"observed-model"}}, nil
}

func TestV2ProviderStaleProbeAndSaveFailureRetainDraft(t *testing.T) {
	port := &providerFixture{state: ProviderEditorState{Workspace: "workspace", Revision: "saved", Saved: config.RuntimeProviderConfig{Provider: "test", Model: "old"}}, saveError: errors.New("disk full")}
	p := NewProviderPane(t.Context(), port, nil, false)
	require.Zero(t, port.probes)
	require.Zero(t, port.saves)
	p.apply(p.open()().(providerResultMsg))
	probe := p.perform("probe")
	p.selected = 2
	p.stage("newer-model")
	p.apply(probe().(providerResultMsg))
	require.Nil(t, p.probe)
	require.Equal(t, "newer-model", p.draft.Model)
	require.Contains(t, p.status, "Stale")
	_, handled := p.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	require.True(t, handled)
	require.Zero(t, port.saves)
	_, handled = p.key(tea.KeyMsg{Type: tea.KeyEsc})
	require.True(t, handled)
	require.Zero(t, port.saves)
	p.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	save, handled := p.key(tea.KeyMsg{Type: tea.KeyEnter})
	require.True(t, handled)
	require.NotNil(t, save)
	p.apply(save().(providerResultMsg))
	require.Equal(t, "newer-model", p.draft.Model)
	require.Equal(t, "old", p.state.Saved.Model)
	require.Contains(t, p.status, "disk full")
	// An old pane's reply can never mutate a new pane after a workspace change.
	other := NewProviderPane(t.Context(), port, nil, false)
	other.request = p.request
	other.apply(providerResultMsg{owner: p, Request: p.request, Operation: "load", State: port.state})
	require.Empty(t, other.state.Revision)
}

func TestV2ProviderRejectsQuestionableFields(t *testing.T) {
	for _, profile := range []config.RuntimeProviderConfig{
		{Provider: "p", Model: "m", Endpoint: "https://user:password@host"},
		{Provider: "p", Model: "m", Endpoint: "https://host?token=secret"},
		{Provider: "p", Model: "m", Endpoint: "file:///tmp/x"},
		{Provider: "p", Model: "m", Timeout: "-1s"},
		{Provider: "p", Model: "m", Timeout: "11m"},
		{Provider: "p", Model: "\x1b[2J"},
		{Provider: "p", Model: strings.Repeat("x", 8193)},
		{Provider: "p"},
	} {
		require.Error(t, ValidateProviderDraft(profile))
	}
	require.NoError(t, ValidateProviderDraft(config.RuntimeProviderConfig{Provider: "p", Model: "m", Endpoint: "https://example.invalid/v1", Timeout: "2m"}))
}

func TestV2LegacyProviderNavigationNeverProbes(t *testing.T) {
	workspace := t.TempDir()
	directory := filepath.Join(workspace, "relurpify_cfg", "model", "provider")
	require.NoError(t, os.MkdirAll(directory, 0700))
	name := "probe-" + string(model.GenerateCallID())
	probes := 0
	llm.RegisterKind(name, func(llm.ProviderConfig, llm.ProviderSecrets, *model.CallGate) (llm.ManagedBackend, error) {
		probes++
		return nil, errors.New("explicit probe fixture")
	})
	writeProviderYAML(t, directory, name, name, "http://example.invalid", false, "")
	_, err := config.SaveRuntimeProviderConfigWithBackup(config.New(workspace).RuntimeProvidersFile(), config.RuntimeProviderConfig{Provider: name, Model: "saved-model"})
	require.NoError(t, err)
	p := NewAIProviderPane(&testAIProviderRuntime{workspace: workspace})
	p.cycleProvider(1)
	p.fieldSel = providerFieldModel
	p.beginEdit()
	p.editBuf = "new-model"
	p.commitEdit()
	require.Contains(t, p.status, "not probed")
	require.NotContains(t, p.status, "backend error")
	require.Empty(t, p.models)
	require.Zero(t, probes, "navigation unexpectedly constructed a backend")
	_, cmd := p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	require.NotNil(t, cmd)
	_, _ = p.Update(cmd())
	require.Equal(t, 1, probes)
	require.Contains(t, p.status, "explicit probe fixture")
}
