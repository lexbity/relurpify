# relurpify_cfg Engineering Specification

## Goal

Standardize `relurpify_cfg` as the sole workspace-local runtime contract, move starter assets into shared installed templates, isolate testsuite execution, use `dev-agent` as the development CLI, and use `relurpish doctor` as the supported setup flow.

---

## Scope

This specification covers:

- directory layout standardization
- runtime path derivation changes
- template resolution and copying
- testsuite workspace derivation
- `dev-agent` specialization
- `relurpish doctor` design and rollout
- documentation updates tied to the new layout

This specification does not require:

- a formal migration utility
- alternate workspace root names
- preserving repo-local template directories as runtime authorities

---

## Target Layout

### Workspace Layout

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

### Shared Template Layout

```text
<shared>/relurpify/templates/
├── workspace/
├── agents/
├── skills/
└── testsuite/
```

### Testsuite Run Layout

```text
relurpify_cfg/test_runs/<run-id>/
├── tmp/
│   └── relurpify_cfg/
├── logs/
├── telemetry/
├── artifacts/
└── report.json
```

---

## Design Requirements

### Workspace Authority

- `relurpify_cfg/` is the only workspace-local source of truth.
- Runtime code must derive all active config/state paths from it.

### Template Semantics

- Shared templates are copy-from only.
- Workspace-copied manifests and skills are editable and authoritative.

### Dependency Checks

- Docker, `runsc`, and Ollama block runtime readiness.
- Chromium is checked by doctor but is warning-only unless required by tool/manfiest policy.

### Testsuite Isolation

- Tests always use derived isolated workspaces.
- Tests must not write into live `logs/`, `memory/`, `sessions/`, or top-level `telemetry/`.

### Tool Boundary

- `relurpish` is the end-user runtime/TUI tool.
- `dev-agent` is the development/integration CLI.

---

## Phases

### Phase 1: Directory Contract

Create the canonical path model and document it.

Work:

- introduce a shared workspace path resolver
- define named accessors for:
  - config path
  - active manifest path
  - agents dir
  - skills dir
  - logs dir
  - telemetry dir/file
  - memory dir
  - sessions dir
  - test run root
- remove ad hoc path concatenation where practical

Acceptance:

- runtime and CLI paths come from one central resolver
- `relurpify_cfg/` layout is documented in user/developer docs

### Phase 2: Shared Template Install Model

Stop treating workspace-local template-like files as builtin inventory.

Work:

- define template discovery for machine-local shared install directory
- add template categories:
  - workspace
  - agents
  - skills
  - testsuite
- refactor code that assumes repo-local template presence

Acceptance:

- builtin starter assets resolve through installed shared templates
- runtime no longer depends on workspace `relurpify_cfg/agents` as implicit builtin inventory

### Phase 3: Workspace Initialization Via Doctor

Implement `relurpish doctor` as the supported setup and validation flow.

Work:

- add doctor command surface
- if `relurpify_cfg/` is missing:
  - prompt to initialize workspace
  - copy starter `config.yaml`
  - copy starter `agent.manifest.yaml`
- validate:
  - Docker
  - `runsc`
  - Ollama
  - Chromium
- classify results:
  - blocking
  - warning
  - informational
- add `--fix` behavior to overwrite/update starter config from templates

Acceptance:

- missing workspace flows through doctor, not a separate init/wizard path
- doctor clearly distinguishes runtime blockers from warnings

### Phase 4: Runtime Refactor For New Layout

Align `relurpish` and related runtime code with the standardized directory contract.

Work:

- move telemetry output to `relurpify_cfg/telemetry/` or explicitly preserve a single file within that directory contract
- ensure logs always write to `relurpify_cfg/logs/`
- ensure memory always writes to `relurpify_cfg/memory/`
- ensure sessions always write to `relurpify_cfg/sessions/`
- ensure any test output is not mixed with live runtime stores

Acceptance:

- all workspace runtime state lands in standardized locations
- no remaining wizard-era or repo-assumption path logic

### Phase 5: Testsuite Derivation Model

Make automated testing derive isolated runtime workspaces from dedicated templates.

Work:

- define testsuite template profile format
- materialize per-run temp workspace config under:
  - `relurpify_cfg/test_runs/<run-id>/tmp/relurpify_cfg/`
- write run logs, telemetry, artifacts, and reports under same run root
- support suite-level and case-level overrides on top of copied templates

Acceptance:

- tests do not mutate live workspace runtime state
- every run has self-contained inputs and outputs

### Phase 6: `dev-agent`

Rename and narrow the development CLI.

Work:

- use `dev-agent` as the sole development CLI entrypoint
- keep scope focused on:
  - testsuite automation
  - dev inspection
  - skill/agent scaffolding and validation
  - development-oriented workspace helpers
- update docs and examples

Acceptance:

- `relurpish` is clearly the end-user tool
- `dev-agent` is clearly the development/integration tool

### Phase 7: Documentation Standardization

Update documentation to reflect the new contract.

Work:

- add/maintain layout spec
- update installation docs for doctor-based initialization
- update configuration docs for source-of-truth rules
- update testing docs for derived testsuite workspaces
- update architecture docs for tool split:
  - `relurpish`
  - `dev-agent`

Acceptance:

- docs no longer imply mixed template/runtime ownership inside workspace config
- docs explain how templates become workspace-owned copies

---

## Code Areas To Touch

Likely implementation areas:

- `app/relurpish/runtime/`
- `app/relurpish/`
- `app/cmd/` or renamed CLI package for `dev-agent`
- testsuite runner code
- configuration/path helper packages
- docs under `docs/`

Likely data/layout areas:

- current `relurpify_cfg/agents`
- current `relurpify_cfg/skills`
- current `relurpify_cfg/test_runs`

---

## Risks

1. Template/runtime ambiguity may persist if workspace-local `agents/` and `skills/` are not explicitly reclassified as copied user-owned assets.
2. Hardcoded absolute filesystem paths in current starter manifests will make shared templates non-portable until parameterized or rewritten during copy.
3. Testsuite isolation will fail if any runtime subsystem still writes to top-level workspace state instead of per-run derived paths.
4. CLI renaming may leave stale documentation and scripts unless all examples are updated consistently.

---

## Completion Criteria

This effort is complete when:

- `relurpify_cfg/` is the sole workspace runtime contract
- shared templates are install-scoped and copy-only
- tests derive isolated `relurpify_cfg` trees per run
- `relurpish doctor` is the supported setup/validation entrypoint
- `dev-agent` is the development-oriented CLI
- docs consistently describe the standardized layout and responsibilities
