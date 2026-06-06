package framework

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/platform/contracts"
	policy "codeburg.org/lexbit/relurpify/governance/policy"
)

// TestFileAccessAllowDeny validates file access permission enforcement
// at the authorization seam.
func TestFileAccessAllowDeny(t *testing.T) {
	t.Run("workspace file read allowed", func(t *testing.T) {
		env := NewTestEnvironment(t)

		// Create a file in the workspace
		testFile := filepath.Join(env.WorkspacePath, "test.txt")
		if err := os.WriteFile(testFile, []byte("test content"), 0o644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}

		// Grant read permission for the workspace
		perms := policy.NewFileSystemPermissionSet(env.WorkspacePath, contracts.FileSystemRead, contracts.FileSystemList)
		manager, err := authorization.NewPermissionManager(env.WorkspacePath, perms, env.AuditSink, nil)
		if err != nil {
			t.Fatalf("failed to create permission manager: %v", err)
		}

		// Check file access - should be allowed
		err = manager.CheckFileAccess(context.Background(), "test-agent", contracts.FileSystemRead, testFile)
		if err != nil {
			t.Errorf("expected file read to be allowed, got error: %v", err)
		}

		// Verify audit record was created for the allowed access
		records := env.AuditSink.Records()
		if len(records) == 0 {
			t.Error("expected audit record for allowed file access")
		}
		if len(records) > 0 && records[0].Result != "granted" {
			t.Errorf("expected audit result 'granted', got %s", records[0].Result)
		}
	})

	t.Run("workspace file write allowed", func(t *testing.T) {
		env := NewTestEnvironment(t)

		// Grant write permission for the workspace
		perms := policy.NewFileSystemPermissionSet(env.WorkspacePath, contracts.FileSystemWrite)
		manager, err := authorization.NewPermissionManager(env.WorkspacePath, perms, env.AuditSink, nil)
		if err != nil {
			t.Fatalf("failed to create permission manager: %v", err)
		}

		testFile := filepath.Join(env.WorkspacePath, "test.txt")

		// Check file access - should be allowed
		err = manager.CheckFileAccess(context.Background(), "test-agent", contracts.FileSystemWrite, testFile)
		if err != nil {
			t.Errorf("expected file write to be allowed, got error: %v", err)
		}
	})

	t.Run("workspace file list allowed", func(t *testing.T) {
		env := NewTestEnvironment(t)

		// Grant list permission for the workspace
		perms := policy.NewFileSystemPermissionSet(env.WorkspacePath, contracts.FileSystemList)
		manager, err := authorization.NewPermissionManager(env.WorkspacePath, perms, env.AuditSink, nil)
		if err != nil {
			t.Fatalf("failed to create permission manager: %v", err)
		}

		// Check file access - should be allowed
		err = manager.CheckFileAccess(context.Background(), "test-agent", contracts.FileSystemList, env.WorkspacePath)
		if err != nil {
			t.Errorf("expected file list to be allowed, got error: %v", err)
		}
	})

	t.Run("workspace file read denied without permission", func(t *testing.T) {
		env := NewTestEnvironment(t)

		// Create a file in the workspace
		testFile := filepath.Join(env.WorkspacePath, "test.txt")
		if err := os.WriteFile(testFile, []byte("test content"), 0o644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}

		// Grant write permission only (not read)
		perms := policy.NewFileSystemPermissionSet(env.WorkspacePath, contracts.FileSystemWrite)
		manager, err := authorization.NewPermissionManager(env.WorkspacePath, perms, env.AuditSink, nil)
		if err != nil {
			t.Fatalf("failed to create permission manager: %v", err)
		}

		// Check file access - should be denied
		err = manager.CheckFileAccess(context.Background(), "test-agent", contracts.FileSystemRead, testFile)
		if err == nil {
			t.Error("expected file read to be denied, got success")
		}

		// Verify audit record was created for the denied access
		records := env.AuditSink.Records()
		if len(records) == 0 {
			t.Error("expected audit record for denied file access")
		}
		if len(records) > 0 && records[0].Result != "denied" {
			t.Errorf("expected audit result 'denied', got %s", records[0].Result)
		}
	})

	t.Run("workspace file write denied without permission", func(t *testing.T) {
		env := NewTestEnvironment(t)

		// Grant only read permission
		perms := policy.NewFileSystemPermissionSet(env.WorkspacePath, contracts.FileSystemRead)
		manager, err := authorization.NewPermissionManager(env.WorkspacePath, perms, env.AuditSink, nil)
		if err != nil {
			t.Fatalf("failed to create permission manager: %v", err)
		}

		testFile := filepath.Join(env.WorkspacePath, "test.txt")

		// Check file access - should be denied
		err = manager.CheckFileAccess(context.Background(), "test-agent", contracts.FileSystemWrite, testFile)
		if err == nil {
			t.Error("expected file write to be denied, got success")
		}
	})

	t.Run("path outside workspace denied", func(t *testing.T) {
		env := NewTestEnvironment(t)

		// Grant read permission for the workspace
		perms := policy.NewFileSystemPermissionSet(env.WorkspacePath, contracts.FileSystemRead)
		manager, err := authorization.NewPermissionManager(env.WorkspacePath, perms, env.AuditSink, nil)
		if err != nil {
			t.Fatalf("failed to create permission manager: %v", err)
		}

		// Try to access a path outside the workspace
		outsidePath := "/etc/passwd"

		// Check file access - should be denied
		err = manager.CheckFileAccess(context.Background(), "test-agent", contracts.FileSystemRead, outsidePath)
		if err == nil {
			t.Error("expected path outside workspace to be denied, got success")
		}

		// Verify audit record was created for the denied access
		records := env.AuditSink.Records()
		if len(records) == 0 {
			t.Error("expected audit record for denied file access")
		}
		if len(records) > 0 && records[0].Result != "denied" {
			t.Errorf("expected audit result 'denied', got %s", records[0].Result)
		}
	})

	t.Run("path traversal denied", func(t *testing.T) {
		env := NewTestEnvironment(t)

		// Grant read permission for the workspace
		perms := policy.NewFileSystemPermissionSet(env.WorkspacePath, contracts.FileSystemRead)
		manager, err := authorization.NewPermissionManager(env.WorkspacePath, perms, env.AuditSink, nil)
		if err != nil {
			t.Fatalf("failed to create permission manager: %v", err)
		}

		// Try to access a path using traversal
		traversalPath := filepath.Join(env.WorkspacePath, "..", "etc", "passwd")

		// Check file access - should be denied
		err = manager.CheckFileAccess(context.Background(), "test-agent", contracts.FileSystemRead, traversalPath)
		if err == nil {
			t.Error("expected path traversal to be denied, got success")
		}
	})
}

// TestNetworkAccessAllowDeny validates network permission enforcement
// at the authorization seam.
func TestNetworkAccessAllowDeny(t *testing.T) {
	t.Run("allowed network egress", func(t *testing.T) {
		env := NewTestEnvironment(t)

		// Grant network permission
		perms := policy.NewFileSystemPermissionSet(env.WorkspacePath)
		perms.Network = []contracts.NetworkPermission{
			{Direction: "egress", Protocol: "tcp", Host: "example.com", Port: 443},
		}
		manager, err := authorization.NewPermissionManager(env.WorkspacePath, perms, env.AuditSink, nil)
		if err != nil {
			t.Fatalf("failed to create permission manager: %v", err)
		}

		// Check network access - should be allowed
		err = manager.CheckNetwork(context.Background(), "test-agent", "egress", "tcp", "example.com", 443)
		if err != nil {
			t.Errorf("expected network access to be allowed, got error: %v", err)
		}

		// Verify audit record was created
		records := env.AuditSink.Records()
		if len(records) == 0 {
			t.Error("expected audit record for allowed network access")
		}
		if len(records) > 0 && records[0].Result != "granted" {
			t.Errorf("expected audit result 'granted', got %s", records[0].Result)
		}
	})

	t.Run("denied network egress without permission", func(t *testing.T) {
		env := NewTestEnvironment(t)

		// Grant filesystem permission but no network permissions
		perms := policy.NewFileSystemPermissionSet(env.WorkspacePath, contracts.FileSystemRead)
		manager, err := authorization.NewPermissionManager(env.WorkspacePath, perms, env.AuditSink, nil)
		if err != nil {
			t.Fatalf("failed to create permission manager: %v", err)
		}

		// Check network access - should be denied
		err = manager.CheckNetwork(context.Background(), "test-agent", "egress", "tcp", "example.com", 443)
		if err == nil {
			t.Error("expected network access to be denied, got success")
		}

		// Verify audit record was created
		records := env.AuditSink.Records()
		if len(records) == 0 {
			t.Error("expected audit record for denied network access")
		}
		if len(records) > 0 && records[0].Result != "denied" {
			t.Errorf("expected audit result 'denied', got %s", records[0].Result)
		}
	})

	t.Run("denied network egress to different host", func(t *testing.T) {
		env := NewTestEnvironment(t)

		// Grant permission for example.com only
		perms := policy.NewFileSystemPermissionSet(env.WorkspacePath)
		perms.Network = []contracts.NetworkPermission{
			{Direction: "egress", Protocol: "tcp", Host: "example.com", Port: 443},
		}
		manager, err := authorization.NewPermissionManager(env.WorkspacePath, perms, env.AuditSink, nil)
		if err != nil {
			t.Fatalf("failed to create permission manager: %v", err)
		}

		// Try to access different host
		err = manager.CheckNetwork(context.Background(), "test-agent", "egress", "tcp", "malicious.com", 443)
		if err == nil {
			t.Error("expected network access to different host to be denied, got success")
		}
	})

	t.Run("denied network egress to different port", func(t *testing.T) {
		env := NewTestEnvironment(t)

		// Grant permission for port 443 only
		perms := policy.NewFileSystemPermissionSet(env.WorkspacePath)
		perms.Network = []contracts.NetworkPermission{
			{Direction: "egress", Protocol: "tcp", Host: "example.com", Port: 443},
		}
		manager, err := authorization.NewPermissionManager(env.WorkspacePath, perms, env.AuditSink, nil)
		if err != nil {
			t.Fatalf("failed to create permission manager: %v", err)
		}

		// Try to access different port
		err = manager.CheckNetwork(context.Background(), "test-agent", "egress", "tcp", "example.com", 8080)
		if err == nil {
			t.Error("expected network access to different port to be denied, got success")
		}
	})

	t.Run("denied network egress with different protocol", func(t *testing.T) {
		env := NewTestEnvironment(t)

		// Grant permission for tcp only
		perms := policy.NewFileSystemPermissionSet(env.WorkspacePath)
		perms.Network = []contracts.NetworkPermission{
			{Direction: "egress", Protocol: "tcp", Host: "example.com", Port: 443},
		}
		manager, err := authorization.NewPermissionManager(env.WorkspacePath, perms, env.AuditSink, nil)
		if err != nil {
			t.Fatalf("failed to create permission manager: %v", err)
		}

		// Try to access with different protocol
		err = manager.CheckNetwork(context.Background(), "test-agent", "egress", "udp", "example.com", 443)
		if err == nil {
			t.Error("expected network access with different protocol to be denied, got success")
		}
	})
}

// TestHITLRequiredPath validates HITL-required permission enforcement
// at the authorization seam.
func TestHITLRequiredPath(t *testing.T) {
	t.Run("HITL required file access denied without provider", func(t *testing.T) {
		env := NewTestEnvironment(t)

		// Create a file in the workspace
		testFile := filepath.Join(env.WorkspacePath, "secret.txt")
		if err := os.WriteFile(testFile, []byte("secret content"), 0o644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}

		// Grant read permission with HITL required
		perms := policy.NewFileSystemPermissionSet(env.WorkspacePath, contracts.FileSystemRead)
		for i := range perms.FileSystem {
			perms.FileSystem[i].HITLRequired = true
		}
		manager, err := authorization.NewPermissionManager(env.WorkspacePath, perms, env.AuditSink, nil)
		if err != nil {
			t.Fatalf("failed to create permission manager: %v", err)
		}

		// Check file access - should be denied (no HITL provider)
		err = manager.CheckFileAccess(context.Background(), "test-agent", contracts.FileSystemRead, testFile)
		if err == nil {
			t.Error("expected HITL-required file access to be denied without provider, got success")
		}

		// Verify audit record was created
		records := env.AuditSink.Records()
		if len(records) == 0 {
			t.Error("expected audit record for denied HITL-required access")
		}
		if len(records) > 0 && records[0].Result != "denied" {
			t.Errorf("expected audit result 'denied', got %s", records[0].Result)
		}
	})

	t.Run("HITL required network access denied without provider", func(t *testing.T) {
		env := NewTestEnvironment(t)

		// Grant network permission with HITL required
		perms := policy.NewFileSystemPermissionSet(env.WorkspacePath)
		perms.Network = []contracts.NetworkPermission{
			{Direction: "egress", Protocol: "tcp", Host: "api.example.com", Port: 443, HITLRequired: true},
		}
		manager, err := authorization.NewPermissionManager(env.WorkspacePath, perms, env.AuditSink, nil)
		if err != nil {
			t.Fatalf("failed to create permission manager: %v", err)
		}

		// Check network access - should be denied (no HITL provider)
		err = manager.CheckNetwork(context.Background(), "test-agent", "egress", "tcp", "api.example.com", 443)
		if err == nil {
			t.Error("expected HITL-required network access to be denied without provider, got success")
		}

		// Verify audit record was created
		records := env.AuditSink.Records()
		if len(records) == 0 {
			t.Error("expected audit record for denied HITL-required access")
		}
		if len(records) > 0 && records[0].Result != "denied" {
			t.Errorf("expected audit result 'denied', got %s", records[0].Result)
		}
	})

	t.Run("HITL required file access allowed with provider approval", func(t *testing.T) {
		env := NewTestEnvironment(t)

		// Create a file in the workspace
		testFile := filepath.Join(env.WorkspacePath, "secret.txt")
		if err := os.WriteFile(testFile, []byte("secret content"), 0o644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}

		// Create a stub HITL provider that approves
		hitl := &stubHITL{
			grants: []*authorization.PermissionGrant{{
				ID: "grant-1",
				Permission: contracts.PermissionDescriptor{
					Type:     contracts.PermissionTypeFilesystem,
					Action:   string(contracts.FileSystemRead),
					Resource: testFile,
				},
				Scope: authorization.GrantScopeSession,
			}},
		}

		// Grant read permission with HITL required
		perms := policy.NewFileSystemPermissionSet(env.WorkspacePath, contracts.FileSystemRead)
		for i := range perms.FileSystem {
			perms.FileSystem[i].HITLRequired = true
		}
		manager, err := authorization.NewPermissionManager(env.WorkspacePath, perms, env.AuditSink, hitl)
		if err != nil {
			t.Fatalf("failed to create permission manager: %v", err)
		}

		// Check file access - should be allowed with HITL approval
		err = manager.CheckFileAccess(context.Background(), "test-agent", contracts.FileSystemRead, testFile)
		if err != nil {
			t.Errorf("expected HITL-required file access to be allowed with approval, got error: %v", err)
		}

		// Verify HITL request was made
		if len(hitl.requests) != 1 {
			t.Errorf("expected exactly one HITL request, got %d", len(hitl.requests))
		}
	})
}

// TestAuditOnDeny validates that denied permission checks produce
// audit records with the correct denial information.
func TestAuditOnDeny(t *testing.T) {
	t.Run("file access denial produces audit record", func(t *testing.T) {
		env := NewTestEnvironment(t)

		// Grant write permission only (not read)
		perms := policy.NewFileSystemPermissionSet(env.WorkspacePath, contracts.FileSystemWrite)
		manager, err := authorization.NewPermissionManager(env.WorkspacePath, perms, env.AuditSink, nil)
		if err != nil {
			t.Fatalf("failed to create permission manager: %v", err)
		}

		testFile := filepath.Join(env.WorkspacePath, "test.txt")

		// Check file access - should be denied
		err = manager.CheckFileAccess(context.Background(), "test-agent", contracts.FileSystemRead, testFile)
		if err == nil {
			t.Error("expected file access to be denied")
		}

		// Verify audit record
		records := env.AuditSink.Records()
		if len(records) != 1 {
			t.Fatalf("expected exactly one audit record, got %d", len(records))
		}

		record := records[0]
		if record.Result != "denied" {
			t.Errorf("expected audit result 'denied', got %s", record.Result)
		}
		if record.AgentID != "test-agent" {
			t.Errorf("expected agent ID 'test-agent', got %s", record.AgentID)
		}
		if record.Type != "filesystem" {
			t.Errorf("expected audit type 'filesystem', got %s", record.Type)
		}
	})

	t.Run("network access denial produces audit record", func(t *testing.T) {
		env := NewTestEnvironment(t)

		// Grant filesystem permission but no network permissions
		perms := policy.NewFileSystemPermissionSet(env.WorkspacePath, contracts.FileSystemRead)
		manager, err := authorization.NewPermissionManager(env.WorkspacePath, perms, env.AuditSink, nil)
		if err != nil {
			t.Fatalf("failed to create permission manager: %v", err)
		}

		// Check network access - should be denied
		err = manager.CheckNetwork(context.Background(), "test-agent", "egress", "tcp", "example.com", 443)
		if err == nil {
			t.Error("expected network access to be denied")
		}

		// Verify audit record
		records := env.AuditSink.Records()
		if len(records) != 1 {
			t.Fatalf("expected exactly one audit record, got %d", len(records))
		}

		record := records[0]
		if record.Result != "denied" {
			t.Errorf("expected audit result 'denied', got %s", record.Result)
		}
		if record.AgentID != "test-agent" {
			t.Errorf("expected agent ID 'test-agent', got %s", record.AgentID)
		}
	})

	t.Run("audit records include denial reason", func(t *testing.T) {
		env := NewTestEnvironment(t)

		// Grant write permission only (not read)
		perms := policy.NewFileSystemPermissionSet(env.WorkspacePath, contracts.FileSystemWrite)
		manager, err := authorization.NewPermissionManager(env.WorkspacePath, perms, env.AuditSink, nil)
		if err != nil {
			t.Fatalf("failed to create permission manager: %v", err)
		}

		testFile := filepath.Join(env.WorkspacePath, "test.txt")

		// Check file access - should be denied
		err = manager.CheckFileAccess(context.Background(), "test-agent", contracts.FileSystemRead, testFile)
		if err == nil {
			t.Error("expected file access to be denied")
		}

		// Verify audit record includes reason
		records := env.AuditSink.Records()
		if len(records) != 1 {
			t.Fatalf("expected exactly one audit record, got %d", len(records))
		}

		record := records[0]
		if record.Action == "" {
			t.Error("expected audit record to include denial reason in Action field")
		}
	})
}

// stubHITL is a stub HITL provider for testing.
type stubHITL struct {
	grants   []*authorization.PermissionGrant
	requests []authorization.PermissionRequest
}

func (s *stubHITL) RequestPermission(ctx context.Context, req authorization.PermissionRequest) (*authorization.PermissionGrant, error) {
	s.requests = append(s.requests, req)
	if len(s.grants) == 0 {
		return &authorization.PermissionGrant{Permission: req.Permission, Scope: authorization.GrantScopeSession}, nil
	}
	grant := s.grants[0]
	s.grants = s.grants[1:]
	if grant.Permission.Action == "" {
		grant.Permission = req.Permission
	}
	return grant, nil
}
