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
	netsession "codeburg.org/lexbit/relurpify/relurpnet/session"
	_ "github.com/mattn/go-sqlite3"
)

var _ netsession.Store = (*SQLiteSessionStore)(nil)

type SQLiteSessionStore struct {
	db          *sql.DB
	BoundaryTTL time.Duration
	now         func() time.Time
}

func NewSQLiteSessionStore(path string) (*SQLiteSessionStore, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("session store path required")
	}
	dsn := fmt.Sprintf("file:%s?_foreign_keys=on&_busy_timeout=5000", filepath.Clean(path))
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}
	store := &SQLiteSessionStore{
		db:          db,
		BoundaryTTL: netsession.DefaultBoundaryIdleTTL,
		now:         time.Now,
	}
	if err := store.init(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *SQLiteSessionStore) init() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS session_boundaries (
		key TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		tenant_id TEXT NOT NULL DEFAULT '',
		partition TEXT NOT NULL,
		scope TEXT NOT NULL,
		actor_id TEXT NOT NULL DEFAULT '',
		owner_kind TEXT NOT NULL DEFAULT '',
		owner_id TEXT NOT NULL DEFAULT '',
		channel_id TEXT NOT NULL DEFAULT '',
		peer_id TEXT NOT NULL DEFAULT '',
		binding_provider TEXT NOT NULL DEFAULT '',
		binding_account_id TEXT NOT NULL DEFAULT '',
		binding_channel_id TEXT NOT NULL DEFAULT '',
		binding_conversation_id TEXT NOT NULL DEFAULT '',
		binding_thread_id TEXT NOT NULL DEFAULT '',
		binding_external_user_id TEXT NOT NULL DEFAULT '',
		trust_class TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL,
		last_activity_at TEXT NOT NULL DEFAULT ''
	);`)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`CREATE TABLE IF NOT EXISTS session_delegations (
		session_id TEXT NOT NULL,
		tenant_id TEXT NOT NULL DEFAULT '',
		grantee_kind TEXT NOT NULL,
		grantee_id TEXT NOT NULL,
		operations_json TEXT NOT NULL DEFAULT '[]',
		created_at TEXT NOT NULL,
		expires_at TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (session_id, grantee_kind, grantee_id)
	);`)
	if err != nil {
		return err
	}
	if _, err := s.db.Exec(`ALTER TABLE session_boundaries ADD COLUMN last_activity_at TEXT NOT NULL DEFAULT ''`); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
		return err
	}
	for _, stmt := range []string{
		`ALTER TABLE session_boundaries ADD COLUMN routing_key TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE session_boundaries ADD COLUMN tenant_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE session_boundaries ADD COLUMN owner_kind TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE session_boundaries ADD COLUMN owner_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE session_boundaries ADD COLUMN binding_provider TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE session_boundaries ADD COLUMN binding_account_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE session_boundaries ADD COLUMN binding_channel_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE session_boundaries ADD COLUMN binding_conversation_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE session_boundaries ADD COLUMN binding_thread_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE session_boundaries ADD COLUMN binding_external_user_id TEXT NOT NULL DEFAULT ''`,
	} {
		if _, err := s.db.Exec(stmt); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
			return err
		}
	}
	if _, err := s.db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_session_boundaries_session_id ON session_boundaries(session_id)`); err != nil {
		return err
	}
	if _, err := s.db.Exec(`UPDATE session_boundaries SET routing_key = key WHERE routing_key = ''`); err != nil && !strings.Contains(strings.ToLower(err.Error()), "no such column") {
		return err
	}
	_, err = s.db.Exec(`UPDATE session_boundaries SET last_activity_at = created_at WHERE last_activity_at = ''`)
	return err
}

func (s *SQLiteSessionStore) GetBoundary(ctx context.Context, key string) (*netsession.SessionBoundary, error) {
	if err := s.deleteExpiredBoundaries(ctx); err != nil {
		return nil, err
	}
	row := s.db.QueryRowContext(ctx, `SELECT session_id, routing_key, tenant_id, partition, scope, actor_id, owner_kind, owner_id, channel_id, peer_id, binding_provider, binding_account_id, binding_channel_id, binding_conversation_id, binding_thread_id, binding_external_user_id, trust_class, created_at, last_activity_at FROM session_boundaries WHERE key = ?`, key)
	var (
		boundary              netsession.SessionBoundary
		scope                 string
		ownerKind             string
		trust                 string
		bindingProvider       string
		bindingAccountID      string
		bindingChannelID      string
		bindingConversationID string
		bindingThreadID       string
		bindingExternalUserID string
		createdAt             string
		lastActivityAt        string
	)
	err := row.Scan(&boundary.SessionID, &boundary.RoutingKey, &boundary.TenantID, &boundary.Partition, &scope, &boundary.ActorID, &ownerKind, &boundary.Owner.ID, &boundary.ChannelID, &boundary.PeerID, &bindingProvider, &bindingAccountID, &bindingChannelID, &bindingConversationID, &bindingThreadID, &bindingExternalUserID, &trust, &createdAt, &lastActivityAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	boundary.Scope = core.SessionScope(scope)
	boundary.TrustClass = core.TrustClass(trust)
	if boundary.Owner.ID != "" || ownerKind != "" {
		boundary.Owner = core.DelegationSubjectRef{
			TenantID: boundary.TenantID,
			Kind:     ownerKind,
			ID:       boundary.Owner.ID,
		}
	}
	if bindingProvider != "" || bindingConversationID != "" {
		boundary.Binding = &core.SessionBinding{
			Provider:       bindingProvider,
			AccountID:      bindingAccountID,
			ChannelID:      bindingChannelID,
			ConversationID: bindingConversationID,
			ThreadID:       bindingThreadID,
			ExternalUserID: bindingExternalUserID,
		}
	}
	boundary.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(lastActivityAt) == "" {
		lastActivityAt = createdAt
	}
	boundary.LastActivityAt, err = time.Parse(time.RFC3339Nano, lastActivityAt)
	if err != nil {
		return nil, err
	}
	return &boundary, nil
}

func (s *SQLiteSessionStore) GetBoundaryBySessionID(ctx context.Context, sessionID string) (*netsession.SessionBoundary, error) {
	if err := s.deleteExpiredBoundaries(ctx); err != nil {
		return nil, err
	}
	row := s.db.QueryRowContext(ctx, `SELECT session_id, routing_key, tenant_id, partition, scope, actor_id, owner_kind, owner_id, channel_id, peer_id, binding_provider, binding_account_id, binding_channel_id, binding_conversation_id, binding_thread_id, binding_external_user_id, trust_class, created_at, last_activity_at FROM session_boundaries WHERE session_id = ?`, sessionID)
	var (
		boundary              netsession.SessionBoundary
		scope                 string
		ownerKind             string
		trust                 string
		bindingProvider       string
		bindingAccountID      string
		bindingChannelID      string
		bindingConversationID string
		bindingThreadID       string
		bindingExternalUserID string
		createdAt             string
		lastActivityAt        string
	)
	err := row.Scan(&boundary.SessionID, &boundary.RoutingKey, &boundary.TenantID, &boundary.Partition, &scope, &boundary.ActorID, &ownerKind, &boundary.Owner.ID, &boundary.ChannelID, &boundary.PeerID, &bindingProvider, &bindingAccountID, &bindingChannelID, &bindingConversationID, &bindingThreadID, &bindingExternalUserID, &trust, &createdAt, &lastActivityAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	boundary.Scope = core.SessionScope(scope)
	boundary.TrustClass = core.TrustClass(trust)
	if boundary.Owner.ID != "" || ownerKind != "" {
		boundary.Owner = core.DelegationSubjectRef{
			TenantID: boundary.TenantID,
			Kind:     ownerKind,
			ID:       boundary.Owner.ID,
		}
	}
	if bindingProvider != "" || bindingConversationID != "" {
		boundary.Binding = &core.SessionBinding{
			Provider:       bindingProvider,
			AccountID:      bindingAccountID,
			ChannelID:      bindingChannelID,
			ConversationID: bindingConversationID,
			ThreadID:       bindingThreadID,
			ExternalUserID: bindingExternalUserID,
		}
	}
	boundary.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(lastActivityAt) == "" {
		lastActivityAt = createdAt
	}
	boundary.LastActivityAt, err = time.Parse(time.RFC3339Nano, lastActivityAt)
	if err != nil {
		return nil, err
	}
	return &boundary, nil
}

func (s *SQLiteSessionStore) UpsertBoundary(ctx context.Context, key string, boundary *netsession.SessionBoundary) error {
	if boundary == nil {
		return errors.New("boundary required")
	}
	if err := s.deleteExpiredBoundaries(ctx); err != nil {
		return err
	}
	if boundary.HasCanonicalOwner() && !boundary.AllowsLegacyActorOwnership() {
		boundary.ActorID = ""
	}
	if boundary.CreatedAt.IsZero() {
		boundary.CreatedAt = s.nowUTC()
	}
	if boundary.LastActivityAt.IsZero() {
		boundary.LastActivityAt = boundary.CreatedAt
	}
	ownerKind := ""
	ownerID := ""
	if boundary.Owner.ID != "" {
		ownerKind = boundary.Owner.Kind
		ownerID = boundary.Owner.ID
	}
	bindingProvider := ""
	bindingAccountID := ""
	bindingChannelID := ""
	bindingConversationID := ""
	bindingThreadID := ""
	bindingExternalUserID := ""
	if boundary.Binding != nil {
		bindingProvider = string(boundary.Binding.Provider)
		bindingAccountID = boundary.Binding.AccountID
		bindingChannelID = boundary.Binding.ChannelID
		bindingConversationID = boundary.Binding.ConversationID
		bindingThreadID = boundary.Binding.ThreadID
		bindingExternalUserID = boundary.Binding.ExternalUserID
	}
	if boundary.RoutingKey == "" {
		boundary.RoutingKey = key
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO session_boundaries (key, session_id, routing_key, tenant_id, partition, scope, actor_id, owner_kind, owner_id, channel_id, peer_id, binding_provider, binding_account_id, binding_channel_id, binding_conversation_id, binding_thread_id, binding_external_user_id, trust_class, created_at, last_activity_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET
			session_id = excluded.session_id,
			routing_key = excluded.routing_key,
			tenant_id = excluded.tenant_id,
			partition = excluded.partition,
			scope = excluded.scope,
			actor_id = excluded.actor_id,
			owner_kind = excluded.owner_kind,
			owner_id = excluded.owner_id,
			channel_id = excluded.channel_id,
			peer_id = excluded.peer_id,
			binding_provider = excluded.binding_provider,
			binding_account_id = excluded.binding_account_id,
			binding_channel_id = excluded.binding_channel_id,
			binding_conversation_id = excluded.binding_conversation_id,
			binding_thread_id = excluded.binding_thread_id,
			binding_external_user_id = excluded.binding_external_user_id,
			trust_class = excluded.trust_class,
			created_at = excluded.created_at,
			last_activity_at = excluded.last_activity_at`,
		key, boundary.SessionID, boundary.RoutingKey, boundary.TenantID, boundary.Partition, string(boundary.Scope), boundary.ActorID, ownerKind, ownerID, boundary.ChannelID, boundary.PeerID, bindingProvider, bindingAccountID, bindingChannelID, bindingConversationID, bindingThreadID, bindingExternalUserID, string(boundary.TrustClass), boundary.CreatedAt.UTC().Format(time.RFC3339Nano), boundary.LastActivityAt.UTC().Format(time.RFC3339Nano))
	return err
}

func (s *SQLiteSessionStore) ListBoundaries(ctx context.Context, partition string) ([]netsession.SessionBoundary, error) {
	if err := s.deleteExpiredBoundaries(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT session_id, routing_key, tenant_id, partition, scope, actor_id, owner_kind, owner_id, channel_id, peer_id, binding_provider, binding_account_id, binding_channel_id, binding_conversation_id, binding_thread_id, binding_external_user_id, trust_class, created_at, last_activity_at FROM session_boundaries WHERE partition = ? ORDER BY last_activity_at ASC, created_at ASC`, partition)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []netsession.SessionBoundary
	for rows.Next() {
		var (
			boundary              netsession.SessionBoundary
			scope                 string
			ownerKind             string
			trust                 string
			bindingProvider       string
			bindingAccountID      string
			bindingChannelID      string
			bindingConversationID string
			bindingThreadID       string
			bindingExternalUserID string
			createdAt             string
			lastActivityAt        string
		)
		if err := rows.Scan(&boundary.SessionID, &boundary.RoutingKey, &boundary.TenantID, &boundary.Partition, &scope, &boundary.ActorID, &ownerKind, &boundary.Owner.ID, &boundary.ChannelID, &boundary.PeerID, &bindingProvider, &bindingAccountID, &bindingChannelID, &bindingConversationID, &bindingThreadID, &bindingExternalUserID, &trust, &createdAt, &lastActivityAt); err != nil {
			return nil, err
		}
		boundary.Scope = core.SessionScope(scope)
		boundary.TrustClass = core.TrustClass(trust)
		if boundary.Owner.ID != "" || ownerKind != "" {
			boundary.Owner = core.DelegationSubjectRef{
				TenantID: boundary.TenantID,
				Kind:     ownerKind,
				ID:       boundary.Owner.ID,
			}
		}
		if bindingProvider != "" || bindingConversationID != "" {
			boundary.Binding = &core.SessionBinding{
				Provider:       bindingProvider,
				AccountID:      bindingAccountID,
				ChannelID:      bindingChannelID,
				ConversationID: bindingConversationID,
				ThreadID:       bindingThreadID,
				ExternalUserID: bindingExternalUserID,
			}
		}
		boundary.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(lastActivityAt) == "" {
			lastActivityAt = createdAt
		}
		boundary.LastActivityAt, err = time.Parse(time.RFC3339Nano, lastActivityAt)
		if err != nil {
			return nil, err
		}
		out = append(out, boundary)
	}
	return out, rows.Err()
}

func (s *SQLiteSessionStore) UpsertDelegation(ctx context.Context, record core.SessionDelegationRecord) error {
	if err := record.Validate(); err != nil {
		return err
	}
	operationsJSON, _ := json.Marshal(record.Operations)
	expiresAt := ""
	if !record.ExpiresAt.IsZero() {
		expiresAt = record.ExpiresAt.UTC().Format(time.RFC3339Nano)
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = s.nowUTC()
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO session_delegations (session_id, tenant_id, grantee_kind, grantee_id, operations_json, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(session_id, grantee_kind, grantee_id) DO UPDATE SET
			tenant_id = excluded.tenant_id,
			operations_json = excluded.operations_json,
			created_at = excluded.created_at,
			expires_at = excluded.expires_at`,
		record.SessionID, record.TenantID, string(record.Grantee.Kind), record.Grantee.ID, string(operationsJSON), record.CreatedAt.UTC().Format(time.RFC3339Nano), expiresAt)
	return err
}

func (s *SQLiteSessionStore) ListDelegationsBySessionID(ctx context.Context, sessionID string) ([]core.SessionDelegationRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT session_id, tenant_id, grantee_kind, grantee_id, operations_json, created_at, expires_at FROM session_delegations WHERE session_id = ? ORDER BY grantee_kind ASC, grantee_id ASC`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []core.SessionDelegationRecord
	for rows.Next() {
		var record core.SessionDelegationRecord
		var granteeKind, operationsJSON, createdAt, expiresAt string
		if err := rows.Scan(&record.SessionID, &record.TenantID, &granteeKind, &record.Grantee.ID, &operationsJSON, &createdAt, &expiresAt); err != nil {
			return nil, err
		}
		record.Grantee = core.DelegationSubjectRef{
			TenantID: record.TenantID,
			Kind:     granteeKind,
			ID:       record.Grantee.ID,
		}
		_ = json.Unmarshal([]byte(operationsJSON), &record.Operations)
		record.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(expiresAt) != "" {
			record.ExpiresAt, err = time.Parse(time.RFC3339Nano, expiresAt)
			if err != nil {
				return nil, err
			}
		}
		if !record.ExpiresAt.IsZero() && s.nowUTC().After(record.ExpiresAt) {
			continue
		}
		out = append(out, record)
	}
	return out, rows.Err()
}

func (s *SQLiteSessionStore) ListDelegationsByTenantID(ctx context.Context, tenantID string) ([]core.SessionDelegationRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT session_id, tenant_id, grantee_kind, grantee_id, operations_json, created_at, expires_at FROM session_delegations WHERE tenant_id = ? ORDER BY session_id ASC, grantee_kind ASC, grantee_id ASC`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []core.SessionDelegationRecord
	for rows.Next() {
		var record core.SessionDelegationRecord
		var granteeKind, operationsJSON, createdAt, expiresAt string
		if err := rows.Scan(&record.SessionID, &record.TenantID, &granteeKind, &record.Grantee.ID, &operationsJSON, &createdAt, &expiresAt); err != nil {
			return nil, err
		}
		record.Grantee = core.DelegationSubjectRef{
			TenantID: record.TenantID,
			Kind:     granteeKind,
			ID:       record.Grantee.ID,
		}
		_ = json.Unmarshal([]byte(operationsJSON), &record.Operations)
		record.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(expiresAt) != "" {
			record.ExpiresAt, err = time.Parse(time.RFC3339Nano, expiresAt)
			if err != nil {
				return nil, err
			}
		}
		if !record.ExpiresAt.IsZero() && s.nowUTC().After(record.ExpiresAt) {
			continue
		}
		out = append(out, record)
	}
	return out, rows.Err()
}

func (s *SQLiteSessionStore) DeleteBoundary(ctx context.Context, key string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM session_boundaries WHERE key = ?`, key)
	return err
}

func (s *SQLiteSessionStore) DeleteExpiredBoundaries(ctx context.Context, before time.Time) (int, error) {
	if s == nil || s.db == nil {
		return 0, nil
	}
	if before.IsZero() {
		before = s.nowUTC()
	}
	rows, err := s.db.QueryContext(ctx, `SELECT key, created_at, last_activity_at FROM session_boundaries`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	expiredKeys := make([]string, 0)
	for rows.Next() {
		var key string
		var createdAt string
		var lastActivityAt string
		if err := rows.Scan(&key, &createdAt, &lastActivityAt); err != nil {
			return 0, err
		}
		if strings.TrimSpace(lastActivityAt) == "" {
			lastActivityAt = createdAt
		}
		lastSeen, err := time.Parse(time.RFC3339Nano, lastActivityAt)
		if err != nil {
			return 0, err
		}
		if !lastSeen.After(before) {
			expiredKeys = append(expiredKeys, key)
		}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	deleted := 0
	for _, key := range expiredKeys {
		if _, err := s.db.ExecContext(ctx, `DELETE FROM session_boundaries WHERE key = ?`, key); err != nil {
			return deleted, err
		}
		deleted++
	}
	return deleted, nil
}

func (s *SQLiteSessionStore) deleteExpiredBoundaries(ctx context.Context) error {
	if s == nil || s.BoundaryTTL <= 0 {
		return nil
	}
	_, err := s.DeleteExpiredBoundaries(ctx, s.nowUTC().Add(-s.BoundaryTTL))
	return err
}

func (s *SQLiteSessionStore) nowUTC() time.Time {
	if s != nil && s.now != nil {
		return s.now().UTC()
	}
	return time.Now().UTC()
}

func (s *SQLiteSessionStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}
