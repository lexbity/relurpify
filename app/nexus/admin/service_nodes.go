package admin

import (
	"context"
	"sort"
	"strings"
	"time"

	nexusbootstrap "github.com/lexcodex/relurpify/app/nexus/bootstrap"
)

func (s *service) ListNodes(ctx context.Context, req ListNodesRequest) (ListNodesResult, error) {
	nodes, err := s.cfg.Nodes.ListNodes(ctx)
	if err != nil {
		return ListNodesResult{}, internalError("list nodes failed", err, nil)
	}
	nodes = filterNodesByTenant(nodes, req.TenantID)
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
	node, err := s.cfg.Nodes.GetNode(ctx, req.NodeID)
	if err != nil {
		return GetNodeResult{}, internalError("get node failed", err, map[string]any{"node_id": req.NodeID})
	}
	if node == nil {
		return GetNodeResult{}, notFound("node not found", map[string]any{"node_id": req.NodeID})
	}
	return GetNodeResult{AdminResult: resultEnvelope(req.AdminRequest), Node: node}, nil
}

func (s *service) RevokeNode(ctx context.Context, req RevokeNodeRequest) (RevokeNodeResult, error) {
	if strings.TrimSpace(req.NodeID) == "" {
		return RevokeNodeResult{}, invalidArgument("node_id required", map[string]any{"field": "node_id"})
	}
	if err := s.cfg.Nodes.RemoveNode(ctx, req.NodeID); err != nil {
		return RevokeNodeResult{}, internalError("revoke node failed", err, map[string]any{"node_id": req.NodeID})
	}
	return RevokeNodeResult{AdminResult: resultEnvelope(req.AdminRequest), NodeID: req.NodeID}, nil
}

func (s *service) ListPendingPairings(ctx context.Context, req ListPendingPairingsRequest) (ListPendingPairingsResult, error) {
	if s.cfg.Nodes != nil {
		_, _ = s.cfg.Nodes.DeleteExpiredPendingPairings(ctx, time.Now().UTC())
	}
	pairings, err := s.cfg.Nodes.ListPendingPairings(ctx)
	if err != nil {
		return ListPendingPairingsResult{}, internalError("list pending pairings failed", err, nil)
	}
	filtered := make([]PendingPairingInfo, 0, len(pairings))
	for _, pairing := range pairings {
		if req.TenantID != "" && pairing.Cred.TenantID != "" && !strings.EqualFold(req.TenantID, pairing.Cred.TenantID) {
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
	pairing, _, _ := s.cfg.NodeManager.PairingStatus(ctx, req.Code)
	if err := s.cfg.NodeManager.ApprovePairing(ctx, req.Code); err != nil {
		return ApprovePairingResult{}, notFound("pairing request not found", map[string]any{"code": req.Code, "cause": err.Error()})
	}
	result := ApprovePairingResult{AdminResult: resultEnvelope(req.AdminRequest)}
	if pairing != nil {
		result.NodeID = pairing.Cred.DeviceID
		result.PairedAt = pairing.Cred.IssuedAt
		if s.cfg.Identities != nil {
			if err := s.cfg.Identities.UpsertNodeEnrollment(ctx, nodeEnrollmentFromPairing(*pairing)); err != nil {
				return ApprovePairingResult{}, internalError("persist node enrollment failed", err, map[string]any{"code": req.Code})
			}
		}
		if s.cfg.Nodes != nil {
			if err := s.cfg.Nodes.UpsertNode(ctx, nexusbootstrap.DefaultNodeDescriptor(pairing.Cred.DeviceID)); err != nil {
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
	if err := s.cfg.NodeManager.RejectPairing(ctx, req.Code); err != nil {
		return RejectPairingResult{}, notFound("pairing request not found", map[string]any{"code": req.Code, "cause": err.Error()})
	}
	return RejectPairingResult{
		AdminResult: resultEnvelope(req.AdminRequest),
		Code:        req.Code,
	}, nil
}

func (s *service) ListTenants(ctx context.Context, req ListTenantsRequest) (ListTenantsResult, error) {
	nodes, err := s.cfg.Nodes.ListNodes(ctx)
	if err != nil {
		return ListTenantsResult{}, internalError("list tenants failed", err, nil)
	}
	set := map[string]struct{}{}
	for _, node := range nodes {
		if strings.TrimSpace(node.TenantID) != "" {
			set[node.TenantID] = struct{}{}
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
