package admin

import (
	"context"
	"sort"
	"strings"
	"time"
)

func (s *service) ListTenantFMPExports(ctx context.Context, req ListTenantFMPExportsRequest) (ListTenantFMPExportsResult, error) {
	tenantID, err := authorizeTenant(req.Principal, req.TenantID)
	if err != nil {
		return ListTenantFMPExportsResult{}, err
	}
	if s.cfg.FMPExports == nil {
		return ListTenantFMPExportsResult{}, notImplemented("tenant fmp export controls not implemented", nil)
	}
	exports, err := s.cfg.FMPExports.ListTenantExports(ctx, tenantID)
	if err != nil {
		return ListTenantFMPExportsResult{}, internalError("list tenant fmp exports failed", err, map[string]any{"tenant_id": tenantID})
	}
	sort.Slice(exports, func(i, j int) bool { return exports[i].ExportName < exports[j].ExportName })
	total := len(exports)
	exports = applyPage(exports, req.Page)
	return ListTenantFMPExportsResult{
		AdminResult: resultEnvelope(req.AdminRequest),
		PageResult:  pageResult(total),
		Exports:     exports,
	}, nil
}

func (s *service) SetTenantFMPExport(ctx context.Context, req SetTenantFMPExportRequest) (SetTenantFMPExportResult, error) {
	tenantID, err := authorizeTenant(req.Principal, req.TenantID)
	if err != nil {
		return SetTenantFMPExportResult{}, err
	}
	if s.cfg.FMPExports == nil {
		return SetTenantFMPExportResult{}, notImplemented("tenant fmp export controls not implemented", nil)
	}
	exportName := strings.TrimSpace(req.ExportName)
	if exportName == "" {
		return SetTenantFMPExportResult{}, invalidArgument("export_name required", map[string]any{"field": "export_name"})
	}
	if err := s.cfg.FMPExports.SetTenantExportEnabled(ctx, tenantID, exportName, req.Enabled); err != nil {
		return SetTenantFMPExportResult{}, internalError("set tenant fmp export failed", err, map[string]any{"tenant_id": tenantID, "export_name": exportName})
	}
	return SetTenantFMPExportResult{
		AdminResult: resultEnvelope(req.AdminRequest),
		Export: TenantFMPExportInfo{
			TenantID:   tenantID,
			ExportName: exportName,
			Enabled:    req.Enabled,
			UpdatedAt:  time.Now().UTC(),
		},
	}, nil
}
