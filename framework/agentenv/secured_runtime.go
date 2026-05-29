package agentenv

import (
	"context"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/agentspec"
	fauthorization "codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/cfgload"
	cfgsecurity "codeburg.org/lexbit/relurpify/framework/cfgload/security"
	"codeburg.org/lexbit/relurpify/framework/sandbox"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// SecuredRuntimeInput carries the parameters for buildSecuredRuntime.
// When ExistingRunner is set it is used directly; otherwise a new runner is
// built from SandboxBackend + SecurityBundle + Manifest.
type SecuredRuntimeInput struct {
	Context          context.Context
	Workspace        string
	SandboxBackend   string
	AgentID          string
	AgentSpec        *agentspec.AgentRuntimeSpec
	PermissionManager *fauthorization.PermissionManager
	SecurityBundle   *cfgsecurity.Bundle
	ExistingRunner   sandbox.CommandRunner
	Manifest         *cfgload.AgentManifest
	Strict           bool
}

// SecuredRuntime bundles the result of the non-bypassable security foundation.
// It is the only way to obtain an AuthorizedRunner + PolicyEngine pair.
type SecuredRuntime struct {
	Runner       *sandbox.AuthorizedRunner
	PolicyEngine fauthorization.PolicyEngine
	CommandPolicy sandbox.CommandPolicy
	Permissions  *fauthorization.PermissionManager
	RunnerConfig *contracts.CommandRunnerConfig
}

// buildSecuredRuntime is the single non-bypassable security foundation.
// It turns (ctx, resolved config, permission manager, agent spec) into an
// *AuthorizedRunner + PolicyEngine — the invariant chain that every entry
// point must traverse.
//
// Invariant chain:
//  1. Select sandbox runtime (fail-closed on unsupported backend)
//  2. Verify the sandbox (fail-closed)
//  3. Build verified runner with manifest-derived CommandRunnerConfig
//  4. Resolve CommandAuthorizationPolicy (default-deny if no PermissionManager)
//  5. NewAuthorizedRunner(verified, policy)
//  6. Compile PolicyEngine from agent spec
//
// buildRunnerForInput is a variable so tests can inject a fake runner
// without requiring a real sandbox backend (runsc/docker) on the host.
var buildRunnerForInput = buildRunnerForInputImpl

func buildSecuredRuntime(ctx context.Context, in SecuredRuntimeInput) (*SecuredRuntime, error) {
	if in.Context == nil {
		in.Context = context.Background()
	}

	// Step 0: Boot invariants — reject inconsistent configurations before
	// any sandbox resources are allocated.
	if err := validateBootInvariants(in); err != nil {
		return nil, fmt.Errorf("boot invariant violation: %w", err)
	}

	// Steps 1–3: Select sandbox, verify, build runner.
	var runner sandbox.CommandRunner
	var runnerConfig *contracts.CommandRunnerConfig
	var err error
	if in.ExistingRunner != nil {
		runner = in.ExistingRunner
	} else {
		runner, runnerConfig, err = buildRunnerForInput(in)
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
		cmdPolicy = fauthorization.NewCommandAuthorizationPolicy(permManager, in.AgentID, in.AgentSpec, "sandbox")
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

	return &SecuredRuntime{
		Runner:       authRunner,
		PolicyEngine: policyEngine,
		CommandPolicy: cmdPolicy,
		Permissions:  permManager,
		RunnerConfig: runnerConfig,
	}, nil
}

// buildRunnerConfig constructs a CommandRunnerConfig from manifest-derived
// hardening fields. Returns a minimal config with just Workspace when the
// manifest is nil.
func buildRunnerConfig(workspace string, manifest *cfgload.AgentManifest) *contracts.CommandRunnerConfig {
	cfg := &contracts.CommandRunnerConfig{
		Workspace: workspace,
	}
	if manifest != nil {
		cfg.Image = manifest.Spec.Image
		cfg.RunAsUser = manifest.Spec.Security.RunAsUser
		cfg.ReadOnlyRoot = manifest.Spec.Security.ReadOnlyRoot
		cfg.NoNewPrivileges = manifest.Spec.Security.NoNewPrivileges
	}
	return cfg
}

// buildRunnerForInputImpl builds a verified runner from scratch using the
// input struct's SandboxBackend, SecurityBundle, and Manifest fields.
func buildRunnerForInputImpl(in SecuredRuntimeInput) (sandbox.CommandRunner, *contracts.CommandRunnerConfig, error) {
	if in.SecurityBundle == nil {
		return nil, nil, fmt.Errorf("security bundle required to build sandbox runner")
	}
	sboxRuntime, err := fauthorization.SelectSandboxRuntime(in.SandboxBackend, sandbox.SandboxConfig{}, "", in.Workspace)
	if err != nil {
		return nil, nil, fmt.Errorf("select sandbox runtime: %w", err)
	}
	runnerConfig := buildRunnerConfig(in.Workspace, in.Manifest)
	sboxPolicy := fauthorization.BuildSandboxPolicy(in.Manifest, append([]string(nil), in.SecurityBundle.Sandbox.ProtectedPaths...))
	runner, err := sandbox.NewVerifiedCommandRunner(in.Context, sboxRuntime, sboxPolicy, runnerConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("build verified runner: %w", err)
	}
	return runner, runnerConfig, nil
}

// defaultDenyPolicy returns a CommandPolicy that denies all tool execution.
// Used when no PermissionManager is supplied (e.g. the nexus path until
// Phase 6 supplies a real one).
func defaultDenyPolicy() sandbox.CommandPolicy {
	return sandbox.CommandPolicyFunc(func(_ context.Context, req sandbox.CommandRequest) error {
		return &sandbox.ExecutionDeniedError{
			Command: strings.Join(req.Args, " "),
			Reason:  "no authorization policy configured — default-deny",
			Policy:  "default-deny",
		}
	})
}
