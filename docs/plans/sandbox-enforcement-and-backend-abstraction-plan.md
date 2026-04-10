# Sandbox Enforcement and Backend Abstraction Plan

## Goals

This plan reworks sandboxing into a security boundary that enforces command
execution decisions inside `framework/sandbox`, with gVisor as the default and
reference backend. It also introduces a backend-neutral policy contract so
alternative implementations can be added under `platform/sandbox/`, starting
with `platform/sandbox/dockersandbox`.

Primary goals:

1. Sandbox should enforce execution blocking, not just expose a runner API.
2. The sandbox policy surface should be generalized so we can support multiple
   sandbox backends without encoding Docker-specific or gVisor-specific details
   into the shared contract.

Secondary goals:

- Preserve current gVisor behavior as the default runtime path.
- Make authorization decisions explicit and centrally enforced.
- Keep alternative implementations isolated from framework-level policy logic.

---

## Current State

The current design has the right pieces, but the enforcement boundary is split:

- `framework/authorization/command_authorization.go` decides whether a command
  is allowed.
- `framework/sandbox/command_runner.go` executes commands but does not itself
  reject commands based on authorization state.
- `framework/sandbox/shell_guard.go` adds blacklist/HITL behavior, but it is a
  wrapper, not the primary security boundary.
- `framework/authorization/runtime.go` always constructs
  `sandbox.NewGVisorRuntime(...)`.
- `framework/sandbox/sandbox.go` defines `SandboxRuntime`, but the interface is
  still gVisor-shaped and policy storage is mostly in-memory state.

Net effect: execution blocking is caller-dependent, and sandbox policy is not
yet a strict backend contract.

---

## Target Architecture

### Core principle

`framework/sandbox` owns execution enforcement. Authorization may decide, but
the sandbox layer must enforce.

### Layering

1. `framework/authorization`
   - Produces policy decisions.
   - Does not directly execute commands.
   - Can attach sandbox policy metadata, but does not own execution enforcement.

2. `framework/sandbox`
   - Defines the canonical sandbox policy contract.
   - Enforces allow/deny before launching a process.
   - Provides the gVisor default backend.
   - Provides enforcement wrappers so callers cannot bypass checks by accident.

3. `platform/sandbox/dockersandbox`
   - Implements the same sandbox policy contract using Docker-specific controls.
   - Lives outside framework code so backend-specific details do not leak upward.

### Reference semantics

gVisor is the reference backend. The shared policy interface should be shaped by
what gVisor can genuinely enforce. Docker implementations must conform to that
contract or explicitly reject unsupported policy features.

### Enforcement model

The sandbox boundary should support three distinct stages:

1. Policy evaluation
   - Determine whether a command, mount, environment, or network action is
     allowed, denied, or requires approval.

2. Policy application
   - Translate abstract policy into backend-specific controls.
   - Fail closed if a policy element cannot be represented safely.

3. Execution enforcement
   - Refuse to spawn the process if policy says deny.
   - Only delegate to the concrete runtime after enforcement succeeds.

---

## Proposed Interfaces

### Sandbox policy contract

`framework/sandbox` should own a backend-neutral policy model with explicit
capabilities and validation.

Suggested core concepts:

```go
type Backend interface {
    Name() string
    Verify(ctx context.Context) error
    Capabilities() Capabilities
    ValidatePolicy(policy Policy) error
    ApplyPolicy(ctx context.Context, policy Policy) error
    Run(ctx context.Context, req CommandRequest) (stdout, stderr string, err error)
}

type Capabilities struct {
    NetworkIsolation   bool
    ReadOnlyRoot       bool
    ProtectedPaths     bool
    NoNewPrivileges    bool
    Seccomp            bool
    UserMapping        bool
    PerCommandWorkdir  bool
    EnvFiltering       bool
}

type Policy struct {
    NetworkRules     []NetworkRule
    ReadOnlyRoot     bool
    ProtectedPaths   []string
    NoNewPrivileges  bool
    SeccompProfile   string
    AllowedEnvKeys   []string
    DeniedEnvKeys    []string
}
```

Notes:

- The interface should describe security intent, not backend flags.
- `ValidatePolicy` must reject unsafe combinations early.
- `ApplyPolicy` must be explicit so policy is not silently assumed.
- If a backend cannot support a policy property, it should fail closed unless a
  deliberate compatibility downgrade is documented and tested.

### Execution enforcement wrapper

Introduce an enforcement wrapper in `framework/sandbox` that owns the final
deny/allow check.

Responsibilities:

- Consult authorization or approved policy state before calling the backend.
- Deny execution when a command fails policy.
- Preserve error messages that identify the blocking policy/rule.
- Keep shell blacklist and HITL as separate, composable filters.

This wrapper should be the default entry point for command execution in the
framework.

### Backend policy translation

Each backend should translate the shared policy into its own runtime controls:

- gVisor backend:
  - Default implementation.
  - Uses runsc/container isolation as the security base.
  - Applies readonly root, network isolation, user isolation, seccomp, and
    protected mount behavior where supported.

- Docker backend:
  - Lives under `platform/sandbox/dockersandbox`.
  - Uses Docker controls to approximate or enforce the same policy contract.
  - Must fail closed on policy features it cannot safely support.

---

## Implementation Phases

### Phase 1: Re-establish sandbox as the enforcement boundary

#### Objective

Move execution blocking into `framework/sandbox` so the sandbox layer cannot be
bypassed by a caller that forgets to run authorization first.

#### Work

- Add a sandbox-side enforcement abstraction that can be composed around any
  `CommandRunner`.
- Make `ShellGuard` a policy wrapper, not the primary enforcement boundary.
- Add explicit pre-execution checks for denied commands before the backend
  process is launched.
- Ensure `CommandRunner` implementations cannot execute a request that has not
  passed sandbox enforcement.

#### Changes

- `framework/sandbox`
  - Add a backend-neutral enforcement interface.
  - Add a standard "denied execution" error type.
  - Add an enforcing runner wrapper.

- `framework/authorization`
  - Keep command authorization logic, but move the final execution gate out of
    authorization and into sandbox.
  - Preserve existing approval flows, but make them feed into sandbox policy
    state rather than directly into execution.

#### Tests

- Denied commands never reach the underlying runner.
- A runner invoked through the enforcing wrapper cannot bypass policy.
- HITL approval still works, but only after the sandbox enforcement layer
  records the approved state.
- Shell blacklist behavior remains intact.

#### Exit criteria

- All execution paths go through sandbox enforcement.
- There is no direct "authorize then execute" pattern outside the sandbox layer.

---

### Phase 2: Define the backend-neutral sandbox policy contract

#### Objective

Replace the gVisor-shaped runtime interface with a policy contract that gVisor
can define, but not monopolize.

#### Work

- Redesign `framework/sandbox` interfaces to separate:
  - backend verification
  - policy validation
  - policy application
  - command execution
- Make capabilities explicit so callers can reason about what the backend can
  truly enforce.
- Define which policy fields are universal and which are optional.

#### Design rules

- The interface must not mention `runsc` or Docker.
- The policy must not require transport-specific concepts.
- Unknown policy capabilities must be rejected explicitly.
- The interface should be stable enough for other sandbox backends.

#### Tests

- Capability reporting matches each backend's supported features.
- Validation rejects unsupported policy combinations.
- Policy round-trips preserve the effective sandbox state.

#### Exit criteria

- `framework/sandbox` owns the policy vocabulary.
- gVisor remains the default implementation, but no longer dictates the
  concrete runtime API shape.

---

### Phase 3: Move gVisor backend to the new contract

#### Objective

Refit the existing gVisor implementation to the new sandbox contract without
changing default behavior.

#### Work

- Keep gVisor as the default backend in `framework/sandbox`.
- Convert the current runtime verification and policy storage logic into the new
  backend interface.
- Preserve existing runtime defaults:
  - `runsc`
  - `kvm`
  - `docker` or `containerd` as the selected container runtime
  - network isolation enabled by default
- Make `ApplyPolicy` enforce or validate policy instead of passively storing it.

#### Enforcement expectations

- Read-only root should be an enforced policy, not just an annotation.
- Network rules should affect runtime launch behavior in a deterministic way.
- Protected paths should be included only if they can be safely mounted or
  otherwise enforced.
- Unsupported policy properties should produce clear errors.

#### Tests

- Default runtime selection still works.
- `Verify` still checks the expected binaries and runtime availability.
- `ApplyPolicy` updates the effective policy state.
- Unsupported policy inputs fail closed.

#### Exit criteria

- Existing gVisor behavior remains the default path.
- gVisor is now a concrete implementation of the new policy contract.

---

### Phase 4: Add Docker backend under `platform/sandbox/dockersandbox`

#### Objective

Introduce the first alternative sandbox implementation without polluting the
framework layer with Docker-specific details.

#### Scope

Create a new package:

- `platform/sandbox/dockersandbox`

Responsibilities:

- Implement the shared sandbox backend contract.
- Translate policy into Docker controls.
- Advertise capabilities honestly.
- Reject unsupported features explicitly.

#### Implementation notes

- Keep the package independent of gVisor internals.
- Use Docker-specific configuration and runtime invocation only inside this
  package.
- If a policy field cannot be expressed safely in Docker, return a validation
  error rather than silently ignoring it.

#### Tests

- Docker backend verifies required binaries or runtime access.
- Docker backend advertises only supported capabilities.
- Docker backend rejects unsupported policy properties.
- Docker backend can execute a command under the shared contract.

#### Exit criteria

- `platform/sandbox/dockersandbox` exists and compiles.
- The backend can be selected without changing framework policy code.

---

### Phase 5: Rewire runtime selection and agent bootstrap

#### Objective

Make backend selection explicit while preserving gVisor as the default.

#### Work

- Replace hardcoded `NewGVisorRuntime` construction in the registration flow
  with a backend factory or selector.
- Default to gVisor when no backend is specified.
- Allow runtime selection by configuration for Docker-based sandboxes.
- Ensure policy application happens through the selected backend.

#### Suggested wiring pattern

- `framework/sandbox` exposes the backend contract.
- `authorization/runtime.go` or a nearby bootstrap layer resolves a configured
  backend.
- The selected backend is injected into permission management and command
  execution wrappers.

#### Tests

- Default config selects gVisor.
- Explicit Docker config selects the Docker backend.
- Registration fails cleanly when the selected backend is unavailable.

#### Exit criteria

- Backend selection is no longer hardcoded.
- The rest of the system continues to consume a single sandbox contract.

---

### Phase 6: Tighten policy propagation and runtime feedback

#### Objective

Make sandbox policy the single source of truth for runtime enforcement state.

#### Work

- Ensure permission updates propagate to the selected backend immediately.
- Avoid policy duplication across authorization and sandbox state.
- Make policy snapshots auditable and inspectable.
- Clarify which policy decisions are global, per-agent, per-task, or per-command.

#### Topics to address

- Network rule accumulation vs. replacement semantics.
- Read-only root and protected path behavior across updates.
- Whether policy state is immutable per run or mutable during execution.
- How denied execution is logged and surfaced to callers.

#### Tests

- Policy updates are visible to the backend.
- Snapshot reads are consistent under concurrency.
- Runtime policy changes do not bypass enforcement.

#### Exit criteria

- Sandbox policy updates are deterministic and auditable.
- There is a clear contract for policy mutation over time.

---

### Phase 7: Cleanup, deprecation, and compatibility hardening

#### Objective

Remove the old architecture seams once the new enforcement model is stable.

#### Work

- Remove or deprecate gVisor-specific names from shared interfaces where they no
  longer belong.
- Remove direct execution assumptions from authorization-facing code.
- Update package docs to describe sandbox as the enforcement boundary.
- Add compatibility shims only where they are needed to avoid breaking callers.

#### Tests

- Backward compatibility remains intact for the default gVisor path.
- Public docs and package comments reflect the new architecture.

#### Exit criteria

- The sandbox package is the canonical enforcement boundary.
- Alternative backends live in platform packages only.
- Documentation matches the implementation.

---

## Dependency Order

```text
Phase 1 -> Phase 2 -> Phase 3 -> Phase 5
                    -> Phase 4 -> Phase 5
Phase 3 -> Phase 6
Phase 4 -> Phase 6
Phase 5 -> Phase 7
Phase 6 -> Phase 7
```

Recommended execution order:

1. Phase 1
2. Phase 2
3. Phase 3
4. Phase 4
5. Phase 5
6. Phase 6
7. Phase 7

Phase 3 and Phase 4 can overlap once the contract in Phase 2 is stable, but
Phase 5 should wait until at least one concrete alternative backend exists.

---

## Risks

- Enforcement can remain leaky if callers keep a direct path to raw runners.
- Policy semantics may become inconsistent if authorization and sandbox each
  keep their own notion of "allowed."
- Docker support may lag behind gVisor capabilities, so unsupported policy
  handling must fail closed and be well-tested.
- Backend selection can become fragmented if it is implemented in multiple
  unrelated entry points.

Mitigation:

- One sandbox enforcement wrapper.
- One shared policy model.
- One backend selection path.
- Exhaustive tests for deny-before-execute behavior.

---

## Acceptance Criteria

- Execution blocking is enforced by `framework/sandbox`.
- gVisor remains the default backend and defines the reference policy semantics.
- Docker-based sandboxing is implemented as a separate backend under
  `platform/sandbox/dockersandbox`.
- The system supports backend capability reporting and policy validation.
- Unsupported policy features fail closed.
- Existing authorization/HITL behavior still works, but does not own the final
  execution gate.

