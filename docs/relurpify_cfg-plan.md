# relurpify_cfg Plan

## Synopsis

This document captures the agreed direction for organizing `relurpify_cfg`, shared template assets, testsuite derivation, and the split between end-user and development tooling.

The central rule is:

- `relurpify_cfg/` is the only workspace-local source of truth for Relurpify runtime configuration and generated workspace state.

Shared templates and testsuite templates are seed material only. They are copied into derived workspaces and are not authoritative at runtime.

---

## Decisions

### Workspace Root

- `relurpify_cfg/` remains the required workspace configuration root.
- It is not optional and is not replaced by arbitrary alternate names.
- It may conceptually behave like a hidden control directory for the workspace, but the canonical name remains `relurpify_cfg`.

### Shared Template Install

- Builtin templates are installed to a machine-local shared directory.
- The shared install location must be configurable/discoverable and not hardcoded to a repo-relative path.
- Templates are copied into workspaces for modification.

### Template Ownership

- Agent and skill templates are builtin starter material.
- After copying into `relurpify_cfg/`, the workspace-owned copies become authoritative.
- The runtime should not keep reading builtin template directories as if they were live workspace state.

### Testsuite Derivation

- Automated tests always run in isolated derived workspaces.
- Test workspaces are materialized under `testsuite/tmp/<run-id>/relurpify_cfg` conceptually.
- In practice, test artifacts should remain inside the workspace tree, under `relurpify_cfg/test_runs/<run-id>/...`.

### Tooling Split

- `relurpish` is the TUI and end-user runtime tool.
- `dev-agent` is the CLI for development, automation, integration, and testsuite workflows.

### Doctor Flow

- `relurpish doctor` replaces the removed wizard/init flow.
- `doctor` performs runtime dependency and configuration checks.
- `doctor --fix` may update or overwrite current workspace configuration.
- If no workspace exists, `doctor` becomes the workspace initialization flow.

### Dependency Policy

- Docker, `runsc`, and Ollama are blocking runtime dependencies.
- Chromium is part of the core local install check, but non-blocking unless a tool/manifest specifically requires it.

### Artifact Placement

- Runtime logs, telemetry, memory, and sessions stay inside workspace `relurpify_cfg/`.
- Test artifacts also stay inside the workspace tree.

### Migration

- No formal migration tool is required for this change set.
- Compatibility shims are optional, but not a planning requirement.

---

## Target Directory Model

### Workspace Runtime Truth

This is the only authoritative workspace-local runtime area.

```text
relurpify_cfg/
├── config.yaml
├── agent.manifest.yaml
├── agents/
├── skills/
├── logs/
├── telemetry/
├── memory/
├── sessions/
└── test_runs/
```

Within `relurpify_cfg/`:

- declarative configuration:
  - `config.yaml`
  - `agent.manifest.yaml`
  - copied workspace-owned manifests in `agents/`
  - copied workspace-owned skills in `skills/`
- generated state:
  - `logs/`
  - `telemetry/`
  - `memory/`
  - `sessions/`
  - `test_runs/`

### Machine-Local Shared Templates

Shared install data is seed material only.

```text
<shared>/relurpify/
└── templates/
    ├── workspace/
    ├── agents/
    ├── skills/
    └── testsuite/
```

These templates are copied into workspaces or derived testsuite workspaces.

### Testsuite Derived Workspaces

Tests should be isolated from live workspace runtime state while still storing artifacts inside the workspace tree.

```text
relurpify_cfg/
└── test_runs/
    └── <run-id>/
        ├── tmp/
        │   └── relurpify_cfg/
        ├── logs/
        ├── telemetry/
        ├── artifacts/
        └── report.json
```

---

## Operating Rules

### Source Of Truth

- Shared templates are never source of truth during runtime execution.
- Once copied, workspace files under `relurpify_cfg/` are authoritative.

### Runtime Reads

- `relurpish` and other Relurpify apps should resolve runtime config and state from workspace `relurpify_cfg/`.
- They should not depend on repo-local template directories for normal execution.

### Test Isolation

- Integration tests must not reuse live workspace logs, memory, sessions, or telemetry.
- Tests derive isolated `relurpify_cfg` trees from test templates and emit artifacts into per-run output directories.

### Tool Responsibilities

- `relurpish` owns end-user runtime interaction and workspace diagnostics.
- `dev-agent` owns development and integration concerns such as testsuite automation, scaffold helpers, and development-oriented inspection flows.

---

## Immediate Consequences

1. Current template-like manifests under `relurpify_cfg/agents/` should no longer be treated as builtin inventory.
2. Current workspace path assumptions should be refactored behind a single path-resolution model rooted in `relurpify_cfg/`.
3. Testsuite execution should materialize temporary derived workspace config instead of coupling to live workspace state.
4. `relurpish doctor` becomes the supported entrypoint for initialization and local install verification.
5. `dev-agent` is the renamed and narrowed development CLI.
