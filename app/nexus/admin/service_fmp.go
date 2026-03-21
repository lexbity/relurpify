package admin

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/lexcodex/relurpify/framework/core"
)

type fmpLineageLister interface {
	ListLineages(ctx context.Context) ([]core.LineageRecord, error)
}

func (s *service) ListFMPContinuations(ctx context.Context, req ListFMPContinuationsRequest) (ListFMPContinuationsResult, error) {
	tenantID, err := authorizeTenant(req.Principal, req.TenantID)
	if err != nil {
		return ListFMPContinuationsResult{}, err
	}
	if s.cfg.FMP == nil || s.cfg.FMP.Ownership == nil {
		return ListFMPContinuationsResult{}, notImplemented("fmp continuation listing not implemented", nil)
	}
	lister, ok := s.cfg.FMP.Ownership.(fmpLineageLister)
	if !ok {
		return ListFMPContinuationsResult{}, notImplemented("fmp ownership listing not implemented", nil)
	}
	lineages, err := lister.ListLineages(ctx)
	if err != nil {
		return ListFMPContinuationsResult{}, internalError("list fmp continuations failed", err, map[string]any{"tenant_id": tenantID})
	}
	continuations := make([]FMPContinuationInfo, 0, len(lineages))
	for _, lineage := range lineages {
		if !strings.EqualFold(strings.TrimSpace(lineage.TenantID), tenantID) {
			continue
		}
		continuations = append(continuations, continuationInfoFromLineage(lineage))
	}
	sort.Slice(continuations, func(i, j int) bool {
		if continuations[i].UpdatedAt.Equal(continuations[j].UpdatedAt) {
			return continuations[i].LineageID < continuations[j].LineageID
		}
		return continuations[i].UpdatedAt.After(continuations[j].UpdatedAt)
	})
	total := len(continuations)
	continuations = applyPage(continuations, req.Page)
	return ListFMPContinuationsResult{
		AdminResult:   resultEnvelope(req.AdminRequest),
		PageResult:    pageResult(total),
		Continuations: continuations,
	}, nil
}

func (s *service) ReadFMPContinuationAudit(ctx context.Context, req ReadFMPContinuationAuditRequest) (ReadFMPContinuationAuditResult, error) {
	tenantID, err := authorizeTenant(req.Principal, req.TenantID)
	if err != nil {
		return ReadFMPContinuationAuditResult{}, err
	}
	lineageID := strings.TrimSpace(req.LineageID)
	if lineageID == "" {
		return ReadFMPContinuationAuditResult{}, invalidArgument("lineage_id required", map[string]any{"field": "lineage_id"})
	}
	if s.cfg.FMP == nil || s.cfg.FMP.Ownership == nil {
		return ReadFMPContinuationAuditResult{}, notImplemented("fmp audit not implemented", nil)
	}
	lineage, ok, err := s.cfg.FMP.Ownership.GetLineage(ctx, lineageID)
	if err != nil {
		return ReadFMPContinuationAuditResult{}, internalError("read fmp lineage failed", err, map[string]any{"lineage_id": lineageID})
	}
	if !ok || lineage == nil {
		return ReadFMPContinuationAuditResult{}, notFound("fmp lineage not found", map[string]any{"lineage_id": lineageID})
	}
	if !strings.EqualFold(lineage.TenantID, tenantID) {
		return ReadFMPContinuationAuditResult{}, AdminError{
			Code:    AdminErrorPolicyDenied,
			Message: "cross-tenant access denied",
			Detail:  map[string]any{"lineage_id": lineageID, "tenant_id": tenantID},
		}
	}
	events := []core.FrameworkEvent{}
	if s.cfg.Events != nil {
		allEvents, err := s.cfg.Events.Read(ctx, s.cfg.Partition, 0, 0, false)
		if err != nil {
			return ReadFMPContinuationAuditResult{}, internalError("read fmp audit failed", err, map[string]any{"lineage_id": lineageID})
		}
		filtered := make([]core.FrameworkEvent, 0)
		for _, ev := range filterEventsByTenant(allEvents, tenantID) {
			if !isFMPEventType(ev.Type) {
				continue
			}
			if !strings.EqualFold(eventLineageID(ev), lineageID) {
				continue
			}
			filtered = append(filtered, ev)
		}
		if limit := req.Limit; limit > 0 && len(filtered) > limit {
			filtered = filtered[len(filtered)-limit:]
		}
		events = filtered
	}
	info := continuationInfoFromLineage(*lineage)
	return ReadFMPContinuationAuditResult{
		AdminResult: resultEnvelope(req.AdminRequest),
		Lineage:     &info,
		Events:      events,
	}, nil
}

func continuationInfoFromLineage(lineage core.LineageRecord) FMPContinuationInfo {
	return FMPContinuationInfo{
		LineageID:           lineage.LineageID,
		TenantID:            lineage.TenantID,
		TaskClass:           lineage.TaskClass,
		ContextClass:        lineage.ContextClass,
		Owner:               lineage.Owner,
		SessionID:           lineage.SessionID,
		TrustClass:          lineage.TrustClass,
		CurrentOwnerAttempt: lineage.CurrentOwnerAttempt,
		CurrentOwnerRuntime: lineage.CurrentOwnerRuntime,
		LineageVersion:      lineage.LineageVersion,
		UpdatedAt:           lineage.UpdatedAt,
		SensitivityClass:    lineage.SensitivityClass,
	}
}

func isFMPEventType(eventType string) bool {
	return strings.HasPrefix(strings.TrimSpace(eventType), "fmp.")
}

func eventLineageID(ev core.FrameworkEvent) string {
	if len(ev.Payload) == 0 {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal(ev.Payload, &payload); err != nil {
		return ""
	}
	if lineageID, _ := payload["lineage_id"].(string); lineageID != "" {
		return lineageID
	}
	return ""
}
