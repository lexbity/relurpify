package session

import (
	"context"
	"sort"
	"sync"
	"time"

	archaeoplans "github.com/lexcodex/relurpify/archaeo/plans"
	"github.com/lexcodex/relurpify/framework/memory"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
)

// SessionIndex provides session listing and enrichment for session
// selection flows.
type SessionIndex struct {
	WorkflowStore memory.WorkflowStateStore
	PlanStore     frameworkplan.PlanStore
}

// List returns the most recently active sessions for the given workspace,
// up to the specified limit. Each record is enriched with plan and BKC
// context information.
func (idx *SessionIndex) List(ctx context.Context, workspaceRoot string, limit int) (SessionList, error) {
	// 1. Over-fetch to account for filtering
	workflows, err := idx.WorkflowStore.ListWorkflows(ctx, limit*2)
	if err != nil {
		return SessionList{}, err
	}

	// 2. Filter by workspace (best-effort; untagged workflows are included)
	var filtered []memory.WorkflowRecord
	for _, wf := range workflows {
		if workspaceRoot == "" {
			filtered = append(filtered, wf)
			continue
		}
		if wfWorkspace, ok := wf.Metadata["workspace"].(string); ok && wfWorkspace == workspaceRoot {
			filtered = append(filtered, wf)
		} else if !ok {
			// Untagged workflows are included
			filtered = append(filtered, wf)
		}
	}

	// 3. Enrich records concurrently
	records := make([]SessionRecord, len(filtered))
	var wg sync.WaitGroup

	for i, wf := range filtered {
		wg.Add(1)
		go func(index int, workflow memory.WorkflowRecord) {
			defer wg.Done()
			records[index] = idx.enrichRecord(ctx, workflow)
		}(i, wf)
	}

	wg.Wait()

	// 7. Sort by LastActiveAt descending
	sort.Slice(records, func(i, j int) bool {
		return records[i].LastActiveAt.After(records[j].LastActiveAt)
	})

	// Truncate to limit
	if len(records) > limit {
		records = records[:limit]
	}

	return SessionList{
		Sessions:  records,
		Workspace: workspaceRoot,
	}, nil
}

// Get returns the full SessionRecord for a single workflow ID.
func (idx *SessionIndex) Get(ctx context.Context, workflowID string) (SessionRecord, bool, error) {
	wf, ok, err := idx.WorkflowStore.GetWorkflow(ctx, workflowID)
	if err != nil || !ok {
		return SessionRecord{}, false, err
	}

	record := idx.enrichRecord(ctx, *wf)
	return record, true, nil
}

// enrichRecord enriches a workflow record with plan and run information.
// Failures during enrichment are silently dropped — a partially-enriched
// record is still useful for selection.
func (idx *SessionIndex) enrichRecord(ctx context.Context, wf memory.WorkflowRecord) SessionRecord {
	record := SessionRecord{
		WorkflowID:  wf.WorkflowID,
		Instruction: wf.Instruction,
		Status:      string(wf.Status),
	}

	if workspace, ok := wf.Metadata["workspace"].(string); ok {
		record.WorkspaceRoot = workspace
	}

	// Resolve mode from metadata (best-effort)
	if mode, ok := wf.Metadata["mode"].(string); ok {
		record.Mode = mode
	}

	// Resolve phase from metadata (best-effort)
	if phase, ok := wf.Metadata["phase"].(string); ok {
		record.Phase = phase
	}

	// 3. Resolve active living plan and extract chunk anchor information
	planSvc := archaeoplans.Service{Store: idx.PlanStore, WorkflowStore: idx.WorkflowStore}
	if plan, err := planSvc.LoadActiveVersion(ctx, wf.WorkflowID); err == nil && plan != nil {
		record.ActivePlanVersion = plan.Version
		record.ActivePlanTitle = plan.Plan.Title
		record.RootChunkIDs = append([]string(nil), plan.RootChunkIDs...)
		record.HasBKCContext = len(record.RootChunkIDs) > 0
	}

	// 5-6. Use workflow UpdatedAt for LastActiveAt
	// Note: ListRunsByStatus is not part of WorkflowStateStore interface,
	// so we derive LastActiveAt from workflow's UpdatedAt and mode from metadata.
	record.LastActiveAt = wf.UpdatedAt

	return record
}

// SessionIndexOption configures optional behavior for SessionIndex.
type SessionIndexOption func(*sessionIndexConfig)

type sessionIndexConfig struct {
	nowFunc func() time.Time
}

// WithNowFunc sets a custom time function for testing.
func WithNowFunc(f func() time.Time) SessionIndexOption {
	return func(c *sessionIndexConfig) {
		c.nowFunc = f
	}
}
