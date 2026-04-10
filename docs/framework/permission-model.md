# Permission Model

## Synopsis

Every action an agent takes — reading a file, running a test, calling git — is checked against a permission contract before it executes. This is not advisory; it is enforced. The permission system is the layer between the agent's intent and actual execution.

---

## Why It Exists

gVisor provides kernel-level isolation for executed commands. But isolation alone does not tell you *what* a command is allowed to do within that sandbox — a sandboxed agent can still delete every file in the workspace if nothing stops it. The permission model provides the logical layer: it defines exactly what actions are permitted, enforces that boundary at runtime, and surfaces anything ambiguous to you for a decision.

The enforcement stack has three layers:

```
Agent wants to run: git commit -m "msg"
        │
        ▼
[1] PolicyEngine: compiled capability rules (trust class, risk class, effect class,
    kind, provider) — per-tool and selector-based policies from the manifest
        │
        ├─ deny  → blocked immediately
        ├─ ask   → HITL pause (see below)
        └─ allow → continue
        │
        ▼
[2] PermissionManager: concrete permission checks
    • executable declared in spec.defaults.executables?
    • args match declared pattern?
    • bash_permissions allow/deny patterns match?
    • filesystem path within declared fs entries?
    • network endpoint within declared network entries?
        │
        ├─ denied by pattern     → blocked, error returned
        ├─ requires approval     → HITL pause
        └─ permitted             → continue
        │
        ▼
[3] ShellGuard + ShellBlacklist: regex-based command filter
    (evaluated against the full command string before execution)
        │
        ├─ action: block → blocked, error returned
        ├─ action: hitl  → HITL pause via CommandApprover
        └─ no match      → pass through
        │
        ▼
If permitted: docker run --runtime=runsc (gVisor)
        │
        ▼
Command executes inside isolated sandbox
```

See [Shell Blacklist](shellblacklist.md) for details on configuring layer 3.

---

## How It Works

### The Three-Level Policy

Every permission check resolves to one of three outcomes:

**Allow** — the action proceeds immediately without interruption.

**Deny** — the action is blocked. An error is returned to the agent, which must either find another approach or report back to you that it cannot proceed.

**Ask** — the action is paused and a notification appears in the TUI banner. You decide in real time whether to permit it.

### What Gets Checked

The permission manager checks four categories:

**Capability calls** — before any tool, prompt, or resource capability executes, compiled capability policy is evaluated first. If execution is allowed, the runtime permission manager then checks any required low-level permissions (filesystem, executable, network, session/provider approval) against the resolved contract.

**File access** — `fs:read`, `fs:write`, `fs:execute`, and `fs:list` actions are first checked by the sandbox file-scope policy, which blocks workspace escapes and protected governance roots before any host I/O occurs. Manifest filesystem entries then apply the remaining policy and HITL rules. Paths still use glob matching with `**` for recursive subtrees.

**Executable invocations** — before a binary is run, its name is matched against `spec.defaults.executables`. Argument patterns (`["*"]` for any args) and environment variable patterns are also checked.

**Network calls** — outgoing connections are checked against `spec.defaults.permissions.network` entries (direction, protocol, host, port).

### Default Posture

Start with explicit capability policy and class policy set to `ask` for risky actions. In the current phase-1 model, the most useful defaults are:

```yaml
spec:
    agent:
        capability_policies:
            - selector:
                kind: tool
                risk_classes: ["destructive"]
              execute: ask
        policies:
            network: ask
            remote-declared-untrusted: ask
```

This keeps risky or untrusted capabilities reviewable without depending on a legacy catch-all tool policy.

---

## Human-in-the-Loop (HITL)

When the policy resolves to `Ask`, execution pauses and a notification bar appears at the bottom of the TUI:

```
[HITL] command:exec: go build ./...
 [y] once  [s] session  [a] always  [n] deny  [d] dismiss
```

### Approval Scopes

| Key | Scope | What it does |
|-----|-------|-------------|
| `y` | One-time | Approves this single invocation; next time it will ask again |
| `s` | Session | Approves for the duration of this TUI session; not persisted |
| `a` | Always | Approves permanently; writes `allow` to the manifest on disk |
| `n` | — | Denies this invocation; returns an error to the agent |
| `d` | — | Dismisses the notification; the action stays suspended |

Pressing `a` (always) does two things: it approves the current request and calls `SaveToolPolicy` to update the manifest file. From that point forward, the tool will be allowed without prompting.

### HITL Timeout

If you do not respond within the HITL timeout (default 45 seconds), the request is automatically denied. This prevents agents from hanging indefinitely waiting for a response.

### HITL Rate Limiting

The runtime enforces a rate limit of **10 HITL requests per key per minute**. If an agent triggers more than 10 approval requests for the same permission key within a rolling one-minute window, subsequent requests are rejected automatically without prompting you. This prevents runaway agents from flooding the approval queue.

---

## Policy Resolution Order

For a given capability invocation, provider activation, or session-bound operation, the runtime resolves in this order:

1. **Descriptor lookup** — resolve the registered tool/prompt/resource/provider target
2. **Compiled policy evaluation** — per-tool policy, selector policy, provider/session policy, trust/risk/effect class policy
3. **Approval handling** — HITL request if the resolved decision is `ask`
4. **Concrete permission checks** — filesystem/executable/network/session checks against the resolved permission set
5. **Runtime safety / revocation** — deny revoked or unavailable runtime entries
6. **Execution**

---

## Bash Permissions

In addition to capability-level policy, executable invocations pass through a second manifest-controlled filter: `bash_permissions`. This is declared per-agent in the manifest and applies to every command the agent runs through the shell tool.

```yaml
spec:
    agent:
        bash_permissions:
            allow_patterns:
                - "go *"
                - "git *"
            deny_patterns:
                - "rm -rf *"
                - "curl *"
            default: ask   # allow | deny | ask
```

Resolution order within `bash_permissions`:

1. Deny patterns are checked first — if any match, the command is blocked immediately.
2. Allow patterns are checked next — if any match, the command proceeds.
3. If no pattern matches, the `default` field applies (`allow`, `deny`, or `ask`).

Patterns use glob matching (same as manifest filesystem paths). The command string matched against is `"binary arg1 arg2 ..."`.

When `default: ask`, the runtime raises a standard HITL request using the `command:exec` action. The approval scopes (`y/s/a/n`) behave the same as for capability approvals.

---

## Policy Compilation

The manifest is not evaluated directly at runtime. On startup (and on every preset switch or reload), `CompileManifestPolicyRules` transforms the manifest's policy surfaces into a sorted slice of `PolicyRule` values. `evaluateCompiledRules` then does a single pass through this slice on each capability request, returning the first matching rule's effect.

### What gets compiled

| Manifest field | Rule priority |
|---|---|
| `spec.agent.policies` (global class keys) | 100 |
| `spec.agent.tool_execution_policy` | 300 |
| `spec.agent.capability_policies` (selectors) | 300 + index |
| `spec.agent.session_policies` | 400 + policy priority |
| `spec.agent.provider_policies` | 500 |

Rules at higher priority values are evaluated first. Within the same priority, rules are sorted by ID for deterministic ordering.

### Selector fields

`capability_policies` entries accept a `selector` that matches on:

- `kind` — `tool`, `prompt`, or `resource`
- `risk_classes` — e.g. `["destructive", "execute"]`
- `effect_classes` — e.g. `["filesystem-mutation", "process-spawn"]`
- `trust_classes` — e.g. `["remote-declared", "provider-local-untrusted"]`
- `runtime_families` — `local-tool`, `provider`, or `relurpic`
- `id` / `name` — exact capability match

Fields that require descriptor-time evaluation (tags, source scopes, coordination roles) are not supported in compiled selectors and will produce a startup error.

### Global class keys

`spec.agent.policies` accepts trust-class, risk-class, effect-class, and runtime-family names directly as keys:

```yaml
policies:
    destructive: ask          # risk class
    filesystem-mutation: ask  # effect class
    remote-declared: ask      # trust class
    network: ask              # risk class
```

The compiled rule for each key matches any capability whose descriptor carries that class.

---

## Sandbox

The `framework/sandbox` package provides the execution backend. All commands permitted by the policy and permission layers run through a `CommandRunner`.

### Runners

**`SandboxCommandRunner`** — launches commands inside a gVisor container using `docker run --runtime=runsc` (or `containerd`). This is the default when `--no-sandbox` is not set. File-scope policy is propagated into sandbox-aware tools so the same protected roots are not exposed through host-side file I/O or command execution.

**`LocalCommandRunner`** — runs commands directly on the host. Used in development or when the sandbox runtime is unavailable. Path traversal is still enforced: the resolved working directory must remain within the declared workspace root.

The active runner is selected at startup based on the workspace config and the `--no-sandbox` flag. `ShellGuard` wraps whichever runner is active to apply the shell blacklist before forwarding.

### gVisor startup verification

Before the first command runs, the sandbox verifies:
1. `runsc` binary is present and responds to `--version`
2. The configured container runtime (`docker` or `containerd`) is installed and reachable

If either check fails the agent does not start. This prevents silent fallback to an unsandboxed runner.

### Sandbox configuration

Sandbox knobs are set via manifest `spec.security` and the gVisor `SandboxConfig`:

```yaml
spec:
    image: ghcr.io/relurpify/runtime:latest
    runtime: gvisor
    security:
        run_as_user: 1000        # UID inside the container
        read_only_root: false    # mount container root read-only
        no_new_privileges: true  # pass --security-opt no-new-privileges
```

Additional config (set programmatically, not in the manifest):

| Field | Default | Description |
|---|---|---|
| `RunscPath` | `runsc` | Path to the runsc binary |
| `Platform` | `kvm` | gVisor platform: `kvm` or `ptrace` |
| `ContainerRuntime` | `docker` | `docker` or `containerd` |
| `NetworkIsolation` | `true` | Use `--network none` when no egress rules are declared |
| `ReadOnlyRoot` | `false` | Mount container root filesystem read-only |
| `SeccompProfile` | `""` | Path to a custom seccomp profile JSON |

### Network isolation

When `NetworkIsolation` is `true` and the manifest declares no network egress rules, the container is started with `--network none`. If egress rules are declared (e.g., the Ollama endpoint), the network flag is omitted and egress enforcement is handled by `PermissionManager` instead.

---

## Delegations

Delegations are bounded capability grants issued by one agent to another for a scoped task. They let a parent agent authorize a sub-agent to use a specific capability without granting unrestricted access.

### How they work

A delegation is created via `DelegationManager.ExecuteDelegation`. This:

1. Resolves the target capability in the registry.
2. Validates the target against the caller's coordination policy.
3. Captures a `PolicySnapshot` at delegation time — the sub-agent executes under the policy that was active when the delegation was issued, not when it runs.
4. Records the delegation as a `DelegationSnapshot` (state: `active`, `completed`, or `failed`).
5. Invokes the capability either inline (foreground) or in a goroutine (background).

### Background delegations

If the target capability is declared as long-running, or `Background: true` is passed, the delegation runs asynchronously. The caller receives a `DelegationSnapshot` immediately and can monitor or cancel the delegation via its ID. Completion is reported through the background runner's result channel.

### Trust and recoverability

The delegation inherits the target capability's `TrustClass`. Recoverability mode (`recoverable`, `non-recoverable`, `best-effort`) controls how the system handles failures and whether retry is permitted.

### Session and resume gating

When a delegation targets a session operation or a resume export, the policy engine enforces additional ownership checks:

- Session operations require the caller to be the session owner, or to hold an explicit delegation for that session.
- Resume operations apply the same ownership/delegation check and additionally require approval if the session is marked as restricted-external.

---

## The Effective Contract as the Source of Truth

Everything above flows from the runtime's effective contract. The manifest is the primary input, but the final contract also includes resolved skills and later overlays. The effective contract is:

- **Validated at startup** — a manifest that declares an invalid permission structure prevents the agent from loading
- **Compiled once per active runtime state** — startup, preset switch, and reload all rebuild policy from the same contract path
- **Reloadable in place when topology is stable** — policy/spec changes can be recompiled without rebuilding wrappers; capability-topology changes still require a restart/rebuild
- **Written by the `a` approval** — pressing always in a HITL prompt is the only runtime-initiated manifest write

This means the live runtime policy stays aligned with the resolved contract. There are no separate raw-manifest and post-skill policy engines drifting apart anymore.

---

## Manifest Policies Quick Reference

```yaml
spec:
    agent:
        tool_execution_policy:
            file_delete:
                execute: deny
        capability_policies:
            - selector:
                kind: tool
                risk_classes: ["destructive"]
              execute: ask
        policies:
            network: ask
            remote-declared-untrusted: ask
        provider_policies:
            remote-mcp:
                activate: ask
                default_trust: remote-declared-untrusted

    defaults:
        permissions:
            filesystem:
                - action: fs:read
                  path: ${workspace}/**
                - action: fs:write
                  path: ${workspace}/**
                  justification: Modify files
            executables:
                - binary: go
                  args: ["*"]
                - binary: git
                  args: ["*"]
            network:
                - direction: egress
                  protocol: tcp
                  host: localhost
                  port: 11434
```

---
