package admin

import "context"

func (s *service) DescribeRexRuntime(ctx context.Context, req DescribeRexRuntimeRequest) (DescribeRexRuntimeResult, error) {
	if _, err := authorizeTenant(req.Principal, req.TenantID); err != nil {
		return DescribeRexRuntimeResult{}, err
	}
	if s.cfg.RexRuntime == nil {
		return DescribeRexRuntimeResult{}, notImplemented("rex runtime not configured", nil)
	}
	return DescribeRexRuntimeResult{
		AdminResult:  resultEnvelope(req.AdminRequest),
		Registration: s.cfg.RexRuntime.Registration(),
		Runtime:      s.cfg.RexRuntime.RuntimeProjection(),
	}, nil
}

func (s *service) ReadRexAdminSnapshot(ctx context.Context, req ReadRexAdminSnapshotRequest) (ReadRexAdminSnapshotResult, error) {
	if _, err := authorizeTenant(req.Principal, req.TenantID); err != nil {
		return ReadRexAdminSnapshotResult{}, err
	}
	if s.cfg.RexRuntime == nil {
		return ReadRexAdminSnapshotResult{}, notImplemented("rex runtime not configured", nil)
	}
	snapshot, err := s.cfg.RexRuntime.AdminSnapshot(ctx)
	if err != nil {
		return ReadRexAdminSnapshotResult{}, internalError("read rex admin snapshot failed", err, nil)
	}
	return ReadRexAdminSnapshotResult{
		AdminResult: resultEnvelope(req.AdminRequest),
		Snapshot:    snapshot,
	}, nil
}
