package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
)

type restoreTestProvider struct {
	desc               core.ProviderDescriptor
	providerSnapshot   *core.ProviderSnapshot
	sessionSnapshots   []core.ProviderSessionSnapshot
	restoreProviderErr error
	restoreSessionErr  error
	restoredProvider   []core.ProviderSnapshot
	restoredSessions   []core.ProviderSessionSnapshot
}

func (p *restoreTestProvider) Descriptor() core.ProviderDescriptor { return p.desc }
func (p *restoreTestProvider) Initialize(context.Context, core.ProviderRuntime) error {
	return nil
}
func (p *restoreTestProvider) RegisterCapabilities(context.Context, core.CapabilityRegistrar) error {
	return nil
}
func (p *restoreTestProvider) ListSessions(context.Context) ([]core.ProviderSession, error) {
	return nil, nil
}
func (p *restoreTestProvider) HealthSnapshot(context.Context) (core.ProviderHealthSnapshot, error) {
	return core.ProviderHealthSnapshot{}, nil
}
func (p *restoreTestProvider) Close(context.Context) error { return nil }
func (p *restoreTestProvider) SnapshotProvider(context.Context) (*core.ProviderSnapshot, error) {
	if p.providerSnapshot == nil {
		return nil, nil
	}
	snapshot := *p.providerSnapshot
	return &snapshot, nil
}
func (p *restoreTestProvider) SnapshotSessions(context.Context) ([]core.ProviderSessionSnapshot, error) {
	return append([]core.ProviderSessionSnapshot(nil), p.sessionSnapshots...), nil
}
func (p *restoreTestProvider) RestoreProvider(_ context.Context, snapshot core.ProviderSnapshot) error {
	p.restoredProvider = append(p.restoredProvider, snapshot)
	return p.restoreProviderErr
}
func (p *restoreTestProvider) RestoreSession(_ context.Context, snapshot core.ProviderSessionSnapshot) error {
	p.restoredSessions = append(p.restoredSessions, snapshot)
	return p.restoreSessionErr
}

func TestCaptureProviderRuntimeStateCapturesSnapshots(t *testing.T) {
	state := core.NewContext()
	provider := &restoreTestProvider{
		desc: core.ProviderDescriptor{ID: "provider-1", Kind: core.ProviderKindBuiltin},
		providerSnapshot: &core.ProviderSnapshot{
			ProviderID: "provider-1",
			Descriptor: core.ProviderDescriptor{ID: "provider-1", Kind: core.ProviderKindBuiltin},
		},
		sessionSnapshots: []core.ProviderSessionSnapshot{{
			Session: core.ProviderSession{ID: "session-1", ProviderID: "provider-1"},
		}},
	}

	restoreState := CaptureProviderRuntimeState(context.Background(), []core.Provider{provider}, state)
	if !restoreState.Restored {
		t.Fatalf("expected restore state to record captured snapshots: %#v", restoreState)
	}
	rawProviders, ok := state.Get("euclo.provider_snapshots")
	if !ok {
		t.Fatalf("expected provider snapshots in state")
	}
	if got, ok := rawProviders.([]core.ProviderSnapshot); !ok || len(got) != 1 {
		t.Fatalf("unexpected provider snapshots: %#v", rawProviders)
	}
	rawSessions, ok := state.Get("euclo.provider_session_snapshots")
	if !ok {
		t.Fatalf("expected provider session snapshots in state")
	}
	if got, ok := rawSessions.([]core.ProviderSessionSnapshot); !ok || len(got) != 1 {
		t.Fatalf("unexpected session snapshots: %#v", rawSessions)
	}
}

func TestApplyProviderRuntimeRestoreRestoresProvidersAndSessions(t *testing.T) {
	state := core.NewContext()
	state.Set("euclo.provider_snapshots", []core.ProviderSnapshot{{
		ProviderID:     "provider-1",
		Recoverability: core.RecoverabilityPersistedRestore,
		Descriptor: core.ProviderDescriptor{
			ID:                 "provider-1",
			Kind:               core.ProviderKindBuiltin,
			RecoverabilityMode: core.RecoverabilityPersistedRestore,
		},
	}})
	state.Set("euclo.provider_session_snapshots", []core.ProviderSessionSnapshot{{
		Session: core.ProviderSession{
			ID:             "session-1",
			ProviderID:     "provider-1",
			Recoverability: core.RecoverabilityPersistedRestore,
		},
	}})

	provider := &restoreTestProvider{
		desc: core.ProviderDescriptor{
			ID:                 "provider-1",
			Kind:               core.ProviderKindBuiltin,
			RecoverabilityMode: core.RecoverabilityPersistedRestore,
		},
	}
	restoreState, err := ApplyProviderRuntimeRestore(context.Background(), []core.Provider{provider}, state)
	if err != nil {
		t.Fatalf("unexpected restore error: %v", err)
	}
	if len(provider.restoredProvider) != 1 || len(provider.restoredSessions) != 1 {
		t.Fatalf("expected provider and session restore to be called: %#v %#v", provider.restoredProvider, provider.restoredSessions)
	}
	if !restoreState.Restored || restoreState.MateriallyRequired == false {
		t.Fatalf("unexpected restore state: %#v", restoreState)
	}
}

func TestApplyProviderRuntimeRestoreFailsWhenPersistedRestoreIsRequired(t *testing.T) {
	state := core.NewContext()
	state.Set("euclo.provider_snapshots", []core.ProviderSnapshot{{
		ProviderID:     "provider-1",
		Recoverability: core.RecoverabilityPersistedRestore,
		Descriptor: core.ProviderDescriptor{
			ID:                 "provider-1",
			Kind:               core.ProviderKindBuiltin,
			RecoverabilityMode: core.RecoverabilityPersistedRestore,
		},
	}})

	provider := &restoreTestProvider{
		desc: core.ProviderDescriptor{
			ID:                 "provider-1",
			Kind:               core.ProviderKindBuiltin,
			RecoverabilityMode: core.RecoverabilityPersistedRestore,
		},
		restoreProviderErr: errors.New("boom"),
	}
	restoreState, err := ApplyProviderRuntimeRestore(context.Background(), []core.Provider{provider}, state)
	if err == nil {
		t.Fatalf("expected restore error")
	}
	if len(restoreState.FailedProviders) != 1 || restoreState.LastRestoreError == "" {
		t.Fatalf("unexpected restore state: %#v", restoreState)
	}
}
