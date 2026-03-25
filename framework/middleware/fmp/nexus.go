package fmp

import (
	"context"
	"sort"
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

type PolicyRuleLookup interface {
	ListRules(ctx context.Context) ([]core.PolicyRule, error)
}

type ResumePolicyRequest struct {
	Lineage      core.LineageRecord
	Offer        core.HandoffOffer
	Destination  core.ExportDescriptor
	SourceDomain string
	Actor        core.SubjectRef
	IsOwner      bool
	IsDelegated  bool
	RouteMode    core.RouteMode
}

type AuthorizationPolicyResolver struct {
	Engine authorization.PolicyEngine
	Rules  PolicyRuleLookup
	TTL    time.Duration
	Now    func() time.Time

	cachedRules []core.PolicyRule
	cachedAt    time.Time
}

func (r *AuthorizationPolicyResolver) EvaluateResume(ctx context.Context, req ResumePolicyRequest) (core.PolicyDecision, error) {
	if decision, ok, err := r.evaluateResumeRules(ctx, req); err != nil {
		return core.PolicyDecision{}, err
	} else if ok {
		return decision, nil
	}
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
		SourceDomain:     req.SourceDomain,
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

func (r *AuthorizationPolicyResolver) evaluateResumeRules(ctx context.Context, req ResumePolicyRequest) (core.PolicyDecision, bool, error) {
	rules, err := r.loadResumeRules(ctx)
	if err != nil || len(rules) == 0 {
		return core.PolicyDecision{}, false, err
	}
	policyReq := core.PolicyRequest{
		Target: core.PolicyTargetResume,
		Actor: core.EventActor{
			Kind:        string(req.Actor.Kind),
			ID:          req.Actor.ID,
			TenantID:    req.Actor.TenantID,
			SubjectKind: req.Actor.Kind,
		},
		Authenticated:    true,
		ActorTenantID:    req.Actor.TenantID,
		ResourceTenantID: req.Lineage.TenantID,
		LineageID:        req.Lineage.LineageID,
		AttemptID:        req.Offer.SourceAttemptID,
		ExportName:       req.Destination.ExportName,
		SourceDomain:     req.SourceDomain,
		ContextClass:     req.Offer.ContextClass,
		SensitivityClass: req.Offer.SensitivityClass,
		RouteMode:        req.RouteMode,
		TrustClass:       req.Lineage.TrustClass,
		SessionID:        req.Lineage.SessionID,
		SessionOperation: core.SessionOperationResume,
		SessionOwnerID:   req.Lineage.Owner.ID,
		IsOwner:          req.IsOwner,
		IsDelegated:      req.IsDelegated,
		Timestamp:        r.nowUTC(),
	}
	decision := authorization.EvaluatePolicyRules(rules, policyReq)
	if decision == nil {
		return core.PolicyDecision{}, false, nil
	}
	return *decision, true, nil
}

func (r *AuthorizationPolicyResolver) loadResumeRules(ctx context.Context) ([]core.PolicyRule, error) {
	if r == nil || r.Rules == nil {
		return nil, nil
	}
	if r.cacheFresh() {
		out := make([]core.PolicyRule, len(r.cachedRules))
		copy(out, r.cachedRules)
		return out, nil
	}
	rules, err := r.Rules.ListRules(ctx)
	if err != nil {
		return nil, err
	}
	filtered := make([]core.PolicyRule, 0, len(rules))
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		if len(rule.Conditions.SessionOperations) > 0 && !containsResumeOperation(rule.Conditions.SessionOperations) {
			continue
		}
		filtered = append(filtered, rule)
	}
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].Priority == filtered[j].Priority {
			return filtered[i].ID < filtered[j].ID
		}
		return filtered[i].Priority < filtered[j].Priority
	})
	r.cachedRules = append([]core.PolicyRule(nil), filtered...)
	r.cachedAt = r.nowUTC()
	out := make([]core.PolicyRule, len(filtered))
	copy(out, filtered)
	return out, nil
}

func (r *AuthorizationPolicyResolver) cacheFresh() bool {
	if r == nil || len(r.cachedRules) == 0 {
		return false
	}
	ttl := r.TTL
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	return r.nowUTC().Before(r.cachedAt.Add(ttl))
}

func (r *AuthorizationPolicyResolver) nowUTC() time.Time {
	if r != nil && r.Now != nil {
		return r.Now().UTC()
	}
	return time.Now().UTC()
}

func containsResumeOperation(values []core.SessionOperation) bool {
	for _, value := range values {
		if value == core.SessionOperationResume {
			return true
		}
	}
	return false
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
