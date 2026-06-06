package framework

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/platform/contracts"
	policy "codeburg.org/lexbit/relurpify/governance/policy"
)

// TestAllowDecisionAudit validates that allow decisions generate observable audit records
// with complete request and decision metadata.
func TestAllowDecisionAudit(t *testing.T) {
	env := NewTestEnvironment(t)

	agentID := "test-agent"
	testPath := filepath.Join(env.WorkspacePath, "allowed.txt")

	// Create a file to test access
	if err := os.WriteFile(testPath, []byte("test content"), 0o644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Clear any existing audit records
	env.AuditSink.Clear()

	// Perform an allowed file access operation
	ctx := context.Background()
	err := env.PermissionManager.CheckFileAccess(ctx, agentID, contracts.FileSystemRead, testPath)
	if err != nil {
		t.Fatalf("expected file access to be allowed, got error: %v", err)
	}

	// Verify an audit record was created
	records := env.AuditSink.Records()
	if len(records) == 0 {
		t.Fatal("expected at least one audit record for allow decision")
	}

	// Get the last record (most recent)
	lastRecord := records[len(records)-1]

	// Assert the audit entry includes the original request
	if lastRecord.AgentID != agentID {
		t.Errorf("audit record AgentID mismatch: got %s, want %s", lastRecord.AgentID, agentID)
	}

	// Assert the evaluated tool/resource is present
	if lastRecord.Type != string(contracts.PermissionTypeFilesystem) {
		t.Errorf("audit record Type mismatch: got %s, want %s", lastRecord.Type, contracts.PermissionTypeFilesystem)
	}

	// Assert the final action is "granted"
	if lastRecord.Result != "granted" {
		t.Errorf("audit record Result mismatch: got %s, want 'granted'", lastRecord.Result)
	}

	// Assert the action field reflects the file access operation
	if lastRecord.Action != string(contracts.FileSystemRead) {
		t.Errorf("audit record Action mismatch: got %s, want %s", lastRecord.Action, contracts.FileSystemRead)
	}

	// Assert correlation ID is set for request/decision tracking
	if lastRecord.Correlation != agentID {
		t.Errorf("audit record Correlation mismatch: got %s, want %s", lastRecord.Correlation, agentID)
	}

	// Assert timestamp is set (non-zero)
	if lastRecord.Timestamp.IsZero() {
		t.Error("audit record Timestamp should be non-zero")
	}
}

// TestDenyDecisionAudit validates that deny decisions generate observable audit records
// with clear denial reasons and distinguishable from allow decisions.
func TestDenyDecisionAudit(t *testing.T) {
	env := NewTestEnvironment(t)
	env.PermissionManager.SetDefaultPolicy(agentspec.AgentPermissionDeny)

	agentID := "test-agent"

	// Attempt to access a path outside the workspace (should be denied)
	outsidePath := "/etc/passwd"

	// Clear any existing audit records
	env.AuditSink.Clear()

	// Perform a denied file access operation
	ctx := context.Background()
	err := env.PermissionManager.CheckFileAccess(ctx, agentID, contracts.FileSystemRead, outsidePath)
	if err == nil {
		t.Fatal("expected file access to be denied, got no error")
	}
	var deniedErr *contracts.PermissionDeniedError
	if !errors.As(err, &deniedErr) {
		t.Fatalf("expected PermissionDeniedError, got %T: %v", err, err)
	}
	if !strings.Contains(deniedErr.Message, "path escapes workspace") {
		t.Fatalf("expected deny reason to mention path escapes workspace, got %q", deniedErr.Message)
	}

	// Verify an audit record was created
	records := env.AuditSink.Records()
	if len(records) != 1 {
		t.Fatalf("expected exactly one audit record for deny decision, got %d", len(records))
	}

	record := records[0]

	// Assert the audit entry includes the original request
	if record.AgentID != agentID {
		t.Errorf("audit record AgentID mismatch: got %s, want %s", record.AgentID, agentID)
	}

	// Assert the evaluated tool/resource is present
	if record.Type != string(contracts.PermissionTypeFilesystem) {
		t.Errorf("audit record Type mismatch: got %s, want %s", record.Type, contracts.PermissionTypeFilesystem)
	}

	// Assert the final action is "denied" (distinguishable from allow)
	if record.Result != "denied" {
		t.Errorf("audit record Result mismatch: got %s, want 'denied'", record.Result)
	}

	// Assert the action field reflects the denied operation
	if record.Action != string(contracts.FileSystemRead) {
		t.Errorf("audit record Action mismatch: got %s, want %s", record.Action, contracts.FileSystemRead)
	}

	// Assert metadata includes the denial reason
	if record.Metadata == nil {
		t.Fatal("audit record Metadata should be present for deny decisions")
	}
	if reason, ok := record.Metadata["reason"]; !ok {
		t.Error("audit record Metadata should include 'reason' field for deny decisions")
	} else if reason == nil {
		t.Error("audit record Metadata 'reason' should not be nil")
	}

	// Assert correlation ID is set
	if record.Correlation != agentID {
		t.Errorf("audit record Correlation mismatch: got %s, want %s", record.Correlation, agentID)
	}
}

// TestHITLAudit validates that HITL-driven decisions generate observable audit records
// that are distinguishable from direct allow/deny decisions.
func TestHITLAudit(t *testing.T) {
	env := NewTestEnvironment(t)

	// Create a stub HITL provider that grants approvals
	hitl := &stubHITLProvider{
		grants: []*authorization.PermissionGrant{
			{
				ID: "grant-1",
				Permission: contracts.PermissionDescriptor{
					Type:     contracts.PermissionTypeNetwork,
					Action:   "net:egress:tcp",
					Resource: "api.service.local:443",
				},
				Scope:      authorization.GrantScopeSession,
				ExpiresAt:  time.Now().Add(time.Hour),
				ApprovedBy: "test-user",
			},
		},
	}

	// Create permission manager with HITL provider
	perms := policy.NewFileSystemPermissionSet(env.WorkspacePath, contracts.FileSystemRead)
	perms.Network = []contracts.NetworkPermission{
		{Direction: "egress", Protocol: "tcp", Host: "api.service.local", Port: 443, HITLRequired: true},
	}
	permManager, err := authorization.NewPermissionManager(env.WorkspacePath, perms, env.AuditSink, hitl)
	if err != nil {
		t.Fatalf("failed to create permission manager: %v", err)
	}

	env.PermissionManager = permManager

	agentID := "test-agent"

	// Clear any existing audit records
	env.AuditSink.Clear()

	// Perform a network access that requires HITL approval
	ctx := context.Background()
	err = permManager.CheckNetwork(ctx, agentID, "egress", "tcp", "api.service.local", 443)
	if err != nil {
		t.Fatalf("expected network access to be allowed via HITL, got error: %v", err)
	}

	// Verify audit records were created
	records := env.AuditSink.Records()
	if len(records) != 1 {
		t.Fatalf("expected exactly one audit record for HITL decision, got %d", len(records))
	}

	record := records[0]

	// Assert the audit entry includes the original request
	if record.AgentID != agentID {
		t.Errorf("audit record AgentID mismatch: got %s, want %s", record.AgentID, agentID)
	}

	// Assert the type is network permission
	if record.Type != string(contracts.PermissionTypeNetwork) {
		t.Errorf("audit record Type mismatch: got %s, want %s", record.Type, contracts.PermissionTypeNetwork)
	}

	// Assert the result is "granted" (HITL was approved)
	if record.Result != "granted" {
		t.Errorf("audit record Result mismatch: got %s, want 'granted'", record.Result)
	}

	// Assert correlation ID is set
	if record.Correlation != agentID {
		t.Errorf("audit record Correlation mismatch: got %s, want %s", record.Correlation, agentID)
	}

	// Verify HITL request was made (distinguishable from direct allow)
	if len(hitl.requests) != 1 {
		t.Errorf("expected exactly one HITL request, got %d", len(hitl.requests))
	}
}

// TestRequestDecisionCorrelation validates that each evaluation creates an audit record
// and that request and decision data match exactly.
func TestRequestDecisionCorrelation(t *testing.T) {
	env := NewTestEnvironment(t)

	agentID := "test-agent"
	testPath := filepath.Join(env.WorkspacePath, "correlation.txt")

	// Create a test file
	if err := os.WriteFile(testPath, []byte("correlation test"), 0o644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Clear any existing audit records
	env.AuditSink.Clear()

	// Perform a file access operation
	ctx := context.Background()
	action := contracts.FileSystemRead
	err := env.PermissionManager.CheckFileAccess(ctx, agentID, action, testPath)
	if err != nil {
		t.Fatalf("expected file access to be allowed, got error: %v", err)
	}

	// Verify exactly one audit record was created (unless policy explicitly batches)
	records := env.AuditSink.Records()
	if len(records) != 1 {
		t.Errorf("expected exactly one audit record for single evaluation, got %d", len(records))
	}

	record := records[0]

	// Assert request identity survives into the audit record
	if record.AgentID != agentID {
		t.Errorf("request AgentID not preserved: got %s, want %s", record.AgentID, agentID)
	}

	// Assert evaluated tool/resource matches the request
	if record.Permission != testPath {
		t.Errorf("request resource not preserved: got %s, want %s", record.Permission, testPath)
	}

	// Assert action matches the request
	if record.Action != string(action) {
		t.Errorf("request action not preserved: got %s, want %s", record.Action, action)
	}

	// Assert decision outcome is correctly captured
	if record.Result != "granted" {
		t.Errorf("decision outcome not captured: got %s, want 'granted'", record.Result)
	}

	// Assert type matches the permission type
	if record.Type != string(contracts.PermissionTypeFilesystem) {
		t.Errorf("permission type not captured: got %s, want %s", record.Type, contracts.PermissionTypeFilesystem)
	}
}

// TestAuditRecordDeterminism validates that audit records are stable and comparable
// across repeated operations with identical inputs.
func TestAuditRecordDeterminism(t *testing.T) {
	env := NewTestEnvironment(t)

	agentID := "test-agent"
	testPath := filepath.Join(env.WorkspacePath, "determinism.txt")

	// Create a test file
	if err := os.WriteFile(testPath, []byte("determinism test"), 0o644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Perform first operation
	env.AuditSink.Clear()
	ctx := context.Background()
	err := env.PermissionManager.CheckFileAccess(ctx, agentID, contracts.FileSystemRead, testPath)
	if err != nil {
		t.Fatalf("first operation failed: %v", err)
	}
	records1 := env.AuditSink.Records()

	// Perform second operation with identical inputs
	env.AuditSink.Clear()
	err = env.PermissionManager.CheckFileAccess(ctx, agentID, contracts.FileSystemRead, testPath)
	if err != nil {
		t.Fatalf("second operation failed: %v", err)
	}
	records2 := env.AuditSink.Records()

	// Verify both operations produced exactly one record
	if len(records1) != 1 || len(records2) != 1 {
		t.Fatalf("expected exactly one record per operation, got %d and %d", len(records1), len(records2))
	}

	// Compare record fields (excluding timestamp which will differ)
	record1 := records1[0]
	record2 := records2[0]

	if record1.AgentID != record2.AgentID {
		t.Errorf("AgentID not stable: %s vs %s", record1.AgentID, record2.AgentID)
	}
	if record1.Action != record2.Action {
		t.Errorf("Action not stable: %s vs %s", record1.Action, record2.Action)
	}
	if record1.Type != record2.Type {
		t.Errorf("Type not stable: %s vs %s", record1.Type, record2.Type)
	}
	if record1.Permission != record2.Permission {
		t.Errorf("Permission not stable: %s vs %s", record1.Permission, record2.Permission)
	}
	if record1.Result != record2.Result {
		t.Errorf("Result not stable: %s vs %s", record1.Result, record2.Result)
	}
	if record1.Correlation != record2.Correlation {
		t.Errorf("Correlation not stable: %s vs %s", record1.Correlation, record2.Correlation)
	}
}

// TestAuditQueryFiltering validates that audit records can be filtered
// by agent ID, action, type, and result.
func TestAuditQueryFiltering(t *testing.T) {
	env := NewTestEnvironment(t)

	agentID := "test-agent"
	otherAgentID := "other-agent"

	testPath := filepath.Join(env.WorkspacePath, "query.txt")
	if err := os.WriteFile(testPath, []byte("query test"), 0o644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	env.AuditSink.Clear()
	ctx := context.Background()

	// Perform operations for test-agent
	env.PermissionManager.CheckFileAccess(ctx, agentID, contracts.FileSystemRead, testPath)

	// Perform operations for other-agent
	env.PermissionManager.CheckFileAccess(ctx, otherAgentID, contracts.FileSystemRead, testPath)

	// Query by agent ID
	filter := policy.AuditQuery{AgentID: agentID}
	records, err := env.AuditSink.Query(ctx, filter)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if len(records) != 1 {
		t.Errorf("expected exactly one record for agent %s, got %d", agentID, len(records))
	}
	if records[0].AgentID != agentID {
		t.Errorf("filtered record AgentID mismatch: got %s, want %s", records[0].AgentID, agentID)
	}

	// Query by result
	filter = policy.AuditQuery{Result: "granted"}
	records, err = env.AuditSink.Query(ctx, filter)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if len(records) != 2 {
		t.Errorf("expected two records with result 'granted', got %d", len(records))
	}

	// Query by type
	filter = policy.AuditQuery{Type: string(contracts.PermissionTypeFilesystem)}
	records, err = env.AuditSink.Query(ctx, filter)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if len(records) != 2 {
		t.Errorf("expected two records with filesystem type, got %d", len(records))
	}
}

// TestDenyAndHITLDistinguishability validates that deny decisions and HITL-driven
// decisions are clearly distinguishable in the audit surface.
func TestDenyAndHITLDistinguishability(t *testing.T) {
	env := NewTestEnvironment(t)
	env.PermissionManager.SetDefaultPolicy(agentspec.AgentPermissionDeny)

	agentID := "test-agent"

	// Test 1: Direct deny (path outside workspace)
	env.AuditSink.Clear()
	ctx := context.Background()
	denyPath := "/etc/passwd"
	if err := env.PermissionManager.CheckFileAccess(ctx, agentID, contracts.FileSystemRead, denyPath); err == nil {
		t.Fatal("expected direct deny path to fail")
	}
	denyRecords := env.AuditSink.Records()

	// Test 2: HITL approval
	hitl := &stubHITLProvider{
		grants: []*authorization.PermissionGrant{
			{
				ID: "grant-1",
				Permission: contracts.PermissionDescriptor{
					Type:     contracts.PermissionTypeNetwork,
					Action:   "net:egress:tcp",
					Resource: "api.service.local:443",
				},
				Scope:      authorization.GrantScopeSession,
				ExpiresAt:  time.Now().Add(time.Hour),
				ApprovedBy: "test-user",
			},
		},
	}
	perms := policy.NewFileSystemPermissionSet(env.WorkspacePath, contracts.FileSystemRead)
	perms.Network = []contracts.NetworkPermission{
		{Direction: "egress", Protocol: "tcp", Host: "api.service.local", Port: 443, HITLRequired: true},
	}
	permManager, err := authorization.NewPermissionManager(env.WorkspacePath, perms, env.AuditSink, hitl)
	if err != nil {
		t.Fatalf("failed to create permission manager: %v", err)
	}

	env.AuditSink.Clear()
	env.PermissionManager = permManager
	if err := permManager.CheckNetwork(ctx, agentID, "egress", "tcp", "api.service.local", 443); err != nil {
		t.Fatalf("expected HITL-approved network access to succeed: %v", err)
	}
	hitlRecords := env.AuditSink.Records()

	// Verify both produced records
	if len(denyRecords) == 0 {
		t.Fatal("deny decision did not produce audit record")
	}
	if len(hitlRecords) == 0 {
		t.Fatal("HITL decision did not produce audit record")
	}
	if len(denyRecords) != 1 {
		t.Fatalf("expected exactly one deny audit record, got %d", len(denyRecords))
	}
	if len(hitlRecords) != 1 {
		t.Fatalf("expected exactly one HITL audit record, got %d", len(hitlRecords))
	}

	denyRecord := denyRecords[0]
	hitlRecord := hitlRecords[0]

	// Deny should have result "denied"
	if denyRecord.Result != "denied" {
		t.Errorf("deny record result should be 'denied', got %s", denyRecord.Result)
	}
	if denyRecord.Metadata == nil {
		t.Fatalf("deny record should carry reason metadata, got nil")
	}
	if reason, ok := denyRecord.Metadata["reason"].(string); !ok || !strings.Contains(reason, "path escapes workspace") {
		t.Fatalf("deny record should carry reason mentioning path escapes workspace, got %#v", denyRecord.Metadata)
	}

	// HITL should have result "granted" (after approval)
	if hitlRecord.Result != "granted" {
		t.Errorf("HITL record result should be 'granted', got %s", hitlRecord.Result)
	}
	if hitlRecord.Correlation != agentID {
		t.Errorf("HITL correlation mismatch: got %s, want %s", hitlRecord.Correlation, agentID)
	}

	// Deny should have metadata with reason
	if denyRecord.Metadata == nil {
		t.Error("deny record should have metadata")
	}
	if denyRecord.Metadata != nil {
		if _, ok := denyRecord.Metadata["reason"]; !ok {
			t.Error("deny record metadata should include 'reason'")
		}
	}

	// Types should be different (filesystem vs network)
	if denyRecord.Type == hitlRecord.Type {
		t.Errorf("deny and HITL records should have different types, both are %s", denyRecord.Type)
	}
}

// TestAuditCaptureHelper validates that the audit capture helper stores entries
// in deterministic order and exposes the last entry for direct comparison.
func TestAuditCaptureHelper(t *testing.T) {
	env := NewTestEnvironment(t)

	ctx := context.Background()
	agentID := "test-agent"

	// Record multiple audit entries
	env.AuditSink.Clear()
	for i := 0; i < 3; i++ {
		record := policy.AuditRecord{
			Timestamp:   time.Now().UTC(),
			AgentID:     agentID,
			Action:      "test_action",
			Type:        "test_type",
			Permission:  "test_permission",
			Result:      "granted",
			Correlation: agentID,
		}
		if err := env.AuditSink.Log(ctx, record); err != nil {
			t.Fatalf("failed to log record %d: %v", i, err)
		}
	}

	// Verify records are stored in order
	records := env.AuditSink.Records()
	if len(records) != 3 {
		t.Fatalf("expected 3 records, got %d", len(records))
	}

	// Verify last entry can be accessed directly
	lastRecord := records[len(records)-1]
	if lastRecord.AgentID != agentID {
		t.Errorf("last record AgentID mismatch: got %s, want %s", lastRecord.AgentID, agentID)
	}

	// Verify Clear() removes all records
	env.AuditSink.Clear()
	records = env.AuditSink.Records()
	if len(records) != 0 {
		t.Errorf("expected 0 records after Clear, got %d", len(records))
	}
}

// stubHITLProvider is a test implementation of HITLProvider that returns pre-configured grants.
type stubHITLProvider struct {
	grants   []*authorization.PermissionGrant
	requests []authorization.PermissionRequest
}

func (s *stubHITLProvider) RequestPermission(ctx context.Context, req authorization.PermissionRequest) (*authorization.PermissionGrant, error) {
	s.requests = append(s.requests, req)
	if len(s.grants) == 0 {
		return &authorization.PermissionGrant{
			Permission: req.Permission,
			Scope:      authorization.GrantScopeSession,
		}, nil
	}
	grant := s.grants[0]
	s.grants = s.grants[1:]
	if grant.Permission.Action == "" {
		grant.Permission = req.Permission
	}
	return grant, nil
}
