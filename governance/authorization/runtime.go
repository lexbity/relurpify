package authorization

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	permissions "codeburg.org/lexbit/relurpify/governance/permissions"
	policy "codeburg.org/lexbit/relurpify/governance/policy"
	governanceports "codeburg.org/lexbit/relurpify/governance/ports"
)

const defaultToolPolicyAllow = "allow"

// runtimeStateDirName is the workspace-relative runtime state directory used as a
// filesystem-guard root fallback when the caller does not supply StateDir. Kept
// local to avoid importing userconfig (mirrors userconfig secretscan.RuntimeStateDirName).
const runtimeStateDirName = ".relurpify_state"

// SandboxBackendFactory creates a governanceports.SandboxRuntime for the given backend.
type SandboxBackendFactory func(ctx context.Context, backend string, cfg governanceports.SandboxConfig, image, workspace string) (governanceports.SandboxRuntime, error)

// SandboxSecurity is the governance-local projection of the security settings a
// sandbox needs. It decouples governance from userconfig.SecuritySpec; the
// composition root maps the config type into this value.
type SandboxSecurity struct {
	RunAsUser       int
	ReadOnlyRoot    bool
	NoNewPrivileges bool
}

// RuntimeConfig describes configuration for agent runtime registration.
//
// DocumentSnapshot and AgentSpec are opaque carriers (consumer-defined-interface
// inversion): governance does not read their internals, it only stores and
// forwards them. The composition root supplies concrete userconfig types and the
// config-aware consumers (probe, runtime) type-assert them back. This keeps
// governance free of any userconfig/config import (breaking the
// governance↔userconfig domain cycle). It mirrors the existing
// CompiledPolicyBundle.Spec any pattern in this package.
type RuntimeConfig struct {
	DocumentSnapshot   any
	AgentSpec          any
	Permissions        permissions.PermissionSet
	DefaultPermissions *permissions.PermissionSet
	Security           SandboxSecurity
	ProtectedPaths     []string
	Image              string
	Runtime            string
	DefaultToolPolicy  string
	ConfigPath         string
	Backend            string
	SandboxCfg         governanceports.SandboxConfig
	BackendFactory     SandboxBackendFactory
	AuditLimit         int
	BaseFS             string
	StateDir           string
	HITLTimeout        time.Duration
}

// AgentRegistration stores runtime metadata. DocumentSnapshot and AgentSpec are
// opaque carriers (see RuntimeConfig).
type AgentRegistration struct {
	ID                string
	DocumentSnapshot  any
	AgentSpec         any
	Permissions       *PermissionManager
	PermissionSet     permissions.PermissionSet
	Policy            PolicyEngine
	Audit             policy.AuditLogger
	HITL              *HITLBroker
	Runtime           governanceports.SandboxRuntime
	Image             string
	SandboxRuntime    string
	Security          SandboxSecurity
	DefaultToolPolicy string
}

// RegisterAgent validates the manifest and builds enforcement primitives.
func RegisterAgent(ctx context.Context, cfg RuntimeConfig) (*AgentRegistration, error) {
	if cfg.DocumentSnapshot == nil {
		return nil, errors.New("document snapshot required")
	}

	var err error
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
			stateDir = filepath.Join(cfg.BaseFS, runtimeStateDirName)
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
	sboxPolicy := buildSandboxPolicy(cfg.Permissions, cfg.Security, cfg.ProtectedPaths)
	if err := runtime.ValidatePolicy(sboxPolicy); err != nil {
		return nil, fmt.Errorf("sandbox policy validation failed: %w", err)
	}
	if err := runtime.ApplyPolicy(ctx, sboxPolicy); err != nil {
		return nil, fmt.Errorf("sandbox policy application failed: %w", err)
	}
	return &AgentRegistration{
		ID:                "",
		DocumentSnapshot:  cfg.DocumentSnapshot,
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
func buildSandboxPolicy(perms permissions.PermissionSet, security SandboxSecurity, protectedPaths []string) governanceports.SandboxPolicy {
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
