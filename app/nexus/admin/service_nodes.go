package admin

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
)

func (s *service) ListNodes(ctx context.Context, req ListNodesRequest) (ListNodesResult, error) {
	tenantID, err := authorizeTenant(req.Principal, req.TenantID)
	if err != nil {
		return ListNodesResult{}, err
	}
	nodes, err := s.cfg.Nodes.ListNodes(ctx)
	if err != nil {
		return ListNodesResult{}, internalError("list nodes failed", err, nil)
	}
	nodes = filterNodesByTenant(nodes, tenantID)
	nodes = applyPage(nodes, req.Page)
	return ListNodesResult{
		AdminResult: resultEnvelope(req.AdminRequest),
		PageResult:  pageResult(len(nodes)),
		Nodes:       nodes,
	}, nil
}

func (s *service) GetNode(ctx context.Context, req GetNodeRequest) (GetNodeResult, error) {
	if strings.TrimSpace(req.NodeID) == "" {
		return GetNodeResult{}, invalidArgument("node_id required", map[string]any{"field": "node_id"})
	}
	tenantID, err := authorizeTenant(req.Principal, req.TenantID)
	if err != nil {
		return GetNodeResult{}, err
	}
	node, err := s.cfg.Nodes.GetNode(ctx, req.NodeID)
	if err != nil {
		return GetNodeResult{}, internalError("get node failed", err, map[string]any{"node_id": req.NodeID})
	}
	if node == nil {
		return GetNodeResult{}, notFound("node not found", map[string]any{"node_id": req.NodeID})
	}
	if tenantID != "" && node.TenantID != "" && !strings.EqualFold(node.TenantID, tenantID) {
		return GetNodeResult{}, notFound("node not found", map[string]any{"node_id": req.NodeID})
	}
	return GetNodeResult{AdminResult: resultEnvelope(req.AdminRequest), Node: node}, nil
}

func (s *service) UpdateNodeCapabilities(ctx context.Context, req UpdateNodeCapabilitiesRequest) (UpdateNodeCapabilitiesResult, error) {
	if strings.TrimSpace(req.NodeID) == "" {
		return UpdateNodeCapabilitiesResult{}, invalidArgument("node_id required", map[string]any{"field": "node_id"})
	}
	tenantID, err := authorizeTenant(req.Principal, req.TenantID)
	if err != nil {
		return UpdateNodeCapabilitiesResult{}, err
	}
	node, err := s.cfg.Nodes.GetNode(ctx, req.NodeID)
	if err != nil {
		return UpdateNodeCapabilitiesResult{}, internalError("get node failed", err, map[string]any{"node_id": req.NodeID})
	}
	if node == nil || (tenantID != "" && node.TenantID != "" && !strings.EqualFold(node.TenantID, tenantID)) {
		return UpdateNodeCapabilitiesResult{}, notFound("node not found", map[string]any{"node_id": req.NodeID})
	}
	node.ApprovedCapabilities = sanitizeApprovedCapabilities(req.Capabilities)
	if err := s.cfg.Nodes.UpsertNode(ctx, *node); err != nil {
		return UpdateNodeCapabilitiesResult{}, internalError("update node capabilities failed", err, map[string]any{"node_id": req.NodeID})
	}
	return UpdateNodeCapabilitiesResult{AdminResult: resultEnvelope(req.AdminRequest), Node: node}, nil
}

func (s *service) RevokeNode(ctx context.Context, req RevokeNodeRequest) (RevokeNodeResult, error) {
	if strings.TrimSpace(req.NodeID) == "" {
		return RevokeNodeResult{}, invalidArgument("node_id required", map[string]any{"field": "node_id"})
	}
	tenantID, err := authorizeTenant(req.Principal, req.TenantID)
	if err != nil {
		return RevokeNodeResult{}, err
	}
	node, err := s.cfg.Nodes.GetNode(ctx, req.NodeID)
	if err != nil {
		return RevokeNodeResult{}, internalError("get node failed", err, map[string]any{"node_id": req.NodeID})
	}
	if node == nil || (tenantID != "" && node.TenantID != "" && !strings.EqualFold(node.TenantID, tenantID)) {
		return RevokeNodeResult{}, notFound("node not found", map[string]any{"node_id": req.NodeID})
	}
	if err := s.cfg.Nodes.RemoveNode(ctx, req.NodeID); err != nil {
		return RevokeNodeResult{}, internalError("revoke node failed", err, map[string]any{"node_id": req.NodeID})
	}
	return RevokeNodeResult{AdminResult: resultEnvelope(req.AdminRequest), NodeID: req.NodeID}, nil
}

func sanitizeApprovedCapabilities(in []core.CapabilityDescriptor) []core.CapabilityDescriptor {
	if len(in) == 0 {
		return nil
	}
	out := make([]core.CapabilityDescriptor, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, desc := range in {
		id := strings.TrimSpace(desc.ID)
		name := strings.TrimSpace(desc.Name)
		if id == "" && name == "" {
			continue
		}
		key := strings.ToLower(firstNonEmpty(id, name))
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		desc.ID = id
		desc.Name = name
		out = append(out, desc)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (s *service) ListPendingPairings(ctx context.Context, req ListPendingPairingsRequest) (ListPendingPairingsResult, error) {
	tenantID, err := authorizeTenant(req.Principal, req.TenantID)
	if err != nil {
		return ListPendingPairingsResult{}, err
	}
	if s.cfg.Nodes != nil {
		_, _ = s.cfg.Nodes.DeleteExpiredPendingPairings(ctx, time.Now().UTC())
	}
	pairings, err := s.cfg.Nodes.ListPendingPairings(ctx)
	if err != nil {
		return ListPendingPairingsResult{}, internalError("list pending pairings failed", err, nil)
	}
	filtered := make([]PendingPairingInfo, 0, len(pairings))
	for _, pairing := range pairings {
		if tenantID != "" && pairing.Cred.TenantID != "" && !strings.EqualFold(tenantID, pairing.Cred.TenantID) {
			continue
		}
		filtered = append(filtered, PendingPairingInfo{
			Code:      pairing.Code,
			DeviceID:  pairing.Cred.DeviceID,
			IssuedAt:  pairing.Cred.IssuedAt,
			ExpiresAt: pairing.ExpiresAt,
		})
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].ExpiresAt.Before(filtered[j].ExpiresAt) })
	filtered = applyPage(filtered, req.Page)
	return ListPendingPairingsResult{
		AdminResult: resultEnvelope(req.AdminRequest),
		PageResult:  pageResult(len(filtered)),
		Pairings:    filtered,
	}, nil
}

func (s *service) ApprovePairing(ctx context.Context, req ApprovePairingRequest) (ApprovePairingResult, error) {
	if strings.TrimSpace(req.Code) == "" {
		return ApprovePairingResult{}, invalidArgument("pairing code required", map[string]any{"field": "code"})
	}
	tenantID, err := authorizeTenant(req.Principal, req.TenantID)
	if err != nil {
		return ApprovePairingResult{}, err
	}
	pairing, _, _ := s.cfg.NodeManager.PairingStatus(ctx, req.Code)
	if pairing != nil && tenantID != "" && pairing.Cred.TenantID != "" && !strings.EqualFold(tenantID, pairing.Cred.TenantID) {
		return ApprovePairingResult{}, notFound("pairing request not found", map[string]any{"code": req.Code})
	}
	if err := s.cfg.NodeManager.ApprovePairing(ctx, req.Code); err != nil {
		return ApprovePairingResult{}, notFound("pairing request not found", map[string]any{"code": req.Code, "cause": err.Error()})
	}
	result := ApprovePairingResult{AdminResult: resultEnvelope(req.AdminRequest)}
	if pairing != nil {
		result.NodeID = pairing.Cred.DeviceID
		result.PairedAt = pairing.Cred.IssuedAt
		if s.cfg.Identities != nil {
			enrollment := nodeEnrollmentFromPairing(*pairing)
			if err := upsertTenantAndSubject(ctx, s.cfg.Identities, enrollment.TenantID, enrollment.Owner.Kind, enrollment.Owner.ID, enrollment.Owner.ID, nil, enrollment.PairedAt); err != nil {
				return ApprovePairingResult{}, internalError("persist subject failed", err, map[string]any{"code": req.Code})
			}
			if err := s.cfg.Identities.UpsertNodeEnrollment(ctx, enrollment); err != nil {
				return ApprovePairingResult{}, internalError("persist node enrollment failed", err, map[string]any{"code": req.Code})
			}
			if s.cfg.Nodes != nil {
				if err := s.cfg.Nodes.UpsertNode(ctx, nodeDescriptorFromEnrollment(enrollment)); err != nil {
					return ApprovePairingResult{}, internalError("persist node descriptor failed", err, map[string]any{"code": req.Code})
				}
			}
		} else if s.cfg.Nodes != nil {
			// Compatibility path for tests/bootstrap modes without an identity store.
			if err := s.cfg.Nodes.UpsertNode(ctx, core.NodeDescriptor{
				ID:         pairing.Cred.DeviceID,
				Name:       pairing.Cred.DeviceID,
				Platform:   core.NodePlatformHeadless,
				TrustClass: core.TrustClassRemoteApproved,
				PairedAt:   pairing.Cred.IssuedAt,
			}); err != nil {
				return ApprovePairingResult{}, internalError("persist node descriptor failed", err, map[string]any{"code": req.Code})
			}
		}
	}
	return result, nil
}

func (s *service) RejectPairing(ctx context.Context, req RejectPairingRequest) (RejectPairingResult, error) {
	if strings.TrimSpace(req.Code) == "" {
		return RejectPairingResult{}, invalidArgument("pairing code required", map[string]any{"field": "code"})
	}
	tenantID, err := authorizeTenant(req.Principal, req.TenantID)
	if err != nil {
		return RejectPairingResult{}, err
	}
	pairing, _, _ := s.cfg.NodeManager.PairingStatus(ctx, req.Code)
	if pairing != nil && tenantID != "" && pairing.Cred.TenantID != "" && !strings.EqualFold(tenantID, pairing.Cred.TenantID) {
		return RejectPairingResult{}, notFound("pairing request not found", map[string]any{"code": req.Code})
	}
	if err := s.cfg.NodeManager.RejectPairing(ctx, req.Code); err != nil {
		return RejectPairingResult{}, notFound("pairing request not found", map[string]any{"code": req.Code, "cause": err.Error()})
	}
	return RejectPairingResult{
		AdminResult: resultEnvelope(req.AdminRequest),
		Code:        req.Code,
	}, nil
}

func (s *service) ListTenants(ctx context.Context, req ListTenantsRequest) (ListTenantsResult, error) {
	tenantID, err := authorizeTenant(req.Principal, req.TenantID)
	if err != nil {
		return ListTenantsResult{}, err
	}
	set := map[string]struct{}{}
	if !hasGlobalTenantScope(req.Principal) {
		if tenantID != "" {
			set[tenantID] = struct{}{}
		}
		tenants := make([]string, 0, len(set))
		for tenantID := range set {
			tenants = append(tenants, tenantID)
		}
		sort.Strings(tenants)
		tenants = applyPage(tenants, req.Page)
		return ListTenantsResult{AdminResult: resultEnvelope(req.AdminRequest), PageResult: pageResult(len(tenants)), Tenants: tenants}, nil
	}
	if s.cfg.Identities != nil {
		tenants, err := s.cfg.Identities.ListTenants(ctx)
		if err != nil {
			return ListTenantsResult{}, internalError("list tenants failed", err, nil)
		}
		for _, tenant := range tenants {
			if strings.TrimSpace(tenant.ID) != "" {
				set[tenant.ID] = struct{}{}
			}
		}
	}
	if s.cfg.Nodes != nil {
		nodes, err := s.cfg.Nodes.ListNodes(ctx)
		if err != nil {
			return ListTenantsResult{}, internalError("list tenants failed", err, nil)
		}
		for _, node := range nodes {
			if strings.TrimSpace(node.TenantID) != "" {
				set[node.TenantID] = struct{}{}
			}
		}
	}
	tenants := make([]string, 0, len(set))
	for tenantID := range set {
		tenants = append(tenants, tenantID)
	}
	sort.Strings(tenants)
	tenants = applyPage(tenants, req.Page)
	return ListTenantsResult{AdminResult: resultEnvelope(req.AdminRequest), PageResult: pageResult(len(tenants)), Tenants: tenants}, nil
}
