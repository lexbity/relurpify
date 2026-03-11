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
	"github.com/lexcodex/relurpify/framework/middleware/node"
	_ "github.com/mattn/go-sqlite3"
)

var _ node.NodeStore = (*SQLiteNodeStore)(nil)

type SQLiteNodeStore struct {
	db *sql.DB
}

func NewSQLiteNodeStore(path string) (*SQLiteNodeStore, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("node store path required")
	}
	dsn := fmt.Sprintf("file:%s?_foreign_keys=on&_busy_timeout=5000", filepath.Clean(path))
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}
	store := &SQLiteNodeStore{db: db}
	if err := store.init(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *SQLiteNodeStore) init() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS nodes (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL DEFAULT '',
			name TEXT NOT NULL,
			platform TEXT NOT NULL,
			trust_class TEXT NOT NULL,
			paired_at TEXT NOT NULL,
			owner_kind TEXT NOT NULL DEFAULT '',
			owner_id TEXT NOT NULL DEFAULT '',
			tags_json TEXT NOT NULL DEFAULT '{}',
			approved_capabilities_json TEXT NOT NULL DEFAULT '[]'
		);`,
		`CREATE TABLE IF NOT EXISTS node_credentials (
			device_id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL DEFAULT '',
			public_key BLOB NOT NULL,
			key_id TEXT NOT NULL DEFAULT '',
			issued_at TEXT NOT NULL,
			expires_at TEXT NOT NULL DEFAULT ''
		);`,
		`CREATE TABLE IF NOT EXISTS pending_pairings (
			code TEXT PRIMARY KEY,
			device_id TEXT NOT NULL,
			tenant_id TEXT NOT NULL DEFAULT '',
			public_key BLOB NOT NULL,
			key_id TEXT NOT NULL DEFAULT '',
			issued_at TEXT NOT NULL,
			expires_at TEXT NOT NULL
		);`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	for _, stmt := range []string{
		`ALTER TABLE nodes ADD COLUMN tenant_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE nodes ADD COLUMN owner_kind TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE nodes ADD COLUMN owner_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE nodes ADD COLUMN approved_capabilities_json TEXT NOT NULL DEFAULT '[]'`,
		`ALTER TABLE node_credentials ADD COLUMN tenant_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE node_credentials ADD COLUMN key_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE pending_pairings ADD COLUMN tenant_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE pending_pairings ADD COLUMN key_id TEXT NOT NULL DEFAULT ''`,
	} {
		if _, err := s.db.Exec(stmt); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
			return err
		}
	}
	return nil
}

func (s *SQLiteNodeStore) GetNode(ctx context.Context, id string) (*core.NodeDescriptor, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, tenant_id, name, platform, trust_class, paired_at, owner_kind, owner_id, tags_json, approved_capabilities_json FROM nodes WHERE id = ?`, id)
	var nodeDesc core.NodeDescriptor
	var pairedAt, ownerKind, ownerID, tagsJSON, approvedCapabilitiesJSON string
	err := row.Scan(&nodeDesc.ID, &nodeDesc.TenantID, &nodeDesc.Name, &nodeDesc.Platform, &nodeDesc.TrustClass, &pairedAt, &ownerKind, &ownerID, &tagsJSON, &approvedCapabilitiesJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	nodeDesc.PairedAt, err = time.Parse(time.RFC3339Nano, pairedAt)
	if err != nil {
		return nil, err
	}
	if ownerID != "" || ownerKind != "" {
		nodeDesc.Owner = core.SubjectRef{
			TenantID: nodeDesc.TenantID,
			Kind:     core.SubjectKind(ownerKind),
			ID:       ownerID,
		}
	}
	nodeDesc.Tags = map[string]string{}
	_ = json.Unmarshal([]byte(tagsJSON), &nodeDesc.Tags)
	_ = json.Unmarshal([]byte(approvedCapabilitiesJSON), &nodeDesc.ApprovedCapabilities)
	return &nodeDesc, nil
}

func (s *SQLiteNodeStore) ListNodes(ctx context.Context) ([]core.NodeDescriptor, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, tenant_id, name, platform, trust_class, paired_at, owner_kind, owner_id, tags_json, approved_capabilities_json FROM nodes ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []core.NodeDescriptor
	for rows.Next() {
		var nodeDesc core.NodeDescriptor
		var pairedAt, ownerKind, ownerID, tagsJSON, approvedCapabilitiesJSON string
		if err := rows.Scan(&nodeDesc.ID, &nodeDesc.TenantID, &nodeDesc.Name, &nodeDesc.Platform, &nodeDesc.TrustClass, &pairedAt, &ownerKind, &ownerID, &tagsJSON, &approvedCapabilitiesJSON); err != nil {
			return nil, err
		}
		nodeDesc.PairedAt, err = time.Parse(time.RFC3339Nano, pairedAt)
		if err != nil {
			return nil, err
		}
		if ownerID != "" || ownerKind != "" {
			nodeDesc.Owner = core.SubjectRef{
				TenantID: nodeDesc.TenantID,
				Kind:     core.SubjectKind(ownerKind),
				ID:       ownerID,
			}
		}
		nodeDesc.Tags = map[string]string{}
		_ = json.Unmarshal([]byte(tagsJSON), &nodeDesc.Tags)
		_ = json.Unmarshal([]byte(approvedCapabilitiesJSON), &nodeDesc.ApprovedCapabilities)
		out = append(out, nodeDesc)
	}
	return out, rows.Err()
}

func (s *SQLiteNodeStore) UpsertNode(ctx context.Context, nodeDesc core.NodeDescriptor) error {
	if nodeDesc.PairedAt.IsZero() {
		nodeDesc.PairedAt = time.Now().UTC()
	}
	tagsJSON, _ := json.Marshal(nodeDesc.Tags)
	approvedCapabilitiesJSON, _ := json.Marshal(nodeDesc.ApprovedCapabilities)
	ownerKind := ""
	ownerID := ""
	if nodeDesc.Owner.ID != "" {
		ownerKind = string(nodeDesc.Owner.Kind)
		ownerID = nodeDesc.Owner.ID
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO nodes (id, tenant_id, name, platform, trust_class, paired_at, owner_kind, owner_id, tags_json, approved_capabilities_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			tenant_id = excluded.tenant_id,
			name = excluded.name,
			platform = excluded.platform,
			trust_class = excluded.trust_class,
			paired_at = excluded.paired_at,
			owner_kind = excluded.owner_kind,
			owner_id = excluded.owner_id,
			tags_json = excluded.tags_json,
			approved_capabilities_json = excluded.approved_capabilities_json`,
		nodeDesc.ID, nodeDesc.TenantID, nodeDesc.Name, string(nodeDesc.Platform), string(nodeDesc.TrustClass), nodeDesc.PairedAt.UTC().Format(time.RFC3339Nano), ownerKind, ownerID, string(tagsJSON), string(approvedCapabilitiesJSON))
	return err
}

func (s *SQLiteNodeStore) RemoveNode(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM nodes WHERE id = ?`, id)
	return err
}

func (s *SQLiteNodeStore) GetCredential(ctx context.Context, deviceID string) (*core.NodeCredential, error) {
	row := s.db.QueryRowContext(ctx, `SELECT device_id, tenant_id, public_key, key_id, issued_at, expires_at FROM node_credentials WHERE device_id = ?`, deviceID)
	var cred core.NodeCredential
	var issuedAt, expiresAt string
	err := row.Scan(&cred.DeviceID, &cred.TenantID, &cred.PublicKey, &cred.KeyID, &issuedAt, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	cred.IssuedAt, err = time.Parse(time.RFC3339Nano, issuedAt)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(expiresAt) != "" {
		cred.ExpiresAt, err = time.Parse(time.RFC3339Nano, expiresAt)
		if err != nil {
			return nil, err
		}
	}
	return &cred, nil
}

func (s *SQLiteNodeStore) SaveCredential(ctx context.Context, cred core.NodeCredential) error {
	expiresAt := ""
	if !cred.ExpiresAt.IsZero() {
		expiresAt = cred.ExpiresAt.UTC().Format(time.RFC3339Nano)
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO node_credentials (device_id, tenant_id, public_key, key_id, issued_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(device_id) DO UPDATE SET
			tenant_id = excluded.tenant_id,
			public_key = excluded.public_key,
			key_id = excluded.key_id,
			issued_at = excluded.issued_at,
			expires_at = excluded.expires_at`,
		cred.DeviceID, cred.TenantID, cred.PublicKey, cred.KeyID, cred.IssuedAt.UTC().Format(time.RFC3339Nano), expiresAt)
	return err
}

func (s *SQLiteNodeStore) SavePendingPairing(ctx context.Context, pairing node.PendingPairing) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO pending_pairings (code, device_id, tenant_id, public_key, key_id, issued_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(code) DO UPDATE SET
			device_id = excluded.device_id,
			tenant_id = excluded.tenant_id,
			public_key = excluded.public_key,
			key_id = excluded.key_id,
			issued_at = excluded.issued_at,
			expires_at = excluded.expires_at`,
		pairing.Code,
		pairing.Cred.DeviceID,
		pairing.Cred.TenantID,
		pairing.Cred.PublicKey,
		pairing.Cred.KeyID,
		pairing.Cred.IssuedAt.UTC().Format(time.RFC3339Nano),
		pairing.ExpiresAt.UTC().Format(time.RFC3339Nano),
	)
	return err
}

func (s *SQLiteNodeStore) GetPendingPairing(ctx context.Context, code string) (*node.PendingPairing, error) {
	if _, err := s.DeleteExpiredPendingPairings(ctx, time.Now().UTC()); err != nil {
		return nil, err
	}
	row := s.db.QueryRowContext(ctx, `SELECT code, device_id, tenant_id, public_key, key_id, issued_at, expires_at FROM pending_pairings WHERE code = ?`, code)
	var (
		pairing             node.PendingPairing
		issuedAt, expiresAt string
	)
	err := row.Scan(&pairing.Code, &pairing.Cred.DeviceID, &pairing.Cred.TenantID, &pairing.Cred.PublicKey, &pairing.Cred.KeyID, &issuedAt, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	pairing.Cred.IssuedAt, err = time.Parse(time.RFC3339Nano, issuedAt)
	if err != nil {
		return nil, err
	}
	pairing.ExpiresAt, err = time.Parse(time.RFC3339Nano, expiresAt)
	if err != nil {
		return nil, err
	}
	pairing.Cred.ExpiresAt = pairing.ExpiresAt
	return &pairing, nil
}

func (s *SQLiteNodeStore) ListPendingPairings(ctx context.Context) ([]node.PendingPairing, error) {
	if _, err := s.DeleteExpiredPendingPairings(ctx, time.Now().UTC()); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT code, device_id, tenant_id, public_key, key_id, issued_at, expires_at FROM pending_pairings ORDER BY expires_at ASC, code ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []node.PendingPairing
	for rows.Next() {
		var (
			pairing             node.PendingPairing
			issuedAt, expiresAt string
		)
		if err := rows.Scan(&pairing.Code, &pairing.Cred.DeviceID, &pairing.Cred.TenantID, &pairing.Cred.PublicKey, &pairing.Cred.KeyID, &issuedAt, &expiresAt); err != nil {
			return nil, err
		}
		pairing.Cred.IssuedAt, err = time.Parse(time.RFC3339Nano, issuedAt)
		if err != nil {
			return nil, err
		}
		pairing.ExpiresAt, err = time.Parse(time.RFC3339Nano, expiresAt)
		if err != nil {
			return nil, err
		}
		pairing.Cred.ExpiresAt = pairing.ExpiresAt
		out = append(out, pairing)
	}
	return out, rows.Err()
}

func (s *SQLiteNodeStore) DeletePendingPairing(ctx context.Context, code string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM pending_pairings WHERE code = ?`, code)
	return err
}

func (s *SQLiteNodeStore) DeleteExpiredPendingPairings(ctx context.Context, before time.Time) (int, error) {
	if s == nil || s.db == nil {
		return 0, nil
	}
	if before.IsZero() {
		before = time.Now().UTC()
	}
	rows, err := s.db.QueryContext(ctx, `SELECT code, expires_at FROM pending_pairings`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	expiredCodes := make([]string, 0)
	for rows.Next() {
		var code string
		var expiresAt string
		if err := rows.Scan(&code, &expiresAt); err != nil {
			return 0, err
		}
		expiry, err := time.Parse(time.RFC3339Nano, expiresAt)
		if err != nil {
			return 0, err
		}
		if !expiry.After(before) {
			expiredCodes = append(expiredCodes, code)
		}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	deleted := 0
	for _, code := range expiredCodes {
		if _, err := s.db.ExecContext(ctx, `DELETE FROM pending_pairings WHERE code = ?`, code); err != nil {
			return deleted, err
		}
		deleted++
	}
	return deleted, nil
}

func (s *SQLiteNodeStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}
