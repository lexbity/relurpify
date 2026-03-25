package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	fwfmp "github.com/lexcodex/relurpify/framework/middleware/fmp"
	_ "github.com/mattn/go-sqlite3"
)

var _ fwfmp.CompatibilityWindowStore = (*SQLiteCompatibilityWindowStore)(nil)

// SQLiteCompatibilityWindowStore provides persistent version compatibility window storage.
type SQLiteCompatibilityWindowStore struct {
	db *sql.DB
}

// NewSQLiteCompatibilityWindowStore creates or opens a SQLite store for compatibility windows.
func NewSQLiteCompatibilityWindowStore(path string) (*SQLiteCompatibilityWindowStore, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("compatibility window store path required")
	}
	dsn := fmt.Sprintf("file:%s?_foreign_keys=on&_busy_timeout=5000", filepath.Clean(path))
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}
	store := &SQLiteCompatibilityWindowStore{db: db}
	if err := store.init(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

// Close closes the database connection.
func (s *SQLiteCompatibilityWindowStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *SQLiteCompatibilityWindowStore) init() error {
	stmt := `CREATE TABLE IF NOT EXISTS fmp_compatibility_windows (
		context_class TEXT PRIMARY KEY,
		min_schema_version TEXT NOT NULL DEFAULT '',
		max_schema_version TEXT NOT NULL DEFAULT '',
		min_runtime_version TEXT NOT NULL DEFAULT '',
		max_runtime_version TEXT NOT NULL DEFAULT ''
	);`
	_, err := s.db.Exec(stmt)
	return err
}

// GetWindow retrieves a compatibility window by context class.
func (s *SQLiteCompatibilityWindowStore) GetWindow(ctx context.Context, contextClass string) (*fwfmp.CompatibilityWindow, bool, error) {
	contextClass = strings.TrimSpace(contextClass)
	if contextClass == "" {
		return nil, false, fmt.Errorf("context class required")
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT context_class, min_schema_version, max_schema_version, min_runtime_version, max_runtime_version
		 FROM fmp_compatibility_windows WHERE context_class = ?`,
		contextClass)
	var w fwfmp.CompatibilityWindow
	if err := row.Scan(&w.ContextClass, &w.MinSchemaVersion, &w.MaxSchemaVersion, &w.MinRuntimeVersion, &w.MaxRuntimeVersion); err != nil {
		if err == sql.ErrNoRows {
			return nil, false, nil
		}
		return nil, false, err
	}
	return &w, true, nil
}

// UpsertWindow inserts or updates a compatibility window.
func (s *SQLiteCompatibilityWindowStore) UpsertWindow(ctx context.Context, window fwfmp.CompatibilityWindow) error {
	if window.ContextClass == "" {
		return fmt.Errorf("context class required")
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO fmp_compatibility_windows (context_class, min_schema_version, max_schema_version, min_runtime_version, max_runtime_version)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(context_class) DO UPDATE SET
		   min_schema_version = excluded.min_schema_version,
		   max_schema_version = excluded.max_schema_version,
		   min_runtime_version = excluded.min_runtime_version,
		   max_runtime_version = excluded.max_runtime_version`,
		window.ContextClass, window.MinSchemaVersion, window.MaxSchemaVersion, window.MinRuntimeVersion, window.MaxRuntimeVersion)
	return err
}

// ListWindows returns all compatibility windows.
func (s *SQLiteCompatibilityWindowStore) ListWindows(ctx context.Context) ([]fwfmp.CompatibilityWindow, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT context_class, min_schema_version, max_schema_version, min_runtime_version, max_runtime_version
		 FROM fmp_compatibility_windows ORDER BY context_class`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var windows []fwfmp.CompatibilityWindow
	for rows.Next() {
		var w fwfmp.CompatibilityWindow
		if err := rows.Scan(&w.ContextClass, &w.MinSchemaVersion, &w.MaxSchemaVersion, &w.MinRuntimeVersion, &w.MaxRuntimeVersion); err != nil {
			return nil, err
		}
		windows = append(windows, w)
	}
	return windows, rows.Err()
}

// DeleteWindow removes a compatibility window.
func (s *SQLiteCompatibilityWindowStore) DeleteWindow(ctx context.Context, contextClass string) error {
	contextClass = strings.TrimSpace(contextClass)
	if contextClass == "" {
		return fmt.Errorf("context class required")
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM fmp_compatibility_windows WHERE context_class = ?`, contextClass)
	return err
}
