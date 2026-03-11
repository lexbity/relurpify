package admin

import (
	"context"
	"strings"
	"time"
)

func (s *service) GetTenant(ctx context.Context, req GetTenantRequest) (GetTenantResult, error) {
	lookupID := strings.TrimSpace(req.TenantLookupID)
	if lookupID == "" {
		lookupID = req.AdminRequest.TenantID
	}
	if _, err := authorizeTenant(req.Principal, lookupID); err != nil {
		return GetTenantResult{}, err
	}
	if s.cfg.Identities == nil {
		return GetTenantResult{AdminResult: resultEnvelope(req.AdminRequest)}, nil
	}
	record, err := s.cfg.Identities.GetTenant(ctx, lookupID)
	if err != nil {
		return GetTenantResult{}, internalError("get tenant failed", err, map[string]any{"tenant_id": lookupID})
	}
	if record == nil {
		return GetTenantResult{}, notFound("tenant not found", map[string]any{"tenant_id": lookupID})
	}
	return GetTenantResult{
		AdminResult: resultEnvelope(req.AdminRequest),
		Tenant: &TenantInfo{
			ID:          record.ID,
			DisplayName: record.DisplayName,
			CreatedAt:   record.CreatedAt,
			DisabledAt:  record.DisabledAt,
		},
	}, nil
}

func (s *service) SetTenantEnabled(ctx context.Context, req SetTenantEnabledRequest) (SetTenantEnabledResult, error) {
	lookupID := strings.TrimSpace(req.TenantLookupID)
	if lookupID == "" {
		lookupID = req.AdminRequest.TenantID
	}
	if strings.TrimSpace(lookupID) == "" {
		return SetTenantEnabledResult{}, invalidArgument("tenant_lookup_id required", map[string]any{"field": "tenant_lookup_id"})
	}
	if _, err := authorizeTenant(req.Principal, lookupID); err != nil {
		return SetTenantEnabledResult{}, err
	}
	if s.cfg.Identities == nil {
		return SetTenantEnabledResult{}, notImplemented("set tenant enabled not implemented", nil)
	}
	record, err := s.cfg.Identities.GetTenant(ctx, lookupID)
	if err != nil {
		return SetTenantEnabledResult{}, internalError("get tenant failed", err, map[string]any{"tenant_id": lookupID})
	}
	if record == nil {
		return SetTenantEnabledResult{}, notFound("tenant not found", map[string]any{"tenant_id": lookupID})
	}
	if req.Enabled {
		record.DisabledAt = nil
	} else {
		now := time.Now().UTC()
		record.DisabledAt = &now
	}
	if err := s.cfg.Identities.UpsertTenant(ctx, *record); err != nil {
		return SetTenantEnabledResult{}, internalError("set tenant enabled failed", err, map[string]any{"tenant_id": lookupID})
	}
	return SetTenantEnabledResult{
		AdminResult:    resultEnvelope(req.AdminRequest),
		TenantLookupID: lookupID,
		Enabled:        req.Enabled,
	}, nil
}

