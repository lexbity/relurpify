package framework

import (
	"context"
	"errors"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// TestNetworkBoundaryEnforcement validates that network permissions are
// enforced correctly at the authorization seam.
func TestNetworkBoundaryEnforcement(t *testing.T) {
	t.Run("allow-listed host access succeeds", func(t *testing.T) {
		env := NewTestEnvironment(t)

		// Create permission manager with network permission
		perms := core.NewFileSystemPermissionSet(env.WorkspacePath, contracts.FileSystemRead)
		perms.Network = []contracts.NetworkPermission{
			{Direction: "egress", Protocol: "tcp", Host: "example.com", Port: 443},
		}
		manager, err := authorization.NewPermissionManager(env.WorkspacePath, perms, env.AuditSink, nil)
		if err != nil {
			t.Fatalf("failed to create permission manager: %v", err)
		}

		// Check network access to allow-listed host
		err = manager.CheckNetwork(context.Background(), "test-agent", "egress", "tcp", "example.com", 443)
		if err != nil {
			t.Errorf("network access to allow-listed host should succeed: %v", err)
		}

		// Verify audit record was created
		records := env.AuditSink.Records()
		if len(records) != 1 {
			t.Fatalf("expected exactly one audit record for allow-listed network access, got %d", len(records))
		}
		record := records[0]
		if record.Result != "granted" {
			t.Errorf("expected audit result 'granted', got %s", record.Result)
		}
		if record.Type != string(contracts.PermissionTypeNetwork) {
			t.Errorf("expected audit type 'network', got %s", record.Type)
		}
		if record.Action != "net:egress" {
			t.Errorf("expected audit action 'net:egress', got %s", record.Action)
		}
		if record.Permission != "example.com:443" {
			t.Errorf("expected audit resource 'example.com:443', got %s", record.Permission)
		}
		if record.Correlation != "test-agent" {
			t.Errorf("expected correlation 'test-agent', got %s", record.Correlation)
		}
	})

	t.Run("denied host access fails", func(t *testing.T) {
		env := NewTestEnvironment(t)

		// Create permission manager with network permission for specific host
		perms := core.NewFileSystemPermissionSet(env.WorkspacePath, contracts.FileSystemRead)
		perms.Network = []contracts.NetworkPermission{
			{Direction: "egress", Protocol: "tcp", Host: "example.com", Port: 443},
		}
		manager, err := authorization.NewPermissionManager(env.WorkspacePath, perms, env.AuditSink, nil)
		if err != nil {
			t.Fatalf("failed to create permission manager: %v", err)
		}
		manager.SetDefaultPolicy(agentspec.AgentPermissionDeny)

		// Check network access to non-allow-listed host
		err = manager.CheckNetwork(context.Background(), "test-agent", "egress", "tcp", "denied.com", 443)
		if err == nil {
			t.Error("network access to denied host should fail")
		}
		var deniedErr *contracts.PermissionDeniedError
		if !errors.As(err, &deniedErr) {
			t.Fatalf("expected PermissionDeniedError, got %T: %v", err, err)
		}
		if !strings.Contains(deniedErr.Message, "network scope missing") {
			t.Fatalf("expected deny reason to mention network scope missing, got %q", deniedErr.Message)
		}

		// Verify audit record was created
		records := env.AuditSink.Records()
		if len(records) != 1 {
			t.Fatalf("expected exactly one audit record for denied network access, got %d", len(records))
		}
		record := records[0]
		if record.Result != "denied" {
			t.Errorf("expected audit result 'denied', got %s", record.Result)
		}
		if record.Action != "net:egress:tcp:denied.com:443" {
			t.Errorf("expected audit action 'net:egress:tcp:denied.com:443', got %s", record.Action)
		}
		if record.Metadata == nil || record.Metadata["reason"] != "network scope missing" {
			t.Fatalf("expected denial reason 'network scope missing', got %#v", record.Metadata)
		}
	})

	t.Run("network permission with HITL requirement", func(t *testing.T) {
		env := NewTestEnvironment(t)

		// Create stub HITL provider
		hitl := &stubHITL{
			grants: []*authorization.PermissionGrant{{
				ID: "grant-1",
				Permission: contracts.PermissionDescriptor{
					Type:     contracts.PermissionTypeNetwork,
					Action:   "net:egress",
					Resource: "api.service.local:443",
				},
				Scope: authorization.GrantScopeSession,
			}},
		}

		// Create permission manager with HITL-required network permission
		perms := core.NewFileSystemPermissionSet(env.WorkspacePath, contracts.FileSystemRead)
		perms.Network = []contracts.NetworkPermission{
			{Direction: "egress", Protocol: "tcp", Host: "api.service.local", Port: 443, HITLRequired: true},
		}
		manager, err := authorization.NewPermissionManager(env.WorkspacePath, perms, env.AuditSink, hitl)
		if err != nil {
			t.Fatalf("failed to create permission manager: %v", err)
		}

		// Check network access with HITL requirement
		err = manager.CheckNetwork(context.Background(), "test-agent", "egress", "tcp", "api.service.local", 443)
		if err != nil {
			t.Errorf("network access with HITL approval should succeed: %v", err)
		}

		// Verify HITL request was made
		if len(hitl.requests) != 1 {
			t.Errorf("expected exactly one HITL request, got %d", len(hitl.requests))
		} else {
			req := hitl.requests[0]
			if req.Permission.Type != contracts.PermissionTypeNetwork {
				t.Errorf("expected HITL request type network, got %s", req.Permission.Type)
			}
			if req.Permission.Action != "net:egress:tcp" {
				t.Errorf("expected HITL request action net:egress:tcp, got %s", req.Permission.Action)
			}
			if req.Permission.Resource != "api.service.local:443" {
				t.Errorf("expected HITL request resource api.service.local:443, got %s", req.Permission.Resource)
			}
			if req.Scope != authorization.GrantScopeSession {
				t.Errorf("expected HITL request scope session, got %s", req.Scope)
			}
		}

		// Verify audit record was created
		records := env.AuditSink.Records()
		if len(records) != 1 {
			t.Fatalf("expected exactly one audit record for HITL network access, got %d", len(records))
		}
		record := records[0]
		if record.Result != "granted" {
			t.Errorf("expected HITL audit result 'granted', got %s", record.Result)
		}
		if record.Action != "net:egress" {
			t.Errorf("expected HITL audit action 'net:egress', got %s", record.Action)
		}
	})

	t.Run("cached HITL approval is reused", func(t *testing.T) {
		env := NewTestEnvironment(t)

		// Create stub HITL provider
		hitl := &stubHITL{
			grants: []*authorization.PermissionGrant{{
				ID: "grant-1",
				Permission: contracts.PermissionDescriptor{
					Type:     contracts.PermissionTypeNetwork,
					Action:   "net:egress",
					Resource: "api.service.local:443",
				},
				Scope: authorization.GrantScopeSession,
			}},
		}

		// Create permission manager with HITL-required network permission
		perms := core.NewFileSystemPermissionSet(env.WorkspacePath, contracts.FileSystemRead)
		perms.Network = []contracts.NetworkPermission{
			{Direction: "egress", Protocol: "tcp", Host: "api.service.local", Port: 443, HITLRequired: true},
		}
		manager, err := authorization.NewPermissionManager(env.WorkspacePath, perms, env.AuditSink, hitl)
		if err != nil {
			t.Fatalf("failed to create permission manager: %v", err)
		}

		// First network access should trigger HITL request
		err = manager.CheckNetwork(context.Background(), "test-agent", "egress", "tcp", "api.service.local", 443)
		if err != nil {
			t.Fatalf("first network access should succeed: %v", err)
		}

		if len(hitl.requests) != 1 {
			t.Fatalf("expected one HITL request after first access, got %d", len(hitl.requests))
		}

		// Second network access should use cached approval
		err = manager.CheckNetwork(context.Background(), "test-agent", "egress", "tcp", "api.service.local", 443)
		if err != nil {
			t.Fatalf("second network access should succeed with cached approval: %v", err)
		}

		// Verify no additional HITL request was made
		if len(hitl.requests) != 1 {
			t.Errorf("expected cached approval to prevent duplicate HITL calls, got %d requests", len(hitl.requests))
		}
		records := env.AuditSink.Records()
		if len(records) != 2 {
			t.Fatalf("expected two granted audit records after cached HITL reuse, got %d", len(records))
		}
		for i, record := range records {
			if record.Result != "granted" {
				t.Errorf("audit record %d should be granted, got %s", i, record.Result)
			}
			if record.Action != "net:egress" {
				t.Errorf("audit record %d should record net:egress, got %s", i, record.Action)
			}
		}
	})
}

// TestNetworkCapabilityGating validates that capability execution is gated
// by network permissions.
func TestNetworkCapabilityGating(t *testing.T) {
	t.Run("tool with network permission executes successfully", func(t *testing.T) {
		env := NewTestEnvironment(t)

		// Create permission manager with network permission
		perms := core.NewFileSystemPermissionSet(env.WorkspacePath, contracts.FileSystemRead)
		perms.Network = []contracts.NetworkPermission{
			{Direction: "egress", Protocol: "tcp", Host: "example.com", Port: 443},
		}
		manager, err := authorization.NewPermissionManager(env.WorkspacePath, perms, env.AuditSink, nil)
		if err != nil {
			t.Fatalf("failed to create permission manager: %v", err)
		}

		// Create capability registry
		registry := capability.NewCapabilityRegistry()
		registry.UsePermissionManager("test-agent", manager)

		// Register a tool that requires network permission
		tool := &networkTestTool{
			name:        "network-tool",
			description: "tool with network access",
			category:    "test",
			permissions: core.NewFileSystemPermissionSet(env.WorkspacePath, contracts.FileSystemRead),
			networkPerms: []contracts.NetworkPermission{
				{Direction: "egress", Protocol: "tcp", Host: "example.com", Port: 443},
			},
			manager: manager,
			agent:   "test-agent",
		}

		if err := registry.Register(tool); err != nil {
			t.Fatalf("failed to register tool: %v", err)
		}

		// Execute the tool
		cap, _ := registry.Get("network-tool")
		_, err = cap.Execute(context.Background(), nil)
		if err != nil {
			t.Errorf("tool execution should succeed with network permission: %v", err)
		}

		// Verify audit record was created
		records := env.AuditSink.Records()
		if len(records) == 0 {
			t.Error("expected audit record for tool execution")
		}
		if tool.executed == false {
			t.Error("expected network tool to execute when permission is granted")
		}
		AssertAuditRecordExists(t, env, core.AuditQuery{Type: string(contracts.PermissionTypeHITL), Action: "tool:network-tool", Permission: "test-agent", Result: "tool_allowed"})
		AssertAuditRecordExists(t, env, core.AuditQuery{Type: string(contracts.PermissionTypeNetwork), Permission: "example.com:443", Result: "granted"})
	})

	t.Run("tool without network permission is denied", func(t *testing.T) {
		env := NewTestEnvironment(t)

		// Create permission manager without network permission
		perms := core.NewFileSystemPermissionSet(env.WorkspacePath, contracts.FileSystemRead)
		manager, err := authorization.NewPermissionManager(env.WorkspacePath, perms, env.AuditSink, nil)
		if err != nil {
			t.Fatalf("failed to create permission manager: %v", err)
		}

		// Create capability registry
		registry := capability.NewCapabilityRegistry()
		registry.UsePermissionManager("test-agent", manager)

		// Register a tool that requires network permission
		tool := &networkTestTool{
			name:        "denied-network-tool",
			description: "tool without network permission",
			category:    "test",
			permissions: core.NewFileSystemPermissionSet(env.WorkspacePath, contracts.FileSystemRead),
			networkPerms: []contracts.NetworkPermission{
				{Direction: "egress", Protocol: "tcp", Host: "denied.com", Port: 443},
			},
			manager: manager,
			agent:   "test-agent",
		}

		if err := registry.Register(tool); err != nil {
			t.Fatalf("failed to register tool: %v", err)
		}

		// Execute the tool - should be denied
		cap, _ := registry.Get("denied-network-tool")
		_, err = cap.Execute(context.Background(), nil)
		if err == nil {
			t.Error("tool execution should be denied without network permission")
		}
		if tool.executed {
			t.Error("expected denied network tool not to execute")
		}

		// Verify audit record was created
		records := env.AuditSink.Records()
		if len(records) == 0 {
			t.Error("expected audit record for denied tool execution")
		}
		AssertAuditRecordExists(t, env, core.AuditQuery{Type: string(contracts.PermissionTypeHITL), Action: "tool:denied-network-tool", Permission: "test-agent", Result: "tool_allowed"})
		AssertAuditRecordExists(t, env, core.AuditQuery{Type: string(contracts.PermissionTypeNetwork), Action: "net:egress:tcp:denied.com:443", Permission: "denied.com", Result: "denied"})
	})
}

// networkTestTool is a tool that checks network permissions before execution.
type networkTestTool struct {
	name         string
	description  string
	category     string
	permissions  *contracts.PermissionSet
	networkPerms []contracts.NetworkPermission
	manager      *authorization.PermissionManager
	agent        string
	executed     bool
}

func (t *networkTestTool) Name() string                          { return t.name }
func (t *networkTestTool) Description() string                   { return t.description }
func (t *networkTestTool) Category() string                      { return t.category }
func (t *networkTestTool) Parameters() []contracts.ToolParameter { return nil }
func (t *networkTestTool) Execute(ctx context.Context, args map[string]interface{}) (*contracts.ToolResult, error) {
	// Check network permissions before execution
	if t.manager != nil && len(t.networkPerms) > 0 {
		for _, netPerm := range t.networkPerms {
			if err := t.manager.CheckNetwork(ctx, t.agent, netPerm.Direction, netPerm.Protocol, netPerm.Host, netPerm.Port); err != nil {
				return nil, err
			}
		}
	}
	t.executed = true
	return &contracts.ToolResult{Success: true}, nil
}
func (t *networkTestTool) IsAvailable(context.Context) bool { return true }
func (t *networkTestTool) Permissions() contracts.ToolPermissions {
	return contracts.ToolPermissions{Permissions: t.permissions}
}
func (t *networkTestTool) Tags() []string { return nil }
