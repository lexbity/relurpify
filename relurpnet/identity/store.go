package identity

import (
	"context"
	"errors"
)

// ErrNotFound is returned by Get* methods when the requested record does not exist.
var ErrNotFound = errors.New("identity: not found")

// TenantStore persists tenant records.
type TenantStore interface {
	UpsertTenant(ctx context.Context, tenant TenantRecord) error
	GetTenant(ctx context.Context, tenantID string) (*TenantRecord, error)
	ListTenants(ctx context.Context) ([]TenantRecord, error)
}

// SubjectStore persists tenant-scoped subjects and external identities.
type SubjectStore interface {
	UpsertSubject(ctx context.Context, subject SubjectRecord) error
	GetSubject(ctx context.Context, tenantID string, kind SubjectKind, subjectID string) (*SubjectRecord, error)
	ListSubjects(ctx context.Context, tenantID string) ([]SubjectRecord, error)

	UpsertExternalIdentity(ctx context.Context, identity ExternalIdentity) error
	GetExternalIdentity(ctx context.Context, tenantID string, provider ExternalProvider, accountID, externalID string) (*ExternalIdentity, error)
	ListExternalIdentities(ctx context.Context, tenantID string) ([]ExternalIdentity, error)
}

// EnrollmentStore persists node enrollment records.
type EnrollmentStore interface {
	UpsertNodeEnrollment(ctx context.Context, enrollment NodeEnrollment) error
	GetNodeEnrollment(ctx context.Context, tenantID, nodeID string) (*NodeEnrollment, error)
	ListNodeEnrollments(ctx context.Context, tenantID string) ([]NodeEnrollment, error)
	DeleteNodeEnrollment(ctx context.Context, tenantID, nodeID string) error
}

// Store is kept as a composed interface for implementations that satisfy all three.
type Store interface {
	TenantStore
	SubjectStore
	EnrollmentStore
}
