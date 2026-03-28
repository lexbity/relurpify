package runtime

import (
	"context"
	"fmt"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
)

type ProviderRestoreState struct {
	WorkflowID           string    `json:"workflow_id,omitempty"`
	RunID                string    `json:"run_id,omitempty"`
	ProviderSnapshotRefs []string  `json:"provider_snapshot_refs,omitempty"`
	SessionSnapshotRefs  []string  `json:"session_snapshot_refs,omitempty"`
	Restored             bool      `json:"restored"`
	RestoreSource        string    `json:"restore_source,omitempty"`
	LastRestoreError     string    `json:"last_restore_error,omitempty"`
	UpdatedAt            time.Time `json:"updated_at,omitempty"`
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
	state.Set("euclo.provider_restore", restoreState)
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
		state.Set("euclo.provider_restore", restoreState)
		return restoreState, err
	}
	sessions, err := store.ListProviderSessionSnapshots(ctx, workflowID, runID)
	if err != nil {
		restoreState.LastRestoreError = err.Error()
		state.Set("euclo.provider_restore", restoreState)
		return restoreState, err
	}
	state.Set("euclo.provider_snapshots", providerSnapshotsFromRecords(providers))
	state.Set("euclo.provider_session_snapshots", providerSessionSnapshotsFromRecords(sessions))
	restoreState.ProviderSnapshotRefs = providerSnapshotRecordRefs(providers)
	restoreState.SessionSnapshotRefs = providerSessionRecordRefs(sessions)
	restoreState.Restored = len(providers) > 0 || len(sessions) > 0
	state.Set("euclo.provider_restore", restoreState)
	return restoreState, nil
}

func providerSnapshotRecordsFromState(state *core.Context, workflowID, runID, taskID string) []memory.WorkflowProviderSnapshotRecord {
	if state == nil {
		return nil
	}
	raw, ok := state.Get("euclo.provider_snapshots")
	if !ok || raw == nil {
		return nil
	}
	switch typed := raw.(type) {
	case []core.ProviderSnapshot:
		out := make([]memory.WorkflowProviderSnapshotRecord, 0, len(typed))
		for _, snapshot := range typed {
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
	case []memory.WorkflowProviderSnapshotRecord:
		return append([]memory.WorkflowProviderSnapshotRecord(nil), typed...)
	default:
		return nil
	}
}

func providerSessionSnapshotRecordsFromState(state *core.Context, workflowID, runID string) []memory.WorkflowProviderSessionSnapshotRecord {
	if state == nil {
		return nil
	}
	raw, ok := state.Get("euclo.provider_session_snapshots")
	if !ok || raw == nil {
		return nil
	}
	switch typed := raw.(type) {
	case []core.ProviderSessionSnapshot:
		out := make([]memory.WorkflowProviderSessionSnapshotRecord, 0, len(typed))
		for _, snapshot := range typed {
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
	case []memory.WorkflowProviderSessionSnapshotRecord:
		return append([]memory.WorkflowProviderSessionSnapshotRecord(nil), typed...)
	default:
		return nil
	}
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
