# Tool Runtime Cleanup Engineering Specification

## Synopsis

This specification defines the cleanup and consolidation of Relurpify's local tool runtime across `framework/` and `tools/`.

Current date of this specification: March 9, 2026.

The main problem is not one bug but an accumulation of overlapping tool surfaces, repeated authorization logic, and expensive recursive traversal behavior. The current runtime works, but it is paying for compatibility and migration overlap in places that now need to be normalized.

---

## Problem Statement

The current tool runtime has four structural issues:

1. Multiple active tool paths exist for essentially the same feature.
2. Command authorization logic is duplicated across wrappers.
3. Recursive file/list/search tools pay high per-path permission-check cost.
4. Some correctness bugs and dead code remain from refactors.

Concrete examples in the current codebase:

- generic execution tools overlap with language-aware tools
- `file_search` overlaps with `search_grep`
- git, CLI, generic execution, and LSP startup all perform similar command authorization logic independently
- `git_commit` can ignore explicit file lists because argument normalization is inconsistent

---

## Goals

- Define one primary runtime path per feature area.
- Centralize command authorization behavior.
- Preserve manifest-based permission enforcement.
- Improve performance for large workspace traversal.
- Remove dead code and migration leftovers from active paths.

---

## Non-Goals

- No replacement of the capability registry.
- No manifest format redesign in this phase.
- No broad search/index architecture redesign.
- No change from local-first execution to remote services.

---

## Fixed Design Decisions

### Primary Tool Surface

Language-aware tools are the required default runtime surface for verification and build actions.

Examples:

- Go: `go_test`, `go_build`
- Python: `python_pytest`, `python_unittest`, compile check
- Rust: cargo-aware Rust tools

Generic execution tools are removed from default runtime registration in this cleanup. They may continue to exist in code temporarily during migration work, but they are not part of the default runtime surface after cutover.

### Primary Recursive Search Surface

One grep-style recursive text-search tool should be the default search primitive for workspace text lookup.

Its feature set must include:

- directory scoping
- substring search
- case-sensitive or case-insensitive mode

Other heuristic search tools must justify themselves separately and should not duplicate the default recursive text-search job.

### Shared Authorization Contract

All command-bearing runtime surfaces must use a shared authorization helper that performs:

- executable permission checks
- bash allow/deny evaluation
- approval prompting
- approval cache-key generation

The runtime should not maintain separate approval behavior for logically equivalent commands only because they entered through different wrappers.

### Permission Check Optimization

Recursive traversal tools must optimize repeated permission evaluation within a single operation.

At minimum:

- memoize per-path or per-subtree decisions during the walk
- avoid repeated rule scans for already-visited directories
- avoid emitting redundant success logs for every internal traversal decision where a coarser-grained log is sufficient

### Compatibility Policy

This cleanup is a hard cutover.

If tool names are removed or narrowed, the rollout must include:

- testsuite updates
- documentation updates
- runtime registration updates

There is no compatibility alias period in the default runtime.

---

## Architecture Changes

## 1. Shared Command Authorization Layer

Add a shared helper under `framework/runtime/` responsible for authorizing command execution before dispatch.

Recommended responsibilities:

- `CheckExecutable(...)`
- evaluate `spec.Bash` allow/deny/default
- construct normalized command string
- perform HITL approval request using a single action family
- expose structured denial reasons

Recommended result shape:

- authorized
- denied with reason
- requires approval and approval granted

Suggested consumers:

- `tools/cli_nix.CommandTool`
- `tools.GitCommandTool`
- `tools.RunTestsTool`
- `tools.RunBuildTool`
- `tools.RunLinterTool`
- `tools.ExecuteCodeTool`
- `tools.NewProcessLSPClientWithPermissions`

### Approval action normalization

Current separate action names such as:

- `bash:exec`
- `bash:git`
- `bash:cli`
- `bash:lsp`

should be replaced by one normalized action family unless there is a strong policy reason to preserve them.

Preferred direction:

- one shared action namespace such as `command:exec`
- optional metadata for source wrapper (`git`, `cli`, `lsp`, `exec-tool`)

This keeps approvals semantically consistent while preserving observability.

## 2. Execution Tool Consolidation

Runtime registration should distinguish between:

- preferred task-level tools
- fallback generic shell-style tools

### Required change

Refactor capability registration so language-aware tools are primary and generic execution tools are removed from default registration.

### Registration policy

Do not register two tools as first-class options if they both represent the same primary user intent, such as:

- run tests
- build project
- grep workspace text

## 3. Recursive Search Consolidation

Merge duplicate grep-style behavior into one implementation.

### Required capabilities

- recursive walk
- generated-directory skipping
- permission-aware traversal
- bounded line scanning
- optional case-insensitive mode

### Heuristic tools

`search_find_similar` and `search_semantic` remain on the main tool surface for now.

They should still be improved:

- add guardrails for large files and binary-like content
- document their heuristic nature clearly
- consider future index-backed implementations later

## 4. Filesystem Permission Walk Optimization

The current recursive walkers repeatedly call into `PermissionManager.CheckFileAccess(...)`.

This should be optimized without changing permission semantics.

### Recommended approach

Introduce an operation-scoped authorization cache keyed by:

- normalized path
- action (`read`, `list`)

This cache should live for the duration of one traversal call and avoid repeated scans of declared permission rules for the same subtree.

### Optional follow-up

If needed, move common path matching data into a precompiled matcher structure inside `PermissionManager`.

Examples:

- pre-normalized filesystem rules
- prefix trees for simple path scopes
- cached glob matchers

## 5. Argument Normalization

Tool argument parsing must be robust to tool-call JSON decoding.

### Required change

Introduce shared argument normalization helpers for common shapes:

- string arrays from `[]string`
- string arrays from `[]interface{}`
- ints from numeric or string-like tool arguments when appropriate

The first required migration target is `git_commit`.

## 6. Dead Code Removal

Remove or justify inactive runtime code.

Known cleanup targets from this review:

- unused `FileLock`
- spec fields stored on tools but never read
- transitional duplication that is no longer part of the intended runtime contract

---

## Code Areas

Primary implementation areas:

- `app/relurpish/runtime/runtime.go`
- `tools/execution.go`
- `tools/git.go`
- `tools/cli_nix/cli_command.go`
- `tools/lsp_process_client.go`
- `tools/files.go`
- `tools/search.go`
- `framework/runtime/permissions.go`
- `framework/runtime/`
- tests under `tools/`, `framework/runtime/`, and testsuite fixtures

Documentation areas:

- `docs/dev/tools.md`
- `docs/permission-model.md`
- `docs/architecture.md`
- testsuite manifests that encode old tool expectations

---

## Rollout Phases

### Phase 1: Correctness and Shared Utilities

Deliverables:

- shared argument normalization helpers
- git file-list fix
- tests covering normalized array inputs

Acceptance:

- explicit git file lists are honored for both `[]string` and `[]interface{}`

### Phase 2: Shared Command Authorization

Deliverables:

- new shared authorization helper
- migrated git, CLI, exec, and LSP wrappers
- normalized approval action model

Acceptance:

- equivalent commands receive equivalent authorization treatment

### Phase 3: Tool Surface Consolidation

Deliverables:

- runtime registration cleanup
- unified recursive grep/search tool
- compatibility adjustments in testsuite and docs

Acceptance:

- duplicate first-class tool paths are removed from default runtime wiring

### Phase 4: Performance Work

Deliverables:

- traversal authorization caching
- at least one benchmark or stress-oriented regression test
- measured before/after behavior on large trees

Acceptance:

- file-list and recursive search operations show materially reduced repeated permission work

### Phase 5: Post-Cleanup Documentation

Deliverables:

- updated tool reference docs
- updated permission/approval docs
- migration note for renamed or removed tools if needed

Acceptance:

- docs reflect the active runtime, not historical overlap

---

## Testing Requirements

Required automated coverage:

- git commit with explicit `files` passed as `[]interface{}`
- shared authorization helper behavior:
  - allow
  - deny
  - ask with approval
  - ask with missing permission manager
- unified search tool behavior:
  - generated-directory skipping
  - case-sensitive and case-insensitive matching
  - permission-aware subtree skipping
- traversal-cache correctness:
  - no skipped allowed files
  - no access to denied files

Recommended additional coverage:

- benchmarks for large-tree list/search traversal
- approval cache-key stability tests
- registration tests asserting no duplicate primary feature tools are present

---

## Risks and Mitigations

### Risk: tests and prompts depend on old tool names

Mitigation:

- update testsuite cases and manifests in the same change set
- do not keep compatibility aliases in the default runtime

### Risk: generic tools still matter for language-agnostic workflows

Mitigation:

- keep them as explicit fallback tools, not co-equal defaults

### Risk: approval-key normalization changes user-visible behavior

Mitigation:

- document the change
- add tests around approval reuse and cache keys

### Risk: permission caching introduces correctness bugs

Mitigation:

- scope cache to one operation
- key by normalized path and action
- add deny/allow subtree regression tests

---

## Resolved Product Decisions

The following decisions are resolved for implementation:

1. Generic execution tools are removed from default runtime registration entirely.
2. Heuristic repo-wide tools such as `search_find_similar` and `search_semantic` remain on the main tool surface.
3. Cleanup is a hard cutover with no compatibility alias period in the default runtime.
