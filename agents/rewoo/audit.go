package rewoo

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/memory"
)

// rewooAuditLogger implements core.AuditLogger and writes to a memory store.
type rewooAuditLogger struct {
	store memory.RuntimeMemoryStore
}

// NewRewooAuditLogger creates an audit logger backed by a memory store.
func NewRewooAuditLogger(store memory.RuntimeMemoryStore) core.AuditLogger {
	return &rewooAuditLogger{store: store}
}

// Log writes an audit record to the memory store.
func (l *rewooAuditLogger) Log(ctx context.Context, record core.AuditRecord) error {
	if l.store == nil {
		// Silent fail: no store configured
		return nil
	}

	// Convert to declarative memory record
	payload, _ := json.Marshal(record)
	declRecord := memory.DeclarativeMemoryRecord{
		RecordID:  fmt.Sprintf("audit_%d", time.Now().UnixNano()),
		Scope:     memory.MemoryScopeProject,
		Kind:      memory.DeclarativeMemoryKindConstraint,
		Title:     fmt.Sprintf("Audit: %s %s", record.Action, record.Type),
		Content:   string(payload),
		Summary:   fmt.Sprintf("%s on %s: %s", record.Action, record.Permission, record.Result),
		CreatedAt: record.Timestamp,
		UpdatedAt: record.Timestamp,
		Tags: []string{
			"audit",
			"agent:" + record.AgentID,
			"action:" + record.Action,
			"result:" + record.Result,
		},
	}

	return l.store.PutDeclarative(ctx, declRecord)
}

// Query retrieves audit records matching a filter.
func (l *rewooAuditLogger) Query(ctx context.Context, filter core.AuditQuery) ([]core.AuditRecord, error) {
	if l.store == nil {
		return nil, nil
	}

	// TODO(phase-3): Implement query against declarative memory store
	// For now, return empty to avoid breaking the interface
	return nil, nil
}
