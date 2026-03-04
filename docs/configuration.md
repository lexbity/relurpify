# Configuration

## Synopsis

Relurpify has two configuration layers: a lightweight workspace config that sets runtime defaults, and agent manifests that declare the full security contract for each agent. Understanding the difference between them is important — the workspace config is about convenience, the manifest is about trust.

---

## Why Two Layers

**`config.yaml`** answers: *what should happen by default in this workspace?* It is minimal — a model name, an agent preference. It is also writable at runtime (the TUI Settings pane saves to it).

**Agent manifests** answer: *what is this agent allowed to do?* They are the security contract. They declare every filesystem path, binary, and network endpoint an agent may touch, plus the default policy for anything not listed. They are validated at startup and cannot be loosened at runtime without a restart.

This separation means you can change your preferred model on the fly, but you cannot silently expand an agent's permissions without editing its manifest.

---

## Workspace Config

**Location:** `relurpify_cfg/config.yaml`

```yaml
default_model:
    name: qwen2.5-coder:14b
```

That's the minimum. When the TUI Settings pane saves a model selection, it writes back to this file.

**WorkspaceConfig** (extended, written by the runtime):

```yaml
model: qwen2.5-coder:14b
agents:
    - coding-go
allowed_tools: []
permission_profile: {}
last_updated: 1709500000
```

The `agents` list determines which agent manifest is loaded by default when no `--agent` flag is passed.

---

## Agent Manifests

**Location:** `relurpify_cfg/agents/<name>.yaml`

Manifests use `apiVersion: relurpify/v1alpha1`. Every field is validated at startup — a malformed or incomplete manifest prevents the agent from loading.

### Annotated Example

```yaml
apiVersion: relurpify/v1alpha1
kind: AgentManifest
metadata:
    name: coding-go
    version: 1.0.0
    description: Go-focused coding agent

spec:
    # Container image used for sandboxed tool execution.
    # All agent-executed commands run inside this image via gVisor.
    image: ghcr.io/lexcodex/relurpify/runtime:latest
    runtime: gvisor   # Must be "gvisor". No other value is accepted.

    defaults:
        permissions:
            # Declare every filesystem scope the agent needs.
            # Paths not listed here follow the default_tool_policy.
            filesystem:
                - action: fs:read
                  path: /path/to/project/**
                  justification: Read workspace files
                - action: fs:write
                  path: /path/to/project/**
                  justification: Modify files
                - action: fs:execute
                  path: /path/to/project/**
                  justification: Run tooling in workspace

            # Declare every binary the agent may invoke.
            # arg patterns use * (any single arg) or ["*"] (any args).
            executables:
                - binary: go
                  args: ["*"]
                - binary: git
                  args: ["*"]
                - binary: bash
                  args: ["-c"]

            # Declare egress network endpoints.
            # localhost:11434 is needed for Ollama if tools make LLM sub-calls.
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
        run_as_user: 1000       # UID inside the container
        read_only_root: false   # Make container root filesystem read-only
        no_new_privileges: true # Prevent privilege escalation inside container

    audit:
        level: verbose
        retention_days: 7

    agent:
        implementation: coding  # coding | planner | react | reflection | eternal
        mode: primary           # For CodingAgent: code | architect | ask | debug | docs
        version: 1.0.0
        prompt: >
            You are coding. Follow project rules, ask before destructive
            actions, and summarise outcomes.
        model:
            provider: ollama
            name: qwen2.5-coder:14b
            temperature: 0.2
            max_tokens: 4096
        ollama_tool_calling: true

    policies:
        # What happens when the agent tries to use a tool not listed above.
        # ask   → pause and prompt you via HITL
        # allow → permit silently
        # deny  → block and return an error to the agent
        default_tool_policy: ask

        # Extra gate for tools tagged as destructive (file_delete, etc.)
        destructive: ask

    skills:
        - system    # Core system instructions
        - coding    # General coding guidance
        - gocoder   # Go-specific rules
```

---

## Skills

Skills are composable prompt and tool packages. Each skill contributes a prompt fragment and optionally constrains the tool set. They are declared in `spec.skills` and loaded in order.

**Location:** `relurpify_cfg/skills/<name>/skill.manifest.yaml`

| Skill | Purpose |
|-------|---------|
| `system` | Core system instructions shared by all agents |
| `coding` | General coding conventions and workflow |
| `gocoder` | Go-specific idioms, test patterns, module conventions |
| `rustcoder` | Rust-specific idioms and ownership patterns |
| `pycoder` | Python conventions, virtual environments |
| `nodecoder` | Node.js / TypeScript patterns |
| `sqlcoder` | SQL and SQLite conventions |
| `devops` | CI/CD, shell scripting, infrastructure patterns |

A Go coding agent typically loads `system + coding + gocoder`. The skills stack onto each other — each one narrows and focuses the agent's behaviour for the task at hand.

Skills are loaded at agent startup. Changing the `spec.skills` list requires restarting the agent.

---

## Per-Tool Policy Overrides

Individual tools can override the `default_tool_policy`. This is managed via the TUI Tools pane (press `5`) and saved back to the manifest:

```yaml
spec:
    agent:
        tool_execution_policy:
            file_delete: deny
            bash_execute: ask
            run_tests: allow
```

When you press `a` (always allow) in a HITL prompt, this is what gets written to the manifest.

---

## Filesystem Paths and Placeholders

Manifest paths support workspace placeholders so manifests are portable across machines:

```yaml
- action: fs:read
  path: ${workspace}/**
```

`${workspace}` and `{{workspace}}` are both expanded to the actual workspace path at runtime. Relative paths without a placeholder are joined to the workspace root automatically.

---

## What Gets Loaded When

```
relurpish starts
    │
    ▼
DefaultConfig() — workspace = current directory
    │
    ▼
Normalize() — resolve all paths relative to workspace
    │
    ▼
LoadWorkspaceConfig(config.yaml) — apply model/agent defaults
    │
    ▼
RegisterAgent(manifest) — validate manifest, build PermissionManager
    │
    ▼
ApplySkills(spec.skills) — merge skill prompts and tool constraints
    │
    ▼
BuildToolRegistry() — register tools, apply per-tool policies
    │
    ▼
Agent is ready
```

---

## See Also

- [Architecture](architecture.md) — how configuration flows into the runtime
- [Permission Model](permission-model.md) — how the manifest's policy fields are enforced
- [Agents](agents.md) — which `implementation` and `mode` values to use
- [TUI](tui.md) — editing configuration at runtime via the Settings pane
