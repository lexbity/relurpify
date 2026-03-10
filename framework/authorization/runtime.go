package authorization

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	"github.com/lexcodex/relurpify/framework/manifest"
	"github.com/lexcodex/relurpify/framework/sandbox"
)

// RuntimeConfig wires sandbox and auditing defaults.
type RuntimeConfig struct {
	ManifestPath string
	ConfigPath   string
	Image        string
	Sandbox      sandbox.SandboxConfig
	AuditLimit   int
	BaseFS       string
	HITLTimeout  time.Duration
}

// AgentRegistration stores runtime metadata.
type AgentRegistration struct {
	ID          string
	Manifest    *manifest.AgentManifest
	Runtime     sandbox.SandboxRuntime
	Permissions *PermissionManager
	Policy      PolicyEngine
	Audit       core.AuditLogger
	HITL        *HITLBroker
}

// RegisterAgent validates the manifest and builds enforcement primitives.
func RegisterAgent(ctx context.Context, cfg RuntimeConfig) (*AgentRegistration, error) {
	if cfg.ManifestPath == "" {
		return nil, errors.New("manifest path required")
	}
	agentManifest, err := manifest.LoadAgentManifest(cfg.ManifestPath)
	if err != nil {
		return nil, fmt.Errorf("load manifest: %w", err)
	}
	effectivePerms, err := manifest.ResolveEffectivePermissions(cfg.BaseFS, agentManifest)
	if err != nil {
		return nil, fmt.Errorf("resolve permissions: %w", err)
	}
	effectiveResources, err := manifest.ResolveEffectiveResources(cfg.BaseFS, agentManifest)
	if err != nil {
		return nil, fmt.Errorf("resolve resources: %w", err)
	}
	agentManifest.Spec.Permissions = effectivePerms
	agentManifest.Spec.Resources = effectiveResources
	runtime := sandbox.NewGVisorRuntime(cfg.Sandbox)
	if err := runtime.Verify(ctx); err != nil {
		return nil, fmt.Errorf("sandbox verification failed: %w", err)
	}
	hitl := NewHITLBroker(cfg.HITLTimeout)
	audit := core.NewInMemoryAuditLogger(cfg.AuditLimit)
	permissions, err := NewPermissionManager(cfg.BaseFS, &agentManifest.Spec.Permissions, audit, hitl)
	if err != nil {
		return nil, fmt.Errorf("permission manager init: %w", err)
	}
	if agentManifest.Spec.Policies != nil {
		if policy, ok := agentManifest.Spec.Policies["default_tool_policy"]; ok {
			permissions.SetDefaultPolicy(policy)
		}
	}
	policyEngine, err := FromManifestWithConfig(agentManifest, agentManifest.Metadata.Name, nil)
	if err != nil {
		return nil, fmt.Errorf("policy engine init: %w", err)
	}
	policyEngine.manager = permissions
	permissions.AttachRuntime(runtime)
	networkRules := buildNetworkPolicy(agentManifest.Spec.Permissions.Network)
	policy := sandbox.SandboxPolicy{
		NetworkRules: networkRules,
		ReadOnlyRoot: agentManifest.Spec.Security.ReadOnlyRoot,
	}
	_ = runtime.EnforcePolicy(policy)
	return &AgentRegistration{
		ID:          agentManifest.Metadata.Name,
		Manifest:    agentManifest,
		Runtime:     runtime,
		Permissions: permissions,
		Policy:      policyEngine,
		Audit:       audit,
		HITL:        hitl,
	}, nil
}

// buildNetworkPolicy converts network permissions into sandbox-friendly rules
// so gVisor enforces the same view of allowed hosts/ports as the permission
// manager.
func buildNetworkPolicy(perms []core.NetworkPermission) []sandbox.NetworkRule {
	var rules []sandbox.NetworkRule
	for _, perm := range perms {
		if perm.Direction != "egress" {
			continue
		}
		rules = append(rules, sandbox.NetworkRule{
			Direction: perm.Direction,
			Protocol:  perm.Protocol,
			Host:      perm.Host,
			Port:      perm.Port,
		})
	}
	return rules
}

// Execute enforces permissions prior to delegating to the agent.
func (r *AgentRegistration) Execute(ctx context.Context, agent graph.Agent, task *core.Task, state *core.Context) (*core.Result, error) {
	if agent == nil {
		return nil, errors.New("agent missing")
	}
	if r == nil || r.Permissions == nil {
		return nil, errors.New("permission subsystem missing")
	}
	if err := agent.Initialize(&core.Config{Name: r.ID, OllamaToolCalling: true}); err != nil {
		return nil, err
	}
	return agent.Execute(ctx, task, state)
}

// QueryAudit proxies queries to the audit store.
func (r *AgentRegistration) QueryAudit(ctx context.Context, filter core.AuditQuery) ([]core.AuditRecord, error) {
	if r == nil || r.Audit == nil {
		return nil, errors.New("audit logger missing")
	}
	return r.Audit.Query(ctx, filter)
}

// GrantPermission allows operators to programmatically approve scopes.
func (r *AgentRegistration) GrantPermission(desc core.PermissionDescriptor, approvedBy string, scope GrantScope, duration time.Duration) {
	if r == nil || r.Permissions == nil {
		return
	}
	grant := GrantManual(desc, approvedBy, scope, duration)
	r.Permissions.mu.Lock()
	defer r.Permissions.mu.Unlock()
	r.Permissions.grants[desc.Action+":"+desc.Resource] = grant
}
