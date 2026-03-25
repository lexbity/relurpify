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
		tripped_at TEXT NOT NULL DEFAULT '',
		recovery_at TEXT NOT NULL DEFAULT '',
		config_json TEXT NOT NULL DEFAULT '{}'
	);`
	_, err := s.db.Exec(stmt)
	return err
}

// GetState retrieves the current circuit state for a trust domain.
func (s *SQLiteCircuitBreakerStore) GetState(ctx context.Context, trustDomain string) (fwfmp.CircuitState, error) {
	trustDomain = strings.TrimSpace(trustDomain)
	if trustDomain == "" {
		return "", fmt.Errorf("trust domain required")
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT state FROM fmp_circuit_breakers WHERE trust_domain = ?`,
		trustDomain)
	var state string
	if err := row.Scan(&state); err != nil {
		if err == sql.ErrNoRows {
			return fwfmp.CircuitClosed, nil
		}
		return "", err
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
	err = tx.QueryRowContext(ctx, `SELECT state FROM fmp_circuit_breakers WHERE trust_domain = ?`, trustDomain).Scan(&state)
	if err != nil && err != sql.ErrNoRows {
		return err
	}

	// If state is half-open, transition to closed
	if state == string(fwfmp.CircuitHalfOpen) {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO fmp_circuit_breakers (trust_domain, state, requests, failures, tripped_at, recovery_at, config_json)
			 VALUES (?, ?, 0, 0, '', '', '{}')
			 ON CONFLICT(trust_domain) DO UPDATE SET
			   state = 'closed',
			   requests = 0,
			   failures = 0,
			   tripped_at = '',
			   recovery_at = ''`,
			trustDomain)
		if err != nil {
			return err
		}
	} else if state == string(fwfmp.CircuitClosed) || state == "" {
		// Increment request count for closed state
		_, err := tx.ExecContext(ctx,
			`INSERT INTO fmp_circuit_breakers (trust_domain, state, requests, failures, tripped_at, recovery_at, config_json)
			 VALUES (?, 'closed', 1, 0, '', '', '{}')
			 ON CONFLICT(trust_domain) DO UPDATE SET requests = requests + 1`,
			trustDomain)
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

	var cfg fwfmp.CircuitBreakerConfig
	if configJSON != "" {
		if err := json.Unmarshal([]byte(configJSON), &cfg); err == nil {
			cfg.TrustDomain = trustDomain
		} else {
			cfg = fwfmp.CircuitBreakerConfig{
				TrustDomain:      trustDomain,
				ErrorThreshold:   0.5,
				MinRequests:      10,
				WindowDuration:   1 * time.Minute,
				RecoveryDuration: 30 * time.Second,
			}
		}
	} else {
		cfg = fwfmp.CircuitBreakerConfig{
			TrustDomain:      trustDomain,
			ErrorThreshold:   0.5,
			MinRequests:      10,
			WindowDuration:   1 * time.Minute,
			RecoveryDuration: 30 * time.Second,
		}
	}

	// Increment failures and requests
	_, err = tx.ExecContext(ctx,
		`INSERT INTO fmp_circuit_breakers (trust_domain, state, requests, failures, config_json)
		 VALUES (?, 'closed', 1, 1, ?)
		 ON CONFLICT(trust_domain) DO UPDATE SET
		   requests = requests + 1,
		   failures = failures + 1`,
		trustDomain, configJSON)
	if err != nil {
		return err
	}

	// Check if threshold exceeded and trip if necessary
	var requests, failures int
	err = tx.QueryRowContext(ctx, `SELECT requests, failures FROM fmp_circuit_breakers WHERE trust_domain = ?`, trustDomain).Scan(&requests, &failures)
	if err != nil {
		return err
	}

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

	var cfg fwfmp.CircuitBreakerConfig
	if configJSON != "" {
		if err := json.Unmarshal([]byte(configJSON), &cfg); err == nil {
			cfg.TrustDomain = trustDomain
		} else {
			cfg = fwfmp.CircuitBreakerConfig{RecoveryDuration: 30 * time.Second}
		}
	} else {
		cfg = fwfmp.CircuitBreakerConfig{RecoveryDuration: 30 * time.Second}
	}

	trippedAt := now.Format(time.RFC3339)
	recoveryAt := now.Add(cfg.RecoveryDuration).Format(time.RFC3339)

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO fmp_circuit_breakers (trust_domain, state, tripped_at, recovery_at, config_json)
		 VALUES (?, 'open', ?, ?, ?)
		 ON CONFLICT(trust_domain) DO UPDATE SET
		   state = 'open',
		   tripped_at = ?,
		   recovery_at = ?`,
		trustDomain, trippedAt, recoveryAt, configJSON, trippedAt, recoveryAt)
	return err
}

// Reset closes the circuit breaker and clears failure counts.
func (s *SQLiteCircuitBreakerStore) Reset(ctx context.Context, trustDomain string, now time.Time) error {
	trustDomain = strings.TrimSpace(trustDomain)
	if trustDomain == "" {
		return fmt.Errorf("trust domain required")
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO fmp_circuit_breakers (trust_domain, state, requests, failures, tripped_at, recovery_at)
		 VALUES (?, 'closed', 0, 0, '', '')
		 ON CONFLICT(trust_domain) DO UPDATE SET
		   state = 'closed',
		   requests = 0,
		   failures = 0,
		   tripped_at = '',
		   recovery_at = ''`,
		trustDomain)
	return err
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
