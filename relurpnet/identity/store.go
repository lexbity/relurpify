package identity

import (
	"context"
)

// Store persists tenant-scoped identities and node enrollments.
type Store interface {
	UpsertTenant(ctx context.Context, tenant TenantRecord) error
	GetTenant(ctx context.Context, tenantID string) (*TenantRecord, error)
	ListTenants(ctx context.Context) ([]TenantRecord, error)

	UpsertSubject(ctx context.Context, subject SubjectRecord) error
	GetSubject(ctx context.Context, tenantID string, kind SubjectKind, subjectID string) (*SubjectRecord, error)
	ListSubjects(ctx context.Context, tenantID string) ([]SubjectRecord, error)

	UpsertExternalIdentity(ctx context.Context, identity ExternalIdentity) error
	GetExternalIdentity(ctx context.Context, tenantID string, provider ExternalProvider, accountID, externalID string) (*ExternalIdentity, error)
	ListExternalIdentities(ctx context.Context, tenantID string) ([]ExternalIdentity, error)

	UpsertNodeEnrollment(ctx context.Context, enrollment NodeEnrollment) error
	GetNodeEnrollment(ctx context.Context, tenantID, nodeID string) (*NodeEnrollment, error)
	ListNodeEnrollments(ctx context.Context, tenantID string) ([]NodeEnrollment, error)
	DeleteNodeEnrollment(ctx context.Context, tenantID, nodeID string) error
}
