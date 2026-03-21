package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/app/nexus/adminapi"
	_ "github.com/mattn/go-sqlite3"
)

type SQLiteFMPExportStore struct {
	db *sql.DB
}

func NewSQLiteFMPExportStore(path string) (*SQLiteFMPExportStore, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("fmp export store path required")
	}
	dsn := fmt.Sprintf("file:%s?_foreign_keys=on&_busy_timeout=5000", filepath.Clean(path))
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}
	store := &SQLiteFMPExportStore{db: db}
	if err := store.init(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *SQLiteFMPExportStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *SQLiteFMPExportStore) init() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS tenant_fmp_exports (
		tenant_id TEXT NOT NULL,
		export_name TEXT NOT NULL,
		enabled INTEGER NOT NULL,
		updated_at TEXT NOT NULL,
		PRIMARY KEY (tenant_id, export_name)
	);`)
	return err
}

func (s *SQLiteFMPExportStore) ListTenantExports(ctx context.Context, tenantID string) ([]adminapi.TenantFMPExportInfo, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT tenant_id, export_name, enabled, updated_at FROM tenant_fmp_exports WHERE tenant_id = ? ORDER BY export_name ASC`, strings.TrimSpace(tenantID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]adminapi.TenantFMPExportInfo, 0)
	for rows.Next() {
		var (
			record    adminapi.TenantFMPExportInfo
			enabled   int
			updatedAt string
		)
		if err := rows.Scan(&record.TenantID, &record.ExportName, &enabled, &updatedAt); err != nil {
			return nil, err
		}
		record.Enabled = enabled != 0
		record.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
		if err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	return out, rows.Err()
}

func (s *SQLiteFMPExportStore) SetTenantExportEnabled(ctx context.Context, tenantID, exportName string, enabled bool) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO tenant_fmp_exports (tenant_id, export_name, enabled, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(tenant_id, export_name) DO UPDATE SET
			enabled = excluded.enabled,
			updated_at = excluded.updated_at`,
		strings.TrimSpace(tenantID),
		strings.TrimSpace(exportName),
		boolToInt(enabled),
		time.Now().UTC().Format(time.RFC3339Nano),
	)
	return err
}

func (s *SQLiteFMPExportStore) IsExportEnabled(ctx context.Context, tenantID, exportName string) (bool, bool, error) {
	row := s.db.QueryRowContext(ctx, `SELECT enabled FROM tenant_fmp_exports WHERE tenant_id = ? AND export_name = ?`, strings.TrimSpace(tenantID), strings.TrimSpace(exportName))
	var enabled int
	err := row.Scan(&enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	return enabled != 0, true, nil
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
