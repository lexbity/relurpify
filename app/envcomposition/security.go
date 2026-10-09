package envcomposition

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/sandbox"
	fauthorization "codeburg.org/lexbit/relurpify/governance/authorization"
	"codeburg.org/lexbit/relurpify/governance/permissions"
	"codeburg.org/lexbit/relurpify/telemetry"
	"codeburg.org/lexbit/relurpify/userconfig/config"
	cfgsecurity "codeburg.org/lexbit/relurpify/userconfig/config/security"
)

// SecurityRuntimeInput carries the parameters for BuildSecurityRuntime.
// When ExistingRunner is set it is used directly; otherwise a new runner is
// built from SandboxBackend + SecurityBundle + the resolved image/runtime/security inputs.
type SecurityRuntimeInput struct {
	Context           context.Context
	Workspace         string
	SandboxBackend    string
	Runtime           string
	Image             string
	AgentID           string
	AgentSpec         *agentspec.AgentRuntimeSpec
	PermissionManager *fauthorization.PermissionManager
	SecurityBundle    *cfgsecurity.Bundle
	Security          config.SecuritySpec
	Permissions       permissions.PermissionSet
	ExistingRunner    sandbox.CommandRunner
	Strict            bool
	// Events receives sandbox posture telemetry (ceiling exceed, protected-path
	// escape, image pin status). Nil is a silent no-op.
	Events telemetry.Telemetry
}

// SecurityRuntime bundles the result of the non-bypassable security foundation.
// It is the only way to obtain an AuthorizedRunner + PolicyEngine pair.
type SecurityRuntime struct {
	Runner        *sandbox.AuthorizedRunner
	PolicyEngine  fauthorization.PolicyEngine
	CommandPolicy sandbox.CommandPolicy
	Permissions   *fauthorization.PermissionManager
	RunnerConfig  *sandbox.CommandRunnerConfig
}

// BuildSecurityRuntime is the single non-bypassable security foundation.
// It turns (ctx, resolved config, permission manager, agent spec) into an
// *AuthorizedRunner + PolicyEngine — the invariant chain that every entry
// point must traverse.
//
// Invariant chain:
//  1. Select sandbox runtime (fail-closed on unsupported backend)
//  2. Verify the sandbox (fail-closed)
//  3. Build verified runner with resolved CommandRunnerConfig
//  4. Resolve CommandAuthorizationPolicy (default-deny if no PermissionManager)
//  5. NewAuthorizedRunner(verified, policy)
//  6. Compile PolicyEngine from agent spec
func BuildSecurityRuntime(ctx context.Context, in SecurityRuntimeInput) (*SecurityRuntime, error) {
	if in.Context == nil {
		in.Context = context.Background()
	}

	// Step 0: Boot invariants — reject inconsistent configurations before
	// any sandbox resources are allocated.
	if err := ValidateSecurityRuntimeInput(in); err != nil {
		return nil, fmt.Errorf("boot invariant violation: %w", err)
	}

	// Step 0.5: Orphan sweep — reclaim containers orphaned by a crashed
	// previous session before this session allocates any. Bounded to 5s; the
	// sweep report is logged, never fatal (availability over purity).
	if in.SecurityBundle != nil && in.SecurityBundle.Sandbox != nil && in.SecurityBundle.Sandbox.ReapOrphans {
		reapCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		report, reapErr := sandbox.ReapOrphans(reapCtx, sandbox.ReapOptions{
			MaxAge: in.SecurityBundle.Sandbox.OrphanMaxAge,
		})
		cancel()
		if reapErr != nil {
			log.Printf("sandbox: orphan sweep failed: %v", reapErr)
		} else if report.Reaped > 0 {
			log.Printf("sandbox: reaped %d orphaned container(s): %v", report.Reaped, report.ReapedNames)
		}
	}

	// Steps 1–3: Select sandbox, verify, build runner.
	var runner sandbox.CommandRunner
	var runnerConfig *sandbox.CommandRunnerConfig
	var err error
	if in.ExistingRunner != nil {
		runner = in.ExistingRunner
	} else {
		runner, runnerConfig, err = buildRunnerImpl(in)
		if err != nil {
			return nil, err
		}
	}

	// Steps 4–5: Resolve policy and wrap in AuthorizedRunner.
	var permManager *fauthorization.PermissionManager
	if in.PermissionManager != nil {
		permManager = in.PermissionManager
	}
	var cmdPolicy sandbox.CommandPolicy
	if permManager != nil {
		// The terminal bash default is ask: an absent agent spec or an unset
		// default never means "allow". An explicit default: allow still works.
		bashCfg := &fauthorization.BashConfig{Default: permissions.DecisionAsk}
		if in.AgentSpec != nil {
			bashCfg.AllowPatterns = in.AgentSpec.Bash.AllowPatterns
			bashCfg.DenyPatterns = in.AgentSpec.Bash.DenyPatterns
			decision, err := permissions.DecisionOr(string(in.AgentSpec.Bash.Default), permissions.DecisionAsk)
			if err != nil {
				return nil, fmt.Errorf("agent bash default: %w", err)
			}
			bashCfg.Default = decision
		}
		authPolicy := fauthorization.NewCommandAuthorizationPolicy(permManager, in.AgentID, bashCfg, "sandbox")
		cmdPolicy = sandbox.CommandPolicyFunc(func(_ context.Context, req sandbox.CommandRequest) error {
			return authPolicy.CheckCommand(ctx, req.Args, req.Env)
		})
	} else {
		cmdPolicy = defaultDenyPolicy()
	}
	authRunner, err := sandbox.NewAuthorizedRunner(runner, cmdPolicy)
	if err != nil {
		return nil, fmt.Errorf("authorize runner: %w", err)
	}

	// Step 6: Compile PolicyEngine.
	policyEngine, err := fauthorization.FromAgentSpecWithConfig(in.AgentSpec, in.AgentID, permManager)
	if err != nil {
		return nil, fmt.Errorf("compile policy engine: %w", err)
	}

	return &SecurityRuntime{
		Runner:        authRunner,
		PolicyEngine:  policyEngine,
		CommandPolicy: cmdPolicy,
		Permissions:   permManager,
		RunnerConfig:  runnerConfig,
	}, nil
}

// buildRunnerConfig constructs a CommandRunnerConfig from manifest-derived
// hardening fields plus the sandbox bundle. Returns a minimal config with just
// Workspace when the bundle is nil.
func buildRunnerConfig(workspace string, image string, security config.SecuritySpec, bundle *cfgsecurity.SandboxPolicy, events telemetry.Telemetry) *sandbox.CommandRunnerConfig {
	cfg := &sandbox.CommandRunnerConfig{
		Workspace: workspace,
	}
	cfg.Image = image
	cfg.RunAsUser = security.RunAsUser
	cfg.ReadOnlyRoot = security.ReadOnlyRoot
	cfg.NoNewPrivileges = security.NoNewPrivileges
	if bundle != nil {
		cfg.Events = events
		// D-12: ceiling-truncated output spills under the workspace state dir.
		cfg.SpillDir = filepath.Join(config.DefaultWorkspaceStateDir(workspace), "spill")
	}
	return cfg
}

// buildRunnerImpl builds a verified runner from scratch using the
// input struct's SandboxBackend, SecurityBundle, and Manifest fields.
func buildRunnerImpl(in SecurityRuntimeInput) (sandbox.CommandRunner, *sandbox.CommandRunnerConfig, error) {
	if in.SecurityBundle == nil {
		return nil, nil, fmt.Errorf("security bundle required to build sandbox runner")
	}
	sboxRuntime, err := sandbox.NewSandboxRuntimeForBackend(in.SandboxBackend, sandbox.SandboxConfig{}, "", in.Workspace)
	if err != nil {
		return nil, nil, fmt.Errorf("select sandbox runtime: %w", err)
	}
	runnerConfig := buildRunnerConfig(in.Workspace, in.Image, in.Security, in.SecurityBundle.Sandbox, in.Events)
	// D-13: resolve the runtime image to a digest at boot (5s bound, never
	// pulls). The resolved ref stays on the runner config; an unpinned boot is
	// loud via the pin event + doctor image-pin status.
	runnerConfig.Image = resolveRunnerImage(in.Context, in.Image, in.SecurityBundle.Sandbox, in.Events)
	sboxPolicy := newSandboxPolicy(in.Security, in.SecurityBundle.Sandbox.ProtectedPaths)
	runner, err := sandbox.NewVerifiedCommandRunner(in.Context, sboxRuntime, sboxPolicy, runnerConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("build verified runner: %w", err)
	}
	return runner, runnerConfig, nil
}

// resolveRunnerImage applies digest pinning to the configured runtime image
// and surfaces sandbox.image_pinned / sandbox.image_unpinned.
func resolveRunnerImage(ctx context.Context, configuredImage string, bundle *cfgsecurity.SandboxPolicy, events telemetry.Telemetry) string {
	ref := configuredImage
	if strings.TrimSpace(ref) == "" {
		ref = sandbox.DefaultRuntimeImage
	}
	digest := ""
	if bundle != nil {
		digest = bundle.ImageDigest
	}
	pin := sandbox.ResolveImageRef(ctx, "docker", ref, digest)
	sandbox.EmitImagePinEvent(ctx, events, pin)
	return pin.Ref
}

// defaultDenyPolicy returns a CommandPolicy that denies all tool execution.
// Used when no PermissionManager is supplied, for example during bootstrap
// before the runtime installs a concrete policy manager.
func defaultDenyPolicy() sandbox.CommandPolicy {
	return sandbox.CommandPolicyFunc(func(_ context.Context, req sandbox.CommandRequest) error {
		return &sandbox.ExecutionDeniedError{
			Command: strings.Join(req.Args, " "),
			Reason:  "no authorization policy configured — default-deny",
			Policy:  "default-deny",
		}
	})
}

// newSandboxPolicy constructs a sandbox policy from a manifest spec.
func newSandboxPolicy(spec config.SecuritySpec, protectedPaths []string) sandbox.SandboxPolicy {
	policy := sandbox.SandboxPolicy{
		ProtectedPaths: append([]string(nil), protectedPaths...),
	}
	policy.ReadOnlyRoot = spec.ReadOnlyRoot
	policy.NoNewPrivileges = spec.NoNewPrivileges
	return policy
}
