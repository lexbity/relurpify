# Sandbox Enforcement

## Purpose

The `framework/sandbox` package is the execution boundary for agent tool
invocations. It is responsible for turning manifest-driven security intent into
concrete process-launch rules and for refusing execution before a command ever
reaches the host shell, container runtime, or backend process.

`framework/authorization` decides whether an action is allowed, denied, or must
wait for human approval. `framework/sandbox` enforces the result at the point of
execution.

## Responsibilities

The sandbox layer owns:

- command execution backends and backend selection
- backend capability reporting
- sandbox policy validation and application
- execution-time refusal before process launch
- workspace-bound file scope helpers for host-side tools

It does not decide tool policy, capability admission, or HITL approvals. Those
reside in authorization and capability compilation.

## Enforcement Flow

The normal execution path is:

1. A tool, shell command, browser launcher, or helper requests execution.
2. `framework/authorization` resolves policy and permission state.
3. A `sandbox.SandboxRuntime` or backend-specific runner is selected.
4. `sandbox.SandboxPolicy` is validated against backend capabilities.
5. The active policy is applied to the backend.
6. `EnforcingCommandRunner` and `ShellGuard` refuse disallowed commands before
   the backend runner launches a process.
7. The backend executes the command with the enforced workspace, network, and
   filesystem constraints.

This separation matters. Authorization can deny or require approval, but the
sandbox is the layer that actually stops unsafe execution from starting.

## Package Contract

The key types in `framework/sandbox` are:

- `SandboxPolicy` - the backend-neutral security intent
- `Backend` - the policy contract implemented by each execution backend
- `SandboxRuntime` - the command-runner-facing extension of `Backend`
- `SandboxRuntimeImpl` - the default runsc-backed implementation
- `Capabilities` - what a backend can honestly enforce
- `CommandRunner` - the execution primitive used by tools and runtime launchers

Backends report capabilities, validate policy, and persist the active policy
before any command is launched.

## Backends

### Default runtime

The default runtime implementation is the runsc-backed sandbox runtime exposed
by `NewSandboxRuntime`. It is the baseline execution environment used when the
workspace or CLI does not request another backend.

### Docker backend

`platform/sandbox/dockersandbox` provides a Docker-backed implementation of the
same sandbox contract. It participates in the same policy model, but it can only
enforce the policy fields that Docker can actually express. Unsupported policy
fields fail closed during validation.

## Manifest Control

Sandbox behavior is controlled indirectly through the agent manifest and the
resolved workspace configuration.

### Manifest fields that affect sandbox execution

- `spec.runtime`
  - This is the runtime contract field required by the current manifest schema.
  - The code still validates `runtime: gvisor` as the canonical manifest value.
- `spec.image`
  - Supplies the container image used by sandbox-backed command runners.
- `spec.security.read_only_root`
  - Maps to read-only filesystem enforcement where the backend supports it.
- `spec.security.no_new_privileges`
  - Requests Linux `no-new-privileges` enforcement where supported.
- `spec.security.run_as_user`
  - Controls the user identity used for containerized execution where supported.
- `spec.permissions`
  - Declares executable, filesystem, and network permissions that are merged
    into the active sandbox policy and checked before execution.
- `spec.policy`
  - The policy wrapper form used by manifest loading and round-tripping.

### What the manifest does not control directly

- The manifest does not bypass sandbox validation.
- The manifest does not replace authorization decisions.
- The manifest does not allow unsupported backend features to be silently
  enabled.

If a backend cannot enforce a policy field, it must reject the configuration
instead of pretending to apply it.

## Relationship To Authorization

Authorization and sandboxing are complementary:

- authorization answers "may this action proceed?"
- sandboxing answers "how is the action executed, and can it be safely launched?"

Authorization compiles policy rules, handles HITL, and tracks permission grants.
Sandboxing enforces the resulting runtime boundaries:

- protected path mounts
- workspace confinement
- network isolation flags
- read-only root handling
- environment filtering where supported
- command refusal before spawn

## Operational Notes

- Policy validation happens before a backend stores policy state.
- Execution denial happens before the backend runner is invoked.
- Backend capabilities are explicit and should be checked before applying a new
  policy shape.
- The manifest runtime contract is still separate from backend selection. That
  schema can be generalized later without changing the enforcement model.

## Related Code

- [`framework/sandbox`](../../framework/sandbox)
- [`framework/authorization`](../../framework/authorization)
- [`framework/manifest`](../../framework/manifest)
- [`platform/sandbox/dockersandbox`](../../platform/sandbox/dockersandbox)
