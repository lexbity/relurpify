package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	fwfmp "github.com/lexcodex/relurpify/framework/middleware/fmp"
	_ "github.com/mattn/go-sqlite3"
)

var _ fwfmp.OperationalLimiter = (*SQLiteOperationalLimiter)(nil)

type SQLiteOperationalLimiter struct {
	db     *sql.DB
	Limits fwfmp.OperationalLimits
}

func NewSQLiteOperationalLimiter(path string, limits fwfmp.OperationalLimits) (*SQLiteOperationalLimiter, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("operational limits store path required")
	}
	dsn := fmt.Sprintf("file:%s?_foreign_keys=on&_busy_timeout=5000", filepath.Clean(path))
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}
	store := &SQLiteOperationalLimiter{db: db, Limits: limits}
	if err := store.init(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *SQLiteOperationalLimiter) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *SQLiteOperationalLimiter) init() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS fmp_operational_window (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			window_start TEXT NOT NULL,
			resume_bytes_window INTEGER NOT NULL DEFAULT 0,
			forward_bytes_window INTEGER NOT NULL DEFAULT 0,
			federated_forwards INTEGER NOT NULL DEFAULT 0
		);`,
		`CREATE TABLE IF NOT EXISTS fmp_active_resume_slots (
			slot_id TEXT PRIMARY KEY,
			size_bytes INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL
		);`,
		`INSERT OR IGNORE INTO fmp_operational_window (id, window_start, resume_bytes_window, forward_bytes_window, federated_forwards)
		 VALUES (1, '', 0, 0, 0);`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLiteOperationalLimiter) AcquireResume(ctx context.Context, slotID string, sizeBytes int64, now time.Time) (*core.TransferRefusal, error) {
	if strings.TrimSpace(slotID) == "" {
		return nil, fmt.Errorf("resume slot id required")
	}
	if sizeBytes < 0 {
		return nil, fmt.Errorf("resume size bytes must be >= 0")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	state, err := s.loadWindowTx(ctx, tx, now)
	if err != nil {
		return nil, err
	}
	existing, err := s.slotExistsTx(ctx, tx, slotID)
	if err != nil {
		return nil, err
	}
	if existing {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return nil, nil
	}
	activeCount, err := s.activeResumeCountTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	if s.Limits.MaxActiveResumeSlots > 0 && activeCount >= s.Limits.MaxActiveResumeSlots {
		return &core.TransferRefusal{Code: core.RefusalDestinationBusy, Message: "resume slot limit reached"}, nil
	}
	if s.Limits.MaxResumeBytesWindow > 0 && state.ResumeBytesInWindow+sizeBytes > s.Limits.MaxResumeBytesWindow {
		return &core.TransferRefusal{Code: core.RefusalTransferBudget, Message: "resume bandwidth limit reached"}, nil
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO fmp_active_resume_slots (slot_id, size_bytes, created_at) VALUES (?, ?, ?)`,
		strings.TrimSpace(slotID), sizeBytes, now.UTC().Format(time.RFC3339Nano)); err != nil {
		return nil, err
	}
	state.ResumeBytesInWindow += sizeBytes
	if err := s.saveWindowTx(ctx, tx, state); err != nil {
		return nil, err
	}
	return nil, tx.Commit()
}

func (s *SQLiteOperationalLimiter) ReleaseResume(ctx context.Context, slotID string) error {
	if strings.TrimSpace(slotID) == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM fmp_active_resume_slots WHERE slot_id = ?`, strings.TrimSpace(slotID))
	return err
}

func (s *SQLiteOperationalLimiter) AllowForward(ctx context.Context, transferID string, sizeBytes int64, now time.Time) (*core.TransferRefusal, error) {
	if strings.TrimSpace(transferID) == "" {
		return nil, fmt.Errorf("transfer id required")
	}
	if sizeBytes < 0 {
		return nil, fmt.Errorf("forward size bytes must be >= 0")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	state, err := s.loadWindowTx(ctx, tx, now)
	if err != nil {
		return nil, err
	}
	if s.Limits.MaxForwardBytesWindow > 0 && state.ForwardBytesInWindow+sizeBytes > s.Limits.MaxForwardBytesWindow {
		return &core.TransferRefusal{Code: core.RefusalTransferBudget, Message: "forward bandwidth limit reached"}, nil
	}
	if s.Limits.MaxFederatedForwards > 0 && state.FederatedForwards >= s.Limits.MaxFederatedForwards {
		return &core.TransferRefusal{Code: core.RefusalDestinationBusy, Message: "federated forward limit reached"}, nil
	}
	state.ForwardBytesInWindow += sizeBytes
	state.FederatedForwards++
	if err := s.saveWindowTx(ctx, tx, state); err != nil {
		return nil, err
	}
	return nil, tx.Commit()
}

func (s *SQLiteOperationalLimiter) Snapshot(ctx context.Context, now time.Time) (fwfmp.OperationalSnapshot, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fwfmp.OperationalSnapshot{}, err
	}
	defer func() { _ = tx.Rollback() }()
	state, err := s.loadWindowTx(ctx, tx, now)
	if err != nil {
		return fwfmp.OperationalSnapshot{}, err
	}
	activeCount, err := s.activeResumeCountTx(ctx, tx)
	if err != nil {
		return fwfmp.OperationalSnapshot{}, err
	}
	state.ActiveResumeSlots = activeCount
	if err := tx.Commit(); err != nil {
		return fwfmp.OperationalSnapshot{}, err
	}
	return state, nil
}

func (s *SQLiteOperationalLimiter) loadWindowTx(ctx context.Context, tx *sql.Tx, now time.Time) (fwfmp.OperationalSnapshot, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	var (
		windowStart        string
		resumeBytesWindow  int64
		forwardBytesWindow int64
		federatedForwards  int
	)
	if err := tx.QueryRowContext(ctx, `SELECT window_start, resume_bytes_window, forward_bytes_window, federated_forwards FROM fmp_operational_window WHERE id = 1`).
		Scan(&windowStart, &resumeBytesWindow, &forwardBytesWindow, &federatedForwards); err != nil {
		return fwfmp.OperationalSnapshot{}, err
	}
	start, err := parseOptionalTime(windowStart)
	if err != nil {
		return fwfmp.OperationalSnapshot{}, err
	}
	window := s.Limits.Window
	if window <= 0 {
		window = time.Minute
	}
	state := fwfmp.OperationalSnapshot{
		WindowStart:          start,
		ResumeBytesInWindow:  resumeBytesWindow,
		ForwardBytesInWindow: forwardBytesWindow,
		FederatedForwards:    federatedForwards,
	}
	if state.WindowStart.IsZero() || now.Sub(state.WindowStart) >= window {
		state.WindowStart = now.UTC()
		state.ResumeBytesInWindow = 0
		state.ForwardBytesInWindow = 0
		state.FederatedForwards = 0
		if _, err := tx.ExecContext(ctx, `DELETE FROM fmp_active_resume_slots`); err != nil {
			return fwfmp.OperationalSnapshot{}, err
		}
		if err := s.saveWindowTx(ctx, tx, state); err != nil {
			return fwfmp.OperationalSnapshot{}, err
		}
	}
	return state, nil
}

func (s *SQLiteOperationalLimiter) saveWindowTx(ctx context.Context, tx *sql.Tx, state fwfmp.OperationalSnapshot) error {
	_, err := tx.ExecContext(ctx, `UPDATE fmp_operational_window
		SET window_start = ?, resume_bytes_window = ?, forward_bytes_window = ?, federated_forwards = ?
		WHERE id = 1`,
		state.WindowStart.UTC().Format(time.RFC3339Nano),
		state.ResumeBytesInWindow,
		state.ForwardBytesInWindow,
		state.FederatedForwards,
	)
	return err
}

func (s *SQLiteOperationalLimiter) activeResumeCountTx(ctx context.Context, tx *sql.Tx) (int, error) {
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM fmp_active_resume_slots`).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *SQLiteOperationalLimiter) slotExistsTx(ctx context.Context, tx *sql.Tx, slotID string) (bool, error) {
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM fmp_active_resume_slots WHERE slot_id = ?`, strings.TrimSpace(slotID)).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}
