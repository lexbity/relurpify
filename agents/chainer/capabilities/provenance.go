package capabilities

import (
	"fmt"
	"sync"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
)

// ProvenanceTracker records and wraps tool invocations for audit and provenance.
//
// Phase 6: Lightweight tracking of which tools were used and how their results
// were presented. Wraps results in CapabilityResultEnvelope for insertion decisions.
type ProvenanceTracker struct {
	mu              sync.RWMutex
	records         []ProvenanceRecord
	taskID          string
}

// ProvenanceRecord documents a single tool invocation.
type ProvenanceRecord struct {
	Timestamp        time.Time
	TaskID           string
	LinkName         string
	ToolID           string
	InsertionAction  core.InsertionAction
	TrustClass       core.TrustClass
	ApprovedBy       string // User/system identifier (if approval required)
	PolicySnapshot   string // Policy ID or description
	ResultSummary    string // Brief description of result
}

// NewProvenanceTracker creates a tracker for a task.
func NewProvenanceTracker(taskID string) *ProvenanceTracker {
	return &ProvenanceTracker{
		taskID:  taskID,
		records: make([]ProvenanceRecord, 0),
	}
}

// Record documents a tool invocation.
func (t *ProvenanceTracker) Record(linkName, toolID string, action core.InsertionAction) error {
	if t == nil {
		return fmt.Errorf("provenance tracker not initialized")
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	record := ProvenanceRecord{
		Timestamp:       time.Now(),
		TaskID:          t.taskID,
		LinkName:        linkName,
		ToolID:          toolID,
		InsertionAction: action,
	}

	t.records = append(t.records, record)
	return nil
}

// RecordWithApproval documents a tool invocation with approval info.
func (t *ProvenanceTracker) RecordWithApproval(linkName, toolID string, action core.InsertionAction, approvedBy string) error {
	if t == nil {
		return fmt.Errorf("provenance tracker not initialized")
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	record := ProvenanceRecord{
		Timestamp:       time.Now(),
		TaskID:          t.taskID,
		LinkName:        linkName,
		ToolID:          toolID,
		InsertionAction: action,
		ApprovedBy:      approvedBy,
	}

	t.records = append(t.records, record)
	return nil
}

// AllRecords returns all provenance records for the task.
func (t *ProvenanceTracker) AllRecords() []ProvenanceRecord {
	if t == nil {
		return nil
	}

	t.mu.RLock()
	defer t.mu.RUnlock()

	// Return copy to prevent external mutation
	records := make([]ProvenanceRecord, len(t.records))
	copy(records, t.records)
	return records
}

// RecordsByLink returns provenance records for a specific link.
func (t *ProvenanceTracker) RecordsByLink(linkName string) []ProvenanceRecord {
	if t == nil {
		return nil
	}

	t.mu.RLock()
	defer t.mu.RUnlock()

	var filtered []ProvenanceRecord
	for _, r := range t.records {
		if r.LinkName == linkName {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

// RecordsByTool returns provenance records for a specific tool.
func (t *ProvenanceTracker) RecordsByTool(toolID string) []ProvenanceRecord {
	if t == nil {
		return nil
	}

	t.mu.RLock()
	defer t.mu.RUnlock()

	var filtered []ProvenanceRecord
	for _, r := range t.records {
		if r.ToolID == toolID {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

// Count returns the number of recorded tool invocations.
func (t *ProvenanceTracker) Count() int {
	if t == nil {
		return 0
	}

	t.mu.RLock()
	defer t.mu.RUnlock()

	return len(t.records)
}

// Clear removes all records.
func (t *ProvenanceTracker) Clear() {
	if t == nil {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	t.records = make([]ProvenanceRecord, 0)
}

// ProvenanceSummary provides high-level tool usage information.
type ProvenanceSummary struct {
	TaskID              string
	TotalTools          int
	UniqueTools         int
	ApprovalRequired    int
	Denied              int
	DirectInclusions    int
	SummarizedResults   int
	MetadataOnlyResults int
}

// Summary generates a provenance summary for the task.
func (t *ProvenanceTracker) Summary() *ProvenanceSummary {
	if t == nil {
		return &ProvenanceSummary{}
	}

	t.mu.RLock()
	defer t.mu.RUnlock()

	summary := &ProvenanceSummary{
		TaskID: t.taskID,
	}

	toolSet := make(map[string]bool)

	for _, record := range t.records {
		summary.TotalTools++
		toolSet[record.ToolID] = true

		switch record.InsertionAction {
		case core.InsertionActionDirect:
			summary.DirectInclusions++
		case core.InsertionActionSummarized:
			summary.SummarizedResults++
		case core.InsertionActionMetadataOnly:
			summary.MetadataOnlyResults++
		case core.InsertionActionHITLRequired:
			summary.ApprovalRequired++
		case core.InsertionActionDenied:
			summary.Denied++
		}
	}

	summary.UniqueTools = len(toolSet)
	return summary
}

// WrapResult wraps a tool result with provenance information.
//
// Phase 6 stub: Records the insertion action for the tool result.
// Full CapabilityResultEnvelope wrapping deferred to Phase 6+.
func (t *ProvenanceTracker) WrapResult(toolID string, result any, action core.InsertionAction) map[string]any {
	return map[string]any{
		"result":           result,
		"insertion_action": action,
		"tool_id":          toolID,
	}
}
