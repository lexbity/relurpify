package policy

import (
	"context"
	"errors"
	"time"
)

// AuditAction categorizes records for downstream processing.
type AuditAction string

const (
	AuditActionFileAccess AuditAction = "file_access"
	AuditActionExec       AuditAction = "exec"
	AuditActionNetwork    AuditAction = "network"
	AuditActionCapability AuditAction = "capability"
	AuditActionIPC        AuditAction = "ipc"
	AuditActionTool       AuditAction = "tool"
	AuditActionRequest    AuditAction = "permission_request"
)

// AuditRecord captures a single trace event.
type AuditRecord struct {
	Timestamp   time.Time      `json:"timestamp"`
	AgentID     string         `json:"agent_id"`
	Action      string         `json:"action"`
	Type        string         `json:"type"`
	Permission  string         `json:"permission"`
	Result      string         `json:"result"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	User        string         `json:"user,omitempty"`
	Correlation string         `json:"correlation_id,omitempty"`
}

// AuditLogger defines the logging backend.
type AuditLogger interface {
	Log(ctx context.Context, record AuditRecord) error
	Query(ctx context.Context, filter AuditQuery) ([]AuditRecord, error)
}

// AuditQuery filters audit entries.
type AuditQuery struct {
	AgentID    string
	Action     string
	Type       string
	TimeStart  time.Time
	TimeEnd    time.Time
	Permission string
	Result     string
}

// AuditChainEntry captures an append-only audit record with integrity metadata.
type AuditChainEntry struct {
	Sequence           int64       `json:"sequence"`
	Record             AuditRecord `json:"record"`
	PreviousHash       string      `json:"previous_hash,omitempty"`
	RecordHash         string      `json:"record_hash"`
	SignatureAlgorithm string      `json:"signature_algorithm,omitempty"`
	Signature          string      `json:"signature,omitempty"`
}

// AuditChainFilter scopes append-only audit queries and verification.
type AuditChainFilter struct {
	AuditQuery
	Correlation string
	LineageID   string
	Limit       int
}

// AuditChainVerification reports the integrity status of a filtered chain view.
type AuditChainVerification struct {
	Verified     bool   `json:"verified"`
	EntryCount   int    `json:"entry_count"`
	LastSequence int64  `json:"last_sequence,omitempty"`
	LastHash     string `json:"last_hash,omitempty"`
	Failure      string `json:"failure,omitempty"`
}

// AuditChainReader extends the audit logger with chain-aware read and verify APIs.
type AuditChainReader interface {
	AuditLogger
	ReadChain(ctx context.Context, filter AuditChainFilter) ([]AuditChainEntry, error)
	VerifyChain(ctx context.Context, filter AuditChainFilter) (AuditChainVerification, error)
}

// AuditStore exposes a read API for servers or dashboards.
type AuditStore struct {
	logger AuditLogger
}

// NewAuditStore builds the store.
func NewAuditStore(logger AuditLogger) *AuditStore {
	return &AuditStore{logger: logger}
}

// Query proxies the request.
func (s *AuditStore) Query(ctx context.Context, filter AuditQuery) ([]AuditRecord, error) {
	if s.logger == nil {
		return nil, errors.New("audit logger missing — audit chain not initialized (workspace state dir unwritable or registration did not complete)")
	}
	return s.logger.Query(ctx, filter)
}
