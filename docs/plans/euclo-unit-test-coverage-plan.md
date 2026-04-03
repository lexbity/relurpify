# Euclo Unit Test Coverage Plan

## Purpose

This plan describes a phased approach to bring `named/euclo` unit test coverage from
its current baseline (~25% effective statement coverage) toward 100%.

The plan is organized by testability tier. Each phase targets a coherent group of
packages so work can be committed in slices without creating test debt in new areas.

## Baseline (as of 2026-04-02)

Overall codebase: **59.4%** statement coverage (`go test ./... -coverpkg=./...`)

`named/euclo` sub-package breakdown:

| Package | Coverage | Test Files |
|---|---|---|
| `relurpicabilities/archaeology` | 57.8% | 2 |
| `runtime/reporting` | 58.4% | 2 |
| `runtime/assurance` | 57.5% | 1 |
| `interaction/gate` | 43.7% | 1 |
| `relurpicabilities/local` | 32.8% | 6 |
| `relurpicabilities/debug` | 33.3% | 1 |
| `relurpicabilities/chat` | 28.7% | 1 |
| `runtime` | 27.5% | 5 |
| `named/euclo` (top-level) | 13.4% | 2 |
| `euclotypes` | 19.1% | 1 |
| `interaction/modes` | 9.9% | 2 |
| `execution/blackboard` | 8.0% | 1 |
| **21 packages** | **0.0%** | **0** |

Packages at 0% that are pure type aliases or forwarding shims and require no tests:
- `core/` (3 files: all type aliases and re-exports)

All other 0% packages have testable logic.

## Root Causes

Two distinct causes account for most of the gap:

1. **No test files written** — 21 packages have source code but zero `_test.go`
   files. These were built using live agent test runs as the primary
   validation path.

2. **Partial test files** — Packages with tests cover specific entry points but
   leave most source files in the package untouched. For example,
   `interaction/modes` has 25 source files and only 2 test files.

## Testing Conventions

All new tests must follow the existing conventions:

- Use `testutil/euclotestutil.Env(t)` / `EnvMinimal(t)` for environment setup
- Use `euclotestutil.StubModel` for deterministic no-op LLM responses
- Use `euclotestutil.NoopExecutor`, `ErrorModel`, `EchoTool` for workflow stubs
- Define struct-based local mocks in the test file (not in shared test helpers)
  unless the mock will be reused across multiple test files
- Table-driven tests for state/condition validation
- No live Ollama calls in unit tests

## Phase Structure

| Phase | Focus | Packages | Estimated New Tests |
|---|---|---|---|
| 1 | Pure logic — no stubs needed | 7 packages | ~60 |
| 2 | Interaction layer | 3 packages | ~70 |
| 3 | Execution wrappers + dispatch | 10 packages | ~45 |
| 4 | Coverage gaps in existing test files | 7 packages | ~80 |
| 5 | Runtime orchestration + restore | 4 packages | ~40 |

---

## Phase 1: Pure Logic — No Stubs Needed

### Goal

Cover all packages whose logic is self-contained: no LLM calls, no filesystem,
no external services. All dependencies are pure Go types or injectable interfaces
already available in the test environment.

### Packages

#### `named/euclo/capabilities/`

Single file: `registry.go`

- `NewDefaultCapabilityRegistry(env)` — registers 17 capabilities
- `Register(cap)` — handles nil and empty ID gracefully
- `Lookup(id)` — returns capability or nil
- `ForProfile(profileID)` — filters + sorts by ID
- `supportsProfile(cap, profileID)` — nested type assertion on annotations

**Test file:** `capabilities/registry_test.go`

Key cases:
- Register nil capability does not panic
- Register capability with empty ID does not panic
- Lookup unknown ID returns nil
- Lookup registered ID returns correct capability
- ForProfile returns only capabilities matching profile, sorted by ID
- ForProfile with unknown profile returns empty slice
- Default registry contains expected capability IDs
- Concurrent Register+Lookup does not race (use `go test -race`)

#### `runtime/transitions/`

Single file: `transitions.go`

- Transition eligibility rules between modes and phases

**Test file:** `runtime/transitions/transitions_test.go`

Key cases:
- Valid transition from each mode returns expected next state
- Invalid transition returns error or sentinel
- Terminal mode has no outgoing transitions
- All transition paths reachable from initial state

#### `runtime/work/`

Single file: `work.go`

- Unit of work assembly: maps capability + task into a work descriptor

**Test file:** `runtime/work/work_test.go`

Key cases:
- Work assembled from capability + task has expected fields
- Missing capability returns error
- Missing task type returns error
- Executor descriptor fields are populated correctly

#### `runtime/context/`

Two files: context adapter between `framework/core.Context` and euclo runtime state.

**Test file:** `runtime/context/context_test.go`

Key cases:
- Adapter reads state keys set by framework context
- Adapter writes keys visible to subsequent framework context reads
- Nil context handled without panic

#### `execution/behavior.go`

Recipe routing: maps capability ID + task type to a `RecipeSpec`.

**Test file:** `execution/behavior_test.go`

Key cases:
- Each of the 19 `RecipeID` constants maps to a non-nil `RecipeSpec`
- Unknown capability ID returns error
- Recipe spec for `chat.implement.architect` has correct executor kind
- Recipe spec for `debug.investigate` has correct executor kind
- Task type override is applied when recipe specifies one

#### `relurpicabilities/` (top-level)

Two files: registration helpers that wire sub-package capabilities into the
capability registry.

**Test file:** `relurpicabilities/registry_test.go`

Key cases:
- Default registration produces non-empty registry
- Each sub-family (chat, debug, archaeology, local) contributes at least
  one capability
- No capability ID is registered twice

#### `euclotypes/` (gap fill)

Current coverage: 19.1%. Uncovered functions:
- `PersistWorkflowArtifacts(state, store)`
- `LoadPersistedArtifacts(store, workflowID)`
- Artifact kind filtering
- State restoration from persisted artifact set

**Test file:** `euclotypes/types_test.go` (extend existing)

Key cases:
- Persist then load round-trips all artifact kinds
- Load on empty store returns empty result
- Kind filter excludes non-matching artifacts
- State restore re-populates expected context keys

### Phase 1 Success Criteria

- All 7 packages have at least one `_test.go` file
- Each package reaches ≥ 80% statement coverage
- `go test -race ./named/euclo/capabilities/... ./named/euclo/runtime/transitions/...
  ./named/euclo/runtime/work/... ./named/euclo/runtime/context/...
  ./named/euclo/execution/ ./named/euclo/relurpicabilities/
  ./named/euclo/euclotypes/` passes clean

### Recommended Commit Slices

1. `capabilities/registry_test.go`
2. `runtime/transitions/transitions_test.go` + `runtime/work/work_test.go`
3. `runtime/context/context_test.go`
4. `execution/behavior_test.go`
5. `relurpicabilities/registry_test.go`
6. `euclotypes/types_test.go` (extended)

---

## Phase 2: Interaction Layer

### Goal

Cover the `interaction/` package tree, which has 15 source files and zero test
files. All logic here is pure state machine, string matching, and data
transformation — no LLM required.

### Packages

#### `interaction/` (top-level, 15 files)

Key sub-areas:

**Agency resolution** (`agency.go`)
- `AgencyResolver.Resolve(mode, userText)` — exact then fuzzy substring match

**Phase machine** (`machine.go`)
- `PhaseMachine.Execute(ctx, phase, input)` — runs phase handler, emits frames

**Budget tracking** (`budget.go`)
- Token + interaction budget increment and exhaustion checks

**Interaction recording** (`recording.go`)
- Append-only log of frames; replay from recorded state

**Session management** (`session.go`)
- Session ID generation, active session tracking

**Transition logic** (`transitions.go`)
- Mode-level transition rules (separate from `runtime/transitions`)

**Test file:** `interaction/agency_test.go`, `interaction/machine_test.go`,
`interaction/budget_test.go`, `interaction/recording_test.go`

Key cases (agency):
- Exact match on known phrase returns correct capability invocation
- Fuzzy match on substring returns correct capability invocation
- Unknown phrase returns nil/not-found
- Mode scoping: phrase registered in debug mode does not match in chat mode

Key cases (machine):
- Phase executes handler and emits output frame
- Phase with skip condition skips and returns empty frame
- Phase failure propagates error
- Phase machine executes sequence in order

Key cases (budget):
- Budget starts at zero
- Increment increases both token and interaction count
- Exhaustion check returns true when limit exceeded
- Reset clears counts

Key cases (recording):
- Append adds frame to log
- Replay returns frames in order
- Empty recording returns empty replay

#### `interaction/gate/` (gap fill)

Current coverage: 43.7%. Only TDD gates are tested.

**Test file:** `interaction/gate/gates_test.go` (extend existing)

Untested gate types:
- Edit gate (requires file change evidence)
- Review gate (requires review finding)
- Archaeology gate (requires exploration artifact)
- Gate failure reason population

Key cases:
- Edit gate passes when file change artifact is present
- Edit gate fails when no file change artifact exists
- Failure reason is non-empty on gate failure
- Gate with no required artifacts always passes

#### `interaction/modes/` (gap fill)

Current coverage: 9.9% across 25 source files. Two test files cover only
`review_phases` and `tdd_phases`.

Mode families to cover:
- `chat.go`, `chat_phases.go` — 2 files
- `code*.go` — 5 files
- `debug*.go` — 4 files
- `planning*.go` — 5 files
- `review*.go` — 2 files (partially tested)
- `tdd*.go` — 3 files (partially tested)

**Test files to add:**
- `interaction/modes/chat_phases_test.go`
- `interaction/modes/code_phases_test.go`
- `interaction/modes/debug_phases_test.go`
- `interaction/modes/planning_phases_test.go`

For each mode family, key cases follow the same pattern:
- Phase with complete input state produces expected output frame
- Phase with missing required state returns error or skips
- Phase skip condition evaluated correctly
- Content grouping/aggregation produces correct result
- All phase handlers in the family are reachable

### Phase 2 Success Criteria

- `interaction/` top-level reaches ≥ 75% coverage
- `interaction/gate/` reaches ≥ 80% coverage
- `interaction/modes/` reaches ≥ 70% coverage
- `go test -race ./named/euclo/interaction/...` passes clean

### Recommended Commit Slices

1. `interaction/agency_test.go`
2. `interaction/machine_test.go`
3. `interaction/budget_test.go` + `recording_test.go`
4. `interaction/gate/gates_test.go` (extended)
5. `interaction/modes/chat_phases_test.go`
6. `interaction/modes/code_phases_test.go`
7. `interaction/modes/debug_phases_test.go`
8. `interaction/modes/planning_phases_test.go`

---

## Phase 3: Execution Wrappers and Dispatch

### Goal

Cover the executor wrapper packages and the dispatch layer. These require
`StubModel` from `euclotestutil` to avoid live LLM calls, but are otherwise
straightforward to test.

### Packages

#### `execution/react/`, `execution/planner/`, `execution/htn/`,
`execution/rewoo/`, `execution/reflection/`, `execution/chainer/`,
`execution/pipe/`, `execution/architect/`

Each of these is a thin wrapper that:
1. Calls `New(env)` or `Execute(ctx, env, task, state, opts...)`
2. Delegates to an agent from `agents/`

**Test pattern** for each:
- `New(env)` with `euclotestutil.Env(t)` returns non-nil executor
- `Execute(ctx, ...)` with `StubModel` completes without error
- Nil environment panics or returns error (not silently corrupts)

**Test file per package:** e.g., `execution/react/react_test.go`

One test file per wrapper; 3-4 cases each.

#### `execution/` top-level — `executors.go`

- `SelectExecutor(factory, work)` — picks executor kind from work descriptor

**Test file:** `execution/executors_test.go`

Key cases:
- Work with React descriptor selects React executor
- Work with Planner descriptor selects Planner executor
- Work with HTN descriptor selects HTN executor
- Default (unknown descriptor) falls back to React executor
- Factory with nil environment returns error

#### `runtime/dispatch/`

- `NewDispatcher()` — registers 7 behaviors + support routines
- `Execute(ctx, in)` — dispatches to behavior by capability ID
- `ExecuteRoutine(ctx, routineID, ...)` — runs a support routine

**Test file:** `runtime/dispatch/dispatcher_test.go`

Key cases:
- Dispatcher registers expected capability IDs on construction
- Execute with known capability ID calls correct behavior stub
- Execute with unknown capability ID returns not-found error
- ExecuteRoutine with known routine ID invokes stub
- ExecuteRoutine with unknown routine ID returns error
- Dispatch is safe to call concurrently (use `-race`)

Use a local `stubBehavior` struct in the test file.

#### `runtime/policy/`

Three files: policy enforcement (security, shared context validation).

**Test file:** `runtime/policy/policy_test.go`

Key cases:
- Policy allows capability with correct trust class
- Policy denies capability with insufficient trust class
- Shared context validation passes when keys are present
- Shared context validation fails when required key is absent
- Policy is read-only safe (calling Evaluate does not mutate policy)

#### `execution/blackboard/` (gap fill)

Current coverage: 8.0%. Existing test covers only type construction.

**Test file:** `execution/blackboard/knowledge_sources_test.go` (extend)

Key cases:
- `KnowledgeSourceExecutor.Execute()` with stub knowledge sources
- Knowledge source prioritization (higher-priority source wins conflict)
- Bridge populates workflow state from blackboard output
- Empty knowledge source list returns empty result

### Phase 3 Success Criteria

- All 8 wrapper packages reach ≥ 70% coverage
- `execution/` top-level reaches ≥ 75% coverage
- `runtime/dispatch/` reaches ≥ 80% coverage
- `runtime/policy/` reaches ≥ 80% coverage
- `execution/blackboard/` reaches ≥ 70% coverage
- `go test -race ./named/euclo/execution/... ./named/euclo/runtime/dispatch/...
  ./named/euclo/runtime/policy/...` passes clean

### Recommended Commit Slices

1. Wrapper tests: `react`, `planner`, `htn` (one commit)
2. Wrapper tests: `rewoo`, `reflection`, `chainer`, `pipe`, `architect` (one commit)
3. `execution/executors_test.go`
4. `runtime/dispatch/dispatcher_test.go`
5. `runtime/policy/policy_test.go`
6. `execution/blackboard/knowledge_sources_test.go` (extended)

---

## Phase 4: Coverage Gaps in Existing Test Files

### Goal

Raise coverage in packages that already have tests but are well below 70%.
This phase adds cases to existing `_test.go` files rather than creating new ones.

### Packages

#### `relurpicabilities/chat/` (28.7% → target 80%)

Three source files: `chat.go`, `routines.go`, `behavior.go`

Uncovered paths in `behavior_test.go`:
- Ask behavior: inquiry phase, options phase, review phase
- Inspect behavior: interface extraction, review handoff
- Implement behavior: pre-edit analysis, post-edit verification handoff
- Supporting routines: local review, targeted verification repair

#### `relurpicabilities/debug/` (33.3% → target 80%)

Six source files: `debug.go`, `pipeline_stages.go`, `investigate_regression.go`,
`routines.go`, `helpers.go`, `behavior.go`

Uncovered paths:
- Regression investigation behavior
- Pipeline stage construction (stage ordering, contract names)
- Repair routine invocation
- Helper: localization evidence from stack trace

#### `relurpicabilities/archaeology/` (57.8% → target 85%)

Three source files: `archaeology.go`, `scenario.go`, `behavior.go`

Uncovered paths:
- Tension reasoning output format
- Plan compilation with conflicting knowledge entries
- Scope expansion assessment output

#### `relurpicabilities/local/` (32.8% → target 80%)

Fifteen source files; 6 test files exist but cover only primary paths.

Uncovered paths:
- Migration behavior
- Design behavior
- Profile selection with multiple candidates
- Trace behavior
- Refactor behavior

Add test files:
- `relurpicabilities/local/migration_test.go`
- `relurpicabilities/local/design_test.go`
- `relurpicabilities/local/profile_selection_test.go`

#### `runtime/` top-level (27.5% → target 75%)

Nineteen source files; 5 test files. Untested file:
- `contracts.go` — capability contract enforcement

Key cases for `contracts.go`:
- Contract for read-only capability rejects file mutation
- Contract for read-write capability accepts file mutation
- Deferred issue is constructed with correct fields
- Multiple constraint violations each produce a deferred issue

#### `runtime/assurance/` (57.5% → target 85%)

One source file; 16 tests exist. Remaining gaps:
- Checkpoint callback hooks (pre/post checkpoint invocation)
- Deferred assurance with partial completion
- Assurance with all checkpoints waived

#### `runtime/reporting/` (58.4% → target 85%)

Five source files; 2 test files. Gaps:
- Chat mode runtime reporting
- Debug mode runtime reporting
- State compaction (reducing large state for reporting)
- Artifact finalization (marking artifacts as finalized in state)

#### `named/euclo` top-level (13.4% → target 60%)

Nine source files; 2 test files. The `Agent.Execute()` method is large
(~1500 loc) and orchestrates everything. Do not attempt to fully cover
`Execute()` in this phase — use integration-level tests in Phase 5.

Testable in isolation:
- `InitializeEnvironment(env)` — already has 1 test; add edge cases
- `RefreshRuntimeExecutionArtifacts(state)` — already has 3 tests; add
  repair-exhausted, deferral with evidence, blocked-with-timeout cases
- Mode + profile selection logic extracted from `Execute()` via helper functions
- Capability routing: given mode + task type, expected capability ID selected

### Phase 4 Success Criteria

- `relurpicabilities/chat/` ≥ 80%
- `relurpicabilities/debug/` ≥ 80%
- `relurpicabilities/archaeology/` ≥ 85%
- `relurpicabilities/local/` ≥ 80%
- `runtime/` ≥ 75%
- `runtime/assurance/` ≥ 85%
- `runtime/reporting/` ≥ 85%
- `named/euclo` top-level ≥ 60%

### Recommended Commit Slices

1. `relurpicabilities/chat/behavior_test.go` (extended)
2. `relurpicabilities/debug/behavior_test.go` (extended)
3. `relurpicabilities/archaeology/behavior_test.go` (extended)
4. `relurpicabilities/local/migration_test.go` + `design_test.go`
5. `relurpicabilities/local/profile_selection_test.go`
6. `runtime/contracts_test.go`
7. `runtime/assurance/assurance_test.go` (extended)
8. `runtime/reporting/` test files (extended)
9. `named/euclo/agent_init_test.go` + `agent_runtime_test.go` (extended)

---

## Phase 5: Runtime Orchestration and Restore

### Goal

Cover the hardest packages: orchestration, restore, and archaeomem. These
require more stubs and are closer to integration tests. The target is not
100% coverage in these packages — it is coverage of the core decision paths,
error branches, and recovery flows.

### Packages

#### `runtime/orchestrate/`

Five files: `controller.go`, `adapters.go`, `interactive.go`, `recovery.go`,
`interfaces.go`

This is the main execution controller. It wires together capability dispatch,
phase management, and result assembly.

**Test file:** `runtime/orchestrate/controller_test.go`

Strategy: test the controller via its `interfaces.go` surface using stubs for
every dependency. Do not test the full end-to-end path in this package — that
belongs in integration tests.

Key cases:
- Controller starts in `pending` state
- Successful dispatch transitions controller to `completed`
- Dispatch error transitions controller to `failed`
- Recovery flow triggered on `failed` state
- Interactive pause and resume round-trip
- Adapter correctly maps capability result envelope to internal result

#### `runtime/restore/`

Three files: restoration from persisted artifacts.

**Test file:** `runtime/restore/restore_test.go`

Strategy: use in-memory artifact store (already available via `euclotestutil`).

Key cases:
- Restore from empty store returns not-found
- Restore from store with matching workflow ID returns expected state
- Restore with corrupted artifact returns error
- Partial restore (some artifacts missing) returns partial state with warning

#### `runtime/archaeomem/`

Single file: wraps archaeo memory access.

**Test file:** `runtime/archaeomem/archaeomem_test.go`

Strategy: stub the underlying archaeo store using the pattern from
`relurpicabilities/archaeology/behavior_test.go` (`mockArchaeoAccess`).

Key cases:
- Lookup on empty store returns nil
- Lookup on populated store returns correct record
- Write followed by lookup returns written record
- Concurrent reads do not race

#### `named/euclo` top-level (60% → target 85%)

Complete coverage of the remaining `agent.go` paths not covered in Phase 4.
This requires the orchestration stubs built in this phase.

Key paths to cover via integration-style tests with `StubModel`:
- `Execute()` with chat mode task completes successfully
- `Execute()` with debug mode task completes successfully
- `Execute()` with planning mode task completes successfully
- `Execute()` with unknown mode returns error
- `Execute()` with cancelled context terminates cleanly

### Phase 5 Success Criteria

- `runtime/orchestrate/` ≥ 70%
- `runtime/restore/` ≥ 80%
- `runtime/archaeomem/` ≥ 80%
- `named/euclo` top-level ≥ 85%
- `go test -race ./named/euclo/...` passes clean

### Recommended Commit Slices

1. `runtime/restore/restore_test.go`
2. `runtime/archaeomem/archaeomem_test.go`
3. `runtime/orchestrate/controller_test.go`
4. `named/euclo/agent_runtime_test.go` (extended with orchestration stubs)

---

## Shared Test Utilities

The following utilities should be added to `testutil/euclotestutil/` when a
second test file needs them. Do not add them preemptively.

**`stubBehavior`** — needed when testing `runtime/dispatch/`
```go
type stubBehavior struct {
    calledWith BehaviorInput
    result     *BehaviorResult
    err        error
}
func (s *stubBehavior) Execute(ctx, in) (*BehaviorResult, error) { ... }
```

**`ArtifactBuilder`** — needed when Phase 4 tests construct complex artifact
sets repeatedly
```go
func BuildArtifact(kind ArtifactKind, content string) Artifact { ... }
```

**`StateBuilder`** — needed when Phase 5 tests construct pre-populated
context states
```go
func BuildState(keys map[string]any) *core.Context { ... }
```

Only promote a helper to `euclotestutil` if it is used in three or more
test files.

---

## Coverage Targets by Phase

| After phase | Target total `named/euclo` coverage |
|---|---|
| Baseline | ~25% effective |
| Phase 1 | ~40% |
| Phase 2 | ~55% |
| Phase 3 | ~65% |
| Phase 4 | ~80% |
| Phase 5 | ~90%+ |

The gap between 90% and 100% will be residual error branches and defensive
nil checks that are unreachable from normal test inputs. These are acceptable
to leave uncovered unless a specific bug surfaces there.

## Non-Goals

- This plan does not cover live agent tests (that is the separate
  `euclo-rapid-relurpic-coverage-plan.md`)
- This plan does not cover packages outside `named/euclo`
- This plan does not require mocking the LLM for any unit test — all tests
  must use `StubModel` or avoid the LLM path entirely
- This plan does not introduce a coverage enforcement gate (that can be added
  after Phase 3 lands)
