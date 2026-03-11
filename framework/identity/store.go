package identity

import (
	"context"

	"github.com/lexcodex/relurpify/framework/core"
)

// Store persists tenant-scoped identities and node enrollments.
type Store interface {
	UpsertTenant(ctx context.Context, tenant core.TenantRecord) error
	GetTenant(ctx context.Context, tenantID string) (*core.TenantRecord, error)
	ListTenants(ctx context.Context) ([]core.TenantRecord, error)

	UpsertSubject(ctx context.Context, subject core.SubjectRecord) error
	GetSubject(ctx context.Context, tenantID string, kind core.SubjectKind, subjectID string) (*core.SubjectRecord, error)
	ListSubjects(ctx context.Context, tenantID string) ([]core.SubjectRecord, error)

	UpsertExternalIdentity(ctx context.Context, identity core.ExternalIdentity) error
	GetExternalIdentity(ctx context.Context, tenantID string, provider core.ExternalProvider, accountID, externalID string) (*core.ExternalIdentity, error)
	ListExternalIdentities(ctx context.Context, tenantID string) ([]core.ExternalIdentity, error)

	UpsertNodeEnrollment(ctx context.Context, enrollment core.NodeEnrollment) error
	GetNodeEnrollment(ctx context.Context, tenantID, nodeID string) (*core.NodeEnrollment, error)
	ListNodeEnrollments(ctx context.Context, tenantID string) ([]core.NodeEnrollment, error)
	DeleteNodeEnrollment(ctx context.Context, tenantID, nodeID string) error
}
