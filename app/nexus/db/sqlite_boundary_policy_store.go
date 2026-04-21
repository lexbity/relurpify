package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/core"
	_ "github.com/mattn/go-sqlite3"
)

type SQLiteBoundaryPolicyStore struct {
	db *sql.DB
}

func NewSQLiteBoundaryPolicyStore(path string) (*SQLiteBoundaryPolicyStore, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("boundary policy store path required")
	}
	dsn := fmt.Sprintf("file:%s?_foreign_keys=on&_busy_timeout=5000", filepath.Clean(path))
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}
	store := &SQLiteBoundaryPolicyStore{db: db}
	if err := store.init(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *SQLiteBoundaryPolicyStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *SQLiteBoundaryPolicyStore) init() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS fmp_boundary_policies (
		trust_domain TEXT PRIMARY KEY,
		accepted_source_domains_json TEXT NOT NULL DEFAULT '[]',
		accepted_source_identities_json TEXT NOT NULL DEFAULT '[]',
		allowed_route_modes_json TEXT NOT NULL DEFAULT '[]',
		require_gateway_authentication INTEGER NOT NULL DEFAULT 0,
		allow_mediation INTEGER NOT NULL DEFAULT 0,
		max_transfer_bytes INTEGER NOT NULL DEFAULT 0,
		max_retries INTEGER NOT NULL DEFAULT 0,
		retry_backoff_seconds INTEGER NOT NULL DEFAULT 0
	);`)
	return err
}

func (s *SQLiteBoundaryPolicyStore) ListBoundaryPolicies(ctx context.Context) ([]core.BoundaryPolicy, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT trust_domain, accepted_source_domains_json, accepted_source_identities_json, allowed_route_modes_json, require_gateway_authentication, allow_mediation, max_transfer_bytes, max_retries, retry_backoff_seconds FROM fmp_boundary_policies ORDER BY trust_domain ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []core.BoundaryPolicy
	for rows.Next() {
		record, err := scanBoundaryPolicy(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *record)
	}
	return out, rows.Err()
}

func (s *SQLiteBoundaryPolicyStore) UpsertBoundaryPolicy(ctx context.Context, policy core.BoundaryPolicy) error {
	if err := policy.Validate(); err != nil {
		return err
	}
	sourceDomains, err := json.Marshal(policy.AcceptedSourceDomains)
	if err != nil {
		return err
	}
	sourceIDs, err := json.Marshal(policy.AcceptedSourceIdentities)
	if err != nil {
		return err
	}
	routeModes, err := json.Marshal(policy.AllowedRouteModes)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO fmp_boundary_policies (
		trust_domain, accepted_source_domains_json, accepted_source_identities_json, allowed_route_modes_json,
		require_gateway_authentication, allow_mediation, max_transfer_bytes, max_retries, retry_backoff_seconds
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(trust_domain) DO UPDATE SET
		accepted_source_domains_json = excluded.accepted_source_domains_json,
		accepted_source_identities_json = excluded.accepted_source_identities_json,
		allowed_route_modes_json = excluded.allowed_route_modes_json,
		require_gateway_authentication = excluded.require_gateway_authentication,
		allow_mediation = excluded.allow_mediation,
		max_transfer_bytes = excluded.max_transfer_bytes,
		max_retries = excluded.max_retries,
		retry_backoff_seconds = excluded.retry_backoff_seconds`,
		policy.TrustDomain,
		string(sourceDomains),
		string(sourceIDs),
		string(routeModes),
		boolToInt(policy.RequireGatewayAuthentication),
		boolToInt(policy.AllowMediation),
		policy.MaxTransferBytes,
		policy.MaxRetries,
		policy.RetryBackoffSeconds,
	)
	return err
}

func (s *SQLiteBoundaryPolicyStore) GetBoundaryPolicy(ctx context.Context, trustDomain string) (*core.BoundaryPolicy, error) {
	row := s.db.QueryRowContext(ctx, `SELECT trust_domain, accepted_source_domains_json, accepted_source_identities_json, allowed_route_modes_json, require_gateway_authentication, allow_mediation, max_transfer_bytes, max_retries, retry_backoff_seconds FROM fmp_boundary_policies WHERE trust_domain = ?`, strings.TrimSpace(trustDomain))
	record, err := scanBoundaryPolicy(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return record, nil
}

func scanBoundaryPolicy(scanner interface{ Scan(dest ...any) error }) (*core.BoundaryPolicy, error) {
	var (
		record                core.BoundaryPolicy
		sourceDomainsJSON     string
		sourceIdentitiesJSON  string
		allowedRouteModesJSON string
		requireGatewayAuth    int
		allowMediation        int
	)
	if err := scanner.Scan(&record.TrustDomain, &sourceDomainsJSON, &sourceIdentitiesJSON, &allowedRouteModesJSON, &requireGatewayAuth, &allowMediation, &record.MaxTransferBytes, &record.MaxRetries, &record.RetryBackoffSeconds); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(sourceDomainsJSON), &record.AcceptedSourceDomains); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(sourceIdentitiesJSON), &record.AcceptedSourceIdentities); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(allowedRouteModesJSON), &record.AllowedRouteModes); err != nil {
		return nil, err
	}
	record.RequireGatewayAuthentication = requireGatewayAuth != 0
	record.AllowMediation = allowMediation != 0
	return &record, nil
}
