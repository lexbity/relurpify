# Tool Runtime Cleanup Implementation Plan

## Synopsis

This document turns the approved cleanup direction into an execution plan for implementation.

Current date of this plan: March 9, 2026.

This is a hard-cutover cleanup. The end state is:

- generic execution tools removed from default runtime registration
- language-aware verification/build tools used as the default execution surface
- one primary recursive grep-style search implementation
- shared command authorization across command-bearing tools
- optimized recursive traversal permission checks

---

## Approved Decisions

- Remove generic execution tools from default runtime registration entirely.
- Keep `search_find_similar` and `search_semantic` on the main tool surface.
- Use a hard cutover with no compatibility alias layer in the default runtime.

---

## Implementation Strategy

Work in this order:

1. fix correctness bugs that can change behavior immediately
2. centralize shared authorization and argument normalization
3. remove duplicate default runtime registrations
4. consolidate duplicate grep-style search paths
5. optimize file traversal permission checks
6. update testsuite/docs to match the new runtime contract

This order reduces the chance of mixing behavioral changes with structural refactors in one step.

---

## Phase 1: Correctness Foundation

## Goals

- eliminate known correctness bugs before refactoring surfaces
- add shared helpers that later phases can reuse

## Tasks

### 1.1 Add shared argument normalization helpers

Create shared helpers for:

- `[]string`
- `[]interface{} -> []string`
- optional scalar coercions where tool-call payloads vary in shape

Likely locations:

- `tools/` shared helper file, or
- `framework/tools/` if intended for broader reuse

### 1.2 Fix git commit file handling

Update `GitCommandTool` to normalize `files` robustly before staging.

Files:

- [git.go](/home/lex/Public/Relurpify/tools/git.go)

### 1.3 Remove obvious dead code

Remove inactive code that is clearly not part of current runtime behavior.

Initial targets:

- unused `FileLock`
- unused stored `spec` fields in tools that never consume them

Files:

- [files.go](/home/lex/Public/Relurpify/tools/files.go)

## Acceptance

- `git_commit` honors explicit file lists regardless of decoded JSON array type
- dead-code cleanup lands without changing visible runtime behavior

---

## Phase 2: Shared Command Authorization

## Goals

- eliminate duplicated command-authorization logic
- normalize approval and cache-key behavior

## Tasks

### 2.1 Introduce shared authorization helper

Add a helper in `framework/runtime/` that:

- checks executable permission
- evaluates bash allow/deny/default
- requests approval when required
- emits a normalized approval action namespace

Suggested target:

- new file near [permissions.go](/home/lex/Public/Relurpify/framework/runtime/permissions.go)

### 2.2 Migrate all command-bearing wrappers

Refactor the following consumers onto the shared helper:

- [execution.go](/home/lex/Public/Relurpify/tools/execution.go)
- [git.go](/home/lex/Public/Relurpify/tools/git.go)
- [cli_command.go](/home/lex/Public/Relurpify/tools/cli_nix/cli_command.go)
- [lsp_process_client.go](/home/lex/Public/Relurpify/tools/lsp_process_client.go)

### 2.3 Normalize approval action naming

Replace wrapper-specific approval namespaces with one shared command-execution action family plus metadata about the source wrapper if needed for observability.

## Acceptance

- equivalent commands receive equivalent authorization treatment regardless of wrapper
- no duplicated bash-pattern enforcement remains in tool wrappers

---

## Phase 3: Runtime Surface Consolidation

## Goals

- remove duplicate default execution paths
- reduce tool-choice ambiguity in the runtime

## Tasks

### 3.1 Remove generic execution tools from default runtime registration

Update capability registry wiring to stop registering:

- `exec_run_tests`
- `exec_run_linter`
- `exec_run_build`
- `exec_run_code` if it is part of the generic execution bundle and should no longer be default-exposed

This should be confirmed during implementation against actual runtime needs, but the default runtime should not expose the generic verification/build tools after this phase.

Primary file:

- [runtime.go](/home/lex/Public/Relurpify/app/relurpish/runtime/runtime.go)

### 3.2 Verify language-aware coverage

Ensure the remaining language-aware tools cover the default coding flows for:

- Go
- Python
- Rust
- Node
- SQLite where applicable

Primary files:

- [cli_registry.go](/home/lex/Public/Relurpify/tools/cli_registry.go)
- [go_tools.go](/home/lex/Public/Relurpify/tools/go_tools.go)
- [python_tools.go](/home/lex/Public/Relurpify/tools/python_tools.go)
- [rust_tools.go](/home/lex/Public/Relurpify/tools/rust_tools.go)
- [node_tools.go](/home/lex/Public/Relurpify/tools/node_tools.go)
- [sqlite_tools.go](/home/lex/Public/Relurpify/tools/sqlite_tools.go)

### 3.3 Update testsuite/tool policy expectations

Replace old generic execution tool expectations with the new active tool names.

Likely areas:

- `testsuite/agenttests/`
- skill policy tests
- agent capability tests

## Acceptance

- default runtime no longer registers duplicate generic verification/build tools
- tests and manifests reflect the new runtime contract

---

## Phase 4: Search Consolidation

## Goals

- reduce overlapping grep-style search tools to one primary implementation
- keep heuristic search tools on the main surface

## Tasks

### 4.1 Choose primary grep implementation

Pick one of:

- `file_search`
- `search_grep`

as the surviving primary recursive text-search tool.

The recommended direction is to keep the implementation with better parameter naming and file-tool integration, then fold the other behavior into it.

### 4.2 Merge missing features

The primary tool should support:

- recursive walk
- generated-directory skipping
- bounded scanning
- directory scoping
- case sensitivity option
- permission-aware traversal

### 4.3 Remove duplicate registration and references

Update runtime registration, docs, and tests to reflect the surviving tool name.

Files:

- [files.go](/home/lex/Public/Relurpify/tools/files.go)
- [search.go](/home/lex/Public/Relurpify/tools/search.go)
- [runtime.go](/home/lex/Public/Relurpify/app/relurpish/runtime/runtime.go)

### 4.4 Retain heuristic tools with guardrails

Keep:

- `search_find_similar`
- `search_semantic`

but add:

- binary/file-size guards
- clearer descriptions that they are heuristic

## Acceptance

- one primary recursive grep-style tool remains in the default runtime
- heuristic tools remain exposed and documented honestly

---

## Phase 5: Traversal Performance

## Goals

- reduce repeated permission evaluation during recursive walks
- preserve exact permission semantics

## Tasks

### 5.1 Add operation-scoped traversal authorization cache

Memoize permission decisions by:

- normalized path
- action

for one list/search operation at a time.

### 5.2 Reduce redundant logging during internal traversal

Avoid success-log noise for repeated internal allow decisions where a coarser traversal-level record is sufficient.

### 5.3 Apply optimization to affected tools

Primary targets:

- `file_list`
- primary grep-style search tool
- any remaining recursive heuristic workspace scans

Files:

- [files.go](/home/lex/Public/Relurpify/tools/files.go)
- [search.go](/home/lex/Public/Relurpify/tools/search.go)
- [permissions.go](/home/lex/Public/Relurpify/framework/runtime/permissions.go)

## Acceptance

- traversal tools avoid repeated rule scans for already-seen paths/subtrees
- no permission regression is introduced

---

## Phase 6: Documentation and Hard Cutover

## Goals

- align docs with the runtime as shipped
- remove references to old default surfaces

## Tasks

### 6.1 Update tool docs

Revise:

- [tools.md](/home/lex/Public/Relurpify/docs/dev/tools.md)

to match actual default runtime registration.

### 6.2 Update architecture and permission docs as needed

Revise:

- [architecture.md](/home/lex/Public/Relurpify/docs/architecture.md)
- [permission-model.md](/home/lex/Public/Relurpify/docs/permission-model.md)

where current text implies old tool/runtime behavior.

### 6.3 Remove outdated testsuite references

Because this is a hard cutover, old names should not remain in default testsuite expectations.

## Acceptance

- docs describe the active tool runtime, not the historical overlap
- testsuite no longer expects removed default tool names

---

## Suggested Change Sets

To keep reviews manageable, split the work into these PR-sized chunks:

1. Correctness helpers + git file handling + dead-code cleanup
2. Shared command authorization refactor
3. Default runtime registration cleanup
4. Search consolidation
5. Traversal performance optimization
6. Docs/testsuite hard-cutover cleanup

---

## Validation Plan

Required validation:

- `go test ./framework/... ./tools/...`
- targeted tests for:
  - git file-list normalization
  - shared authorization helper behavior
  - runtime registration expectations
  - unified search-tool behavior
  - traversal permission caching behavior

Recommended:

- add one or more benchmarks for large recursive workspace traversal

---

## Immediate Next Step

Begin with Phase 1:

1. add shared argument normalization helpers
2. fix `git_commit` file-list handling
3. remove obvious dead code

That gives a safe first cleanup slice with low architectural risk and immediate payoff.
