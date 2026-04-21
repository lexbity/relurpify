package fmp

import (
	"context"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/core"
)

type SessionLineageRequest struct {
	LineageID                string
	SessionID                string
	TaskClass                string
	ContextClass             string
	CapabilityEnvelope       core.CapabilityEnvelope
	SensitivityClass         core.SensitivityClass
	AllowedFederationTargets []string
}

type AuthorizedActor struct {
	Subject    core.SubjectRef
	Delegated  bool
	SessionID  string
	TenantID   string
	TrustClass core.TrustClass
}

func (s *Service) CreateLineageFromSession(ctx context.Context, req SessionLineageRequest) (*core.LineageRecord, error) {
	if s.Nexus.Sessions == nil {
		return nil, fmt.Errorf("session lookup unavailable")
	}
	boundary, err := s.Nexus.Sessions.GetBoundaryBySessionID(ctx, strings.TrimSpace(req.SessionID))
	if err != nil {
		return nil, err
	}
	if boundary == nil {
		return nil, fmt.Errorf("session %s not found", req.SessionID)
	}
	if !boundary.HasCanonicalOwner() {
		return nil, fmt.Errorf("session %s has no canonical owner", req.SessionID)
	}
	if err := s.ensureTenantAndOwner(ctx, boundary.TenantID, boundary.Owner); err != nil {
		return nil, err
	}
	delegations, err := s.loadSessionDelegations(ctx, boundary.SessionID)
	if err != nil {
		return nil, err
	}
	lineage := &core.LineageRecord{
		LineageID:                strings.TrimSpace(req.LineageID),
		TenantID:                 boundary.TenantID,
		TaskClass:                strings.TrimSpace(req.TaskClass),
		ContextClass:             strings.TrimSpace(req.ContextClass),
		CapabilityEnvelope:       req.CapabilityEnvelope,
		SensitivityClass:         req.SensitivityClass,
		AllowedFederationTargets: append([]string(nil), req.AllowedFederationTargets...),
		Owner:                    boundary.Owner,
		SessionID:                boundary.SessionID,
		SessionBinding:           cloneSessionBinding(boundary.Binding),
		Delegations:              delegations,
		TrustClass:               boundary.TrustClass,
		CreatedAt:                s.nowUTC(),
		UpdatedAt:                s.nowUTC(),
	}
	if err := s.CreateLineage(ctx, *lineage); err != nil {
		return nil, err
	}
	return lineage, nil
}

func (s *Service) AuthorizeResumeActor(ctx context.Context, lineageID string, actor core.SubjectRef, operation core.SessionOperation) (*AuthorizedActor, error) {
	lineage, ok, err := s.Ownership.GetLineage(ctx, lineageID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("lineage %s not found", lineageID)
	}
	if err := actor.Validate(); err != nil {
		return nil, err
	}
	if !strings.EqualFold(actor.TenantID, lineage.TenantID) {
		return nil, fmt.Errorf("actor tenant %s cannot access lineage %s in tenant %s", actor.TenantID, lineageID, lineage.TenantID)
	}
	if s.Nexus.Subjects != nil {
		subject, err := s.Nexus.Subjects.GetSubject(ctx, actor.TenantID, actor.Kind, actor.ID)
		if err != nil {
			return nil, err
		}
		if subject == nil {
			return nil, fmt.Errorf("subject %s/%s not found in tenant %s", actor.Kind, actor.ID, actor.TenantID)
		}
	}
	if actor == lineage.Owner {
		return &AuthorizedActor{
			Subject:    actor,
			Delegated:  false,
			SessionID:  lineage.SessionID,
			TenantID:   lineage.TenantID,
			TrustClass: lineage.TrustClass,
		}, nil
	}
	eventActor := core.EventActor{
		ID:          actor.ID,
		TenantID:    actor.TenantID,
		SubjectKind: actor.Kind,
	}
	for _, delegation := range lineage.Delegations {
		if delegation.Allows(eventActor, operation, s.nowUTC()) {
			return &AuthorizedActor{
				Subject:    actor,
				Delegated:  true,
				SessionID:  lineage.SessionID,
				TenantID:   lineage.TenantID,
				TrustClass: lineage.TrustClass,
			}, nil
		}
	}
	return nil, fmt.Errorf("actor %s cannot resume lineage %s without explicit delegation", actor.ID, lineageID)
}

func (s *Service) AcceptHandoffForNode(ctx context.Context, offer core.HandoffOffer, destination core.ExportDescriptor, runtimeID, nodeID string, actor core.SubjectRef) (*core.HandoffAccept, *AuthorizedActor, error) {
	authorized, err := s.AuthorizeResumeActor(ctx, offer.LineageID, actor, core.SessionOperationResume)
	if err != nil {
		return nil, nil, err
	}
	if err := s.validateDestinationNode(ctx, authorized.TenantID, nodeID); err != nil {
		return nil, nil, err
	}
	accept, refusal, err := s.tryAcceptHandoff(ctx, offer, destination, runtimeID, authorized)
	if err != nil {
		return nil, nil, err
	}
	if refusal != nil {
		return nil, nil, fmt.Errorf("resume refused: %s", refusal.Message)
	}
	return accept, authorized, nil
}

func (s *Service) ensureTenantAndOwner(ctx context.Context, tenantID string, owner core.SubjectRef) error {
	if strings.TrimSpace(tenantID) == "" {
		return fmt.Errorf("tenant id required")
	}
	if s.Nexus.Tenants != nil {
		tenant, err := s.Nexus.Tenants.GetTenant(ctx, tenantID)
		if err != nil {
			return err
		}
		if tenant == nil {
			return fmt.Errorf("tenant %s not found", tenantID)
		}
		if tenant.DisabledAt != nil && !tenant.DisabledAt.After(s.nowUTC()) {
			return fmt.Errorf("tenant %s disabled", tenantID)
		}
	}
	if s.Nexus.Subjects != nil {
		subject, err := s.Nexus.Subjects.GetSubject(ctx, owner.TenantID, owner.Kind, owner.ID)
		if err != nil {
			return err
		}
		if subject == nil {
			return fmt.Errorf("owner %s/%s not found in tenant %s", owner.Kind, owner.ID, owner.TenantID)
		}
		if subject.DisabledAt != nil && !subject.DisabledAt.After(s.nowUTC()) {
			return fmt.Errorf("owner %s/%s disabled", owner.Kind, owner.ID)
		}
	}
	return nil
}

func (s *Service) loadSessionDelegations(ctx context.Context, sessionID string) ([]core.SessionDelegationRecord, error) {
	if s.Nexus.Sessions == nil {
		return nil, nil
	}
	records, err := s.Nexus.Sessions.ListDelegationsBySessionID(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	now := s.nowUTC()
	out := make([]core.SessionDelegationRecord, 0, len(records))
	for _, record := range records {
		if err := record.Validate(); err != nil {
			return nil, err
		}
		if !record.ExpiresAt.IsZero() && now.After(record.ExpiresAt) {
			continue
		}
		out = append(out, record)
	}
	return out, nil
}

func (s *Service) validateDestinationNode(ctx context.Context, tenantID, nodeID string) error {
	if strings.TrimSpace(nodeID) == "" {
		return fmt.Errorf("node id required")
	}
	if s.Nexus.Nodes == nil {
		return nil
	}
	enrollment, err := s.Nexus.Nodes.GetNodeEnrollment(ctx, tenantID, nodeID)
	if err != nil {
		return err
	}
	if enrollment == nil {
		return fmt.Errorf("node enrollment not found for tenant %s node %s", tenantID, nodeID)
	}
	if !strings.EqualFold(enrollment.TenantID, tenantID) {
		return fmt.Errorf("node %s enrolled in tenant %s, not tenant %s", nodeID, enrollment.TenantID, tenantID)
	}
	if !enrollment.LastVerifiedAt.IsZero() && enrollment.LastVerifiedAt.Before(enrollment.PairedAt) {
		return fmt.Errorf("node %s enrollment verification invalid", nodeID)
	}
	return nil
}

func cloneSessionBinding(binding *core.ExternalSessionBinding) *core.ExternalSessionBinding {
	if binding == nil {
		return nil
	}
	copy := *binding
	return &copy
}
