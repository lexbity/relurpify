package authorization

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/cfgload"
	cfgsecurity "codeburg.org/lexbit/relurpify/framework/cfgload/security"
	"codeburg.org/lexbit/relurpify/framework/cfgload/secretscan"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/sandbox"
	"codeburg.org/lexbit/relurpify/platform/contracts"
	"codeburg.org/lexbit/relurpify/platform/sandbox/dockersandbox"
)

// RuntimeConfig describes configuration for agent runtime registration.
type RuntimeConfig struct {
	ManifestPath     string
	ManifestSnapshot *cfgload.AgentManifestSnapshot
	SecurityBundle   *cfgsecurity.Bundle
	ConfigPath       string
	Image            string
	Backend          string
	Sandbox          sandbox.SandboxConfig
	AuditLimit       int
	BaseFS           string
	StateDir         string
	HITLTimeout      time.Duration
}

// AgentRegistration stores runtime metadata.
type AgentRegistration struct {
	ID               string
	Manifest         *cfgload.AgentManifest
	ManifestSnapshot *cfgload.AgentManifestSnapshot
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
	if cfg.SecurityBundle == nil {
		return nil, errors.New("security bundle required")
	}
	manifestSnapshot := cfg.ManifestSnapshot
	var err error
	if manifestSnapshot == nil {
		manifestSnapshot, err = cfgload.LoadAgentManifestSnapshot(cfg.ManifestPath)
		if err != nil {
			return nil, fmt.Errorf("load manifest: %w", err)
		}
	}
	agentManifest, err := cfgload.CloneAgentManifest(manifestSnapshot.Manifest)
	if err != nil {
		return nil, fmt.Errorf("clone manifest: %w", err)
	}
	if agentManifest == nil {
		return nil, errors.New("manifest missing")
	}
	effectivePerms, err := cfgload.ResolveEffectivePermissions(cfg.BaseFS, agentManifest)
	if err != nil {
		return nil, fmt.Errorf("resolve permissions: %w", err)
	}
	effectiveResources, err := cfgload.ResolveEffectiveResources(cfg.BaseFS, agentManifest)
	if err != nil {
		return nil, fmt.Errorf("resolve resources: %w", err)
	}
	agentManifest.Spec.Permissions = effectivePerms
	agentManifest.Spec.Resources = effectiveResources
	image := cfg.Image
	if image == "" && agentManifest != nil {
		image = agentManifest.Spec.Image
	}
	runtime, err := SelectSandboxRuntime(cfg.Backend, cfg.Sandbox, image, cfg.BaseFS)
	if err != nil {
		return nil, err
	}
	if err := runtime.Verify(ctx); err != nil {
		return nil, fmt.Errorf("sandbox verification failed: %w", err)
	}
	hitl := NewHITLBroker(cfg.HITLTimeout)
	audit := core.NewInMemoryAuditLogger(cfg.AuditLimit)
	var permissions *PermissionManager
	if len(agentManifest.Spec.Permissions.FileSystem) > 0 ||
		len(agentManifest.Spec.Permissions.Executables) > 0 ||
		len(agentManifest.Spec.Permissions.Network) > 0 ||
		len(agentManifest.Spec.Permissions.Capabilities) > 0 ||
		len(agentManifest.Spec.Permissions.IPC) > 0 {
		permissions, err = NewPermissionManager(cfg.BaseFS, &agentManifest.Spec.Permissions, audit, hitl)
		if err != nil {
			return nil, fmt.Errorf("permission manager init: %w", err)
		}
	}
	if permissions != nil {
		stateDir := cfg.StateDir
		if strings.TrimSpace(stateDir) == "" && strings.TrimSpace(cfg.BaseFS) != "" {
			stateDir = filepath.Join(cfg.BaseFS, secretscan.RuntimeStateDirName)
		}
		permissions.SetFilesystemGuardRoots(
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
				permissions.SetDefaultPolicy(policy)
			}
		}
		permissions.AttachRuntime(runtime)
	}
	policy := BuildSandboxPolicy(agentManifest, cfg.SecurityBundle.Sandbox.ProtectedPaths)
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

// SelectSandboxRuntime returns a sandbox runtime for the given backend.
// Supported backends: "" (defaults to gvisor), "gvisor", "docker".
// Unsupported backends ("local", "none", or any unknown value) return an error.
// This is the central, policy-resolved chokepoint for obtaining a sandbox runtime.
// The supported-backend vocabulary is defined by sandbox.SupportedSandboxBackends;
// cfgload validation derives from the same source.
func SelectSandboxRuntime(backend string, sandboxCfg sandbox.SandboxConfig, image, workspace string) (sandbox.SandboxRuntime, error) {
	b := strings.ToLower(strings.TrimSpace(backend))
	if b == "" {
		b = "gvisor"
	}
	if !sandbox.IsSupportedSandboxBackend(b) {
		supported := strings.Join(sandbox.SupportedSandboxBackends(), ", ")
		return nil, fmt.Errorf("unsupported sandbox backend %q (supported: %s)", backend, supported)
	}
	switch b {
	case "gvisor":
		return sandbox.NewSandboxRuntime(sandboxCfg), nil
	case "docker":
		return dockersandbox.NewBackend(dockersandbox.Config{
			DockerPath: sandboxCfg.ContainerRuntime,
			Image:      image,
			Workspace:  workspace,
		}), nil
	default:
		return nil, fmt.Errorf("unreachable: unsupported sandbox backend %q", b)
	}
}

// BuildSandboxPolicy constructs a sandbox policy from an agent manifest and
// protected paths. Protected paths are always included; manifest-derived
// rules (network, read-only, no-new-privileges) are added when the manifest
// is non-nil.
func BuildSandboxPolicy(agentManifest *cfgload.AgentManifest, protectedPaths []string) sandbox.SandboxPolicy {
	policy := sandbox.SandboxPolicy{
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

// buildNetworkPolicy converts network permissions into sandbox-friendly rules
// so the selected backend enforces the same view of allowed hosts/ports as the
// permission manager.
func buildNetworkPolicy(perms []contracts.NetworkPermission) []sandbox.NetworkRule {
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
func (r *AgentRegistration) Execute(ctx context.Context, agent core.AgentExecutor, task *core.Task, state *contextdata.Envelope) (*core.Result, error) {
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
func (r *AgentRegistration) GrantPermission(desc contracts.PermissionDescriptor, approvedBy string, scope GrantScope, duration time.Duration) {
	if r == nil || r.Permissions == nil {
		return
	}
	grant := GrantManual(desc, approvedBy, scope, duration)
	r.Permissions.mu.Lock()
	defer r.Permissions.mu.Unlock()
	r.Permissions.grants[desc.Action+":"+desc.Resource] = grant
}
