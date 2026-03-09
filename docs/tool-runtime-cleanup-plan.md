# Tool Runtime Cleanup Plan

## Synopsis

This plan addresses the cleanup, duplication, correctness, and performance issues identified in the current `framework/` and `tools/` runtime surface.

Current date of this plan: March 9, 2026.

The work is centered on four outcomes:

- remove duplicate tool execution paths for the same user-facing job
- centralize command authorization and approval behavior
- reduce high-cost recursive filesystem permission checks
- tighten correctness around tool argument handling and registration

---

## Goals

- Keep one primary tool path per feature area.
- Preserve the capability registry and manifest-driven permission model.
- Reduce model confusion by narrowing overlapping tools.
- Improve large-repo performance for file listing and search.
- Remove dead code and migration leftovers that no longer carry their weight.

---

## Non-Goals

- No redesign of the capability registry.
- No broad UI changes.
- No changes to the sandbox runtime contract beyond authorization plumbing.
- No removal of language-aware tools in favor of raw shell wrappers.

---

## Workstreams

### 1. Execution Tool Consolidation

Problem:

- Generic execution tools (`exec_run_tests`, `exec_run_build`, `exec_run_linter`) overlap with language-aware tools such as `go_test`, `go_build`, `python_pytest`, and `rust_cargo_test`.

Plan:

- define language-aware tools as the default verification/build surface
- narrow generic execution tools to explicit fallback or compatibility-only use
- remove duplicate registration from runtime wiring where a better language-aware tool already exists
- update testsuite expectations and policy docs to prefer the language-aware names

Acceptance:

- one obvious test/build tool path exists per language
- runtime does not register both generic and language-specific tools for the same primary workflow without an explicit reason

### 2. Search Tool Consolidation

Problem:

- `file_search` and `search_grep` both perform recursive substring search over workspace files

Plan:

- merge these into a single grep-style implementation
- support case sensitivity and directory selection in one tool
- keep higher-cost heuristic tools only if they have a justified product role
- otherwise demote or remove `search_find_similar` and `search_semantic`

Acceptance:

- one primary recursive text-search tool remains
- search documentation and tests reference the unified surface

### 3. Shared Command Authorization

Problem:

- command authorization is reimplemented in multiple wrappers with different approval action names and slightly different behavior

Plan:

- introduce a shared runtime helper for:
  - executable permission checks
  - bash allow/deny pattern evaluation
  - approval request generation
  - consistent approval cache keys
- refactor `CommandTool`, git tools, execution tools, and process LSP startup to use the shared helper

Acceptance:

- all command-bearing tools route through one authorization path
- equivalent commands share consistent approval semantics

### 4. Filesystem Walk Performance

Problem:

- list/search tools perform per-path permission checks and logging during recursive walks

Plan:

- add memoization for directory and path authorization decisions during a single walk
- avoid repeated normalization and repeated permission-rule scans for the same subtree
- evaluate a directory-scope allow/skip decision before visiting children
- reduce audit verbosity for repeated allow decisions generated during internal traversal

Acceptance:

- file traversal tools do less repeated work on large trees
- permission correctness remains unchanged

### 5. Correctness and Dead Code Cleanup

Problem:

- `git_commit` handles `files` incorrectly when tool-call JSON decodes to `[]interface{}`
- unused helpers and spec fields remain in active code

Plan:

- normalize array argument parsing for git and similar tools
- remove unused `FileLock` unless a real write-serialization use case is introduced
- remove unused manifest/spec fields from tools that do not consume them
- add focused tests for argument normalization and registration behavior

Acceptance:

- explicit file lists for git commit are honored
- dead code identified in review is removed or justified

### 6. Context Clone Cost Review

Problem:

- graph execution clones `core.Context` for parallel branches using gob-based deep copy

Plan:

- instrument or benchmark clone cost on realistic contexts
- determine whether graph parallelism is frequent enough to justify optimization now
- if needed, move to a cheaper snapshot strategy for branch execution

Acceptance:

- clone cost is measured, not guessed
- either optimization lands or the current approach is explicitly deferred with data

---

## Delivery Phases

### Phase 1: Correctness First

Work:

- fix git `files` argument normalization
- add tests for `[]interface{}` and `[]string` handling
- remove obviously dead code with no runtime references

### Phase 2: Consolidate Runtime Surfaces

Work:

- consolidate execution tool registration
- consolidate recursive search tools
- update runtime wiring and testsuite references

### Phase 3: Centralize Authorization

Work:

- add shared authorization helper
- migrate command-bearing tool wrappers to it
- normalize approval action naming

### Phase 4: Performance Pass

Work:

- optimize permission checks during recursive walks
- add benchmarks or at least deterministic large-tree tests
- review clone cost and decide whether to optimize now

### Phase 5: Documentation and Rollout

Work:

- update tool docs
- update permission-model docs where approval semantics changed
- document deprecated or removed tool names

---

## Risks

- testsuites and prompts may currently rely on old tool names
- some generic tools may still be needed for language-agnostic or fallback workflows
- approval-key changes can alter existing HITL behavior and should be treated as a user-visible contract change

---

## Recommended Implementation Order

1. Fix `git_commit` argument parsing.
2. Add shared command authorization helper.
3. Move git, CLI, execution, and LSP process wrappers onto the helper.
4. Consolidate duplicate execution and search tool registration.
5. Optimize recursive walk permission checking.
6. Clean remaining dead code and update docs/tests.
