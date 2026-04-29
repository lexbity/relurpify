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

	"codeburg.org/lexbit/relurpify/relurpnet/identity"
	_ "github.com/mattn/go-sqlite3"
)

type SQLiteAdminTokenStore struct {
	db *sql.DB
}

func NewSQLiteAdminTokenStore(path string) (*SQLiteAdminTokenStore, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("admin token store path required")
	}
	dsn := fmt.Sprintf("file:%s?_foreign_keys=on&_busy_timeout=5000", filepath.Clean(path))
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}
	store := &SQLiteAdminTokenStore{db: db}
	if err := store.init(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *SQLiteAdminTokenStore) init() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS admin_tokens (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL DEFAULT '',
		tenant_id TEXT NOT NULL DEFAULT '',
		subject_kind TEXT NOT NULL DEFAULT '',
		subject_id TEXT NOT NULL DEFAULT '',
		token_hash TEXT NOT NULL,
		scopes_json TEXT NOT NULL DEFAULT '[]',
		issued_at TEXT NOT NULL,
		expires_at TEXT NOT NULL DEFAULT '',
		last_used_at TEXT NOT NULL DEFAULT '',
		revoked_at TEXT NOT NULL DEFAULT ''
	);`)
	if err != nil {
		return err
	}
	for _, stmt := range []string{
		`ALTER TABLE admin_tokens ADD COLUMN tenant_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE admin_tokens ADD COLUMN subject_kind TEXT NOT NULL DEFAULT ''`,
		`CREATE INDEX IF NOT EXISTS idx_admin_tokens_token_hash ON admin_tokens(token_hash)`,
	} {
		if _, err := s.db.Exec(stmt); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
			return err
		}
	}
	return nil
}

// listTokensMaxDefault is the upper bound applied by ListTokens when no explicit
// limit is provided, preventing unbounded result sets.
const listTokensMaxDefault = 200

// ListTokens returns up to listTokensMaxDefault tokens ordered by issued_at DESC.
// Use ListTokensPaged for explicit pagination control.
func (s *SQLiteAdminTokenStore) ListTokens(ctx context.Context) ([]identity.AdminTokenRecord, error) {
	return s.ListTokensPaged(ctx, listTokensMaxDefault, 0)
}

// ListTokensPaged returns a page of admin token records. limit must be positive
// and is capped at 1000; offset is the zero-based row offset.
func (s *SQLiteAdminTokenStore) ListTokensPaged(ctx context.Context, limit, offset int) ([]identity.AdminTokenRecord, error) {
	if limit <= 0 {
		limit = listTokensMaxDefault
	}
	if limit > 1000 {
		limit = 1000
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, tenant_id, subject_kind, subject_id, token_hash, scopes_json, issued_at, expires_at, last_used_at, revoked_at
		 FROM admin_tokens ORDER BY issued_at DESC, id ASC LIMIT ? OFFSET ?`,
		limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []identity.AdminTokenRecord
	for rows.Next() {
		record, err := scanAdminToken(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	return out, rows.Err()
}

func (s *SQLiteAdminTokenStore) GetToken(ctx context.Context, id string) (*identity.AdminTokenRecord, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, name, tenant_id, subject_kind, subject_id, token_hash, scopes_json, issued_at, expires_at, last_used_at, revoked_at FROM admin_tokens WHERE id = ?`, id)
	record, err := scanAdminToken(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &record, nil
}

func (s *SQLiteAdminTokenStore) GetTokenByHash(ctx context.Context, tokenHash string) (*identity.AdminTokenRecord, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, name, tenant_id, subject_kind, subject_id, token_hash, scopes_json, issued_at, expires_at, last_used_at, revoked_at FROM admin_tokens WHERE token_hash = ?`, tokenHash)
	record, err := scanAdminToken(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &record, nil
}

func (s *SQLiteAdminTokenStore) CreateToken(ctx context.Context, record identity.AdminTokenRecord) error {
	if strings.TrimSpace(record.ID) == "" {
		return errors.New("token id required")
	}
	if strings.TrimSpace(record.TokenHash) == "" {
		return errors.New("token hash required")
	}
	if record.IssuedAt.IsZero() {
		record.IssuedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO admin_tokens (id, name, tenant_id, subject_kind, subject_id, token_hash, scopes_json, issued_at, expires_at, last_used_at, revoked_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		record.ID,
		record.Name,
		record.TenantID,
		string(record.SubjectKind),
		record.SubjectID,
		record.TokenHash,
		marshalStringSlice(record.Scopes),
		record.IssuedAt.UTC().Format(time.RFC3339Nano),
		formatOptTime(record.ExpiresAt),
		formatOptTime(record.LastUsedAt),
		formatOptTime(record.RevokedAt),
	)
	return err
}

func (s *SQLiteAdminTokenStore) RevokeToken(ctx context.Context, id string, revokedAt time.Time) error {
	if revokedAt.IsZero() {
		revokedAt = time.Now().UTC()
	}
	result, err := s.db.ExecContext(ctx, `UPDATE admin_tokens SET revoked_at = ? WHERE id = ?`, revokedAt.UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *SQLiteAdminTokenStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func scanAdminToken(scan func(dest ...any) error) (identity.AdminTokenRecord, error) {
	var (
		record                identity.AdminTokenRecord
		subjectKind           string
		scopesJSON            string
		issuedAt, expiresAt   string
		lastUsedAt, revokedAt string
	)
	if err := scan(&record.ID, &record.Name, &record.TenantID, &subjectKind, &record.SubjectID, &record.TokenHash, &scopesJSON, &issuedAt, &expiresAt, &lastUsedAt, &revokedAt); err != nil {
		return identity.AdminTokenRecord{}, err
	}
	record.SubjectKind = identity.SubjectKind(subjectKind)
	parsedIssuedAt, err := time.Parse(time.RFC3339Nano, issuedAt)
	if err != nil {
		return identity.AdminTokenRecord{}, err
	}
	record.IssuedAt = parsedIssuedAt
	record.Scopes = unmarshalStringSlice(scopesJSON)
	if record.ExpiresAt, err = parseOptTime(expiresAt); err != nil {
		return identity.AdminTokenRecord{}, err
	}
	if record.LastUsedAt, err = parseOptTime(lastUsedAt); err != nil {
		return identity.AdminTokenRecord{}, err
	}
	if record.RevokedAt, err = parseOptTime(revokedAt); err != nil {
		return identity.AdminTokenRecord{}, err
	}
	return record, nil
}

func marshalStringSlice(values []string) string {
	data, _ := json.Marshal(values)
	return string(data)
}

func unmarshalStringSlice(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var out []string
	_ = json.Unmarshal([]byte(raw), &out)
	return out
}

func formatOptTime(value *time.Time) string {
	if value == nil || value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func parseOptTime(value string) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}
