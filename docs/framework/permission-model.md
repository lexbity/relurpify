# Permission Model

## Synopsis

Every action an agent takes — reading a file, running a test, calling git — is checked against a permission contract before it executes. This is not advisory; it is enforced. The permission system is the layer between the agent's intent and actual execution.

---

## Why It Exists

gVisor provides kernel-level isolation for executed commands. But isolation alone does not tell you *what* a command is allowed to do within that sandbox — a sandboxed agent can still delete every file in the workspace if nothing stops it. The permission model provides the logical layer: it defines exactly what actions are permitted, enforces that boundary at runtime, and surfaces anything ambiguous to you for a decision.

The two layers work together:

```
Agent wants to run: git commit -m "msg"
        │
        ▼
PermissionManager: is "git" declared in spec.defaults.executables?
        │
        ├─ Yes, with matching args → permitted
        ├─ No, capability/class policy = ask → pause, ask you (HITL)
        ├─ No, capability/class policy = deny → blocked, error returned
        └─ No explicit override → runtime default policy applies
        │
        ▼
If permitted: docker run --runtime=runsc (gVisor)
        │
        ▼
Command executes inside sandbox
```

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

**File access** — `fs:read`, `fs:write`, `fs:execute`, and `fs:list` actions are checked against the filesystem permission entries in the manifest. Paths use glob matching with `**` for recursive subtrees.

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
