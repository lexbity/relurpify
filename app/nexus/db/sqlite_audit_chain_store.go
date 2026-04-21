package db

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	fwfmp "codeburg.org/lexbit/relurpify/relurpnet/fmp"
	_ "github.com/mattn/go-sqlite3"
)

var _ core.AuditChainReader = (*SQLiteAuditChainStore)(nil)

type SQLiteAuditChainStore struct {
	db       *sql.DB
	signer   fwfmp.PayloadSigner
	verifier fwfmp.PayloadVerifier
}

func NewSQLiteAuditChainStore(path string, signer fwfmp.PayloadSigner, verifier fwfmp.PayloadVerifier) (*SQLiteAuditChainStore, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("audit chain store path required")
	}
	dsn := fmt.Sprintf("file:%s?_foreign_keys=on&_busy_timeout=5000", filepath.Clean(path))
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}
	store := &SQLiteAuditChainStore{
		db:       db,
		signer:   signer,
		verifier: verifier,
	}
	if err := store.init(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *SQLiteAuditChainStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *SQLiteAuditChainStore) init() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS fmp_audit_chain (
			seq INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp TEXT NOT NULL,
			agent_id TEXT NOT NULL DEFAULT '',
			action TEXT NOT NULL DEFAULT '',
			type TEXT NOT NULL DEFAULT '',
			permission TEXT NOT NULL DEFAULT '',
			result TEXT NOT NULL DEFAULT '',
			metadata_json TEXT NOT NULL DEFAULT '{}',
			user_id TEXT NOT NULL DEFAULT '',
			correlation_id TEXT NOT NULL DEFAULT '',
			previous_hash TEXT NOT NULL DEFAULT '',
			record_hash TEXT NOT NULL,
			signature_algorithm TEXT NOT NULL DEFAULT '',
			signature TEXT NOT NULL DEFAULT ''
		);`,
		`CREATE INDEX IF NOT EXISTS idx_fmp_audit_chain_type_ts ON fmp_audit_chain(type, timestamp);`,
		`CREATE INDEX IF NOT EXISTS idx_fmp_audit_chain_correlation ON fmp_audit_chain(correlation_id);`,
		`CREATE INDEX IF NOT EXISTS idx_fmp_audit_chain_lineage ON fmp_audit_chain(json_extract(metadata_json, '$.lineage_id'));`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLiteAuditChainStore) Log(ctx context.Context, record core.AuditRecord) error {
	record = normalizeAuditRecord(record)
	metadataJSON, err := json.Marshal(record.Metadata)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	previousHash, err := s.lastHashTx(ctx, tx)
	if err != nil {
		return err
	}
	payload, err := canonicalAuditChainPayload(record, previousHash)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(payload)
	recordHash := hex.EncodeToString(sum[:])
	signatureAlgorithm := ""
	signature := ""
	if s.signer != nil {
		signatureAlgorithm = s.signer.Algorithm()
		signature, err = s.signer.SignPayload(payload)
		if err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO fmp_audit_chain (
		timestamp, agent_id, action, type, permission, result, metadata_json, user_id, correlation_id, previous_hash, record_hash, signature_algorithm, signature
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		record.Timestamp.UTC().Format(time.RFC3339Nano),
		record.AgentID,
		record.Action,
		record.Type,
		record.Permission,
		record.Result,
		string(metadataJSON),
		record.User,
		record.Correlation,
		previousHash,
		recordHash,
		signatureAlgorithm,
		signature,
	)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLiteAuditChainStore) Query(ctx context.Context, filter core.AuditQuery) ([]core.AuditRecord, error) {
	entries, err := s.ReadChain(ctx, core.AuditChainFilter{AuditQuery: filter})
	if err != nil {
		return nil, err
	}
	records := make([]core.AuditRecord, 0, len(entries))
	for _, entry := range entries {
		records = append(records, entry.Record)
	}
	return records, nil
}

func (s *SQLiteAuditChainStore) ReadChain(ctx context.Context, filter core.AuditChainFilter) ([]core.AuditChainEntry, error) {
	query, args := buildAuditChainQuery(filter)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := make([]core.AuditChainEntry, 0)
	for rows.Next() {
		entry, err := scanAuditChainEntry(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

func (s *SQLiteAuditChainStore) VerifyChain(ctx context.Context, filter core.AuditChainFilter) (core.AuditChainVerification, error) {
	entries, err := s.ReadChain(ctx, filter)
	if err != nil {
		return core.AuditChainVerification{}, err
	}
	result := core.AuditChainVerification{
		Verified:   true,
		EntryCount: len(entries),
	}
	previousHash := ""
	for _, entry := range entries {
		result.LastSequence = entry.Sequence
		result.LastHash = entry.RecordHash
		if entry.PreviousHash != previousHash {
			result.Verified = false
			result.Failure = fmt.Sprintf("hash link mismatch at sequence %d", entry.Sequence)
			return result, nil
		}
		payload, err := canonicalAuditChainPayload(normalizeAuditRecord(entry.Record), entry.PreviousHash)
		if err != nil {
			return core.AuditChainVerification{}, err
		}
		sum := sha256.Sum256(payload)
		expectedHash := hex.EncodeToString(sum[:])
		if entry.RecordHash != expectedHash {
			result.Verified = false
			result.Failure = fmt.Sprintf("record hash mismatch at sequence %d", entry.Sequence)
			return result, nil
		}
		if strings.TrimSpace(entry.Signature) != "" && s.verifier != nil {
			if err := s.verifier.VerifyPayload(payload, entry.SignatureAlgorithm, entry.Signature); err != nil {
				result.Verified = false
				result.Failure = fmt.Sprintf("signature verification failed at sequence %d: %v", entry.Sequence, err)
				return result, nil
			}
		}
		previousHash = entry.RecordHash
	}
	return result, nil
}

func (s *SQLiteAuditChainStore) lastHashTx(ctx context.Context, tx *sql.Tx) (string, error) {
	var previousHash string
	err := tx.QueryRowContext(ctx, `SELECT record_hash FROM fmp_audit_chain ORDER BY seq DESC LIMIT 1`).Scan(&previousHash)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return previousHash, err
}

func buildAuditChainQuery(filter core.AuditChainFilter) (string, []any) {
	where := make([]string, 0, 8)
	args := make([]any, 0, 8)
	if v := strings.TrimSpace(filter.AgentID); v != "" {
		where = append(where, "agent_id = ?")
		args = append(args, v)
	}
	if v := strings.TrimSpace(filter.Action); v != "" {
		where = append(where, "action = ?")
		args = append(args, v)
	}
	if v := strings.TrimSpace(filter.Type); v != "" {
		where = append(where, "type = ?")
		args = append(args, v)
	}
	if v := strings.TrimSpace(filter.Permission); v != "" {
		where = append(where, "permission = ?")
		args = append(args, v)
	}
	if v := strings.TrimSpace(filter.Result); v != "" {
		where = append(where, "result = ?")
		args = append(args, v)
	}
	if !filter.TimeStart.IsZero() {
		where = append(where, "timestamp >= ?")
		args = append(args, filter.TimeStart.UTC().Format(time.RFC3339Nano))
	}
	if !filter.TimeEnd.IsZero() {
		where = append(where, "timestamp <= ?")
		args = append(args, filter.TimeEnd.UTC().Format(time.RFC3339Nano))
	}
	if v := strings.TrimSpace(filter.Correlation); v != "" {
		where = append(where, "correlation_id = ?")
		args = append(args, v)
	}
	if v := strings.TrimSpace(filter.LineageID); v != "" {
		where = append(where, "json_extract(metadata_json, '$.lineage_id') = ?")
		args = append(args, v)
	}
	base := `SELECT seq, timestamp, agent_id, action, type, permission, result, metadata_json, user_id, correlation_id, previous_hash, record_hash, signature_algorithm, signature FROM fmp_audit_chain`
	if len(where) > 0 {
		base += " WHERE " + strings.Join(where, " AND ")
	}
	if filter.Limit > 0 {
		base = `SELECT * FROM (` + base + ` ORDER BY seq DESC LIMIT ?) ORDER BY seq ASC`
		args = append(args, filter.Limit)
		return base, args
	}
	base += ` ORDER BY seq ASC`
	return base, args
}

func scanAuditChainEntry(scanner interface {
	Scan(dest ...any) error
}) (core.AuditChainEntry, error) {
	var (
		entry        core.AuditChainEntry
		timestampRaw string
		metadataJSON string
	)
	err := scanner.Scan(
		&entry.Sequence,
		&timestampRaw,
		&entry.Record.AgentID,
		&entry.Record.Action,
		&entry.Record.Type,
		&entry.Record.Permission,
		&entry.Record.Result,
		&metadataJSON,
		&entry.Record.User,
		&entry.Record.Correlation,
		&entry.PreviousHash,
		&entry.RecordHash,
		&entry.SignatureAlgorithm,
		&entry.Signature,
	)
	if err != nil {
		return core.AuditChainEntry{}, err
	}
	entry.Record.Timestamp, err = time.Parse(time.RFC3339Nano, timestampRaw)
	if err != nil {
		return core.AuditChainEntry{}, err
	}
	if strings.TrimSpace(metadataJSON) == "" {
		metadataJSON = "{}"
	}
	if err := json.Unmarshal([]byte(metadataJSON), &entry.Record.Metadata); err != nil {
		return core.AuditChainEntry{}, err
	}
	if entry.Record.Metadata == nil {
		entry.Record.Metadata = map[string]any{}
	}
	return entry, nil
}

func normalizeAuditRecord(record core.AuditRecord) core.AuditRecord {
	if record.Timestamp.IsZero() {
		record.Timestamp = time.Now().UTC()
	}
	record.Timestamp = record.Timestamp.UTC()
	if record.Metadata == nil {
		record.Metadata = map[string]any{}
	}
	return record
}

func canonicalAuditChainPayload(record core.AuditRecord, previousHash string) ([]byte, error) {
	normalized := normalizeAuditRecord(record)
	return json.Marshal(struct {
		Timestamp   string         `json:"timestamp"`
		AgentID     string         `json:"agent_id,omitempty"`
		Action      string         `json:"action,omitempty"`
		Type        string         `json:"type,omitempty"`
		Permission  string         `json:"permission,omitempty"`
		Result      string         `json:"result,omitempty"`
		Metadata    map[string]any `json:"metadata,omitempty"`
		User        string         `json:"user,omitempty"`
		Correlation string         `json:"correlation_id,omitempty"`
		Previous    string         `json:"previous_hash,omitempty"`
	}{
		Timestamp:   normalized.Timestamp.Format(time.RFC3339Nano),
		AgentID:     normalized.AgentID,
		Action:      normalized.Action,
		Type:        normalized.Type,
		Permission:  normalized.Permission,
		Result:      normalized.Result,
		Metadata:    normalized.Metadata,
		User:        normalized.User,
		Correlation: normalized.Correlation,
		Previous:    strings.TrimSpace(previousHash),
	})
}
