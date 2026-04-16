package restore

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	euclostate "github.com/lexcodex/relurpify/named/euclo/runtime/state"
)

type ProviderRestoreOutcome struct {
	ProviderID  string `json:"provider_id,omitempty"`
	SessionID   string `json:"session_id,omitempty"`
	Kind        string `json:"kind,omitempty"`
	Status      string `json:"status,omitempty"`
	SnapshotRef string `json:"snapshot_ref,omitempty"`
	Reason      string `json:"reason,omitempty"`
	Error       string `json:"error,omitempty"`
}

type ProviderRestoreState struct {
	WorkflowID           string                   `json:"workflow_id,omitempty"`
	RunID                string                   `json:"run_id,omitempty"`
	ProviderSnapshotRefs []string                 `json:"provider_snapshot_refs,omitempty"`
	SessionSnapshotRefs  []string                 `json:"session_snapshot_refs,omitempty"`
	RestoredProviders    []string                 `json:"restored_providers,omitempty"`
	RestoredSessions     []string                 `json:"restored_sessions,omitempty"`
	FailedProviders      []string                 `json:"failed_providers,omitempty"`
	FailedSessions       []string                 `json:"failed_sessions,omitempty"`
	SkippedProviders     []string                 `json:"skipped_providers,omitempty"`
	SkippedSessions      []string                 `json:"skipped_sessions,omitempty"`
	Outcomes             []ProviderRestoreOutcome `json:"outcomes,omitempty"`
	Restored             bool                     `json:"restored"`
	Partial              bool                     `json:"partial"`
	MateriallyRequired   bool                     `json:"materially_required"`
	RestoreSource        string                   `json:"restore_source,omitempty"`
	LastRestoreError     string                   `json:"last_restore_error,omitempty"`
	UpdatedAt            time.Time                `json:"updated_at,omitempty"`
}

func CaptureProviderRuntimeState(ctx context.Context, providers []core.Provider, state *core.Context) ProviderRestoreState {
	if state == nil || len(providers) == 0 {
		return ProviderRestoreState{}
	}
	restoreState, _ := providerRestoreStateFromContext(state)
	restoreState.UpdatedAt = time.Now().UTC()
	var providerSnapshots []core.ProviderSnapshot
	var sessionSnapshots []core.ProviderSessionSnapshot
	for _, provider := range dedupeProviders(providers) {
		if provider == nil {
			continue
		}
		desc := provider.Descriptor()
		if snapshotter, ok := provider.(core.ProviderSnapshotter); ok {
			snapshot, err := snapshotter.SnapshotProvider(ctx)
			if err != nil {
				restoreState.Outcomes = append(restoreState.Outcomes, ProviderRestoreOutcome{
					ProviderID: desc.ID,
					Kind:       "provider",
					Status:     "failed",
					Reason:     "snapshot_provider_failed",
					Error:      err.Error(),
				})
				restoreState.FailedProviders = append(restoreState.FailedProviders, desc.ID)
				continue
			}
			if snapshot != nil {
				if snapshot.ProviderID == "" {
					snapshot.ProviderID = desc.ID
				}
				if snapshot.Descriptor.ID == "" {
					snapshot.Descriptor = desc
				}
				providerSnapshots = append(providerSnapshots, *snapshot)
				restoreState.Outcomes = append(restoreState.Outcomes, ProviderRestoreOutcome{
					ProviderID:  snapshot.ProviderID,
					Kind:        "provider",
					Status:      "captured",
					SnapshotRef: providerSnapshotRecordID(*snapshot),
				})
			}
		} else {
			restoreState.Outcomes = append(restoreState.Outcomes, ProviderRestoreOutcome{
				ProviderID: desc.ID,
				Kind:       "provider",
				Status:     "skipped",
				Reason:     "provider_snapshot_unsupported",
			})
			restoreState.SkippedProviders = append(restoreState.SkippedProviders, desc.ID)
		}
		if snapshotter, ok := provider.(core.ProviderSessionSnapshotter); ok {
			snapshots, err := snapshotter.SnapshotSessions(ctx)
			if err != nil {
				restoreState.Outcomes = append(restoreState.Outcomes, ProviderRestoreOutcome{
					ProviderID: desc.ID,
					Kind:       "session",
					Status:     "failed",
					Reason:     "snapshot_sessions_failed",
					Error:      err.Error(),
				})
				restoreState.FailedProviders = append(restoreState.FailedProviders, desc.ID)
				continue
			}
			for _, snapshot := range snapshots {
				if snapshot.Session.ProviderID == "" {
					snapshot.Session.ProviderID = desc.ID
				}
				sessionSnapshots = append(sessionSnapshots, snapshot)
				restoreState.Outcomes = append(restoreState.Outcomes, ProviderRestoreOutcome{
					ProviderID:  snapshot.Session.ProviderID,
					SessionID:   snapshot.Session.ID,
					Kind:        "session",
					Status:      "captured",
					SnapshotRef: fmt.Sprintf("%s:%s", snapshot.Session.ProviderID, snapshot.Session.ID),
				})
			}
		}
	}
	if len(providerSnapshots) > 0 {
		euclostate.SetProviderSnapshots(state, providerSnapshots)
	}
	if len(sessionSnapshots) > 0 {
		euclostate.SetProviderSessionSnapshots(state, sessionSnapshots)
	}
	restoreState.Restored = restoreState.Restored || len(providerSnapshots) > 0 || len(sessionSnapshots) > 0
	restoreState.Partial = restoreState.Partial || len(restoreState.FailedProviders) > 0 || len(restoreState.FailedSessions) > 0
	if restoreState.Restored || restoreState.Partial || len(restoreState.Outcomes) > 0 {
		euclostate.SetProviderRestore(state, restoreState)
	}
	return restoreState
}

func PersistProviderSnapshotState(ctx context.Context, store memory.WorkflowStateStore, workflowID, runID string, state *core.Context, taskID string) (ProviderRestoreState, error) {
	if store == nil || state == nil || workflowID == "" || runID == "" {
		return ProviderRestoreState{}, nil
	}
	restoreState := ProviderRestoreState{
		WorkflowID: workflowID,
		RunID:      runID,
		UpdatedAt:  time.Now().UTC(),
	}
	if current, ok := providerRestoreStateFromContext(state); ok {
		restoreState.RestoredProviders = append([]string(nil), current.RestoredProviders...)
		restoreState.RestoredSessions = append([]string(nil), current.RestoredSessions...)
		restoreState.FailedProviders = append([]string(nil), current.FailedProviders...)
		restoreState.FailedSessions = append([]string(nil), current.FailedSessions...)
		restoreState.SkippedProviders = append([]string(nil), current.SkippedProviders...)
		restoreState.SkippedSessions = append([]string(nil), current.SkippedSessions...)
		restoreState.Outcomes = append([]ProviderRestoreOutcome(nil), current.Outcomes...)
		restoreState.Restored = current.Restored
		restoreState.Partial = current.Partial
		restoreState.MateriallyRequired = current.MateriallyRequired
		restoreState.RestoreSource = current.RestoreSource
		restoreState.LastRestoreError = current.LastRestoreError
	}
	providerRecords := providerSnapshotRecordsFromState(state, workflowID, runID, taskID)
	sessionRecords := providerSessionSnapshotRecordsFromState(state, workflowID, runID)
	if err := store.ReplaceProviderSnapshots(ctx, workflowID, runID, providerRecords); err != nil {
		restoreState.LastRestoreError = err.Error()
		return restoreState, err
	}
	if err := store.ReplaceProviderSessionSnapshots(ctx, workflowID, runID, sessionRecords); err != nil {
		restoreState.LastRestoreError = err.Error()
		return restoreState, err
	}
	restoreState.ProviderSnapshotRefs = providerSnapshotRecordRefs(providerRecords)
	restoreState.SessionSnapshotRefs = providerSessionRecordRefs(sessionRecords)
	euclostate.SetProviderRestore(state, restoreState)
	return restoreState, nil
}

func RestoreProviderSnapshotState(ctx context.Context, store memory.WorkflowStateStore, workflowID, runID string, state *core.Context) (ProviderRestoreState, error) {
	if store == nil || state == nil || workflowID == "" || runID == "" {
		return ProviderRestoreState{}, nil
	}
	restoreState := ProviderRestoreState{
		WorkflowID:    workflowID,
		RunID:         runID,
		RestoreSource: "workflow_store",
		UpdatedAt:     time.Now().UTC(),
	}
	providers, err := store.ListProviderSnapshots(ctx, workflowID, runID)
	if err != nil {
		restoreState.LastRestoreError = err.Error()
		euclostate.SetProviderRestore(state, restoreState)
		return restoreState, err
	}
	sessions, err := store.ListProviderSessionSnapshots(ctx, workflowID, runID)
	if err != nil {
		restoreState.LastRestoreError = err.Error()
		euclostate.SetProviderRestore(state, restoreState)
		return restoreState, err
	}
	euclostate.SetProviderSnapshots(state, providerSnapshotsFromRecords(providers))
	euclostate.SetProviderSessionSnapshots(state, providerSessionSnapshotsFromRecords(sessions))
	restoreState.ProviderSnapshotRefs = providerSnapshotRecordRefs(providers)
	restoreState.SessionSnapshotRefs = providerSessionRecordRefs(sessions)
	restoreState.Restored = len(providers) > 0 || len(sessions) > 0
	euclostate.SetProviderRestore(state, restoreState)
	return restoreState, nil
}

func ApplyProviderRuntimeRestore(ctx context.Context, providers []core.Provider, state *core.Context) (ProviderRestoreState, error) {
	restoreState, _ := providerRestoreStateFromContext(state)
	if state == nil {
		return restoreState, nil
	}
	providerSnapshots, _ := providerSnapshotsFromState(state)
	sessionSnapshots, _ := providerSessionSnapshotsFromState(state)
	if len(providerSnapshots) == 0 && len(sessionSnapshots) == 0 {
		euclostate.SetProviderRestore(state, restoreState)
		return restoreState, nil
	}
	restoreState.UpdatedAt = time.Now().UTC()
	providerByID := make(map[string]core.Provider, len(providers))
	for _, provider := range dedupeProviders(providers) {
		if provider == nil {
			continue
		}
		desc := provider.Descriptor()
		if strings.TrimSpace(desc.ID) != "" {
			providerByID[desc.ID] = provider
		}
	}
	var requiredFailures []string
	for _, snapshot := range providerSnapshots {
		required := providerSnapshotRestoreRequired(snapshot)
		provider := providerByID[strings.TrimSpace(snapshot.ProviderID)]
		outcome := ProviderRestoreOutcome{
			ProviderID:  strings.TrimSpace(snapshot.ProviderID),
			Kind:        "provider",
			SnapshotRef: providerSnapshotRecordID(snapshot),
		}
		switch {
		case provider == nil:
			outcome.Status = "skipped"
			outcome.Reason = "provider_unavailable"
			restoreState.SkippedProviders = append(restoreState.SkippedProviders, outcome.ProviderID)
			if required {
				outcome.Status = "failed"
				outcome.Error = "persisted restore required but provider unavailable"
				restoreState.FailedProviders = append(restoreState.FailedProviders, outcome.ProviderID)
				requiredFailures = append(requiredFailures, outcome.Error)
			}
		case !supportsProviderRestore(provider):
			outcome.Status = "skipped"
			outcome.Reason = "provider_restore_unsupported"
			restoreState.SkippedProviders = append(restoreState.SkippedProviders, outcome.ProviderID)
			if required {
				outcome.Status = "failed"
				outcome.Error = "persisted restore required but provider restore unsupported"
				restoreState.FailedProviders = append(restoreState.FailedProviders, outcome.ProviderID)
				requiredFailures = append(requiredFailures, outcome.Error)
			}
		default:
			if err := provider.(core.ProviderRestorer).RestoreProvider(ctx, snapshot); err != nil {
				outcome.Status = "failed"
				outcome.Reason = "provider_restore_failed"
				outcome.Error = err.Error()
				restoreState.FailedProviders = append(restoreState.FailedProviders, outcome.ProviderID)
				if required {
					requiredFailures = append(requiredFailures, err.Error())
				}
			} else {
				outcome.Status = "restored"
				restoreState.RestoredProviders = append(restoreState.RestoredProviders, outcome.ProviderID)
			}
		}
		restoreState.Outcomes = append(restoreState.Outcomes, outcome)
		if required {
			restoreState.MateriallyRequired = true
		}
	}
	for _, snapshot := range sessionSnapshots {
		required := sessionSnapshotRestoreRequired(snapshot)
		provider := providerByID[strings.TrimSpace(snapshot.Session.ProviderID)]
		outcome := ProviderRestoreOutcome{
			ProviderID:  strings.TrimSpace(snapshot.Session.ProviderID),
			SessionID:   strings.TrimSpace(snapshot.Session.ID),
			Kind:        "session",
			SnapshotRef: fmt.Sprintf("%s:%s", snapshot.Session.ProviderID, snapshot.Session.ID),
		}
		switch {
		case provider == nil:
			outcome.Status = "skipped"
			outcome.Reason = "provider_unavailable"
			restoreState.SkippedSessions = append(restoreState.SkippedSessions, outcome.SessionID)
			if required {
				outcome.Status = "failed"
				outcome.Error = "persisted session restore required but provider unavailable"
				restoreState.FailedSessions = append(restoreState.FailedSessions, outcome.SessionID)
				requiredFailures = append(requiredFailures, outcome.Error)
			}
		case !supportsSessionRestore(provider):
			outcome.Status = "skipped"
			outcome.Reason = "session_restore_unsupported"
			restoreState.SkippedSessions = append(restoreState.SkippedSessions, outcome.SessionID)
			if required {
				outcome.Status = "failed"
				outcome.Error = "persisted session restore required but provider session restore unsupported"
				restoreState.FailedSessions = append(restoreState.FailedSessions, outcome.SessionID)
				requiredFailures = append(requiredFailures, outcome.Error)
			}
		default:
			if err := provider.(core.ProviderSessionRestorer).RestoreSession(ctx, snapshot); err != nil {
				outcome.Status = "failed"
				outcome.Reason = "session_restore_failed"
				outcome.Error = err.Error()
				restoreState.FailedSessions = append(restoreState.FailedSessions, outcome.SessionID)
				if required {
					requiredFailures = append(requiredFailures, err.Error())
				}
			} else {
				outcome.Status = "restored"
				restoreState.RestoredSessions = append(restoreState.RestoredSessions, outcome.SessionID)
			}
		}
		restoreState.Outcomes = append(restoreState.Outcomes, outcome)
		if required {
			restoreState.MateriallyRequired = true
		}
	}
	restoreState.Restored = len(restoreState.RestoredProviders) > 0 || len(restoreState.RestoredSessions) > 0
	restoreState.Partial = (len(restoreState.FailedProviders) > 0 || len(restoreState.FailedSessions) > 0) && restoreState.Restored
	if len(requiredFailures) > 0 {
		restoreState.LastRestoreError = strings.Join(requiredFailures, "; ")
	}
	euclostate.SetProviderRestore(state, restoreState)
	if len(requiredFailures) > 0 {
		return restoreState, fmt.Errorf("%s", restoreState.LastRestoreError)
	}
	return restoreState, nil
}

func providerRestoreStateFromContext(state *core.Context) (ProviderRestoreState, bool) {
	if state == nil {
		return ProviderRestoreState{}, false
	}
	raw, ok := euclostate.GetProviderRestore(state)
	if !ok {
		return ProviderRestoreState{}, false
	}
	typed, ok := raw.(ProviderRestoreState)
	return typed, ok
}

func ProviderRestoreStateFromContext(state *core.Context) (ProviderRestoreState, bool) {
	return providerRestoreStateFromContext(state)
}

func providerSnapshotsFromState(state *core.Context) ([]core.ProviderSnapshot, bool) {
	if state == nil {
		return nil, false
	}
	return euclostate.GetProviderSnapshots(state)
}

func providerSessionSnapshotsFromState(state *core.Context) ([]core.ProviderSessionSnapshot, bool) {
	if state == nil {
		return nil, false
	}
	return euclostate.GetProviderSessionSnapshots(state)
}

func supportsProviderRestore(provider core.Provider) bool {
	_, ok := provider.(core.ProviderRestorer)
	return ok
}

func supportsSessionRestore(provider core.Provider) bool {
	_, ok := provider.(core.ProviderSessionRestorer)
	return ok
}

func providerSnapshotRestoreRequired(snapshot core.ProviderSnapshot) bool {
	mode := snapshot.Recoverability
	if mode == "" {
		mode = snapshot.Descriptor.RecoverabilityMode
	}
	return mode == core.RecoverabilityPersistedRestore
}

func sessionSnapshotRestoreRequired(snapshot core.ProviderSessionSnapshot) bool {
	return snapshot.Session.Recoverability == core.RecoverabilityPersistedRestore
}

func dedupeProviders(providers []core.Provider) []core.Provider {
	if len(providers) == 0 {
		return nil
	}
	out := make([]core.Provider, 0, len(providers))
	seen := map[string]struct{}{}
	for _, provider := range providers {
		if provider == nil {
			continue
		}
		desc := provider.Descriptor()
		key := strings.TrimSpace(desc.ID)
		if key == "" {
			key = fmt.Sprintf("%T", provider)
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, provider)
	}
	return out
}

func providerSnapshotRecordsFromState(state *core.Context, workflowID, runID, taskID string) []memory.WorkflowProviderSnapshotRecord {
	if state == nil {
		return nil
	}
	raw, ok := euclostate.GetProviderSnapshots(state)
	if !ok || raw == nil {
		return nil
	}
	out := make([]memory.WorkflowProviderSnapshotRecord, 0, len(raw))
	for _, snapshot := range raw {
		out = append(out, memory.WorkflowProviderSnapshotRecord{
			SnapshotID:     providerSnapshotRecordID(snapshot),
			WorkflowID:     workflowID,
			RunID:          runID,
			ProviderID:     snapshot.ProviderID,
			Recoverability: snapshot.Recoverability,
			Descriptor:     snapshot.Descriptor,
			Health:         snapshot.Health,
			CapabilityIDs:  append([]string(nil), snapshot.CapabilityIDs...),
			TaskID:         firstNonEmpty(snapshot.TaskID, taskID),
			Metadata:       cloneAnyMap(snapshot.Metadata),
			State:          snapshot.State,
			CapturedAt:     parseProviderSnapshotTime(snapshot.CapturedAt),
		})
	}
	return out
}

func providerSessionSnapshotRecordsFromState(state *core.Context, workflowID, runID string) []memory.WorkflowProviderSessionSnapshotRecord {
	if state == nil {
		return nil
	}
	raw, ok := euclostate.GetProviderSessionSnapshots(state)
	if !ok || raw == nil {
		return nil
	}
	out := make([]memory.WorkflowProviderSessionSnapshotRecord, 0, len(raw))
	for _, snapshot := range raw {
		out = append(out, memory.WorkflowProviderSessionSnapshotRecord{
			SnapshotID: fmt.Sprintf("%s:%s", snapshot.Session.ProviderID, snapshot.Session.ID),
			WorkflowID: workflowID,
			RunID:      runID,
			Session:    snapshot.Session,
			Metadata:   cloneAnyMap(snapshot.Metadata),
			State:      snapshot.State,
			CapturedAt: parseProviderSnapshotTime(snapshot.CapturedAt),
		})
	}
	return out
}

func providerSnapshotsFromRecords(records []memory.WorkflowProviderSnapshotRecord) []core.ProviderSnapshot {
	out := make([]core.ProviderSnapshot, 0, len(records))
	for _, record := range records {
		out = append(out, core.ProviderSnapshot{
			ProviderID:     record.ProviderID,
			Recoverability: record.Recoverability,
			Descriptor:     record.Descriptor,
			Health:         record.Health,
			CapabilityIDs:  append([]string(nil), record.CapabilityIDs...),
			WorkflowID:     record.WorkflowID,
			TaskID:         record.TaskID,
			Metadata:       cloneAnyMap(record.Metadata),
			State:          record.State,
			CapturedAt:     record.CapturedAt.Format(time.RFC3339Nano),
		})
	}
	return out
}

func providerSessionSnapshotsFromRecords(records []memory.WorkflowProviderSessionSnapshotRecord) []core.ProviderSessionSnapshot {
	out := make([]core.ProviderSessionSnapshot, 0, len(records))
	for _, record := range records {
		out = append(out, core.ProviderSessionSnapshot{
			Session:    record.Session,
			Metadata:   cloneAnyMap(record.Metadata),
			State:      record.State,
			CapturedAt: record.CapturedAt.Format(time.RFC3339Nano),
		})
	}
	return out
}

func providerSnapshotRecordRefs(records []memory.WorkflowProviderSnapshotRecord) []string {
	refs := make([]string, 0, len(records))
	for _, record := range records {
		if record.SnapshotID != "" {
			refs = append(refs, record.SnapshotID)
		}
	}
	return refs
}

func providerSessionRecordRefs(records []memory.WorkflowProviderSessionSnapshotRecord) []string {
	refs := make([]string, 0, len(records))
	for _, record := range records {
		if record.SnapshotID != "" {
			refs = append(refs, record.SnapshotID)
		}
	}
	return refs
}

func providerSnapshotRecordID(snapshot core.ProviderSnapshot) string {
	if snapshot.ProviderID != "" {
		return snapshot.ProviderID
	}
	return "provider"
}

func parseProviderSnapshotTime(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Now().UTC()
	}
	return parsed.UTC()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func cloneAnyMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}
