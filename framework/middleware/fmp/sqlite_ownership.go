package fmp

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	_ "github.com/mattn/go-sqlite3"
)

var _ OwnershipStore = (*SQLiteOwnershipStore)(nil)

type SQLiteOwnershipStore struct {
	db  *sql.DB
	now func() time.Time
}

func NewSQLiteOwnershipStore(path string) (*SQLiteOwnershipStore, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("ownership store path required")
	}
	dsn := fmt.Sprintf("file:%s?_foreign_keys=on&_busy_timeout=5000", filepath.Clean(path))
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}
	store := &SQLiteOwnershipStore{
		db:  db,
		now: func() time.Time { return time.Now().UTC() },
	}
	if err := store.init(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *SQLiteOwnershipStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *SQLiteOwnershipStore) init() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS fmp_lineages (
			lineage_id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			parent_lineage_id TEXT NOT NULL DEFAULT '',
			task_class TEXT NOT NULL,
			context_class TEXT NOT NULL,
			current_owner_attempt TEXT NOT NULL DEFAULT '',
			current_owner_runtime TEXT NOT NULL DEFAULT '',
			capability_envelope_json TEXT NOT NULL DEFAULT '{}',
			sensitivity_class TEXT NOT NULL DEFAULT '',
			allowed_federation_targets_json TEXT NOT NULL DEFAULT '[]',
			owner_json TEXT NOT NULL,
			session_id TEXT NOT NULL DEFAULT '',
			session_binding_json TEXT NOT NULL DEFAULT '',
			delegations_json TEXT NOT NULL DEFAULT '[]',
			trust_class TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL DEFAULT '',
			lineage_version INTEGER NOT NULL DEFAULT 0
		);`,
		`CREATE TABLE IF NOT EXISTS fmp_attempts (
			attempt_id TEXT PRIMARY KEY,
			lineage_id TEXT NOT NULL,
			runtime_id TEXT NOT NULL,
			state TEXT NOT NULL,
			lease_id TEXT NOT NULL DEFAULT '',
			lease_expiry TEXT NOT NULL DEFAULT '',
			start_time TEXT NOT NULL DEFAULT '',
			last_progress_time TEXT NOT NULL DEFAULT '',
			fenced INTEGER NOT NULL DEFAULT 0,
			fencing_epoch INTEGER NOT NULL DEFAULT 0,
			previous_attempt_id TEXT NOT NULL DEFAULT '',
			FOREIGN KEY(lineage_id) REFERENCES fmp_lineages(lineage_id)
		);`,
		`CREATE TABLE IF NOT EXISTS fmp_active_leases (
			lineage_id TEXT PRIMARY KEY,
			lease_id TEXT NOT NULL,
			attempt_id TEXT NOT NULL,
			issuer TEXT NOT NULL,
			issued_at TEXT NOT NULL,
			expiry TEXT NOT NULL,
			fencing_epoch INTEGER NOT NULL,
			signature TEXT NOT NULL DEFAULT '',
			FOREIGN KEY(lineage_id) REFERENCES fmp_lineages(lineage_id),
			FOREIGN KEY(attempt_id) REFERENCES fmp_attempts(attempt_id)
		);`,
		`CREATE TABLE IF NOT EXISTS fmp_resume_commits (
			lineage_id TEXT PRIMARY KEY,
			old_attempt_id TEXT NOT NULL,
			new_attempt_id TEXT NOT NULL,
			destination_runtime_id TEXT NOT NULL,
			receipt_ref TEXT NOT NULL,
			commit_time TEXT NOT NULL,
			signature TEXT NOT NULL DEFAULT '',
			FOREIGN KEY(lineage_id) REFERENCES fmp_lineages(lineage_id)
		);`,
		`CREATE TABLE IF NOT EXISTS fmp_idempotency_markers (
			scope TEXT NOT NULL,
			marker_key TEXT NOT NULL,
			payload_json TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL,
			PRIMARY KEY (scope, marker_key)
		);`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLiteOwnershipStore) CreateLineage(ctx context.Context, lineage core.LineageRecord) error {
	if err := lineage.Validate(); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO fmp_lineages (
		lineage_id, tenant_id, parent_lineage_id, task_class, context_class,
		current_owner_attempt, current_owner_runtime, capability_envelope_json,
		sensitivity_class, allowed_federation_targets_json, owner_json,
		session_id, session_binding_json, delegations_json, trust_class,
		created_at, updated_at, lineage_version
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		lineage.LineageID,
		lineage.TenantID,
		lineage.ParentLineageID,
		lineage.TaskClass,
		lineage.ContextClass,
		lineage.CurrentOwnerAttempt,
		lineage.CurrentOwnerRuntime,
		mustMarshalJSON(lineage.CapabilityEnvelope),
		string(lineage.SensitivityClass),
		mustMarshalJSON(lineage.AllowedFederationTargets),
		mustMarshalJSON(lineage.Owner),
		lineage.SessionID,
		marshalOptionalJSON(lineage.SessionBinding),
		mustMarshalJSON(lineage.Delegations),
		string(lineage.TrustClass),
		formatOptionalTime(lineage.CreatedAt),
		formatOptionalTime(lineage.UpdatedAt),
		lineage.LineageVersion,
	)
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "unique") {
		return fmt.Errorf("lineage %s already exists", lineage.LineageID)
	}
	return err
}

func (s *SQLiteOwnershipStore) GetLineage(ctx context.Context, lineageID string) (*core.LineageRecord, bool, error) {
	row := s.db.QueryRowContext(ctx, `SELECT
		lineage_id, tenant_id, parent_lineage_id, task_class, context_class,
		current_owner_attempt, current_owner_runtime, capability_envelope_json,
		sensitivity_class, allowed_federation_targets_json, owner_json,
		session_id, session_binding_json, delegations_json, trust_class,
		created_at, updated_at, lineage_version
		FROM fmp_lineages WHERE lineage_id = ?`, lineageID)
	lineage, err := scanLineageRecord(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return lineage, true, nil
}

func (s *SQLiteOwnershipStore) ListLineages(ctx context.Context) ([]core.LineageRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT
		lineage_id, tenant_id, parent_lineage_id, task_class, context_class,
		current_owner_attempt, current_owner_runtime, capability_envelope_json,
		sensitivity_class, allowed_federation_targets_json, owner_json,
		session_id, session_binding_json, delegations_json, trust_class,
		created_at, updated_at, lineage_version
		FROM fmp_lineages
		ORDER BY updated_at DESC, lineage_id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]core.LineageRecord, 0)
	for rows.Next() {
		lineage, err := scanLineageRecord(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *lineage)
	}
	return out, rows.Err()
}

func (s *SQLiteOwnershipStore) UpsertAttempt(ctx context.Context, attempt core.AttemptRecord) error {
	if err := attempt.Validate(); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO fmp_attempts (
		attempt_id, lineage_id, runtime_id, state, lease_id, lease_expiry,
		start_time, last_progress_time, fenced, fencing_epoch, previous_attempt_id
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(attempt_id) DO UPDATE SET
		lineage_id = excluded.lineage_id,
		runtime_id = excluded.runtime_id,
		state = excluded.state,
		lease_id = excluded.lease_id,
		lease_expiry = excluded.lease_expiry,
		start_time = excluded.start_time,
		last_progress_time = excluded.last_progress_time,
		fenced = excluded.fenced,
		fencing_epoch = excluded.fencing_epoch,
		previous_attempt_id = excluded.previous_attempt_id`,
		attempt.AttemptID,
		attempt.LineageID,
		attempt.RuntimeID,
		string(attempt.State),
		attempt.LeaseID,
		formatOptionalTime(attempt.LeaseExpiry),
		formatOptionalTime(attempt.StartTime),
		formatOptionalTime(attempt.LastProgressTime),
		boolToInt(attempt.Fenced),
		attempt.FencingEpoch,
		attempt.PreviousAttemptID,
	)
	return err
}

func (s *SQLiteOwnershipStore) GetAttempt(ctx context.Context, attemptID string) (*core.AttemptRecord, bool, error) {
	row := s.db.QueryRowContext(ctx, `SELECT
		attempt_id, lineage_id, runtime_id, state, lease_id, lease_expiry,
		start_time, last_progress_time, fenced, fencing_epoch, previous_attempt_id
		FROM fmp_attempts WHERE attempt_id = ?`, attemptID)
	attempt, err := scanAttemptRecord(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return attempt, true, nil
}

func (s *SQLiteOwnershipStore) IssueLease(ctx context.Context, lineageID, attemptID, issuer string, ttl time.Duration) (*core.LeaseToken, error) {
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	now := s.now()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer rollback(tx)

	if !recordExists(ctx, tx.QueryRowContext(ctx, `SELECT 1 FROM fmp_lineages WHERE lineage_id = ?`, lineageID)) {
		return nil, fmt.Errorf("lineage %s not found", lineageID)
	}
	if !recordExists(ctx, tx.QueryRowContext(ctx, `SELECT 1 FROM fmp_attempts WHERE attempt_id = ? AND lineage_id = ?`, attemptID, lineageID)) {
		return nil, fmt.Errorf("attempt %s not found for lineage %s", attemptID, lineageID)
	}

	var previousEpoch int64
	var previousLeaseID string
	row := tx.QueryRowContext(ctx, `SELECT lease_id, fencing_epoch FROM fmp_active_leases WHERE lineage_id = ?`, lineageID)
	switch err := row.Scan(&previousLeaseID, &previousEpoch); {
	case errors.Is(err, sql.ErrNoRows):
		previousEpoch = 0
	case err != nil:
		return nil, err
	}

	token := core.LeaseToken{
		LeaseID:      fmt.Sprintf("%s:%s:%s", lineageID, attemptID, now.Format(time.RFC3339Nano)),
		LineageID:    lineageID,
		AttemptID:    attemptID,
		Issuer:       issuer,
		IssuedAt:     now,
		Expiry:       now.Add(ttl),
		FencingEpoch: previousEpoch + 1,
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO fmp_active_leases (
		lineage_id, lease_id, attempt_id, issuer, issued_at, expiry, fencing_epoch, signature
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(lineage_id) DO UPDATE SET
		lease_id = excluded.lease_id,
		attempt_id = excluded.attempt_id,
		issuer = excluded.issuer,
		issued_at = excluded.issued_at,
		expiry = excluded.expiry,
		fencing_epoch = excluded.fencing_epoch,
		signature = excluded.signature`,
		token.LineageID,
		token.LeaseID,
		token.AttemptID,
		token.Issuer,
		formatRequiredTime(token.IssuedAt),
		formatRequiredTime(token.Expiry),
		token.FencingEpoch,
		token.Signature,
	); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE fmp_attempts
		SET lease_id = ?, lease_expiry = ?, fencing_epoch = CASE WHEN fencing_epoch > ? THEN fencing_epoch ELSE ? END
		WHERE attempt_id = ?`,
		token.LeaseID,
		formatRequiredTime(token.Expiry),
		token.FencingEpoch,
		token.FencingEpoch,
		attemptID,
	); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &token, nil
}

func (s *SQLiteOwnershipStore) ValidateLease(ctx context.Context, lease core.LeaseToken, now time.Time) error {
	if err := lease.Validate(); err != nil {
		return err
	}
	if now.IsZero() {
		now = s.now()
	}
	row := s.db.QueryRowContext(ctx, `SELECT lease_id, attempt_id, issuer, issued_at, expiry, fencing_epoch, signature
		FROM fmp_active_leases WHERE lineage_id = ?`, lease.LineageID)
	var (
		current         core.LeaseToken
		issuedAtRFC3339 string
		expiryRFC3339   string
	)
	current.LineageID = lease.LineageID
	err := row.Scan(
		&current.LeaseID,
		&current.AttemptID,
		&current.Issuer,
		&issuedAtRFC3339,
		&expiryRFC3339,
		&current.FencingEpoch,
		&current.Signature,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("lease not found for lineage %s", lease.LineageID)
	}
	if err != nil {
		return err
	}
	current.IssuedAt, err = parseOptionalTime(issuedAtRFC3339)
	if err != nil {
		return err
	}
	current.Expiry, err = parseOptionalTime(expiryRFC3339)
	if err != nil {
		return err
	}
	if current.LeaseID != lease.LeaseID {
		return fmt.Errorf("lease %s superseded", lease.LeaseID)
	}
	if now.After(current.Expiry) {
		return fmt.Errorf("lease %s expired", lease.LeaseID)
	}
	return nil
}

func (s *SQLiteOwnershipStore) CommitHandoff(ctx context.Context, commit core.ResumeCommit) error {
	if err := commit.Validate(); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollback(tx)

	lineage, err := scanLineageRecord(tx.QueryRowContext(ctx, `SELECT
		lineage_id, tenant_id, parent_lineage_id, task_class, context_class,
		current_owner_attempt, current_owner_runtime, capability_envelope_json,
		sensitivity_class, allowed_federation_targets_json, owner_json,
		session_id, session_binding_json, delegations_json, trust_class,
		created_at, updated_at, lineage_version
		FROM fmp_lineages WHERE lineage_id = ?`, commit.LineageID))
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("lineage %s not found", commit.LineageID)
	}
	if err != nil {
		return err
	}
	newAttempt, err := scanAttemptRecord(tx.QueryRowContext(ctx, `SELECT
		attempt_id, lineage_id, runtime_id, state, lease_id, lease_expiry,
		start_time, last_progress_time, fenced, fencing_epoch, previous_attempt_id
		FROM fmp_attempts WHERE attempt_id = ?`, commit.NewAttemptID))
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("attempt %s not found", commit.NewAttemptID)
	}
	if err != nil {
		return err
	}

	var existingReceiptRef, existingNewAttemptID string
	commitRow := tx.QueryRowContext(ctx, `SELECT receipt_ref, new_attempt_id FROM fmp_resume_commits WHERE lineage_id = ?`, commit.LineageID)
	switch err := commitRow.Scan(&existingReceiptRef, &existingNewAttemptID); {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return err
	default:
		if existingReceiptRef == commit.ReceiptRef && existingNewAttemptID == commit.NewAttemptID {
			return tx.Commit()
		}
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO fmp_resume_commits (
		lineage_id, old_attempt_id, new_attempt_id, destination_runtime_id, receipt_ref, commit_time, signature
	) VALUES (?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(lineage_id) DO UPDATE SET
		old_attempt_id = excluded.old_attempt_id,
		new_attempt_id = excluded.new_attempt_id,
		destination_runtime_id = excluded.destination_runtime_id,
		receipt_ref = excluded.receipt_ref,
		commit_time = excluded.commit_time,
		signature = excluded.signature`,
		commit.LineageID,
		commit.OldAttemptID,
		commit.NewAttemptID,
		commit.DestinationRuntimeID,
		commit.ReceiptRef,
		formatRequiredTime(commit.CommitTime),
		commit.Signature,
	); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO fmp_idempotency_markers (scope, marker_key, payload_json, created_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(scope, marker_key) DO UPDATE SET payload_json = excluded.payload_json`,
		"resume_commit",
		commit.ReceiptRef,
		mustMarshalJSON(commit),
		formatRequiredTime(commit.CommitTime),
	); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE fmp_lineages
		SET current_owner_attempt = ?, current_owner_runtime = ?, updated_at = ?, lineage_version = ?
		WHERE lineage_id = ?`,
		commit.NewAttemptID,
		commit.DestinationRuntimeID,
		formatRequiredTime(commit.CommitTime),
		lineage.LineageVersion+1,
		commit.LineageID,
	); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE fmp_attempts
		SET state = ?, last_progress_time = ?
		WHERE attempt_id = ?`,
		string(core.AttemptStateRunning),
		formatRequiredTime(commit.CommitTime),
		commit.NewAttemptID,
	); err != nil {
		return err
	}
	if commit.OldAttemptID != "" {
		if _, err := tx.ExecContext(ctx, `UPDATE fmp_attempts
			SET state = ?
			WHERE attempt_id = ?`,
			string(core.AttemptStateCommittedRemote),
			commit.OldAttemptID,
		); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM fmp_active_leases WHERE lineage_id = ? AND attempt_id = ?`,
		commit.LineageID, commit.OldAttemptID,
	); err != nil {
		return err
	}
	newAttempt.State = core.AttemptStateRunning
	newAttempt.LastProgressTime = commit.CommitTime.UTC()
	_ = newAttempt
	return tx.Commit()
}

func (s *SQLiteOwnershipStore) Fence(ctx context.Context, notice core.FenceNotice) error {
	if err := notice.Validate(); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollback(tx)

	var currentEpoch int64
	row := tx.QueryRowContext(ctx, `SELECT fencing_epoch FROM fmp_attempts WHERE attempt_id = ?`, notice.AttemptID)
	switch err := row.Scan(&currentEpoch); {
	case errors.Is(err, sql.ErrNoRows):
		return fmt.Errorf("attempt %s not found", notice.AttemptID)
	case err != nil:
		return err
	}
	if notice.FencingEpoch < currentEpoch {
		return fmt.Errorf("fencing epoch %d stale for attempt %s", notice.FencingEpoch, notice.AttemptID)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE fmp_attempts
		SET fenced = 1, fencing_epoch = ?, state = ?
		WHERE attempt_id = ?`,
		notice.FencingEpoch,
		string(core.AttemptStateFenced),
		notice.AttemptID,
	); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO fmp_idempotency_markers (scope, marker_key, payload_json, created_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(scope, marker_key) DO UPDATE SET payload_json = excluded.payload_json`,
		"fence_notice",
		fmt.Sprintf("%s:%s:%d", notice.LineageID, notice.AttemptID, notice.FencingEpoch),
		mustMarshalJSON(notice),
		formatRequiredTime(zeroToNow(notice.IssuedAt, s.now)),
	); err != nil {
		return err
	}
	return tx.Commit()
}

func scanLineageRecord(scanner interface{ Scan(...any) error }) (*core.LineageRecord, error) {
	var (
		lineage                        core.LineageRecord
		capabilityEnvelopeJSON         string
		allowedFederationTargetsJSON   string
		ownerJSON                      string
		sessionBindingJSON             string
		delegationsJSON                string
		createdAtRFC3339, updatedAtRFC string
	)
	err := scanner.Scan(
		&lineage.LineageID,
		&lineage.TenantID,
		&lineage.ParentLineageID,
		&lineage.TaskClass,
		&lineage.ContextClass,
		&lineage.CurrentOwnerAttempt,
		&lineage.CurrentOwnerRuntime,
		&capabilityEnvelopeJSON,
		&lineage.SensitivityClass,
		&allowedFederationTargetsJSON,
		&ownerJSON,
		&lineage.SessionID,
		&sessionBindingJSON,
		&delegationsJSON,
		&lineage.TrustClass,
		&createdAtRFC3339,
		&updatedAtRFC,
		&lineage.LineageVersion,
	)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(capabilityEnvelopeJSON), &lineage.CapabilityEnvelope); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(allowedFederationTargetsJSON), &lineage.AllowedFederationTargets); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(ownerJSON), &lineage.Owner); err != nil {
		return nil, err
	}
	if strings.TrimSpace(sessionBindingJSON) != "" {
		var binding core.ExternalSessionBinding
		if err := json.Unmarshal([]byte(sessionBindingJSON), &binding); err != nil {
			return nil, err
		}
		lineage.SessionBinding = &binding
	}
	if err := json.Unmarshal([]byte(delegationsJSON), &lineage.Delegations); err != nil {
		return nil, err
	}
	lineage.CreatedAt, err = parseOptionalTime(createdAtRFC3339)
	if err != nil {
		return nil, err
	}
	lineage.UpdatedAt, err = parseOptionalTime(updatedAtRFC)
	if err != nil {
		return nil, err
	}
	return &lineage, nil
}

func scanAttemptRecord(scanner interface{ Scan(...any) error }) (*core.AttemptRecord, error) {
	var (
		attempt                              core.AttemptRecord
		state                                string
		leaseExpiryRFC3339, startTimeRFC3339 string
		lastProgressTimeRFC3339              string
		fencedInt                            int
	)
	err := scanner.Scan(
		&attempt.AttemptID,
		&attempt.LineageID,
		&attempt.RuntimeID,
		&state,
		&attempt.LeaseID,
		&leaseExpiryRFC3339,
		&startTimeRFC3339,
		&lastProgressTimeRFC3339,
		&fencedInt,
		&attempt.FencingEpoch,
		&attempt.PreviousAttemptID,
	)
	if err != nil {
		return nil, err
	}
	attempt.State = core.AttemptState(state)
	attempt.Fenced = fencedInt == 1
	var parseErr error
	attempt.LeaseExpiry, parseErr = parseOptionalTime(leaseExpiryRFC3339)
	if parseErr != nil {
		return nil, parseErr
	}
	attempt.StartTime, parseErr = parseOptionalTime(startTimeRFC3339)
	if parseErr != nil {
		return nil, parseErr
	}
	attempt.LastProgressTime, parseErr = parseOptionalTime(lastProgressTimeRFC3339)
	if parseErr != nil {
		return nil, parseErr
	}
	return &attempt, nil
}

func marshalOptionalJSON(value any) string {
	if value == nil {
		return ""
	}
	return mustMarshalJSON(value)
}

func mustMarshalJSON(value any) string {
	buf, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(buf)
}

func parseOptionalTime(value string) (time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339Nano, value)
}

func formatOptionalTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func formatRequiredTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

// Phase 6 optional interface implementations

// HasActiveAttemptForLineage returns true if the lineage has any active handoff-related attempt.
func (s *SQLiteOwnershipStore) HasActiveAttemptForLineage(ctx context.Context, lineageID string) (bool, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT 1 FROM fmp_attempts
		 WHERE lineage_id = ?
		   AND state IN ('HANDOFF_OFFERED', 'HANDOFF_ACCEPTED', 'RESUME_PENDING')
		   AND fenced = 0
		 LIMIT 1`,
		lineageID)
	return recordExists(ctx, row), nil
}

// ListActiveAttemptsByLineage returns all non-terminal, non-fenced attempts for the lineage.
func (s *SQLiteOwnershipStore) ListActiveAttemptsByLineage(ctx context.Context, lineageID string) ([]core.AttemptRecord, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT attempt_id, lineage_id, runtime_id, state, lease_id, lease_expiry,
		        start_time, last_progress_time, fenced, fencing_epoch, previous_attempt_id
		 FROM fmp_attempts
		 WHERE lineage_id = ?
		   AND state NOT IN ('COMPLETED', 'FAILED', 'ORPHANED', 'COMMITTED_REMOTE', 'FENCED')
		   AND fenced = 0
		 ORDER BY start_time DESC`,
		lineageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []core.AttemptRecord
	for rows.Next() {
		var attempt core.AttemptRecord
		var leaseIDStr, leaseExpiryStr, startTimeStr, lastProgressStr, prevAttemptStr string
		if err := rows.Scan(&attempt.AttemptID, &attempt.LineageID, &attempt.RuntimeID, &attempt.State,
			&leaseIDStr, &leaseExpiryStr, &startTimeStr, &lastProgressStr, &attempt.Fenced,
			&attempt.FencingEpoch, &prevAttemptStr); err != nil {
			return nil, err
		}
		attempt.LeaseID = leaseIDStr
		if leaseExpiryStr != "" {
			if t, err := time.Parse(time.RFC3339Nano, leaseExpiryStr); err == nil {
				attempt.LeaseExpiry = t
			}
		}
		if startTimeStr != "" {
			if t, err := time.Parse(time.RFC3339Nano, startTimeStr); err == nil {
				attempt.StartTime = t
			}
		}
		if lastProgressStr != "" {
			if t, err := time.Parse(time.RFC3339Nano, lastProgressStr); err == nil {
				attempt.LastProgressTime = t
			}
		}
		attempt.PreviousAttemptID = prevAttemptStr
		out = append(out, attempt)
	}
	return out, rows.Err()
}

// ListExpiredLeases returns all leases that have expired.
func (s *SQLiteOwnershipStore) ListExpiredLeases(ctx context.Context, now time.Time) ([]core.LeaseToken, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT lineage_id, lease_id, attempt_id, issuer, issued_at, expiry, fencing_epoch
		 FROM fmp_active_leases
		 WHERE expiry < ?`,
		now.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []core.LeaseToken
	for rows.Next() {
		var token core.LeaseToken
		var issuedStr, expiryStr string
		if err := rows.Scan(&token.LineageID, &token.LeaseID, &token.AttemptID, &token.Issuer,
			&issuedStr, &expiryStr, &token.FencingEpoch); err != nil {
			return nil, err
		}
		if issuedStr != "" {
			if t, err := time.Parse(time.RFC3339Nano, issuedStr); err == nil {
				token.IssuedAt = t
			}
		}
		if expiryStr != "" {
			if t, err := time.Parse(time.RFC3339Nano, expiryStr); err == nil {
				token.Expiry = t
			}
		}
		out = append(out, token)
	}
	return out, rows.Err()
}

// HasCommitForLineage returns true if there is a commit record for the lineage.
func (s *SQLiteOwnershipStore) HasCommitForLineage(ctx context.Context, lineageID string) (bool, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT 1 FROM fmp_resume_commits WHERE lineage_id = ? LIMIT 1`,
		lineageID)
	return recordExists(ctx, row), nil
}

func rollback(tx *sql.Tx) {
	_ = tx.Rollback()
}

func recordExists(_ context.Context, row *sql.Row) bool {
	var marker int
	return row.Scan(&marker) == nil
}

func zeroToNow(value time.Time, now func() time.Time) time.Time {
	if value.IsZero() {
		return now()
	}
	return value.UTC()
}
