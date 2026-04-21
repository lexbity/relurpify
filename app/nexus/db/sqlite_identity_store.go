package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/identity"
	_ "github.com/mattn/go-sqlite3"
)

var _ identity.Store = (*SQLiteIdentityStore)(nil)

type SQLiteIdentityStore struct {
	db *sql.DB
}

func NewSQLiteIdentityStore(path string) (*SQLiteIdentityStore, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("identity store path required")
	}
	dsn := fmt.Sprintf("file:%s?_foreign_keys=on&_busy_timeout=5000", filepath.Clean(path))
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}
	store := &SQLiteIdentityStore{db: db}
	if err := store.init(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *SQLiteIdentityStore) init() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS tenants (
			tenant_id TEXT PRIMARY KEY,
			display_name TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT '',
			disabled_at TEXT NOT NULL DEFAULT ''
		);`,
		`CREATE TABLE IF NOT EXISTS subjects (
			tenant_id TEXT NOT NULL,
			subject_kind TEXT NOT NULL,
			subject_id TEXT NOT NULL,
			display_name TEXT NOT NULL DEFAULT '',
			roles_json TEXT NOT NULL DEFAULT '[]',
			created_at TEXT NOT NULL DEFAULT '',
			disabled_at TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (tenant_id, subject_kind, subject_id)
		);`,
		`CREATE TABLE IF NOT EXISTS external_identities (
			tenant_id TEXT NOT NULL,
			provider TEXT NOT NULL,
			account_id TEXT NOT NULL DEFAULT '',
			external_id TEXT NOT NULL,
			subject_kind TEXT NOT NULL,
			subject_id TEXT NOT NULL,
			verified_at TEXT NOT NULL DEFAULT '',
			last_seen_at TEXT NOT NULL DEFAULT '',
			display_name TEXT NOT NULL DEFAULT '',
			provider_label TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (tenant_id, provider, account_id, external_id)
		);`,
		`CREATE TABLE IF NOT EXISTS node_enrollments (
			tenant_id TEXT NOT NULL,
			node_id TEXT NOT NULL,
			owner_kind TEXT NOT NULL,
			owner_id TEXT NOT NULL,
			trust_class TEXT NOT NULL,
			public_key BLOB NOT NULL DEFAULT '',
			key_id TEXT NOT NULL DEFAULT '',
			paired_at TEXT NOT NULL DEFAULT '',
			last_verified_at TEXT NOT NULL DEFAULT '',
			auth_method TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (tenant_id, node_id)
		);`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLiteIdentityStore) UpsertTenant(ctx context.Context, tenant core.TenantRecord) error {
	if err := tenant.Validate(); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO tenants
		(tenant_id, display_name, created_at, disabled_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(tenant_id) DO UPDATE SET
			display_name = excluded.display_name,
			created_at = excluded.created_at,
			disabled_at = excluded.disabled_at`,
		tenant.ID,
		tenant.DisplayName,
		formatOptionalTime(tenant.CreatedAt),
		formatOptionalTimePtr(tenant.DisabledAt),
	)
	return err
}

func (s *SQLiteIdentityStore) GetTenant(ctx context.Context, tenantID string) (*core.TenantRecord, error) {
	row := s.db.QueryRowContext(ctx, `SELECT tenant_id, display_name, created_at, disabled_at FROM tenants WHERE tenant_id = ?`, tenantID)
	tenant, err := scanTenantRecord(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return tenant, nil
}

func (s *SQLiteIdentityStore) ListTenants(ctx context.Context) ([]core.TenantRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT tenant_id, display_name, created_at, disabled_at FROM tenants ORDER BY tenant_id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []core.TenantRecord
	for rows.Next() {
		tenant, err := scanTenantRecord(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *tenant)
	}
	return out, rows.Err()
}

func (s *SQLiteIdentityStore) UpsertSubject(ctx context.Context, subject core.SubjectRecord) error {
	if err := subject.Validate(); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO subjects
		(tenant_id, subject_kind, subject_id, display_name, roles_json, created_at, disabled_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(tenant_id, subject_kind, subject_id) DO UPDATE SET
			display_name = excluded.display_name,
			roles_json = excluded.roles_json,
			created_at = excluded.created_at,
			disabled_at = excluded.disabled_at`,
		subject.TenantID,
		string(subject.Kind),
		subject.ID,
		subject.DisplayName,
		marshalStringSlice(subject.Roles),
		formatOptionalTime(subject.CreatedAt),
		formatOptionalTimePtr(subject.DisabledAt),
	)
	return err
}

func (s *SQLiteIdentityStore) GetSubject(ctx context.Context, tenantID string, kind core.SubjectKind, subjectID string) (*core.SubjectRecord, error) {
	row := s.db.QueryRowContext(ctx, `SELECT tenant_id, subject_kind, subject_id, display_name, roles_json, created_at, disabled_at
		FROM subjects WHERE tenant_id = ? AND subject_kind = ? AND subject_id = ?`, tenantID, string(kind), subjectID)
	subject, err := scanSubjectRecord(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return subject, nil
}

func (s *SQLiteIdentityStore) ListSubjects(ctx context.Context, tenantID string) ([]core.SubjectRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT tenant_id, subject_kind, subject_id, display_name, roles_json, created_at, disabled_at
		FROM subjects WHERE tenant_id = ? ORDER BY subject_kind ASC, subject_id ASC`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []core.SubjectRecord
	for rows.Next() {
		subject, err := scanSubjectRecord(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *subject)
	}
	return out, rows.Err()
}

func (s *SQLiteIdentityStore) UpsertExternalIdentity(ctx context.Context, identity core.ExternalIdentity) error {
	if err := identity.Validate(); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO external_identities
		(tenant_id, provider, account_id, external_id, subject_kind, subject_id, verified_at, last_seen_at, display_name, provider_label)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(tenant_id, provider, account_id, external_id) DO UPDATE SET
			subject_kind = excluded.subject_kind,
			subject_id = excluded.subject_id,
			verified_at = excluded.verified_at,
			last_seen_at = excluded.last_seen_at,
			display_name = excluded.display_name,
			provider_label = excluded.provider_label`,
		identity.TenantID,
		string(identity.Provider),
		identity.AccountID,
		identity.ExternalID,
		string(identity.Subject.Kind),
		identity.Subject.ID,
		formatOptionalTime(identity.VerifiedAt),
		formatOptionalTime(identity.LastSeenAt),
		identity.DisplayName,
		identity.ProviderLabel,
	)
	return err
}

func (s *SQLiteIdentityStore) GetExternalIdentity(ctx context.Context, tenantID string, provider core.ExternalProvider, accountID, externalID string) (*core.ExternalIdentity, error) {
	row := s.db.QueryRowContext(ctx, `SELECT tenant_id, provider, account_id, external_id, subject_kind, subject_id, verified_at, last_seen_at, display_name, provider_label
		FROM external_identities WHERE tenant_id = ? AND provider = ? AND account_id = ? AND external_id = ?`,
		tenantID, string(provider), accountID, externalID)
	identity, err := scanExternalIdentity(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return identity, nil
}

func (s *SQLiteIdentityStore) ListExternalIdentities(ctx context.Context, tenantID string) ([]core.ExternalIdentity, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT tenant_id, provider, account_id, external_id, subject_kind, subject_id, verified_at, last_seen_at, display_name, provider_label
		FROM external_identities WHERE tenant_id = ? ORDER BY provider ASC, account_id ASC, external_id ASC`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []core.ExternalIdentity
	for rows.Next() {
		identity, err := scanExternalIdentity(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *identity)
	}
	return out, rows.Err()
}

func (s *SQLiteIdentityStore) UpsertNodeEnrollment(ctx context.Context, enrollment core.NodeEnrollment) error {
	if err := enrollment.Validate(); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO node_enrollments
		(tenant_id, node_id, owner_kind, owner_id, trust_class, public_key, key_id, paired_at, last_verified_at, auth_method)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(tenant_id, node_id) DO UPDATE SET
			owner_kind = excluded.owner_kind,
			owner_id = excluded.owner_id,
			trust_class = excluded.trust_class,
			public_key = excluded.public_key,
			key_id = excluded.key_id,
			paired_at = excluded.paired_at,
			last_verified_at = excluded.last_verified_at,
			auth_method = excluded.auth_method`,
		enrollment.TenantID,
		enrollment.NodeID,
		string(enrollment.Owner.Kind),
		enrollment.Owner.ID,
		string(enrollment.TrustClass),
		enrollment.PublicKey,
		enrollment.KeyID,
		formatOptionalTime(enrollment.PairedAt),
		formatOptionalTime(enrollment.LastVerifiedAt),
		string(enrollment.AuthMethod),
	)
	return err
}

func (s *SQLiteIdentityStore) GetNodeEnrollment(ctx context.Context, tenantID, nodeID string) (*core.NodeEnrollment, error) {
	row := s.db.QueryRowContext(ctx, `SELECT tenant_id, node_id, owner_kind, owner_id, trust_class, public_key, key_id, paired_at, last_verified_at, auth_method
		FROM node_enrollments WHERE tenant_id = ? AND node_id = ?`, tenantID, nodeID)
	enrollment, err := scanNodeEnrollment(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return enrollment, nil
}

func (s *SQLiteIdentityStore) ListNodeEnrollments(ctx context.Context, tenantID string) ([]core.NodeEnrollment, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT tenant_id, node_id, owner_kind, owner_id, trust_class, public_key, key_id, paired_at, last_verified_at, auth_method
		FROM node_enrollments WHERE tenant_id = ? ORDER BY node_id ASC`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []core.NodeEnrollment
	for rows.Next() {
		enrollment, err := scanNodeEnrollment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *enrollment)
	}
	return out, rows.Err()
}

func (s *SQLiteIdentityStore) DeleteNodeEnrollment(ctx context.Context, tenantID, nodeID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM node_enrollments WHERE tenant_id = ? AND node_id = ?`, tenantID, nodeID)
	return err
}

func (s *SQLiteIdentityStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanTenantRecord(row scanner) (*core.TenantRecord, error) {
	var (
		record     core.TenantRecord
		createdAt  string
		disabledAt string
		err        error
	)
	if err := row.Scan(&record.ID, &record.DisplayName, &createdAt, &disabledAt); err != nil {
		return nil, err
	}
	if record.CreatedAt, err = parseOptionalTime(createdAt); err != nil {
		return nil, err
	}
	if record.DisabledAt, err = parseOptionalTimePtr(disabledAt); err != nil {
		return nil, err
	}
	return &record, nil
}

func scanSubjectRecord(row scanner) (*core.SubjectRecord, error) {
	var (
		record      core.SubjectRecord
		subjectKind string
		rolesJSON   string
		createdAt   string
		disabledAt  string
		err         error
	)
	if err := row.Scan(&record.TenantID, &subjectKind, &record.ID, &record.DisplayName, &rolesJSON, &createdAt, &disabledAt); err != nil {
		return nil, err
	}
	record.Kind = core.SubjectKind(subjectKind)
	record.Roles = unmarshalStringSlice(rolesJSON)
	if record.CreatedAt, err = parseOptionalTime(createdAt); err != nil {
		return nil, err
	}
	if record.DisabledAt, err = parseOptionalTimePtr(disabledAt); err != nil {
		return nil, err
	}
	return &record, nil
}

func scanExternalIdentity(row scanner) (*core.ExternalIdentity, error) {
	var (
		identity    core.ExternalIdentity
		provider    string
		subjectKind string
		verifiedAt  string
		lastSeenAt  string
	)
	if err := row.Scan(&identity.TenantID, &provider, &identity.AccountID, &identity.ExternalID, &subjectKind, &identity.Subject.ID, &verifiedAt, &lastSeenAt, &identity.DisplayName, &identity.ProviderLabel); err != nil {
		return nil, err
	}
	identity.Provider = core.ExternalProvider(provider)
	identity.Subject = core.SubjectRef{
		TenantID: identity.TenantID,
		Kind:     core.SubjectKind(subjectKind),
		ID:       identity.Subject.ID,
	}
	var err error
	if identity.VerifiedAt, err = parseOptionalTime(verifiedAt); err != nil {
		return nil, err
	}
	if identity.LastSeenAt, err = parseOptionalTime(lastSeenAt); err != nil {
		return nil, err
	}
	return &identity, nil
}

func scanNodeEnrollment(row scanner) (*core.NodeEnrollment, error) {
	var (
		enrollment   core.NodeEnrollment
		ownerKind    string
		trustClass   string
		pairedAt     string
		lastVerified string
		authMethod   string
	)
	if err := row.Scan(&enrollment.TenantID, &enrollment.NodeID, &ownerKind, &enrollment.Owner.ID, &trustClass, &enrollment.PublicKey, &enrollment.KeyID, &pairedAt, &lastVerified, &authMethod); err != nil {
		return nil, err
	}
	enrollment.Owner = core.SubjectRef{
		TenantID: enrollment.TenantID,
		Kind:     core.SubjectKind(ownerKind),
		ID:       enrollment.Owner.ID,
	}
	enrollment.TrustClass = core.TrustClass(trustClass)
	enrollment.AuthMethod = core.AuthMethod(authMethod)
	var err error
	if enrollment.PairedAt, err = parseOptionalTime(pairedAt); err != nil {
		return nil, err
	}
	if enrollment.LastVerifiedAt, err = parseOptionalTime(lastVerified); err != nil {
		return nil, err
	}
	return &enrollment, nil
}

func formatOptionalTimePtr(value *time.Time) string {
	if value == nil {
		return ""
	}
	return formatOptionalTime(*value)
}

func parseOptionalTimePtr(raw string) (*time.Time, error) {
	value, err := parseOptionalTime(raw)
	if err != nil {
		return nil, err
	}
	if value.IsZero() {
		return nil, nil
	}
	return &value, nil
}

func formatOptionalTime(ts time.Time) string {
	if ts.IsZero() {
		return ""
	}
	return ts.UTC().Format(time.RFC3339Nano)
}

func parseOptionalTime(raw string) (time.Time, error) {
	if strings.TrimSpace(raw) == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339Nano, raw)
}
