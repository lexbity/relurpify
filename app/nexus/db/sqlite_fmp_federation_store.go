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

	"codeburg.org/lexbit/relurpify/framework/core"
	_ "github.com/mattn/go-sqlite3"
)

type SQLiteFMPFederationStore struct {
	db *sql.DB
}

func NewSQLiteFMPFederationStore(path string) (*SQLiteFMPFederationStore, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("fmp federation store path required")
	}
	dsn := fmt.Sprintf("file:%s?_foreign_keys=on&_busy_timeout=5000", filepath.Clean(path))
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}
	store := &SQLiteFMPFederationStore{db: db}
	if err := store.init(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *SQLiteFMPFederationStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *SQLiteFMPFederationStore) init() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS tenant_fmp_federation_policies (
		tenant_id TEXT PRIMARY KEY,
		allowed_trust_domains_json TEXT NOT NULL DEFAULT '[]',
		allowed_route_modes_json TEXT NOT NULL DEFAULT '[]',
		allow_mediation INTEGER NOT NULL DEFAULT 0,
		max_transfer_bytes INTEGER NOT NULL DEFAULT 0,
		updated_at TEXT NOT NULL
	);`)
	if err != nil {
		return err
	}
	if _, err := s.db.Exec(`ALTER TABLE tenant_fmp_federation_policies ADD COLUMN allowed_route_modes_json TEXT NOT NULL DEFAULT '[]'`); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
		return err
	}
	if _, err := s.db.Exec(`ALTER TABLE tenant_fmp_federation_policies ADD COLUMN max_transfer_bytes INTEGER NOT NULL DEFAULT 0`); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
		return err
	}
	return nil
}

func (s *SQLiteFMPFederationStore) GetTenantFederationPolicy(ctx context.Context, tenantID string) (*core.TenantFederationPolicy, error) {
	row := s.db.QueryRowContext(ctx, `SELECT tenant_id, allowed_trust_domains_json, allowed_route_modes_json, allow_mediation, max_transfer_bytes, updated_at FROM tenant_fmp_federation_policies WHERE tenant_id = ?`, strings.TrimSpace(tenantID))
	var (
		policy                  core.TenantFederationPolicy
		allowedTrustDomainsJSON string
		allowedRouteModesJSON   string
		allowMediation          int
		maxTransferBytes        int64
		updatedAt               string
	)
	err := row.Scan(&policy.TenantID, &allowedTrustDomainsJSON, &allowedRouteModesJSON, &allowMediation, &maxTransferBytes, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(allowedTrustDomainsJSON), &policy.AllowedTrustDomains); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(allowedRouteModesJSON), &policy.AllowedRouteModes); err != nil {
		return nil, err
	}
	policy.AllowMediation = allowMediation != 0
	policy.MaxTransferBytes = maxTransferBytes
	policy.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return nil, err
	}
	return &policy, nil
}

func (s *SQLiteFMPFederationStore) SetTenantFederationPolicy(ctx context.Context, policy core.TenantFederationPolicy) error {
	if err := policy.Validate(); err != nil {
		return err
	}
	if policy.UpdatedAt.IsZero() {
		policy.UpdatedAt = time.Now().UTC()
	}
	data, err := json.Marshal(policy.AllowedTrustDomains)
	if err != nil {
		return err
	}
	routeModeData, err := json.Marshal(policy.AllowedRouteModes)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO tenant_fmp_federation_policies (tenant_id, allowed_trust_domains_json, allowed_route_modes_json, allow_mediation, max_transfer_bytes, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(tenant_id) DO UPDATE SET
			allowed_trust_domains_json = excluded.allowed_trust_domains_json,
			allowed_route_modes_json = excluded.allowed_route_modes_json,
			allow_mediation = excluded.allow_mediation,
			max_transfer_bytes = excluded.max_transfer_bytes,
			updated_at = excluded.updated_at`,
		policy.TenantID,
		string(data),
		string(routeModeData),
		boolToInt(policy.AllowMediation),
		policy.MaxTransferBytes,
		policy.UpdatedAt.Format(time.RFC3339Nano),
	)
	return err
}
