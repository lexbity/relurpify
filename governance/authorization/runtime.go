package authorization

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/governance/permissions"
	governanceports "codeburg.org/lexbit/relurpify/governance/ports"
	policy "codeburg.org/lexbit/relurpify/governance/policy"
	"codeburg.org/lexbit/relurpify/userconfig/config"
	"codeburg.org/lexbit/relurpify/userconfig/config/secretscan"
	cfgsecurity "codeburg.org/lexbit/relurpify/userconfig/config/security"
)

// SandboxBackendFactory creates a governanceports.SandboxRuntime for the given backend.
type SandboxBackendFactory func(ctx context.Context, backend string, cfg governanceports.SandboxConfig, image, workspace string) (governanceports.SandboxRuntime, error)

// RuntimeConfig describes configuration for agent runtime registration.
type RuntimeConfig struct {
	ManifestPath     string
	ManifestSnapshot *config.AgentManifestSnapshot
	SecurityBundle   *cfgsecurity.Bundle
	ConfigPath       string
	Image            string
	Backend          string
	SandboxCfg       governanceports.SandboxConfig
	BackendFactory   SandboxBackendFactory
	AuditLimit       int
	BaseFS           string
	StateDir         string
	HITLTimeout      time.Duration
}

// AgentRegistration stores runtime metadata.
type AgentRegistration struct {
	ID               string
	Manifest         *config.AgentManifest
	ManifestSnapshot *config.AgentManifestSnapshot
	Runtime          governanceports.SandboxRuntime
	Permissions      *PermissionManager
	Policy           PolicyEngine
	Audit            policy.AuditLogger
	HITL             *HITLBroker
}

// RegisterAgent validates the manifest and builds enforcement primitives.
func RegisterAgent(ctx context.Context, cfg RuntimeConfig) (*AgentRegistration, error) {
	if cfg.ManifestSnapshot == nil && cfg.ManifestPath == "" {
		return nil, errors.New("manifest path required")
	}
	if cfg.SecurityBundle == nil {
		return nil, errors.New("security bundle required")
	}
	manifestSnapshot := cfg.ManifestSnapshot
	var err error
	if manifestSnapshot == nil {
		manifestSnapshot, err = config.LoadAgentManifestSnapshot(cfg.ManifestPath)
		if err != nil {
			return nil, fmt.Errorf("load manifest: %w", err)
		}
	}
	agentManifest, err := config.CloneAgentManifest(manifestSnapshot.Manifest)
	if err != nil {
		return nil, fmt.Errorf("clone manifest: %w", err)
	}
	if agentManifest == nil {
		return nil, errors.New("manifest missing")
	}
	effectivePerms, err := config.ResolveEffectivePermissions(cfg.BaseFS, agentManifest)
	if err != nil {
		return nil, fmt.Errorf("resolve permissions: %w", err)
	}
	effectiveResources, err := config.ResolveEffectiveResources(cfg.BaseFS, agentManifest)
	if err != nil {
		return nil, fmt.Errorf("resolve resources: %w", err)
	}
	agentManifest.Spec.Permissions = effectivePerms
	agentManifest.Spec.Resources = effectiveResources
	image := cfg.Image
	if image == "" && agentManifest != nil {
		image = agentManifest.Spec.Image
	}
	runtime, err := selectSandboxRuntime(ctx, cfg.Backend, cfg.SandboxCfg, image, cfg.BaseFS, cfg.BackendFactory)
	if err != nil {
		return nil, err
	}
	if err := runtime.Verify(ctx); err != nil {
		return nil, fmt.Errorf("sandbox verification failed: %w", err)
	}
	hitl := NewHITLBroker(cfg.HITLTimeout)
	audit := policy.NewInMemoryAuditLogger(cfg.AuditLimit)
	var permManager *PermissionManager
	if len(agentManifest.Spec.Permissions.FileSystem) > 0 ||
		len(agentManifest.Spec.Permissions.Executables) > 0 ||
		len(agentManifest.Spec.Permissions.Network) > 0 ||
		len(agentManifest.Spec.Permissions.Capabilities) > 0 ||
		len(agentManifest.Spec.Permissions.IPC) > 0 {
		permManager, err = NewPermissionManager(cfg.BaseFS, &agentManifest.Spec.Permissions, audit, hitl)
		if err != nil {
			return nil, fmt.Errorf("permission manager init: %w", err)
		}
	}
	if permManager != nil {
		stateDir := cfg.StateDir
		if strings.TrimSpace(stateDir) == "" && strings.TrimSpace(cfg.BaseFS) != "" {
			stateDir = filepath.Join(cfg.BaseFS, secretscan.RuntimeStateDirName)
		}
		permManager.SetFilesystemGuardRoots(
			[]string{
				filepath.Join(cfg.BaseFS, "relurpify_cfg"),
				filepath.Join(cfg.BaseFS, ".git"),
			},
			[]string{stateDir},
		)
		if agentManifest.Spec.Policies != nil {
			if policy, ok := agentManifest.Spec.Policies["default_tool_policy"]; ok {
				if string(policy) == "allow" {
					return nil, errors.New(
						"agent spec sets default_policy=allow which is not permitted; " +
							"use default_policy=ask for HITL or declare explicit permissions")
				}
				permManager.SetDefaultPolicy(string(policy))
			}
		}
		permManager.AttachRuntime(runtime)
	}
	sboxPolicy := buildSandboxPolicy(agentManifest, cfg.SecurityBundle.Sandbox.ProtectedPaths)
	if err := runtime.ValidatePolicy(sboxPolicy); err != nil {
		return nil, fmt.Errorf("sandbox policy validation failed: %w", err)
	}
	if err := runtime.ApplyPolicy(ctx, sboxPolicy); err != nil {
		return nil, fmt.Errorf("sandbox policy application failed: %w", err)
	}
	return &AgentRegistration{
		ID:               agentManifest.Metadata.Name,
		Manifest:         agentManifest,
		ManifestSnapshot: manifestSnapshot,
		Runtime:          runtime,
		Permissions:      permManager,
		Audit:            audit,
		HITL:             hitl,
	}, nil
}

// selectSandboxRuntime returns a sandbox runtime using the provided factory.
func selectSandboxRuntime(ctx context.Context, backend string, sandboxCfg governanceports.SandboxConfig, image, workspace string, factory SandboxBackendFactory) (governanceports.SandboxRuntime, error) {
	if factory != nil {
		return factory(ctx, backend, sandboxCfg, image, workspace)
	}
	return nil, fmt.Errorf("no sandbox backend factory configured")
}

// buildSandboxPolicy constructs a sandbox policy from an agent manifest and
// protected paths.
func buildSandboxPolicy(agentManifest *config.AgentManifest, protectedPaths []string) governanceports.SandboxPolicy {
	policy := governanceports.SandboxPolicy{
		ProtectedPaths: append([]string(nil), protectedPaths...),
	}
	if agentManifest == nil {
		return policy
	}
	policy.NetworkRules = buildNetworkPolicy(agentManifest.Spec.Permissions.Network)
	policy.ReadOnlyRoot = agentManifest.Spec.Security.ReadOnlyRoot
	policy.NoNewPrivileges = agentManifest.Spec.Security.NoNewPrivileges
	return policy
}

// buildNetworkPolicy converts network permissions into sandbox-friendly rules.
func buildNetworkPolicy(perms []permissions.NetworkPermission) []governanceports.SandboxNetworkRule {
	var rules []governanceports.SandboxNetworkRule
	for _, perm := range perms {
		if perm.Direction != "egress" {
			continue
		}
		rules = append(rules, governanceports.SandboxNetworkRule{
			Direction: perm.Direction,
			Protocol:  perm.Protocol,
			Host:      perm.Host,
			Port:      perm.Port,
		})
	}
	return rules
}

// Execute enforces permissions prior to delegating to the runtime executor.
func (r *AgentRegistration) Execute(ctx context.Context, agent execution.AgentExecutor, task *execution.Task, state *contextdata.Envelope) (*execution.Result, error) {
	if agent == nil {
		return nil, errors.New("agent missing")
	}
	if r == nil || r.Permissions == nil {
		return nil, errors.New("permission subsystem missing")
	}
	if err := agent.Initialize(&execution.Config{Name: r.ID, NativeToolCalling: true}); err != nil {
		return nil, err
	}
	return agent.Execute(ctx, task, state)
}

// QueryAudit proxies queries to the audit store.
func (r *AgentRegistration) QueryAudit(ctx context.Context, filter policy.AuditQuery) ([]policy.AuditRecord, error) {
	if r == nil || r.Audit == nil {
		return nil, errors.New("audit logger missing")
	}
	return r.Audit.Query(ctx, filter)
}

// GrantPermission allows operators to programmatically approve scopes.
func (r *AgentRegistration) GrantPermission(desc permissions.PermissionDescriptor, approvedBy string, scope policy.GrantScope, duration time.Duration) {
	if r == nil || r.Permissions == nil {
		return
	}
	grant := GrantManual(desc, approvedBy, scope, duration)
	r.Permissions.mu.Lock()
	defer r.Permissions.mu.Unlock()
	r.Permissions.grants[desc.Action+":"+desc.Resource] = grant
}
