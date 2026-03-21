package fmp

import (
	"context"
	"time"

	"github.com/lexcodex/relurpify/framework/authorization"
	"github.com/lexcodex/relurpify/framework/core"
)

type TenantLookup interface {
	GetTenant(ctx context.Context, tenantID string) (*core.TenantRecord, error)
}

type SubjectLookup interface {
	GetSubject(ctx context.Context, tenantID string, kind core.SubjectKind, subjectID string) (*core.SubjectRecord, error)
}

type NodeEnrollmentLookup interface {
	GetNodeEnrollment(ctx context.Context, tenantID, nodeID string) (*core.NodeEnrollment, error)
}

type SessionBoundaryLookup interface {
	GetBoundaryBySessionID(ctx context.Context, sessionID string) (*core.SessionBoundary, error)
	ListDelegationsBySessionID(ctx context.Context, sessionID string) ([]core.SessionDelegationRecord, error)
}

type TenantExportLookup interface {
	IsExportEnabled(ctx context.Context, tenantID, exportName string) (bool, bool, error)
}

type TenantFederationPolicyLookup interface {
	GetTenantFederationPolicy(ctx context.Context, tenantID string) (*core.TenantFederationPolicy, error)
}

type PolicyResolver interface {
	EvaluateResume(ctx context.Context, req ResumePolicyRequest) (core.PolicyDecision, error)
}

type ResumePolicyRequest struct {
	Lineage     core.LineageRecord
	Offer       core.HandoffOffer
	Destination core.ExportDescriptor
	Actor       core.SubjectRef
	IsOwner     bool
	IsDelegated bool
	RouteMode   core.RouteMode
}

type AuthorizationPolicyResolver struct {
	Engine authorization.PolicyEngine
}

func (r AuthorizationPolicyResolver) EvaluateResume(ctx context.Context, req ResumePolicyRequest) (core.PolicyDecision, error) {
	actor := core.EventActor{
		Kind:        string(req.Actor.Kind),
		ID:          req.Actor.ID,
		TenantID:    req.Actor.TenantID,
		SubjectKind: req.Actor.Kind,
	}
	policyReq := core.PolicyRequest{
		Target:           core.PolicyTargetResume,
		Actor:            actor,
		Authenticated:    true,
		ActorTenantID:    req.Actor.TenantID,
		ResourceTenantID: req.Lineage.TenantID,
		LineageID:        req.Lineage.LineageID,
		AttemptID:        req.Offer.SourceAttemptID,
		ExportName:       req.Destination.ExportName,
		ContextClass:     req.Offer.ContextClass,
		SensitivityClass: req.Offer.SensitivityClass,
		RouteMode:        req.RouteMode,
		TrustClass:       req.Lineage.TrustClass,
		SessionID:        req.Lineage.SessionID,
		SessionOperation: core.SessionOperationResume,
		SessionOwnerID:   req.Lineage.Owner.ID,
		IsOwner:          req.IsOwner,
		IsDelegated:      req.IsDelegated,
		Timestamp:        time.Now().UTC(),
	}
	return authorization.EvaluatePolicyRequest(ctx, r.Engine, policyReq)
}

// NexusAdapter is part of the Phase 1 frozen FMP surface and marks the
// tenant-aware boundary between middleware protocol mechanics and Nexus
// control-plane authority.
type NexusAdapter struct {
	Tenants    TenantLookup
	Subjects   SubjectLookup
	Nodes      NodeEnrollmentLookup
	Sessions   SessionBoundaryLookup
	Exports    TenantExportLookup
	Federation TenantFederationPolicyLookup
	Policies   PolicyResolver
}
