# Shell Blacklist

## Synopsis

The shell blacklist is a regex-based command filter that sits between the permission model and the sandbox runner. Where `bash_permissions` matches glob patterns from the manifest and `capability_policies` operate on capability metadata, the shell blacklist operates directly on the full command string at execution time. It is the last line of defence before a command enters the sandbox.

---

## Where It Fits

```
PermissionManager (capability + executable checks)
        │
        ▼
ShellGuard  ←── wraps the active CommandRunner
        │
        ├── ShellBlacklist.Check(cmdString)
        │        │
        │        ├── action: block → error, command does not run
        │        ├── action: hitl  → HITL approval via PermissionManager
        │        └── no match      → pass through
        │
        ▼
CommandRunner (SandboxCommandRunner or LocalCommandRunner)
```

`ShellGuard` wraps whichever runner is active (sandboxed or local). It reconstructs the full command string as `"binary arg1 arg2 ..."` and passes it to `ShellBlacklist.Check` before forwarding to the inner runner. The blacklist is a nil-safe no-op — if no file is present, all commands pass through.

---

## File Location

The blacklist file is loaded from:

```
<workspace>/relurpify_cfg/shell_blacklist.yaml
```

If the file does not exist, the blacklist is empty and no commands are filtered. Create it alongside your `agent.manifest.yaml` to activate it.

---

## File Format

```yaml
version: "1.0"
rules:
  - id: block-rm-rf
    pattern: "rm\\s+-[a-zA-Z]*r[a-zA-Z]*f.*"
    reason: "Recursive force-delete is destructive and rarely needed"
    action: block

  - id: hitl-curl-external
    pattern: "curl\\s+.*https?://(?!localhost).*"
    reason: "External HTTP requests require approval"
    action: hitl

  - id: block-sudo
    pattern: "sudo\\s+.*"
    reason: "Privilege escalation is not permitted"
    action: block
```

### Fields

| Field | Required | Description |
|---|---|---|
| `id` | yes | Unique rule identifier, used in error messages and HITL approval metadata |
| `pattern` | yes | Go `regexp` pattern matched against the full command string |
| `reason` | yes | Human-readable explanation shown in error messages and HITL prompts |
| `action` | yes | `block` or `hitl` |

---

## Actions

### `block`

The command is rejected immediately. An error is returned to the agent:

```
shell filter blocked [block-rm-rf]: Recursive force-delete is destructive and rarely needed
```

The agent receives this as a tool error and must either find another approach or report back that it cannot proceed.

### `hitl`

Execution pauses and a HITL approval request is raised. The TUI notification bar displays the rule's reason and the command string. Standard approval scopes apply:

| Key | Scope |
|-----|-------|
| `y` | Approve this invocation once |
| `s` | Approve for the session |
| `a` | Approve permanently (writes to manifest) |
| `n` | Deny this invocation |

If no `CommandApprover` is configured (e.g., in headless mode), `hitl` rules behave as `block`.

---

## Pattern Matching

Patterns are Go regular expressions (`regexp.Compile`). They are matched against the full command string, which is constructed as:

```
args[0] args[1] args[2] ...
```

This is the same format that `AuthorizeCommand` uses for `bash_permissions` glob matching, so pattern authors have a consistent mental model across both layers.

**First match wins.** Rules are evaluated in declaration order; the first matching rule's action applies. Subsequent rules are not checked.

A nil or empty blacklist passes all commands through without error.

---

## Examples

### Block common destructive patterns

```yaml
version: "1.0"
rules:
  - id: block-rm-rf
    pattern: "rm\\s+-[a-zA-Z]*r[a-zA-Z]*f"
    reason: "Recursive force-delete requires explicit user action"
    action: block

  - id: block-drop-table
    pattern: "(?i)drop\\s+table"
    reason: "DROP TABLE is irreversible"
    action: block

  - id: block-git-push-force
    pattern: "git\\s+push\\s+.*--force"
    reason: "Force-push can overwrite remote history"
    action: block
```

### Require approval for sensitive operations

```yaml
version: "1.0"
rules:
  - id: hitl-package-install
    pattern: "(pip|npm|cargo|go get)\\s+install.*"
    reason: "Installing packages changes the environment"
    action: hitl

  - id: hitl-env-write
    pattern: ".*>\\s*\\.env"
    reason: "Writing to .env may expose credentials"
    action: hitl
```

### Mixed policy

```yaml
version: "1.0"
rules:
  - id: block-sudo
    pattern: "sudo\\s+.*"
    reason: "Privilege escalation is not permitted"
    action: block

  - id: hitl-curl
    pattern: "curl\\s+.*"
    reason: "Outbound HTTP requires approval"
    action: hitl

  - id: hitl-git-push
    pattern: "git\\s+push.*"
    reason: "Pushing to remote requires approval"
    action: hitl
```

---

## Relationship to Other Permission Layers

| Layer | Operates on | Declared in |
|---|---|---|
| `PolicyEngine` compiled rules | Capability metadata (kind, trust, risk, effect class) | `spec.agent.capability_policies` |
| `PermissionManager` executable check | Binary name + argument list | `spec.defaults.executables` |
| `bash_permissions` | Full command string (glob) | `spec.agent.bash_permissions` |
| **Shell blacklist** | **Full command string (regex)** | **`relurpify_cfg/shell_blacklist.yaml`** |
| gVisor sandbox | Kernel syscalls | Runtime enforcement |

The shell blacklist and `bash_permissions` cover similar ground but serve different purposes. `bash_permissions` is part of the manifest and is agent-specific. The shell blacklist is a workspace-level file that applies to all agents running in that workspace, regardless of their individual manifest configuration.

---
