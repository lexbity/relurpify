# Configuration

## Synopsis

Relurpify has two configuration layers: a workspace config that stores runtime defaults for the current repo, and agent manifests that define the security contract and runtime behavior for an agent. The workspace config is about convenience. The manifest is about trust.

---

## Why Two Layers

`relurpify_cfg/config.yaml` answers: *which model and agent should this workspace prefer by default?* It is writable at runtime.

Agent manifests answer: *what is this agent allowed to do, and how should it behave?* They declare filesystem, executable, and network permissions plus the runtime-level agent settings consumed by `relurpish` and `dev-agent`.

This split lets you change a preferred model or agent without silently widening the agent's security envelope.

---

## Source Of Truth

Relurpify now has a strict ownership model:

- shared templates are starter assets only
- repo-local templates are development fallbacks only
- files copied into `relurpify_cfg/` are the live workspace source of truth

This matters most for `relurpify_cfg/agents/` and `relurpify_cfg/skills/`. They are no longer treated as builtin inventory. If a manifest or skill exists there, it is a workspace-owned copy and runtime code should prefer it over any shared template.

See [Workspace Layout](workspace-layout.md) for the full directory contract.

---

## Workspace Config

**Location:** `relurpify_cfg/config.yaml`

The runtime persists workspace selections in this shape:

```yaml
model: qwen2.5-coder:14b
agents:
    - coding-go
allowed_capabilities: []
permission_profile: workspace_write
last_updated: 1709500000
```

Field meanings:

| Field | Purpose |
|-------|---------|
| `model` | Default Ollama model override for this workspace |
| `agents` | Preferred agent presets or definitions |
| `allowed_capabilities` | Restrict capability visibility with selector-based bundles |
| `permission_profile` | Last selected workspace permission profile |
| `last_updated` | Unix timestamp of the last save |

This file is optional. If it is absent, runtime defaults come from the manifest and CLI flags.

---

## Agent Manifests

**Location:** `relurpify_cfg/agent.manifest.yaml` or `relurpify_cfg/agents/<name>.yaml`

Manifests use `apiVersion: relurpify/v1alpha1`. They are validated at startup, and invalid manifests are rejected before the runtime can execute.

Starter manifests may be copied into the workspace by `relurpish doctor` or from shared templates manually. After that copy happens, the workspace copy is authoritative.

### Annotated Example

```yaml
apiVersion: relurpify/v1alpha1
kind: AgentManifest
metadata:
    name: coding-go
    version: 1.0.0
    description: Go-focused coding agent manifest

spec:
    image: ghcr.io/lexcodex/relurpify/runtime:latest
    runtime: gvisor

    defaults:
        permissions:
            filesystem:
                - action: fs:read
                  path: ${workspace}/**
                  justification: Read workspace
                - action: fs:list
                  path: ${workspace}/**
                  justification: List workspace
                - action: fs:write
                  path: ${workspace}/**
                  justification: Modify workspace
                - action: fs:execute
                  path: ${workspace}/**
                  justification: Execute tooling inside workspace
            executables:
                - binary: go
                  args: ["*"]
                - binary: git
                  args: ["*"]
                - binary: bash
                  args: ["-c", "*"]
            network:
                - direction: egress
                  protocol: tcp
                  host: localhost
                  port: 11434
                  description: Ollama
        resources:
            limits:
                cpu: "2"
                memory: 4Gi
                disk_io: 200MBps

    security:
        run_as_user: 1000
        read_only_root: false
        no_new_privileges: true

    audit:
        level: verbose
        retention_days: 7

    agent:
        implementation: coding
        mode: primary
        version: 1.0.0
        prompt: >
            You are coding. Follow project rules, ask before destructive
            actions, and summarize outcomes.
        model:
            provider: ollama
            name: qwen2.5-coder:14b
            temperature: 0.2
            max_tokens: 4096
        allowed_capabilities:
            - name: file_read
              kind: tool
            - name: file_write
              kind: tool
            - name: file_edit
              kind: tool
            - name: file_search
              kind: tool
        bash_permissions:
            default: ask
            allow_patterns: ["git diff*", "git status"]
            deny_patterns: ["rm -rf*", "sudo*"]
        file_permissions:
            write:
                default: ask
                allow_patterns: ["**/*.go", "docs/**/*.md"]
            edit:
                default: ask
                require_approval: true
        invocation:
            can_invoke_subagents: true
            max_depth: 2
        context:
            max_files: 20
            max_tokens: 20000
            include_dependencies: true
        ollama_tool_calling: true

        capability_policies:
            - selector:
                kind: tool
                risk_classes: ["destructive"]
              execute: ask
        policies:
            destructive: ask
            network: ask
        provider_policies:
            remote-mcp:
                activate: ask
                default_trust: remote-declared-untrusted

    skills:
        - system
        - coding
        - gocoder
```

### `spec.agent` Semantics

Two different "mode" concepts exist and should not be confused:

| Field | Meaning |
|-------|---------|
| `spec.agent.implementation` | Which top-level agent implementation to instantiate |
| `spec.agent.mode` | Manifest role: `primary`, `subagent`, or `system` |

CodingAgent task modes such as `code`, `architect`, `ask`, `debug`, and `docs` are selected per task by the caller through task metadata or task context. They are not stored in `spec.agent.mode`.

### Common `spec.agent` Fields

| Field | Purpose |
|-------|---------|
| `allowed_capabilities` | Restrict capability visibility to explicit selectors |
| `tool_execution_policy` | Per-tool allow/ask/deny overrides |
| `capability_policies` | Selector-based capability execution rules keyed by kind, trust, risk, and effect |
| `policies` | Capability class policy keyed by trust/risk/effect labels |
| `provider_policies` | Provider activation and trust defaults |
| `bash_permissions` | Pattern-based shell command gating |
| `file_permissions` | Separate write/edit policies and approval rules |
| `invocation` | Subagent recursion limits |
| `context` | Context window and dependency-loading limits |
| `lsp` | Language-server feature toggles and server mapping |
| `search` | Hybrid/vector/AST search preferences |
| `logging` | Agent and LLM debug logging toggles |
| `metadata` | Display and registry metadata |

---

## Skills

Skills are composable prompt and policy packages declared in `spec.skills` and loaded in order.

**Location:** `relurpify_cfg/skills/<name>/skill.manifest.yaml`

| Skill | Purpose |
|-------|---------|
| `system` | Core system instructions shared by all agents |
| `coding` | General coding conventions and workflow |
| `gocoder` | Go-specific idioms, test patterns, module conventions |
| `rustcoder` | Rust-specific idioms and ownership patterns |
| `pycoder` | Python conventions and environment patterns |
| `nodecoder` | Node.js and TypeScript conventions |
| `sqlcoder` | SQL and SQLite conventions |
| `devops` | CI/CD and shell automation conventions |

Skills can contribute:

- prompt snippets and prompt capabilities
- resource capabilities backed by contained workspace files
- additional allowed-capability selectors
- capability, provider, session, insertion, and global policy
- planning, verification, and recovery guidance

Skills do not bypass manifest permissions or sandbox enforcement.

Like manifests, skill packages under `relurpify_cfg/skills/` are workspace-owned copies after initialization or manual copying. Shared skill templates are not used as runtime state directly.

Skill resource paths are validated for workspace containment during contract resolution. At runtime, skill resource reads still pass through the permission manager, so a skill cannot use direct filesystem reads to escape the manifest contract.

---

## Per-Tool Policy Overrides

Individual local tools can override the global tool policy. The TUI Tools pane writes these overrides back into the manifest:

```yaml
spec:
    agent:
        tool_execution_policy:
            file_delete:
                execute: deny
            git_commit:
                execute: ask
```

Use this for one-off policy exceptions without relaxing the entire manifest.

---

## Filesystem Paths And Placeholders

Manifest paths support workspace placeholders so manifests remain portable across machines:

```yaml
- action: fs:read
  path: ${workspace}/**
```

`${workspace}` and `{{workspace}}` are expanded to the absolute workspace path at runtime. Relative paths without placeholders are joined to the workspace root automatically.

---

## What Gets Loaded When

```text
relurpish starts
    |
    v
DefaultConfig() -> workspace = current directory
    |
    v
Normalize() -> resolve all filesystem paths
    |
    v
LoadWorkspaceConfig(config.yaml) -> apply workspace defaults
    |
    v
RegisterAgent(manifest) -> validate manifest + build PermissionManager
    |
    v
ResolveEffectiveAgentContract() -> merge manifest defaults, skills, overlays
    |
    v
BuildFromContract() -> compile one policy bundle from effective contract
    |
    v
BuildBuiltinCapabilityBundle() -> register builtin runtime capabilities
    |
    v
Admit skill/provider capabilities against final selector set
    |
    v
Agent is ready
```

The same contract-first path is reused for:

- initial runtime startup
- preset switching (`SwitchAgent`)
- live policy/spec reload when capability topology is unchanged

---

## See Also

- [Architecture](architecture.md) — how configuration flows into the runtime
- [Permission Model](permission-model.md) — how manifest policy fields are enforced
- [Agents](agents.md) — agent implementations and CodingAgent task modes
- [TUI](Relurpish_TUI.md) — editing configuration at runtime via the Settings pane
