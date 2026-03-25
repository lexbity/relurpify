package admin

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	fwfmp "github.com/lexcodex/relurpify/framework/middleware/fmp"
	"github.com/lexcodex/relurpify/framework/core"
)

type fmpLineageLister interface {
	ListLineages(ctx context.Context) ([]core.LineageRecord, error)
}

type fmpAuditChainReader interface {
	core.AuditChainReader
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
	var auditChain []core.AuditChainEntry
	var verification *core.AuditChainVerification
	if reader, ok := s.auditChainReader(); ok {
		entries, err := reader.ReadChain(ctx, core.AuditChainFilter{LineageID: lineageID, Limit: req.Limit})
		if err != nil {
			return ReadFMPContinuationAuditResult{}, internalError("read fmp audit chain failed", err, map[string]any{"lineage_id": lineageID})
		}
		auditChain = entries
		result, err := reader.VerifyChain(ctx, core.AuditChainFilter{LineageID: lineageID, Limit: req.Limit})
		if err != nil {
			return ReadFMPContinuationAuditResult{}, internalError("verify fmp audit chain failed", err, map[string]any{"lineage_id": lineageID})
		}
		verification = &result
	}
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
		AdminResult:  resultEnvelope(req.AdminRequest),
		Lineage:      &info,
		Events:       events,
		AuditChain:   auditChain,
		Verification: verification,
	}, nil
}

func (s *service) VerifyFMPAuditTrail(ctx context.Context, req VerifyFMPAuditTrailRequest) (VerifyFMPAuditTrailResult, error) {
	tenantID, lineage, err := s.authorizeFMPLineage(ctx, req.Principal, req.TenantID, req.LineageID)
	if err != nil {
		return VerifyFMPAuditTrailResult{}, err
	}
	reader, ok := s.auditChainReader()
	if !ok {
		return VerifyFMPAuditTrailResult{}, notImplemented("fmp audit chain verification not implemented", nil)
	}
	verification, err := reader.VerifyChain(ctx, core.AuditChainFilter{LineageID: strings.TrimSpace(req.LineageID), Limit: req.Limit})
	if err != nil {
		return VerifyFMPAuditTrailResult{}, internalError("verify fmp audit chain failed", err, map[string]any{"lineage_id": strings.TrimSpace(req.LineageID), "tenant_id": tenantID})
	}
	info := continuationInfoFromLineage(*lineage)
	return VerifyFMPAuditTrailResult{
		AdminResult:  resultEnvelope(req.AdminRequest),
		Lineage:      &info,
		Verification: verification,
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

func (s *service) authorizeFMPLineage(ctx context.Context, principal core.AuthenticatedPrincipal, requestedTenantID, lineageID string) (string, *core.LineageRecord, error) {
	tenantID, err := authorizeTenant(principal, requestedTenantID)
	if err != nil {
		return "", nil, err
	}
	lineageID = strings.TrimSpace(lineageID)
	if lineageID == "" {
		return "", nil, invalidArgument("lineage_id required", map[string]any{"field": "lineage_id"})
	}
	if s.cfg.FMP == nil || s.cfg.FMP.Ownership == nil {
		return "", nil, notImplemented("fmp audit not implemented", nil)
	}
	lineage, ok, err := s.cfg.FMP.Ownership.GetLineage(ctx, lineageID)
	if err != nil {
		return "", nil, internalError("read fmp lineage failed", err, map[string]any{"lineage_id": lineageID})
	}
	if !ok || lineage == nil {
		return "", nil, notFound("fmp lineage not found", map[string]any{"lineage_id": lineageID})
	}
	if !strings.EqualFold(lineage.TenantID, tenantID) {
		return "", nil, AdminError{
			Code:    AdminErrorPolicyDenied,
			Message: "cross-tenant access denied",
			Detail:  map[string]any{"lineage_id": lineageID, "tenant_id": tenantID},
		}
	}
	return tenantID, lineage, nil
}

func (s *service) auditChainReader() (fmpAuditChainReader, bool) {
	if s.cfg.FMP == nil || s.cfg.FMP.Audit == nil {
		return nil, false
	}
	reader, ok := s.cfg.FMP.Audit.(fmpAuditChainReader)
	return reader, ok
}

// Phase 6.4: Compatibility Window Management

func (s *service) ListFMPCompatibilityWindows(ctx context.Context, req ListFMPCompatibilityWindowsRequest) (ListFMPCompatibilityWindowsResult, error) {
	if err := authorizeGlobalFMPAdmin(req.Principal); err != nil {
		return ListFMPCompatibilityWindowsResult{}, err
	}
	if s.cfg.FMP == nil || s.cfg.FMP.CompatibilityWindows == nil {
		return ListFMPCompatibilityWindowsResult{}, notImplemented("compatibility windows not configured", nil)
	}
	windows, err := s.cfg.FMP.CompatibilityWindows.ListWindows(ctx)
	if err != nil {
		return ListFMPCompatibilityWindowsResult{}, internalError("list compatibility windows failed", err, nil)
	}
	result := ListFMPCompatibilityWindowsResult{
		AdminResult: resultEnvelope(req.AdminRequest),
		Windows:     make([]FMPCompatibilityWindow, 0, len(windows)),
	}
	for _, w := range windows {
		result.Windows = append(result.Windows, FMPCompatibilityWindow{
			ContextClass:      w.ContextClass,
			MinSchemaVersion:  w.MinSchemaVersion,
			MaxSchemaVersion:  w.MaxSchemaVersion,
			MinRuntimeVersion: w.MinRuntimeVersion,
			MaxRuntimeVersion: w.MaxRuntimeVersion,
		})
	}
	return result, nil
}

func (s *service) SetFMPCompatibilityWindow(ctx context.Context, req SetFMPCompatibilityWindowRequest) (SetFMPCompatibilityWindowResult, error) {
	if err := authorizeGlobalFMPAdmin(req.Principal); err != nil {
		return SetFMPCompatibilityWindowResult{}, err
	}
	if s.cfg.FMP == nil || s.cfg.FMP.CompatibilityWindows == nil {
		return SetFMPCompatibilityWindowResult{}, notImplemented("compatibility windows not configured", nil)
	}
	if strings.TrimSpace(req.Window.ContextClass) == "" {
		return SetFMPCompatibilityWindowResult{}, invalidArgument("context_class required", map[string]any{"field": "context_class"})
	}
	// Convert API window type to framework type
	window := fwfmp.CompatibilityWindow{
		ContextClass:      strings.TrimSpace(req.Window.ContextClass),
		MinSchemaVersion:  strings.TrimSpace(req.Window.MinSchemaVersion),
		MaxSchemaVersion:  strings.TrimSpace(req.Window.MaxSchemaVersion),
		MinRuntimeVersion: strings.TrimSpace(req.Window.MinRuntimeVersion),
		MaxRuntimeVersion: strings.TrimSpace(req.Window.MaxRuntimeVersion),
	}
	if err := s.cfg.FMP.CompatibilityWindows.UpsertWindow(ctx, window); err != nil {
		return SetFMPCompatibilityWindowResult{}, internalError("set compatibility window failed", err, map[string]any{"context_class": window.ContextClass})
	}
	return SetFMPCompatibilityWindowResult{
		AdminResult: resultEnvelope(req.AdminRequest),
		Window:      req.Window,
	}, nil
}

func (s *service) DeleteFMPCompatibilityWindow(ctx context.Context, req DeleteFMPCompatibilityWindowRequest) (DeleteFMPCompatibilityWindowResult, error) {
	if err := authorizeGlobalFMPAdmin(req.Principal); err != nil {
		return DeleteFMPCompatibilityWindowResult{}, err
	}
	if s.cfg.FMP == nil || s.cfg.FMP.CompatibilityWindows == nil {
		return DeleteFMPCompatibilityWindowResult{}, notImplemented("compatibility windows not configured", nil)
	}
	contextClass := strings.TrimSpace(req.ContextClass)
	if contextClass == "" {
		return DeleteFMPCompatibilityWindowResult{}, invalidArgument("context_class required", map[string]any{"field": "context_class"})
	}
	if err := s.cfg.FMP.CompatibilityWindows.DeleteWindow(ctx, contextClass); err != nil {
		return DeleteFMPCompatibilityWindowResult{}, internalError("delete compatibility window failed", err, map[string]any{"context_class": contextClass})
	}
	return DeleteFMPCompatibilityWindowResult{
		AdminResult:  resultEnvelope(req.AdminRequest),
		ContextClass: contextClass,
	}, nil
}

// Phase 6.5: Circuit Breaker Management

func (s *service) ListFMPCircuitBreakers(ctx context.Context, req ListFMPCircuitBreakersRequest) (ListFMPCircuitBreakersResult, error) {
	if err := authorizeGlobalFMPAdmin(req.Principal); err != nil {
		return ListFMPCircuitBreakersResult{}, err
	}
	if s.cfg.FMP == nil || s.cfg.FMP.CircuitBreakers == nil {
		return ListFMPCircuitBreakersResult{}, notImplemented("circuit breakers not configured", nil)
	}
	statuses, err := s.cfg.FMP.CircuitBreakers.ListStates(ctx)
	if err != nil {
		return ListFMPCircuitBreakersResult{}, internalError("list circuit breaker states failed", err, nil)
	}
	result := ListFMPCircuitBreakersResult{
		AdminResult:     resultEnvelope(req.AdminRequest),
		CircuitBreakers: make([]FMPCircuitBreakerStatus, 0, len(statuses)),
	}
	for _, status := range statuses {
		cb := FMPCircuitBreakerStatus{
			TrustDomain: status.TrustDomain,
			State:       string(status.State),
			ErrorRate:   status.ErrorRate,
			Requests:    status.Requests,
		}
		if status.TrippedAt != nil {
			ns := status.TrippedAt.UnixNano()
			cb.TrippedAt = &ns
		}
		if status.RecoveryAt != nil {
			ns := status.RecoveryAt.UnixNano()
			cb.RecoveryAt = &ns
		}
		result.CircuitBreakers = append(result.CircuitBreakers, cb)
	}
	return result, nil
}

func (s *service) SetFMPCircuitBreakerConfig(ctx context.Context, req SetFMPCircuitBreakerConfigRequest) (SetFMPCircuitBreakerConfigResult, error) {
	if err := authorizeGlobalFMPAdmin(req.Principal); err != nil {
		return SetFMPCircuitBreakerConfigResult{}, err
	}
	if s.cfg.FMP == nil || s.cfg.FMP.CircuitBreakers == nil {
		return SetFMPCircuitBreakerConfigResult{}, notImplemented("circuit breakers not configured", nil)
	}
	trustDomain := strings.TrimSpace(req.TrustDomain)
	if trustDomain == "" {
		return SetFMPCircuitBreakerConfigResult{}, invalidArgument("trust_domain required", map[string]any{"field": "trust_domain"})
	}
	if req.ErrorThreshold < 0 || req.ErrorThreshold > 1 {
		return SetFMPCircuitBreakerConfigResult{}, invalidArgument("error_threshold must be 0.0-1.0", map[string]any{"field": "error_threshold", "value": req.ErrorThreshold})
	}
	if req.MinRequests < 1 {
		return SetFMPCircuitBreakerConfigResult{}, invalidArgument("min_requests must be >= 1", map[string]any{"field": "min_requests", "value": req.MinRequests})
	}
	cfg := fwfmp.CircuitBreakerConfig{
		TrustDomain:      trustDomain,
		ErrorThreshold:   req.ErrorThreshold,
		MinRequests:      req.MinRequests,
		WindowDuration:   0, // Note: not configurable via admin API
		RecoveryDuration: 0, // Will be set to default if <= 0
	}
	if req.RecoveryDurationSec > 0 {
		cfg.RecoveryDuration = time.Duration(req.RecoveryDurationSec) * time.Second
	}
	if err := s.cfg.FMP.CircuitBreakers.SetConfig(ctx, cfg); err != nil {
		return SetFMPCircuitBreakerConfigResult{}, internalError("set circuit breaker config failed", err, map[string]any{"trust_domain": trustDomain})
	}
	return SetFMPCircuitBreakerConfigResult{
		AdminResult: resultEnvelope(req.AdminRequest),
		TrustDomain: trustDomain,
	}, nil
}

func (s *service) ResetFMPCircuitBreaker(ctx context.Context, req ResetFMPCircuitBreakerRequest) (ResetFMPCircuitBreakerResult, error) {
	if err := authorizeGlobalFMPAdmin(req.Principal); err != nil {
		return ResetFMPCircuitBreakerResult{}, err
	}
	if s.cfg.FMP == nil || s.cfg.FMP.CircuitBreakers == nil {
		return ResetFMPCircuitBreakerResult{}, notImplemented("circuit breakers not configured", nil)
	}
	trustDomain := strings.TrimSpace(req.TrustDomain)
	if trustDomain == "" {
		return ResetFMPCircuitBreakerResult{}, invalidArgument("trust_domain required", map[string]any{"field": "trust_domain"})
	}
	if err := s.cfg.FMP.CircuitBreakers.Reset(ctx, trustDomain, time.Now().UTC()); err != nil {
		return ResetFMPCircuitBreakerResult{}, internalError("reset circuit breaker failed", err, map[string]any{"trust_domain": trustDomain})
	}
	return ResetFMPCircuitBreakerResult{
		AdminResult: resultEnvelope(req.AdminRequest),
		TrustDomain: trustDomain,
	}, nil
}
