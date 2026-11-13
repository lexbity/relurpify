package demo

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"codeburg.org/lexbit/relurpify/app/relurpish/tui"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

// ProviderSettings supplies a detached scenario configuration.
// DEMO ONLY: saved and active facts are in memory; no workspace is read.
// LIVE REPLACEMENT: P05 settings/generation service; no automatic live fallback.
func (s *Session) ProviderSettings(ctx context.Context) (tui.ProviderEditorState, error) {
	if err := ctx.Err(); err != nil {
		return tui.ProviderEditorState{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneProvider(s.provider), nil
}

// SaveProviderSettings conditionally updates only saved scenario preferences.
// DEMO ONLY: CAS and revisions exercise the real frontend save review. Saving
// never changes the scenario's active selection or invokes a provider.
// LIVE REPLACEMENT: recoverable P05 settings save; memory state is removable.
func (s *Session) SaveProviderSettings(ctx context.Context, request tui.ProviderSaveRequest) (tui.ProviderEditorState, error) {
	if err := ctx.Err(); err != nil {
		return tui.ProviderEditorState{}, err
	}
	if err := tui.ValidateProviderDraft(request.Profile); err != nil {
		return tui.ProviderEditorState{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if request.Workspace != demoWorkspace || request.ExpectedRevision != s.provider.Revision {
		return tui.ProviderEditorState{}, fmt.Errorf("DEMO provider save conflict; the stored revision was retained")
	}
	s.providerVersion++
	s.provider.Saved = request.Profile
	s.provider.Revision = fmt.Sprintf("demo-provider-%d", s.providerVersion)
	s.provider.Notice = "DEMO: saved preferences changed; active scenario did not change"
	return cloneProvider(s.provider), nil
}

// ProbeProviderSettings is a deterministic inspection fixture, never a network
// request or a model call. "unavailable" in the endpoint selects a failed probe.
// LIVE REPLACEMENT: explicit bounded provider health/model-list operations only.
func (s *Session) ProbeProviderSettings(ctx context.Context, profile config.RuntimeProviderConfig) (tui.ProviderProbeResult, error) {
	if err := ctx.Err(); err != nil {
		return tui.ProviderProbeResult{}, err
	}
	if err := tui.ValidateProviderDraft(profile); err != nil {
		return tui.ProviderProbeResult{}, err
	}
	if strings.Contains(profile.Endpoint, "unavailable") {
		return tui.ProviderProbeResult{}, fmt.Errorf("DEMO: simulated endpoint is unavailable")
	}
	return tui.ProviderProbeResult{State: "ready", Detail: "DEMO: simulated health and model listing; no provider contacted", Models: []string{"demo-small", "demo-large", "demo-coder"}}, nil
}

// ActivateProviderSettings changes the scenario's active selection only after
// exact-revision review and quiescence. provider-failure retains last good state.
// DEMO ONLY: no backend, service, generation, or transport is created or closed.
// LIVE REPLACEMENT: P05 generation activation with real resource ownership.
func (s *Session) ActivateProviderSettings(ctx context.Context, revision string) (tui.ProviderEditorState, error) {
	if err := ctx.Err(); err != nil {
		return tui.ProviderEditorState{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if revision != s.provider.Revision {
		return tui.ProviderEditorState{}, fmt.Errorf("DEMO: activation revision changed; review again")
	}
	if s.state.Running {
		return tui.ProviderEditorState{}, fmt.Errorf("DEMO: execution is active; stop it before activation")
	}
	if s.scenario == "provider-failure" {
		return tui.ProviderEditorState{}, fmt.Errorf("DEMO: staged composition failed; last good active scenario retained")
	}
	s.provider.ActiveRevision = s.provider.Revision
	s.provider.ActiveSummary = s.provider.Saved.Provider + " / " + s.provider.Saved.Model
	s.provider.Notice = "DEMO: saved preferences activated in scenario memory"
	return cloneProvider(s.provider), nil
}

func cloneProvider(state tui.ProviderEditorState) tui.ProviderEditorState {
	state.Catalog = slices.Clone(state.Catalog)
	return state
}
