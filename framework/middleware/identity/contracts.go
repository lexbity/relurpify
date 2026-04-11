package identity

import (
	"context"

	"github.com/lexcodex/relurpify/framework/core"
	fwgateway "github.com/lexcodex/relurpify/framework/middleware/gateway"
)

// TokenLookupStore resolves a presented bearer token hash to the stored token
// metadata used to build a connection principal.
type TokenLookupStore interface {
	GetTokenByHash(ctx context.Context, tokenHash string) (*core.AdminTokenRecord, error)
}

// SubjectLookupStore resolves tenants and subjects explicitly by tenant, kind,
// and subject identifier. The resolver must not infer subject kind from the
// bare subject ID.
type SubjectLookupStore interface {
	GetTenant(ctx context.Context, tenantID string) (*core.TenantRecord, error)
	GetSubject(ctx context.Context, tenantID string, kind core.SubjectKind, subjectID string) (*core.SubjectRecord, error)
}

// PrincipalResolver resolves a bearer token into a gateway connection
// principal. Gateway and other network-facing servers use this interface to
// separate transport auth from application composition.
type PrincipalResolver interface {
	ResolvePrincipal(ctx context.Context, token string) (fwgateway.ConnectionPrincipal, error)
}

// StaticTokenBinding describes a configured bearer token that should resolve
// without consulting the persistent token store.
type StaticTokenBinding struct {
	Token       string
	TenantID    string
	Role        string
	SubjectKind core.SubjectKind
	SubjectID   string
	Scopes      []string
}
