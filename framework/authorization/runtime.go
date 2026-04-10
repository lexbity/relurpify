package authorization

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/framework/config"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	"github.com/lexcodex/relurpify/framework/manifest"
	"github.com/lexcodex/relurpify/framework/sandbox"
	"github.com/lexcodex/relurpify/platform/sandbox/dockersandbox"
)

// RuntimeConfig wires sandbox and auditing defaults.
type RuntimeConfig struct {
	ManifestPath     string
	ManifestSnapshot *manifest.AgentManifestSnapshot
	ConfigPath       string
	Image            string
	Backend          string
	Sandbox          sandbox.SandboxConfig
	AuditLimit       int
	BaseFS           string
	HITLTimeout      time.Duration
}

// AgentRegistration stores runtime metadata.
type AgentRegistration struct {
	ID               string
	Manifest         *manifest.AgentManifest
	ManifestSnapshot *manifest.AgentManifestSnapshot
	Runtime          sandbox.SandboxRuntime
	Permissions      *PermissionManager
	Policy           PolicyEngine
	Audit            core.AuditLogger
	HITL             *HITLBroker
}

// RegisterAgent validates the manifest and builds enforcement primitives.
func RegisterAgent(ctx context.Context, cfg RuntimeConfig) (*AgentRegistration, error) {
	if cfg.ManifestSnapshot == nil && cfg.ManifestPath == "" {
		return nil, errors.New("manifest path required")
	}
	manifestSnapshot := cfg.ManifestSnapshot
	var err error
	if manifestSnapshot == nil {
		manifestSnapshot, err = manifest.LoadAgentManifestSnapshot(cfg.ManifestPath)
		if err != nil {
			return nil, fmt.Errorf("load manifest: %w", err)
		}
	}
	agentManifest, err := manifest.CloneAgentManifest(manifestSnapshot.Manifest)
	if err != nil {
		return nil, fmt.Errorf("clone manifest: %w", err)
	}
	if agentManifest == nil {
		return nil, errors.New("manifest missing")
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
	runtime, err := selectSandboxRuntime(cfg, agentManifest)
	if err != nil {
		return nil, err
	}
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
	permissions.AttachRuntime(runtime)
	policy := buildSandboxPolicy(cfg.BaseFS, agentManifest)
	if err := runtime.ValidatePolicy(policy); err != nil {
		return nil, fmt.Errorf("sandbox policy validation failed: %w", err)
	}
	if err := runtime.ApplyPolicy(ctx, policy); err != nil {
		return nil, fmt.Errorf("sandbox policy application failed: %w", err)
	}
	return &AgentRegistration{
		ID:               agentManifest.Metadata.Name,
		Manifest:         agentManifest,
		ManifestSnapshot: manifestSnapshot,
		Runtime:          runtime,
		Permissions:      permissions,
		Audit:            audit,
		HITL:             hitl,
	}, nil
}

func selectSandboxRuntime(cfg RuntimeConfig, agentManifest *manifest.AgentManifest) (sandbox.SandboxRuntime, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Backend)) {
	case "", "gvisor":
		return sandbox.NewSandboxRuntime(cfg.Sandbox), nil
	case "docker":
		image := cfg.Image
		if image == "" && agentManifest != nil {
			image = agentManifest.Spec.Image
		}
		return dockersandbox.NewBackend(dockersandbox.Config{
			DockerPath: cfg.Sandbox.ContainerRuntime,
			Image:      image,
			Workspace:  cfg.BaseFS,
		}), nil
	default:
		return nil, fmt.Errorf("unsupported sandbox backend %q", cfg.Backend)
	}
}

func buildSandboxPolicy(baseFS string, agentManifest *manifest.AgentManifest) sandbox.SandboxPolicy {
	policy := sandbox.SandboxPolicy{}
	if agentManifest == nil {
		return policy
	}
	policy.NetworkRules = buildNetworkPolicy(agentManifest.Spec.Permissions.Network)
	policy.ReadOnlyRoot = agentManifest.Spec.Security.ReadOnlyRoot
	policy.NoNewPrivileges = agentManifest.Spec.Security.NoNewPrivileges
	policy.ProtectedPaths = config.New(baseFS).GovernanceRoots()
	return policy
}

// buildNetworkPolicy converts network permissions into sandbox-friendly rules
// so the selected backend enforces the same view of allowed hosts/ports as the
// permission manager.
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

// Execute enforces permissions prior to delegating to the runtime executor.
func (r *AgentRegistration) Execute(ctx context.Context, agent graph.WorkflowExecutor, task *core.Task, state *core.Context) (*core.Result, error) {
	if agent == nil {
		return nil, errors.New("agent missing")
	}
	if r == nil || r.Permissions == nil {
		return nil, errors.New("permission subsystem missing")
	}
	if err := agent.Initialize(&core.Config{Name: r.ID, NativeToolCalling: true}); err != nil {
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
