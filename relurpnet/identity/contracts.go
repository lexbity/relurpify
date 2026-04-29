package identity

import (
	"context"
	"time"
)

// AdminTokenRecord is the persistent metadata for a runtime-issued admin token.
// Defined here to avoid import cycles with framework/authorization.
type AdminTokenRecord struct {
	ID          string      `json:"id" yaml:"id"`
	Name        string      `json:"name,omitempty" yaml:"name,omitempty"`
	TenantID    string      `json:"tenant_id,omitempty" yaml:"tenant_id,omitempty"`
	SubjectKind SubjectKind `json:"subject_kind,omitempty" yaml:"subject_kind,omitempty"`
	SubjectID   string      `json:"subject_id,omitempty" yaml:"subject_id,omitempty"`
	TokenHash   string      `json:"-" yaml:"token_hash"`
	Scopes      []string    `json:"scopes,omitempty" yaml:"scopes,omitempty"`
	IssuedAt    time.Time   `json:"issued_at" yaml:"issued_at"`
	ExpiresAt   *time.Time  `json:"expires_at,omitempty" yaml:"expires_at,omitempty"`
	LastUsedAt  *time.Time  `json:"last_used_at,omitempty" yaml:"last_used_at,omitempty"`
	RevokedAt   *time.Time  `json:"revoked_at,omitempty" yaml:"revoked_at,omitempty"`
}

// TokenLookupStore resolves a presented bearer token hash to the stored token
// metadata used to build a connection principal.
type TokenLookupStore interface {
	GetTokenByHash(ctx context.Context, tokenHash string) (*AdminTokenRecord, error)
}

// SubjectLookupStore resolves tenants and subjects explicitly by tenant, kind,
// and subject identifier. The resolver must not infer subject kind from the
// bare subject ID.
type SubjectLookupStore interface {
	GetTenant(ctx context.Context, tenantID string) (*TenantRecord, error)
	GetSubject(ctx context.Context, tenantID string, kind SubjectKind, subjectID string) (*SubjectRecord, error)
}

// PrincipalResolver resolves a bearer token into a gateway connection
// principal. Gateway and other network-facing servers use this interface to
// separate transport auth from application composition.
type PrincipalResolver interface {
	ResolvePrincipal(ctx context.Context, token string) (ConnectionPrincipal, error)
}

// StaticTokenBinding describes a configured bearer token that should resolve
// without consulting the persistent token store.
type StaticTokenBinding struct {
	Token       string
	TenantID    string
	Role        string
	SubjectKind SubjectKind
	SubjectID   string
	Scopes      []string
}
