package authorization

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	permissions "codeburg.org/lexbit/relurpify/governance/permissions"
	policy "codeburg.org/lexbit/relurpify/governance/policy"
	governanceports "codeburg.org/lexbit/relurpify/governance/ports"
	"codeburg.org/lexbit/relurpify/userconfig/config"
	"codeburg.org/lexbit/relurpify/userconfig/config/secretscan"
	cfgsecurity "codeburg.org/lexbit/relurpify/userconfig/config/security"
)

const defaultToolPolicyAllow = "allow"

// SandboxBackendFactory creates a governanceports.SandboxRuntime for the given backend.
type SandboxBackendFactory func(ctx context.Context, backend string, cfg governanceports.SandboxConfig, image, workspace string) (governanceports.SandboxRuntime, error)

// RuntimeConfig describes configuration for agent runtime registration.
type RuntimeConfig struct {
	ManifestPath       string
	DocumentSnapshot   *config.DocumentSnapshot
	AgentSpec          *agentspec.AgentRuntimeSpec
	Permissions        permissions.PermissionSet
	DefaultPermissions *permissions.PermissionSet
	Security           config.SecuritySpec
	Image              string
	Runtime            string
	DefaultToolPolicy  string
	SecurityBundle     *cfgsecurity.Bundle
	ConfigPath         string
	Backend            string
	SandboxCfg         governanceports.SandboxConfig
	BackendFactory     SandboxBackendFactory
	AuditLimit         int
	BaseFS             string
	StateDir           string
	HITLTimeout        time.Duration
}

// AgentRegistration stores runtime metadata.
type AgentRegistration struct {
	ID                string
	DocumentSnapshot  *config.DocumentSnapshot
	AgentSpec         *agentspec.AgentRuntimeSpec
	Permissions       *PermissionManager
	PermissionSet     permissions.PermissionSet
	Policy            PolicyEngine
	Audit             policy.AuditLogger
	HITL              *HITLBroker
	Runtime           governanceports.SandboxRuntime
	Image             string
	SandboxRuntime    string
	Security          config.SecuritySpec
	DefaultToolPolicy string
}

// RegisterAgent validates the manifest and builds enforcement primitives.
func RegisterAgent(ctx context.Context, cfg RuntimeConfig) (*AgentRegistration, error) {
	if cfg.DocumentSnapshot == nil && cfg.ManifestPath == "" {
		return nil, errors.New("manifest path required")
	}
	if cfg.SecurityBundle == nil {
		return nil, errors.New("security bundle required")
	}
	documentSnapshot := cfg.DocumentSnapshot
	var err error
	if documentSnapshot == nil {
		docSnapshot, docErr := config.LoadDocument(cfg.ManifestPath)
		if docErr != nil {
			return nil, fmt.Errorf("load document: %w", docErr)
		}
		documentSnapshot = docSnapshot
	}
	if documentSnapshot == nil || documentSnapshot.Document == nil {
		return nil, errors.New("document missing")
	}

	effectivePerms := permissions.ResolveEffective(cfg.DefaultPermissions, &cfg.Permissions)
	image := strings.TrimSpace(cfg.Image)
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
	if len(effectivePerms.FileSystem) > 0 ||
		len(effectivePerms.Executables) > 0 ||
		len(effectivePerms.Network) > 0 ||
		len(effectivePerms.Capabilities) > 0 ||
		len(effectivePerms.IPC) > 0 {
		permManager, err = NewPermissionManager(cfg.BaseFS, &effectivePerms, audit, hitl)
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
		if strings.TrimSpace(cfg.DefaultToolPolicy) != "" {
			if strings.ToLower(strings.TrimSpace(cfg.DefaultToolPolicy)) == defaultToolPolicyAllow {
				return nil, errors.New(
					"agent spec sets default_policy=allow which is not permitted; " +
						"use default_policy=ask for HITL or declare explicit permissions")
			}
			permManager.SetDefaultPolicy(cfg.DefaultToolPolicy)
		}
		permManager.AttachRuntime(ctx, runtime)
	}
	sboxPolicy := buildSandboxPolicy(cfg.Permissions, cfg.Security, cfg.SecurityBundle.Sandbox.ProtectedPaths)
	if err := runtime.ValidatePolicy(sboxPolicy); err != nil {
		return nil, fmt.Errorf("sandbox policy validation failed: %w", err)
	}
	if err := runtime.ApplyPolicy(ctx, sboxPolicy); err != nil {
		return nil, fmt.Errorf("sandbox policy application failed: %w", err)
	}
	return &AgentRegistration{
		ID:                "",
		DocumentSnapshot:  documentSnapshot,
		AgentSpec:         cfg.AgentSpec,
		Permissions:       permManager,
		PermissionSet:     cfg.Permissions,
		Audit:             audit,
		HITL:              hitl,
		Runtime:           runtime,
		Image:             image,
		SandboxRuntime:    cfg.Runtime,
		Security:          cfg.Security,
		DefaultToolPolicy: cfg.DefaultToolPolicy,
	}, nil
}

// selectSandboxRuntime returns a sandbox runtime using the provided factory.
func selectSandboxRuntime(ctx context.Context, backend string, sandboxCfg governanceports.SandboxConfig, image, workspace string, factory SandboxBackendFactory) (governanceports.SandboxRuntime, error) {
	if factory != nil {
		return factory(ctx, backend, sandboxCfg, image, workspace)
	}
	return nil, fmt.Errorf("no sandbox backend factory configured")
}

// buildSandboxPolicy constructs a sandbox policy from typed permissions and
// protected paths.
func buildSandboxPolicy(perms permissions.PermissionSet, security config.SecuritySpec, protectedPaths []string) governanceports.SandboxPolicy {
	policy := governanceports.SandboxPolicy{
		ProtectedPaths: append([]string(nil), protectedPaths...),
	}
	policy.NetworkRules = buildNetworkPolicy(perms.Network)
	policy.ReadOnlyRoot = security.ReadOnlyRoot
	policy.NoNewPrivileges = security.NoNewPrivileges
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
