package authorization

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/sandbox"
)

type stubSandboxRuntimePhase2 struct {
	policies []sandbox.SandboxPolicy
	err      error
}

func (s *stubSandboxRuntimePhase2) Name() string { return "stub" }

func (s *stubSandboxRuntimePhase2) Verify(ctx context.Context) error { return nil }

func (s *stubSandboxRuntimePhase2) Capabilities() sandbox.Capabilities { return sandbox.Capabilities{} }

func (s *stubSandboxRuntimePhase2) ValidatePolicy(policy sandbox.SandboxPolicy) error { return nil }

func (s *stubSandboxRuntimePhase2) ApplyPolicy(_ context.Context, policy sandbox.SandboxPolicy) error {
	if s.err != nil {
		return s.err
	}
	s.policies = append(s.policies, policy)
	return nil
}

func (s *stubSandboxRuntimePhase2) RunConfig() sandbox.SandboxConfig { return sandbox.SandboxConfig{} }

func (s *stubSandboxRuntimePhase2) Policy() sandbox.SandboxPolicy {
	if len(s.policies) == 0 {
		return sandbox.SandboxPolicy{}
	}
	return s.policies[len(s.policies)-1]
}

type stubHITLProviderPhase2 struct {
	requests []PermissionRequest
}

func (s *stubHITLProviderPhase2) RequestPermission(ctx context.Context, req PermissionRequest) (*PermissionGrant, error) {
	s.requests = append(s.requests, req)
	return &PermissionGrant{
		ID:         req.ID,
		Permission: req.Permission,
		Scope:      req.Scope,
		ApprovedBy: "stub",
		GrantedAt:  time.Now().UTC(),
	}, nil
}

type stubPolicyEnginePhase2 struct {
	decision core.PolicyDecision
	err      error
}

type taskGrantTool struct{}

func (taskGrantTool) Name() string                     { return "task-grant-tool" }
func (taskGrantTool) Description() string              { return "task grant tool" }
func (taskGrantTool) Category() string                 { return "test" }
func (taskGrantTool) Parameters() []core.ToolParameter { return nil }
func (taskGrantTool) Execute(context.Context, *core.Context, map[string]interface{}) (*core.ToolResult, error) {
	return &core.ToolResult{Success: true}, nil
}
func (taskGrantTool) IsAvailable(context.Context, *core.Context) bool { return true }
func (taskGrantTool) Permissions() core.ToolPermissions               { return core.ToolPermissions{} }
func (taskGrantTool) Tags() []string                                  { return []string{"read-only"} }

func (s stubPolicyEnginePhase2) Evaluate(ctx context.Context, req core.PolicyRequest) (core.PolicyDecision, error) {
	return s.decision, s.err
}

func baseAuthzPermissionSet(base string) *core.PermissionSet {
	return &core.PermissionSet{
		FileSystem: []core.FileSystemPermission{{
			Action: core.FileSystemRead,
			Path:   filepath.ToSlash(filepath.Join(base, "**")),
		}},
		Executables: []core.ExecutablePermission{{
			Binary: "git",
			Args:   []string{"status", "*"},
			Env:    []string{"HOME=*"},
		}},
		Network: []core.NetworkPermission{{
			Direction: "egress",
			Protocol:  "tcp",
			Host:      "example.com",
			Port:      443,
		}},
		Capabilities: []core.CapabilityPermission{{
			Capability: "cap_sys_admin",
		}},
		IPC: []core.IPCPermission{{
			Kind:   "pipe",
			Target: "worker",
		}},
	}
}

func TestAuthorizationPolicyEngineHelpersAndFallbacks(t *testing.T) {
	req := core.PolicyRequest{
		CapabilityName:   "capability-name",
		CapabilityID:     "capability-id",
		ExportName:       "artifact",
		SessionOperation: core.SessionOperationResume,
		SessionID:        "session-1",
		LineageID:        "lineage-1",
		Actor:            core.EventActor{ID: "actor-1"},
		Target:           core.PolicyTargetResume,
	}
	if got := permissionActionForRequest(req); got != "capability-name" {
		t.Fatalf("unexpected action precedence: %q", got)
	}
	req.CapabilityName = ""
	if got := permissionActionForRequest(req); got != "capability-id" {
		t.Fatalf("unexpected capability-id action: %q", got)
	}
	req.CapabilityID = ""
	if got := permissionActionForRequest(req); got != "resume:artifact" {
		t.Fatalf("unexpected resume action: %q", got)
	}
	req.Target = core.PolicyTargetSession
	req.ExportName = ""
	if got := permissionActionForRequest(req); got != "session:resume" {
		t.Fatalf("unexpected session action: %q", got)
	}
	req.Target = core.PolicyTargetProvider
	if got := permissionActionForRequest(req); got != "provider" {
		t.Fatalf("unexpected provider action: %q", got)
	}
	if got := permissionResourceForRequest(req); got != "lineage-1" {
		t.Fatalf("unexpected resource precedence: %q", got)
	}
	req.LineageID = ""
	if got := permissionResourceForRequest(req); got != "session-1" {
		t.Fatalf("unexpected session resource precedence: %q", got)
	}
	req.SessionID = ""
	if got := permissionResourceForRequest(req); got != "actor-1" {
		t.Fatalf("unexpected actor resource precedence: %q", got)
	}

	engine := stubPolicyEnginePhase2{decision: core.PolicyDecisionAllow("allow")}
	if decision, err := EvaluatePolicyRequest(context.Background(), nil, req); err != nil || decision.Effect != "allow" {
		t.Fatalf("expected nil engine to allow, got %#v, %v", decision, err)
	}
	decision, err := EvaluatePolicyRequest(context.Background(), engine, req)
	if err != nil || decision.Effect != "allow" {
		t.Fatalf("unexpected policy evaluation result: %#v, %v", decision, err)
	}

	approvalBase := t.TempDir()
	approvalMgr := newTestPM(t, approvalBase, baseAuthzPermissionSet(approvalBase))
	approvalMgr.hitl = &stubHITLProviderPhase2{}
	approvalReq := ApprovalRequest{
		AgentID:       "agent-1",
		Manager:       approvalMgr,
		Permission:    core.PermissionDescriptor{Type: core.PermissionTypeFilesystem, Action: "fs:read", Resource: "/tmp/file"},
		Justification: "review required",
		Scope:         GrantScopeSession,
		Risk:          RiskLevelMedium,
	}
	decision = core.PolicyDecisionRequireApproval(nil)
	_, err = EnforcePolicyRequest(context.Background(), stubPolicyEnginePhase2{decision: decision}, req, approvalReq)
	if err != nil {
		t.Fatalf("expected approval flow to succeed, got %v", err)
	}

	denyDecision := core.PolicyDecisionDeny("blocked")
	if _, err := EnforcePolicyRequest(context.Background(), stubPolicyEnginePhase2{decision: denyDecision}, req, approvalReq); err == nil {
		t.Fatal("expected deny decision to return an error")
	}

	if _, err := EnforcePolicyRequest(context.Background(), stubPolicyEnginePhase2{decision: core.PolicyDecisionRequireApproval(nil)}, req, ApprovalRequest{}); err == nil {
		t.Fatal("expected missing manager to fail approval request")
	}
}

func TestPermissionManagerAttachRuntimePreservesExistingRuntimePolicy(t *testing.T) {
	base := t.TempDir()
	pm := newTestPM(t, base, baseAuthzPermissionSet(base))
	runtime := &stubSandboxRuntimePhase2{}
	if err := runtime.ApplyPolicy(context.Background(), sandbox.SandboxPolicy{
		ReadOnlyRoot:   true,
		ProtectedPaths: []string{filepath.Join(base, "relurpify_cfg", "agent.manifest.yaml")},
	}); err != nil {
		t.Fatalf("seed runtime policy: %v", err)
	}
	pm.AttachRuntime(runtime)
	if len(runtime.policies) == 0 {
		t.Fatal("expected attach runtime to apply merged policy")
	}
	current := runtime.Policy()
	if !current.ReadOnlyRoot {
		t.Fatal("expected read-only root to be preserved")
	}
	if len(current.ProtectedPaths) == 0 {
		t.Fatal("expected protected paths to be preserved")
	}
	snapshot := pm.Policy()
	if len(snapshot.ProtectedPaths) == 0 {
		t.Fatal("expected snapshot to preserve protected paths")
	}
}

func TestPermissionManagerRuntimePolicyErrorIsRecorded(t *testing.T) {
	base := t.TempDir()
	pm := newTestPM(t, base, baseAuthzPermissionSet(base))
	runtime := &stubSandboxRuntimePhase2{err: context.Canceled}
	pm.AttachRuntime(runtime)
	if err := pm.RuntimePolicyError(); err == nil {
		t.Fatal("expected runtime policy sync error to be recorded")
	}
}

func TestAuthorizationCompileGlobalAndAgentSpecPolicies(t *testing.T) {
	rules, err := CompileAgentSpecPolicyRules(&core.AgentRuntimeSpec{
		GlobalPolicies: map[string]core.AgentPermissionLevel{
			string(core.TrustClassBuiltinTrusted):        core.AgentPermissionAllow,
			string(core.CapabilityRuntimeFamilyProvider): core.AgentPermissionDeny,
		},
		CapabilityPolicies: []core.CapabilityPolicy{{
			Selector: core.CapabilitySelector{
				ID:   "capability:one",
				Kind: core.CapabilityKindTool,
			},
			Execute: core.AgentPermissionAllow,
		}},
	})
	if err != nil {
		t.Fatalf("CompileAgentSpecPolicyRules: %v", err)
	}
	if len(rules) != 3 {
		t.Fatalf("expected 3 compiled rules, got %d", len(rules))
	}

	if _, err := compileGlobalPolicy("unsupported-class", core.AgentPermissionAllow); err == nil {
		t.Fatal("expected unsupported global policy class to fail")
	}

	rule, ok := compileToolExecutionPolicy("tool-name", core.ToolPolicy{Execute: core.AgentPermissionDeny})
	if !ok || rule.Effect.Action != "deny" {
		t.Fatalf("unexpected tool policy compilation: %#v, %v", rule, ok)
	}
}

func TestPermissionManagerLifecycleAndHITL(t *testing.T) {
	base := t.TempDir()
	pm := newTestPM(t, base, baseAuthzPermissionSet(base))
	runtime := &stubSandboxRuntimePhase2{}

	pm.netPolicy = []sandbox.NetworkRule{{Direction: "egress", Protocol: "tcp", Host: "bootstrap", Port: 443}}
	pm.AttachRuntime(runtime)
	if len(runtime.policies) != 1 {
		t.Fatalf("expected attach runtime to enforce preloaded policy, got %#v", runtime.policies)
	}

	pm.SetDefaultPolicy(core.AgentPermissionAllow)
	if got := pm.DefaultPolicy(); got != core.AgentPermissionAllow {
		t.Fatalf("unexpected default policy: %v", got)
	}
	pm.SetEventLogger(func(context.Context, core.PermissionDescriptor, string, string, map[string]interface{}) {})

	if err := pm.CheckCapability(context.Background(), "agent-1", "cap_sys_admin"); err != nil {
		t.Fatalf("CheckCapability: %v", err)
	}
	if err := pm.CheckCapability(context.Background(), "agent-1", "missing"); err == nil {
		t.Fatal("expected missing capability to be denied")
	}

	if err := pm.CheckFileAccess(context.Background(), "agent-1", core.FileSystemRead, filepath.Join(base, "pkg", "file.go")); err != nil {
		t.Fatalf("CheckFileAccess: %v", err)
	}
	if err := pm.CheckFileAccess(context.Background(), "agent-1", core.FileSystemRead, "../escape.go"); err == nil {
		t.Fatal("expected traversal path to be rejected")
	}

	if err := pm.CheckExecutable(context.Background(), "agent-1", "git", []string{"status", "repo"}, []string{"HOME=/tmp"}); err != nil {
		t.Fatalf("CheckExecutable: %v", err)
	}
	if err := pm.CheckExecutable(context.Background(), "agent-1", "git", []string{"status", "repo"}, []string{"USER=/other"}); err == nil {
		t.Fatal("expected env mismatch to be denied")
	}

	if err := pm.CheckNetwork(context.Background(), "agent-1", "egress", "tcp", "example.com", 443); err != nil {
		t.Fatalf("CheckNetwork: %v", err)
	}
	if len(runtime.policies) == 0 || len(runtime.Policy().NetworkRules) == 0 {
		t.Fatal("expected network policy to be propagated to runtime")
	}

	if err := pm.CheckIPC(context.Background(), "agent-1", "pipe", "worker"); err != nil {
		t.Fatalf("CheckIPC: %v", err)
	}

	if err := pm.RegisterTaskGrant("run-1", []string{"read-only"}); err != nil {
		t.Fatalf("RegisterTaskGrant: %v", err)
	}
	taskCtx := core.WithTaskContext(context.Background(), core.TaskContext{ID: "run-1"})
	if !pm.toolAllowedByTaskGrant(taskCtx, taskGrantTool{}) {
		t.Fatal("expected task grant to allow matching tool tags")
	}
	pm.RevokeTaskGrant("run-1")
	if pm.toolAllowedByTaskGrant(taskCtx, taskGrantTool{}) {
		t.Fatal("expected revoked task grant to stop matching")
	}

	desc := core.PermissionDescriptor{Type: core.PermissionTypeFilesystem, Action: "fs:read", Resource: "file"}
	pm.GrantPermission(desc, "reviewer", GrantScopeSession, time.Minute)
	pm.mu.RLock()
	_, ok := pm.grants[desc.Action+":"+desc.Resource]
	pm.mu.RUnlock()
	if !ok {
		t.Fatal("expected manual grant to be stored")
	}

	approval := &stubHITLProviderPhase2{}
	pm.hitl = approval
	if err := pm.RequireApproval(context.Background(), "agent-1", core.PermissionDescriptor{Type: core.PermissionTypeFilesystem, Action: "fs:write", Resource: "file"}, "needs review", GrantScopeSession, RiskLevelMedium, 0); err != nil {
		t.Fatalf("RequireApproval: %v", err)
	}
	if len(approval.requests) != 1 {
		t.Fatalf("expected hitl provider to receive a request, got %#v", approval.requests)
	}
}

func TestHITLBrokerRequestApproveDenyAndPending(t *testing.T) {
	broker := NewHITLBroker(200 * time.Millisecond)
	broker.clock = func() time.Time { return time.Unix(1, 0) }
	events, cancel := broker.Subscribe(4)
	defer cancel()

	resultCh := make(chan struct {
		grant *PermissionGrant
		err   error
	}, 1)
	go func() {
		grant, err := broker.RequestPermission(context.Background(), PermissionRequest{
			Permission: core.PermissionDescriptor{Type: core.PermissionTypeFilesystem, Action: "fs:read", Resource: "file"},
			Scope:      GrantScopeSession,
		})
		resultCh <- struct {
			grant *PermissionGrant
			err   error
		}{grant: grant, err: err}
	}()

	var requested PermissionRequest
	select {
	case ev := <-events:
		if ev.Type != HITLEventRequested {
			t.Fatalf("expected request event, got %s", ev.Type)
		}
		requested = *ev.Request
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for requested event")
	}
	if err := broker.Approve(PermissionDecision{
		RequestID:  requested.ID,
		Approved:   true,
		ApprovedBy: "alice",
		Scope:      GrantScopeSession,
	}); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	select {
	case result := <-resultCh:
		if result.err != nil {
			t.Fatalf("RequestPermission returned error: %v", result.err)
		}
		if result.grant == nil || result.grant.ApprovedBy != "alice" {
			t.Fatalf("unexpected grant: %#v", result.grant)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for approved grant")
	}

	asyncID, err := broker.SubmitAsync(PermissionRequest{
		Permission: core.PermissionDescriptor{Type: core.PermissionTypeFilesystem, Action: "fs:write", Resource: "file"},
		Scope:      GrantScopeOneTime,
	})
	if err != nil {
		t.Fatalf("SubmitAsync: %v", err)
	}
	if len(broker.PendingRequests()) != 1 {
		t.Fatalf("expected one pending request after async submit")
	}
	if err := broker.Deny(asyncID, "blocked"); err != nil {
		t.Fatalf("Deny: %v", err)
	}
	if len(broker.PendingRequests()) != 0 {
		t.Fatalf("expected pending queue to clear after denial")
	}

	timeoutBroker := NewHITLBroker(1 * time.Millisecond)
	_, err = timeoutBroker.RequestPermission(context.Background(), PermissionRequest{
		Permission:      core.PermissionDescriptor{Type: core.PermissionTypeFilesystem, Action: "fs:read", Resource: "file"},
		Scope:           GrantScopeSession,
		TimeoutBehavior: HITLTimeoutBehaviorSkip,
	})
	if err != nil {
		t.Fatalf("expected timeout skip to return grant, got %v", err)
	}
}
