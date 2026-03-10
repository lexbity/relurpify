package identity

import (
	"context"

	"github.com/lexcodex/relurpify/framework/core"
)

// Store persists tenant-scoped identities and node enrollments.
type Store interface {
	UpsertExternalIdentity(ctx context.Context, identity core.ExternalIdentity) error
	GetExternalIdentity(ctx context.Context, tenantID string, provider core.ExternalProvider, accountID, externalID string) (*core.ExternalIdentity, error)
	ListExternalIdentities(ctx context.Context, tenantID string) ([]core.ExternalIdentity, error)

	UpsertNodeEnrollment(ctx context.Context, enrollment core.NodeEnrollment) error
	GetNodeEnrollment(ctx context.Context, tenantID, nodeID string) (*core.NodeEnrollment, error)
	ListNodeEnrollments(ctx context.Context, tenantID string) ([]core.NodeEnrollment, error)
}
