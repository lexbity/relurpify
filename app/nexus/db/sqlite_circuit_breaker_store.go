package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	fwfmp "github.com/lexcodex/relurpify/framework/middleware/fmp"
	_ "github.com/mattn/go-sqlite3"
)

var _ fwfmp.CircuitBreakerStore = (*SQLiteCircuitBreakerStore)(nil)

// SQLiteCircuitBreakerStore provides persistent circuit breaker state storage per trust domain.
type SQLiteCircuitBreakerStore struct {
	db *sql.DB
}

// NewSQLiteCircuitBreakerStore creates or opens a SQLite store for circuit breaker state.
func NewSQLiteCircuitBreakerStore(path string) (*SQLiteCircuitBreakerStore, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("circuit breaker store path required")
	}
	dsn := fmt.Sprintf("file:%s?_foreign_keys=on&_busy_timeout=5000", filepath.Clean(path))
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}
	store := &SQLiteCircuitBreakerStore{db: db}
	if err := store.init(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

// Close closes the database connection.
func (s *SQLiteCircuitBreakerStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *SQLiteCircuitBreakerStore) init() error {
	stmt := `CREATE TABLE IF NOT EXISTS fmp_circuit_breakers (
		trust_domain TEXT PRIMARY KEY,
		state TEXT NOT NULL DEFAULT 'closed',
		requests INTEGER NOT NULL DEFAULT 0,
		failures INTEGER NOT NULL DEFAULT 0,
		window_started_at TEXT NOT NULL DEFAULT '',
		tripped_at TEXT NOT NULL DEFAULT '',
		recovery_at TEXT NOT NULL DEFAULT '',
		config_json TEXT NOT NULL DEFAULT '{}'
	);`
	if _, err := s.db.Exec(stmt); err != nil {
		return err
	}
	_, err := s.db.Exec(`ALTER TABLE fmp_circuit_breakers ADD COLUMN window_started_at TEXT NOT NULL DEFAULT ''`)
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
		return err
	}
	return nil
}

// GetState retrieves the current circuit state for a trust domain.
func (s *SQLiteCircuitBreakerStore) GetState(ctx context.Context, trustDomain string) (fwfmp.CircuitState, error) {
	trustDomain = strings.TrimSpace(trustDomain)
	if trustDomain == "" {
		return "", fmt.Errorf("trust domain required")
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT state, recovery_at FROM fmp_circuit_breakers WHERE trust_domain = ?`,
		trustDomain)
	var state, recoveryAt string
	if err := row.Scan(&state, &recoveryAt); err != nil {
		if err == sql.ErrNoRows {
			return fwfmp.CircuitClosed, nil
		}
		return "", err
	}
	if state == string(fwfmp.CircuitOpen) && recoveryAt != "" {
		if t, err := time.Parse(time.RFC3339, recoveryAt); err == nil && !time.Now().UTC().Before(t) {
			if _, err := s.db.ExecContext(ctx,
				`UPDATE fmp_circuit_breakers SET state = ? WHERE trust_domain = ?`,
				string(fwfmp.CircuitHalfOpen), trustDomain); err != nil {
				return "", err
			}
			return fwfmp.CircuitHalfOpen, nil
		}
	}
	return fwfmp.CircuitState(state), nil
}

// RecordSuccess records a successful operation for the trust domain.
func (s *SQLiteCircuitBreakerStore) RecordSuccess(ctx context.Context, trustDomain string, now time.Time) error {
	trustDomain = strings.TrimSpace(trustDomain)
	if trustDomain == "" {
		return fmt.Errorf("trust domain required")
	}

	// Use transaction to ensure atomic state transitions
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var state string
	var recoveryAt string
	var configJSON string
	err = tx.QueryRowContext(ctx, `SELECT state, recovery_at, config_json FROM fmp_circuit_breakers WHERE trust_domain = ?`, trustDomain).Scan(&state, &recoveryAt, &configJSON)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	cfg := decodeCircuitBreakerConfig(trustDomain, configJSON)
	requests, failures, windowStartedAt, err := s.loadAndRollWindow(ctx, tx, trustDomain, cfg, now)
	if err != nil {
		return err
	}
	_ = requests
	_ = failures
	if state == string(fwfmp.CircuitOpen) && recoveryAt != "" {
		if t, err := time.Parse(time.RFC3339, recoveryAt); err == nil && !now.Before(t) {
			state = string(fwfmp.CircuitHalfOpen)
		}
	}

	// If state is half-open, transition to closed
	if state == string(fwfmp.CircuitHalfOpen) {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO fmp_circuit_breakers (trust_domain, state, requests, failures, window_started_at, tripped_at, recovery_at, config_json)
			 VALUES (?, ?, 0, 0, ?, '', '', ?)
			 ON CONFLICT(trust_domain) DO UPDATE SET
			   state = 'closed',
			   requests = 0,
			   failures = 0,
			   window_started_at = excluded.window_started_at,
			   tripped_at = '',
			   recovery_at = '',
			   config_json = excluded.config_json`,
			trustDomain, string(fwfmp.CircuitHalfOpen), windowStartedAt.Format(time.RFC3339), encodeCircuitBreakerConfig(cfg))
		if err != nil {
			return err
		}
	} else if state == string(fwfmp.CircuitClosed) || state == "" {
		// Increment request count for closed state
		_, err := tx.ExecContext(ctx,
			`INSERT INTO fmp_circuit_breakers (trust_domain, state, requests, failures, window_started_at, tripped_at, recovery_at, config_json)
			 VALUES (?, 'closed', 1, 0, ?, '', '', ?)
			 ON CONFLICT(trust_domain) DO UPDATE SET
			   state = 'closed',
			   requests = requests + 1,
			   window_started_at = excluded.window_started_at,
			   config_json = excluded.config_json`,
			trustDomain, windowStartedAt.Format(time.RFC3339), encodeCircuitBreakerConfig(cfg))
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// RecordFailure records a failed operation, potentially tripping the breaker.
func (s *SQLiteCircuitBreakerStore) RecordFailure(ctx context.Context, trustDomain string, now time.Time) error {
	trustDomain = strings.TrimSpace(trustDomain)
	if trustDomain == "" {
		return fmt.Errorf("trust domain required")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Get current config or use defaults
	var configJSON string
	err = tx.QueryRowContext(ctx, `SELECT config_json FROM fmp_circuit_breakers WHERE trust_domain = ?`, trustDomain).Scan(&configJSON)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	cfg := decodeCircuitBreakerConfig(trustDomain, configJSON)
	requests, failures, windowStartedAt, err := s.loadAndRollWindow(ctx, tx, trustDomain, cfg, now)
	if err != nil {
		return err
	}

	// Increment failures and requests
	_, err = tx.ExecContext(ctx,
		`INSERT INTO fmp_circuit_breakers (trust_domain, state, requests, failures, window_started_at, config_json)
		 VALUES (?, 'closed', 1, 1, ?, ?)
		 ON CONFLICT(trust_domain) DO UPDATE SET
		   state = 'closed',
		   requests = requests + 1,
		   failures = failures + 1,
		   window_started_at = excluded.window_started_at,
		   config_json = excluded.config_json`,
		trustDomain, windowStartedAt.Format(time.RFC3339), encodeCircuitBreakerConfig(cfg))
	if err != nil {
		return err
	}

	requests++
	failures++

	if requests >= cfg.MinRequests {
		errorRate := float64(failures) / float64(requests)
		if errorRate >= cfg.ErrorThreshold {
			trippedAt := now.Format(time.RFC3339)
			recoveryAt := now.Add(cfg.RecoveryDuration).Format(time.RFC3339)
			_, err := tx.ExecContext(ctx,
				`UPDATE fmp_circuit_breakers SET state = 'open', tripped_at = ?, recovery_at = ? WHERE trust_domain = ?`,
				trippedAt, recoveryAt, trustDomain)
			if err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

// Trip manually opens the circuit breaker.
func (s *SQLiteCircuitBreakerStore) Trip(ctx context.Context, trustDomain string, now time.Time) error {
	trustDomain = strings.TrimSpace(trustDomain)
	if trustDomain == "" {
		return fmt.Errorf("trust domain required")
	}

	// Get config for recovery duration
	var configJSON string
	err := s.db.QueryRowContext(ctx, `SELECT config_json FROM fmp_circuit_breakers WHERE trust_domain = ?`, trustDomain).Scan(&configJSON)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	cfg := decodeCircuitBreakerConfig(trustDomain, configJSON)

	trippedAt := now.Format(time.RFC3339)
	recoveryAt := now.Add(cfg.RecoveryDuration).Format(time.RFC3339)

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO fmp_circuit_breakers (trust_domain, state, window_started_at, tripped_at, recovery_at, config_json)
		 VALUES (?, 'open', ?, ?, ?, ?)
		 ON CONFLICT(trust_domain) DO UPDATE SET
		   state = 'open',
		   window_started_at = ?,
		   tripped_at = ?,
		   recovery_at = ?`,
		trustDomain, now.Format(time.RFC3339), trippedAt, recoveryAt, encodeCircuitBreakerConfig(cfg), now.Format(time.RFC3339), trippedAt, recoveryAt)
	return err
}

// Reset closes the circuit breaker and clears failure counts.
func (s *SQLiteCircuitBreakerStore) Reset(ctx context.Context, trustDomain string, now time.Time) error {
	trustDomain = strings.TrimSpace(trustDomain)
	if trustDomain == "" {
		return fmt.Errorf("trust domain required")
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO fmp_circuit_breakers (trust_domain, state, requests, failures, window_started_at, tripped_at, recovery_at)
		 VALUES (?, 'closed', 0, 0, ?, '', '')
		 ON CONFLICT(trust_domain) DO UPDATE SET
		   state = 'closed',
		   requests = 0,
		   failures = 0,
		   window_started_at = excluded.window_started_at,
		   tripped_at = '',
		   recovery_at = ''`,
		trustDomain, now.Format(time.RFC3339))
	return err
}

func (s *SQLiteCircuitBreakerStore) loadAndRollWindow(ctx context.Context, tx *sql.Tx, trustDomain string, cfg fwfmp.CircuitBreakerConfig, now time.Time) (int, int, time.Time, error) {
	var requests, failures int
	var windowStartedAt string
	err := tx.QueryRowContext(ctx,
		`SELECT requests, failures, window_started_at FROM fmp_circuit_breakers WHERE trust_domain = ?`,
		trustDomain,
	).Scan(&requests, &failures, &windowStartedAt)
	if err != nil && err != sql.ErrNoRows {
		return 0, 0, time.Time{}, err
	}
	windowStart := now
	if parsed, ok := parseCircuitBreakerTime(windowStartedAt); ok {
		windowStart = parsed
	}
	if cfg.WindowDuration > 0 && now.Sub(windowStart) >= cfg.WindowDuration {
		requests = 0
		failures = 0
		windowStart = now
		if _, err := tx.ExecContext(ctx,
			`UPDATE fmp_circuit_breakers SET requests = 0, failures = 0, window_started_at = ? WHERE trust_domain = ?`,
			windowStart.Format(time.RFC3339), trustDomain); err != nil {
			return 0, 0, time.Time{}, err
		}
	}
	return requests, failures, windowStart, nil
}

func decodeCircuitBreakerConfig(trustDomain, raw string) fwfmp.CircuitBreakerConfig {
	cfg := fwfmp.CircuitBreakerConfig{
		TrustDomain:      trustDomain,
		ErrorThreshold:   0.5,
		MinRequests:      10,
		WindowDuration:   time.Minute,
		RecoveryDuration: 30 * time.Second,
	}
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &cfg); err == nil {
			cfg.TrustDomain = trustDomain
		}
	}
	if cfg.MinRequests < 1 {
		cfg.MinRequests = 10
	}
	if cfg.WindowDuration <= 0 {
		cfg.WindowDuration = time.Minute
	}
	if cfg.RecoveryDuration <= 0 {
		cfg.RecoveryDuration = 30 * time.Second
	}
	return cfg
}

func encodeCircuitBreakerConfig(cfg fwfmp.CircuitBreakerConfig) string {
	payload, err := json.Marshal(cfg)
	if err != nil {
		return "{}"
	}
	return string(payload)
}

func parseCircuitBreakerTime(value string) (time.Time, bool) {
	if strings.TrimSpace(value) == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}

// ListStates returns the status of all circuit breakers.
func (s *SQLiteCircuitBreakerStore) ListStates(ctx context.Context) ([]fwfmp.CircuitBreakerStatus, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT trust_domain, state, requests, failures, tripped_at, recovery_at FROM fmp_circuit_breakers ORDER BY trust_domain`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []fwfmp.CircuitBreakerStatus
	for rows.Next() {
		var domain, state, trippedAtStr, recoveryAtStr string
		var requests, failures int
		if err := rows.Scan(&domain, &state, &requests, &failures, &trippedAtStr, &recoveryAtStr); err != nil {
			return nil, err
		}

		errorRate := 0.0
		if requests > 0 {
			errorRate = float64(failures) / float64(requests)
		}

		status := fwfmp.CircuitBreakerStatus{
			TrustDomain: domain,
			State:       fwfmp.CircuitState(state),
			ErrorRate:   errorRate,
			Requests:    requests,
		}

		if trippedAtStr != "" {
			if t, err := time.Parse(time.RFC3339, trippedAtStr); err == nil {
				status.TrippedAt = &t
			}
		}
		if recoveryAtStr != "" {
			if t, err := time.Parse(time.RFC3339, recoveryAtStr); err == nil {
				status.RecoveryAt = &t
			}
		}

		out = append(out, status)
	}
	return out, rows.Err()
}

// SetConfig sets or updates the configuration for a trust domain.
func (s *SQLiteCircuitBreakerStore) SetConfig(ctx context.Context, cfg fwfmp.CircuitBreakerConfig) error {
	if cfg.TrustDomain == "" {
		return fmt.Errorf("trust domain required")
	}
	if cfg.ErrorThreshold < 0 || cfg.ErrorThreshold > 1 {
		return fmt.Errorf("error threshold must be 0.0-1.0")
	}
	if cfg.MinRequests < 1 {
		cfg.MinRequests = 10
	}
	if cfg.WindowDuration <= 0 {
		cfg.WindowDuration = 1 * time.Minute
	}
	if cfg.RecoveryDuration <= 0 {
		cfg.RecoveryDuration = 30 * time.Second
	}

	configJSON, err := json.Marshal(cfg)
	if err != nil {
		return err
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO fmp_circuit_breakers (trust_domain, config_json) VALUES (?, ?)
		 ON CONFLICT(trust_domain) DO UPDATE SET config_json = ?`,
		cfg.TrustDomain, configJSON, configJSON)
	return err
}
