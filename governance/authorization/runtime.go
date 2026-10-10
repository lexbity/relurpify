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
	ucperms "codeburg.org/lexbit/relurpify/userconfig/permissions"
)

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
	Permissions        ucperms.PermissionSet
	DefaultPermissions *ucperms.PermissionSet
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
	AuditEnforcement   string // "strict" (default) | "best_effort"
	BaseFS             string
	StateDir           string
	HITLTimeout        time.Duration
	// WorkspaceID and AgentName identify the agent for audit attribution. The
	// registration ID is derived deterministically from them.
	WorkspaceID string
	AgentName   string
}

// AgentRegistration stores runtime metadata. DocumentSnapshot and AgentSpec are
// opaque carriers (see RuntimeConfig).
type AgentRegistration struct {
	ID                string
	DocumentSnapshot  any
	AgentSpec         any
	Permissions       *PermissionManager
	PermissionSet     ucperms.PermissionSet
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

	agentID := generateAgentID(cfg.WorkspaceID, cfg.AgentName)
	stateDir := cfg.StateDir
	if strings.TrimSpace(stateDir) == "" && strings.TrimSpace(cfg.BaseFS) != "" {
		stateDir = filepath.Join(cfg.BaseFS, runtimeStateDirName)
	}
	if strings.TrimSpace(stateDir) == "" {
		return nil, errors.New("audit chain requires a state dir (StateDir or BaseFS)")
	}

	// The audit chain is the durable, tamper-evident trail. Registration fails
	// closed when the chain cannot be initialized: an audit-less runtime must
	// not silently register (SBH-1 D-10).
	bestEffort, enforcementErr := policy.ParseAuditEnforcement(cfg.AuditEnforcement)
	if enforcementErr != nil {
		return nil, fmt.Errorf("audit.enforcement: %w", enforcementErr)
	}
	queueSize := cfg.AuditLimit
	if queueSize < 128 {
		queueSize = 128
	}
	chainLog, auditErr := policy.NewFileChainAuditLogger(
		filepath.Join(stateDir, "audit", agentID),
		policy.FileChainOptions{QueueSize: queueSize, BestEffort: bestEffort},
	)
	if auditErr != nil {
		return nil, fmt.Errorf("audit chain init: %w", auditErr)
	}
	audit := policy.AuditLogger(chainLog)
	closeChain := func() { _ = chainLog.Close() }

	var err error
	effectivePerms := permissions.ResolveEffective(cfg.DefaultPermissions, &cfg.Permissions)
	image := strings.TrimSpace(cfg.Image)
	runtime, err := selectSandboxRuntime(ctx, cfg.Backend, cfg.SandboxCfg, image, cfg.BaseFS, cfg.BackendFactory)
	if err != nil {
		closeChain()
		return nil, err
	}
	if err := runtime.Verify(ctx); err != nil {
		closeChain()
		return nil, fmt.Errorf("sandbox verification failed: %w", err)
	}
	hitl := NewHITLBroker(cfg.HITLTimeout, nil)
	var permManager *PermissionManager
	if len(effectivePerms.FileSystem) > 0 ||
		len(effectivePerms.Executables) > 0 ||
		len(effectivePerms.Network) > 0 ||
		len(effectivePerms.Capabilities) > 0 ||
		len(effectivePerms.IPC) > 0 {
		permManager, err = NewPermissionManager(cfg.BaseFS, &effectivePerms, audit, hitl)
		if err != nil {
			closeChain()
			return nil, fmt.Errorf("permission manager init: %w", err)
		}
	}
	if permManager != nil {
		permManager.SetFilesystemGuardRoots(
			[]string{
				filepath.Join(cfg.BaseFS, "relurpify_cfg"),
				filepath.Join(cfg.BaseFS, ".git"),
			},
			[]string{stateDir},
		)
		if strings.TrimSpace(cfg.DefaultToolPolicy) != "" {
			decision, err := permissions.ParseDecision(cfg.DefaultToolPolicy)
			if err != nil {
				closeChain()
				return nil, fmt.Errorf("agent spec default_policy: %w", err)
			}
			if err := permManager.SetDefaultDecision(decision); err != nil {
				closeChain()
				return nil, fmt.Errorf("agent spec default_policy: %w", err)
			}
		}
		permManager.AttachRuntime(ctx, runtime)
	}
	sboxPolicy := buildSandboxPolicy(cfg.Permissions, cfg.Security, cfg.ProtectedPaths)
	if err := runtime.ValidatePolicy(sboxPolicy); err != nil {
		closeChain()
		return nil, fmt.Errorf("sandbox policy validation failed: %w", err)
	}
	if err := runtime.ApplyPolicy(ctx, sboxPolicy); err != nil {
		closeChain()
		return nil, fmt.Errorf("sandbox policy application failed: %w", err)
	}
	return &AgentRegistration{
		ID:                agentID,
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

// generateAgentID derives a stable, deterministic agent ID from the workspace
// and agent name for audit attribution. Empty components fall back to
// "unknown"; each component is sanitized to lowercase alphanumerics joined by
// single hyphens.
func generateAgentID(workspaceID, agentName string) string {
	return "agent-" + sanitizeIDPart(workspaceID) + "-" + sanitizeIDPart(agentName)
}

func sanitizeIDPart(s string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "unknown"
	}
	return out
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
func buildSandboxPolicy(perms ucperms.PermissionSet, security SandboxSecurity, protectedPaths []string) governanceports.SandboxPolicy {
	policy := governanceports.SandboxPolicy{
		ProtectedPaths: append([]string(nil), protectedPaths...),
	}
	policy.NetworkRules = buildNetworkPolicy(perms.Network)
	policy.ReadOnlyRoot = security.ReadOnlyRoot
	policy.NoNewPrivileges = security.NoNewPrivileges
	return policy
}

// buildNetworkPolicy converts network permissions into sandbox-friendly rules.
func buildNetworkPolicy(perms []ucperms.NetworkPermission) []governanceports.SandboxNetworkRule {
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
func (r *AgentRegistration) GrantPermission(desc ucperms.PermissionDescriptor, approvedBy string, scope policy.GrantScope, duration time.Duration) {
	if r == nil || r.Permissions == nil {
		return
	}
	grant := GrantManual(desc, approvedBy, scope, duration)
	r.Permissions.mu.Lock()
	defer r.Permissions.mu.Unlock()
	r.Permissions.putGrant(desc.Action+":"+desc.Resource, grant)
}
