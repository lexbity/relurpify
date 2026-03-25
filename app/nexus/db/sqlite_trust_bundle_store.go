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

	"github.com/lexcodex/relurpify/framework/core"
	_ "github.com/mattn/go-sqlite3"
)

type SQLiteTrustBundleStore struct {
	db *sql.DB
}

func NewSQLiteTrustBundleStore(path string) (*SQLiteTrustBundleStore, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("trust bundle store path required")
	}
	dsn := fmt.Sprintf("file:%s?_foreign_keys=on&_busy_timeout=5000", filepath.Clean(path))
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}
	store := &SQLiteTrustBundleStore{db: db}
	if err := store.init(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *SQLiteTrustBundleStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *SQLiteTrustBundleStore) init() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS fmp_trust_bundles (
		trust_domain TEXT PRIMARY KEY,
		bundle_id TEXT NOT NULL,
		gateway_identities_json TEXT NOT NULL DEFAULT '[]',
		trust_anchors_json TEXT NOT NULL DEFAULT '[]',
		recipient_keys_json TEXT NOT NULL DEFAULT '[]',
		issued_at TEXT NOT NULL,
		expires_at TEXT NOT NULL DEFAULT '',
		signature TEXT NOT NULL DEFAULT ''
	);`,
		`ALTER TABLE fmp_trust_bundles ADD COLUMN recipient_keys_json TEXT NOT NULL DEFAULT '[]';`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
			return err
		}
	}
	return nil
}

func (s *SQLiteTrustBundleStore) ListTrustBundles(ctx context.Context) ([]core.TrustBundle, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT trust_domain, bundle_id, gateway_identities_json, trust_anchors_json, recipient_keys_json, issued_at, expires_at, signature FROM fmp_trust_bundles ORDER BY trust_domain ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []core.TrustBundle
	for rows.Next() {
		record, err := scanTrustBundle(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *record)
	}
	return out, rows.Err()
}

func (s *SQLiteTrustBundleStore) UpsertTrustBundle(ctx context.Context, bundle core.TrustBundle) error {
	if err := bundle.Validate(); err != nil {
		return err
	}
	if bundle.IssuedAt.IsZero() {
		bundle.IssuedAt = time.Now().UTC()
	}
	ids, err := json.Marshal(bundle.GatewayIdentities)
	if err != nil {
		return err
	}
	anchors, err := json.Marshal(bundle.TrustAnchors)
	if err != nil {
		return err
	}
	recipientKeys, err := json.Marshal(bundle.RecipientKeys)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO fmp_trust_bundles (
		trust_domain, bundle_id, gateway_identities_json, trust_anchors_json, recipient_keys_json, issued_at, expires_at, signature
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(trust_domain) DO UPDATE SET
		bundle_id = excluded.bundle_id,
		gateway_identities_json = excluded.gateway_identities_json,
		trust_anchors_json = excluded.trust_anchors_json,
		recipient_keys_json = excluded.recipient_keys_json,
		issued_at = excluded.issued_at,
		expires_at = excluded.expires_at,
		signature = excluded.signature`,
		bundle.TrustDomain,
		bundle.BundleID,
		string(ids),
		string(anchors),
		string(recipientKeys),
		bundle.IssuedAt.Format(time.RFC3339Nano),
		formatOptionalTime(bundle.ExpiresAt),
		bundle.Signature,
	)
	return err
}

func (s *SQLiteTrustBundleStore) GetTrustBundle(ctx context.Context, trustDomain string) (*core.TrustBundle, error) {
	row := s.db.QueryRowContext(ctx, `SELECT trust_domain, bundle_id, gateway_identities_json, trust_anchors_json, recipient_keys_json, issued_at, expires_at, signature FROM fmp_trust_bundles WHERE trust_domain = ?`, strings.TrimSpace(trustDomain))
	record, err := scanTrustBundle(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return record, nil
}

func scanTrustBundle(scanner interface{ Scan(dest ...any) error }) (*core.TrustBundle, error) {
	var (
		record            core.TrustBundle
		identitiesJSON    string
		trustAnchorsJSON  string
		recipientKeysJSON string
		issuedAt          string
		expiresAt         string
	)
	if err := scanner.Scan(&record.TrustDomain, &record.BundleID, &identitiesJSON, &trustAnchorsJSON, &recipientKeysJSON, &issuedAt, &expiresAt, &record.Signature); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(identitiesJSON), &record.GatewayIdentities); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(trustAnchorsJSON), &record.TrustAnchors); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(recipientKeysJSON), &record.RecipientKeys); err != nil {
		return nil, err
	}
	var err error
	record.IssuedAt, err = time.Parse(time.RFC3339Nano, issuedAt)
	if err != nil {
		return nil, err
	}
	record.ExpiresAt, err = parseOptionalTime(expiresAt)
	if err != nil {
		return nil, err
	}
	return &record, nil
}
