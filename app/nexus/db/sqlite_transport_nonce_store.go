package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type SQLiteTransportNonceStore struct {
	db  *sql.DB
	Now func() time.Time
}

func NewSQLiteTransportNonceStore(path string) (*SQLiteTransportNonceStore, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("transport nonce store path required")
	}
	dsn := fmt.Sprintf("file:%s?_foreign_keys=on&_busy_timeout=5000", filepath.Clean(path))
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}
	store := &SQLiteTransportNonceStore{db: db}
	if err := store.init(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *SQLiteTransportNonceStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *SQLiteTransportNonceStore) init() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS gateway_transport_nonces (
		scope TEXT NOT NULL,
		nonce TEXT NOT NULL,
		expires_at TEXT NOT NULL,
		PRIMARY KEY (scope, nonce)
	);`)
	return err
}

func (s *SQLiteTransportNonceStore) Reserve(ctx context.Context, scope, nonce string, expiresAt time.Time) error {
	scope = strings.TrimSpace(scope)
	nonce = strings.TrimSpace(nonce)
	if scope == "" {
		return fmt.Errorf("transport nonce scope required")
	}
	if nonce == "" {
		return fmt.Errorf("transport nonce required")
	}
	now := time.Now().UTC()
	if s != nil && s.Now != nil {
		now = s.Now().UTC()
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM gateway_transport_nonces WHERE expires_at != '' AND expires_at <= ?`, now.Format(time.RFC3339Nano)); err != nil {
		return err
	}
	var existing string
	err = tx.QueryRowContext(ctx, `SELECT nonce FROM gateway_transport_nonces WHERE scope = ? AND nonce = ?`, scope, nonce).Scan(&existing)
	if err == nil {
		return fmt.Errorf("transport nonce replay detected")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO gateway_transport_nonces (scope, nonce, expires_at) VALUES (?, ?, ?)`,
		scope, nonce, expiresAt.UTC().Format(time.RFC3339Nano)); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return fmt.Errorf("transport nonce replay detected")
		}
		return err
	}
	return tx.Commit()
}
