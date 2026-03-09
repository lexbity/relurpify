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

**Capability calls** — before any tool capability executes, its declared permission requirements are compared against the manifest's `spec.defaults.permissions`. Capability trust/risk/effect policy is evaluated first, then the runtime permission manager checks whether the required low-level permissions are actually declared.

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

For a given tool call, the runtime resolves in this order:

1. **Per-tool policy** — `spec.agent.tool_execution_policy.<tool_name>` in the manifest
2. **Trust policy** — `spec.agent.policies.<trust_class>`
3. **Capability selector policy** — `spec.agent.capability_policies[]`
4. **Class policy** — `spec.agent.policies.<risk_or_effect_class>`
5. **Runtime permission checks** — filesystem/executable/network declarations under `spec.defaults.permissions`
6. **Fallback** — `Ask` if nothing else matches

---

## The Manifest as the Source of Truth

Everything above flows from the agent manifest. The manifest is:

- **Validated at startup** — a manifest that declares an invalid permission structure prevents the agent from loading
- **Read-only at runtime** — you cannot loosen permissions without editing the file and restarting
- **Written by the `a` approval** — pressing always in a HITL prompt is the only runtime-initiated manifest write

This means the manifest always reflects the actual permission state. There are no hidden in-memory overrides that don't survive a restart.

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

## See Also

- [Configuration](configuration.md) — full manifest schema
- [TUI](Relurpish_TUI.md) — the Tools pane for editing policies interactively
- [Architecture](architecture.md) — how the permission layer fits into the execution path
