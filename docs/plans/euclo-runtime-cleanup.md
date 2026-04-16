# Euclo Runtime Cleanup — Engineering Specification

**Status:** Draft  
**Scope:** `named/euclo/` — resolves implementation gaps from phases 1–5 of `euclo-runtime-rework.md`  
**Constraint:** Each phase must leave `go build ./...` and `go test ./...` green at its exit

---

## Background

The five-phase runtime rework (`euclo-runtime-rework.md`) has been implemented through phase 4 with phase 5 in progress. Code review against the original exit criteria and the actual implementation found six categories of gaps:

1. **Key package fragmentation** — the plan specified one `runtime/state/` package; three were created (`runtime/state/`, `runtime/statekeys/`, `runtime/statebus/`) with duplicate key constants between `state` and `statekeys`.
2. **EucloExecutionState still partially untyped** — 8 `map[string]any` fields and 5 bare `any` fields remain in the typed state struct.
3. **Production write-site migration incomplete** — 77 files still contain raw `.Set("euclo.*")` / `.Get("euclo.*")` calls; the typed accessor layer exists but the sweep through `relurpicabilities/*/` and `runtime/orchestrate/` was not done.
4. **Phase 4 deprecated interfaces not deleted** — `execution.Behavior`, `euclorelurpic.SupportingRoutine`, `relurpicabilities/planning.PlanningBehavior`, and all `Behavior`→`Invocable` wrapping adapters still exist with tests, four copies of `convertInvokeSupportingToRunSupporting`, and the legacy `ExecuteInput.RunSupportingRoutine` field.
5. **Phase 2 classifier not deleted** — `runtime/capability_classifier.go` was supposed to be deleted after logic moved to `intake/`; it still exists and is wrapped by a `capabilityClassifierAdapter` in `intake/pipeline.go`.
6. **Phase 5 exit criteria not fully met** — `recipeResultToCoreResult` exists as a private function in `thoughtrecipes/invocable.go`; two state keys written in `dispatcher.go` (`euclo.sequence_step_N_completed`, `euclo.or_selected_capability`) are not in the registry; `ToDescriptor()` sets `RecipePath` to the plan name rather than the file path. None of the five CI enforcement scripts from the plan were created.

---

## Guiding Constraints

- No changes cross the `named/euclo/` boundary unless a type genuinely belongs in `framework/`.
- Each cleanup phase is a clean cut-over. Compatibility shims present within a phase to keep tests green must be removed before the phase exits.
- `go build ./...` and `go test ./...` must pass at every phase boundary.
- After Cleanup Phase 1 exits, no new raw state key strings may be introduced in `named/euclo/` without a corresponding entry in `runtime/state/keys.go`.

---

## Sequencing

```
Cleanup Phase 1 (State Package Consolidation + Typing)
    ↓  [state/keys.go is the settled authority; EucloExecutionState is fully typed]
    ├── Cleanup Phase 2 (Production Write-Site Migration)
    ├── Cleanup Phase 3 (Deprecated Interface Elimination)
    └── Cleanup Phase 4 (Classifier Deletion + Recipe Completion + Stale Comments)
            ↓  (all three can run in parallel after Phase 1)
Cleanup Phase 5 (CI Enforcement)
    ↓  [runs last; encodes all exit criteria as shell checks]
```

Phases 2, 3, and 4 are independent of each other and may be worked in parallel after Phase 1 completes.

---

## Cleanup Phase 1 — State Package Consolidation and Full Typing

### Goal

Collapse `runtime/statekeys/` into `runtime/state/` so the latter is the single import authority for all euclo state keys. Document the role of `runtime/statebus/` as a distinct, cycle-safe generic accessor utility (not a key registry). Complete the `EucloExecutionState` struct so every field is a typed Go type — no `map[string]any`, no bare `any`.

### Problem in Detail

**Key duplication.** `runtime/state/keys.go` and `runtime/statekeys/keys.go` define the same constants under the same string values. Callers in `agent.go`, `execution/behavior.go`, and several `relurpicabilities/` packages import `statekeys` directly because `runtime/state` had import cycle issues during implementation. The cause must be diagnosed before deleting `statekeys`.

**Diagnosing the import cycle.** `runtime/state/accessors.go` imports `runtime/session` (for `SessionResumeContext`). `runtime/session` imports `runtime` (the parent package). `runtime/state` also imports `runtime` (the parent package). Any package that imports both `runtime/state` and `runtime/session` indirectly is fine; but if `runtime/session` or `runtime` were to import `runtime/state`, that would be a cycle. The likely cause of `statekeys` being created is that some package in `relurpicabilities/` or `execution/` needs the key constants but can't import `runtime/state` because `runtime/state` imports from `runtime` which imports from those packages — creating a cycle. The fix is to break the cycle, not to maintain a duplicate package.

**Cycle resolution strategy.** Move the `SessionResumeContext` type used in `accessors.go` out of `runtime/session/resume.go` into `runtime/types.go` (where most runtime types already live), or expose it through `runtime` directly. This eliminates the `runtime/state → runtime/session` import, breaking the cycle. If the cycle still exists after that move, investigate further with `go list -f '{{.Imports}}' ./named/euclo/runtime/state/` before writing the fix.

**EucloExecutionState untyped fields.** The following fields in `runtime/state/state.go` need typed struct replacements:

| Field | Current type | Required type |
|-------|-------------|---------------|
| `Waiver` | `any` | `eucloruntime.ExecutionWaiver` (already defined in runtime/types.go) |
| `ProviderRestore` | `any` | `euclorestore.ProviderRestoreState` (check restore package for the right type) |
| `InteractionScript` | `any` | `[]eucloruntime.InteractionScriptEntry` (define if not exists) |
| `DeferralPlan` | `any` | `*guidance.DeferralPlan` |
| `SessionResumeContext` | `any` | `*euclosession.SessionResumeContext` |
| `VerificationSummary` | `map[string]any` | `eucloruntime.VerificationSummary` (define) |
| `ReviewFindings` | `map[string]any` | `eucloruntime.ReviewFindings` (define) |
| `RootCause` | `map[string]any` | `eucloruntime.RootCause` (define) |
| `RootCauseCandidates` | `map[string]any` | `eucloruntime.RootCauseCandidates` (define) |
| `RegressionAnalysis` | `map[string]any` | `eucloruntime.RegressionAnalysis` (define) |
| `PlanCandidates` | `map[string]any` | `eucloruntime.PlanCandidates` (define) |
| `EditIntent` | `map[string]any` | `eucloruntime.EditIntent` (define) |

For types not already defined in `runtime/types.go`, define minimal named structs there. They do not need to be rich at this stage; they just need to be typed. A struct with a `Data map[string]any` field is an improvement over a bare `map[string]any` at the call site because it is refactorable.

Pipeline state fields (`PipelineExplore`, `PipelineAnalyze`, `PipelinePlan`, `PipelineCode`, `PipelineVerify`, `PipelineFinalOutput`, `PipelineWorkflowRetrieval`) are already typed as `map[string]any` because the pipeline stage output shapes are not yet pinned — leave these as-is.

### What to Build

**Step 1 — Import cycle diagnosis.** Run `go list -f '{{.ImportPath}} imports {{.Imports}}' ./named/euclo/runtime/...` to map the actual import graph. Identify which packages import `runtime/state` and which could create a cycle. Document the cause in a comment at the top of `runtime/state/accessors.go`.

**Step 2 — Cycle fix.** Move `SessionResumeContext` out of `runtime/state/accessors.go`'s import chain if needed. Most likely: move the accessor for `KeySessionResumeContext` to use `any` with a typed cast wrapper, or change `accessors.go` to import only `runtime` (the parent) and not `runtime/session`. If the `SessionResumeContext` type is used in accessors, define a minimal interface or use the type from `runtime/types.go` directly.

**Step 3 — Delete `runtime/statekeys/`.** Update all importers to use `runtime/state` instead:
- `named/euclo/agent.go` (imports `statekeys`)
- `named/euclo/execution/behavior.go` (imports `statekeys`)
- Any other file importing `statekeys` (run `grep -r '".*statekeys"' named/euclo/`)

**Step 4 — Document `runtime/statebus/`.** `statebus` provides generic `Get[T]` and `Set[T]` helpers that are genuinely different from the per-field accessors in `runtime/state/accessors.go`. It stays. Add a package-level comment to `statebus/bus.go` that says: "Package statebus provides generic typed get/set helpers over core.Context. It is import-cycle safe and does not define key constants — use runtime/state for canonical key names." Remove any key constant usage from `statebus` itself if any exist.

**Step 5 — Type all untyped `EucloExecutionState` fields.** For each field in the table above: define the missing type in `runtime/types.go`, update `state.go`, update the corresponding getter/setter pair in `accessors.go`, update `LoadFromContext` and `FlushToContext` in `migration.go`. The accessors for previously-`any` fields should use a direct type assertion (same pattern as all other accessors).

### File Dependencies

**Deleted:**
- `named/euclo/runtime/statekeys/` — entire package

**Modified:**
- `named/euclo/runtime/state/state.go` — all `any` and `map[string]any` fields replaced
- `named/euclo/runtime/state/accessors.go` — updated accessors for newly-typed fields; import `runtime/session` dependency possibly removed
- `named/euclo/runtime/state/migration.go` — `LoadFromContext`/`FlushToContext` updated for newly-typed fields
- `named/euclo/runtime/types.go` — new typed structs for `VerificationSummary`, `ReviewFindings`, `RootCause`, `RootCauseCandidates`, `RegressionAnalysis`, `PlanCandidates`, `EditIntent`, `InteractionScriptEntry`
- `named/euclo/agent.go` — import `statekeys` → `runtime/state`
- `named/euclo/execution/behavior.go` — import `statekeys` → `runtime/state`
- All other importers of `statekeys`

### Unit Tests to Write

**`runtime/state/keys_test.go`** (add to existing)
- All key constants in `state/keys.go` are present in `statekeys/keys.go` before deletion (parity test — run once, then delete the statekeys file)
- After deletion: no test imports `statekeys`

**`runtime/state/state_test.go`** (new)
- `EucloExecutionState` has no fields of type `any` or `map[string]any` except the pinned pipeline fields — verified via `reflect.TypeOf` field scan
- Zero-valued `EucloExecutionState` passes `IsZero()` check

**`runtime/state/migration_test.go`** (extend existing)
- `LoadFromContext` correctly reads each newly-typed field (e.g., `Waiver`, `DeferralPlan`, `SessionResumeContext`)
- `FlushToContext` followed by `LoadFromContext` round-trips all newly-typed fields without loss

### Exit Criteria

1. `go build ./...` and `go test ./...` pass.
2. `runtime/statekeys/` directory does not exist.
3. No file in `named/euclo/` imports `runtime/statekeys`. (CI grep.)
4. `EucloExecutionState` has no fields typed `any` or `map[string]any` except the seven pipeline fields (`PipelineExplore` … `PipelineWorkflowRetrieval`). (CI grep on `state/state.go`.)
5. `runtime/statebus/bus.go` package comment documents its role as a cycle-safe generic utility.

---

## Cleanup Phase 2 — Production Write-Site Migration Completion

### Goal

Eliminate all remaining raw `.Set("euclo.*")` and `.Set("pipeline.*")` calls in production code. After this phase, the CI check `scripts/check-euclo-state-keys.sh` passes cleanly. Test files are exempt — they may use raw keys in arrange/assert steps where the goal is to verify state values, not to drive behavior.

### Problem in Detail

The sweep did not reach `relurpicabilities/*/` production files or `runtime/orchestrate/` because those packages have high line counts and many raw state accesses. Two specific keys in `runtime/dispatch/dispatcher.go` are not in the registry at all:

```go
// dispatcher.go, executeANDSequence — key not in registry
in.State.Set(fmt.Sprintf("euclo.sequence_step_%d_completed", i+1), capabilityID)

// dispatcher.go, executeORSequence — key not in registry
in.State.Set("euclo.or_selected_capability", capabilityID)
```

### What to Build

**Step 1 — Register the two missing keys.** Add to `runtime/state/keys.go`:

```go
KeySequenceStepCompleted  Key = "euclo.sequence_step_completed" // prefix; step N written as key+".N"
KeyORSelectedCapability   Key = "euclo.or_selected_capability"
```

Note: `KeySequenceStepCompleted` is a prefix used dynamically (`fmt.Sprintf("%s.%d", state.KeySequenceStepCompleted, i+1)`). Document this pattern in the constant's comment. The write in `dispatcher.go` becomes:

```go
in.State.Set(fmt.Sprintf("%s.%d", state.KeySequenceStepCompleted, i+1), capabilityID)
euclostate.SetORSelectedCapability(in.State, capabilityID)
```

Add `SetORSelectedCapability` and `GetORSelectedCapability` to `accessors.go`.

**Step 2 — Sweep `runtime/orchestrate/`.** Convert raw state reads/writes in:
- `runtime/orchestrate/interactive.go`
- `runtime/orchestrate/observability.go`
- `runtime/orchestrate/profile_engine.go`

Each file: replace each `.Set("euclo.*", v)` with the corresponding `euclostate.SetX(ctx, v)` call, and each `.Get("euclo.*")` with `euclostate.GetX(ctx)`. Use the typed form unless the key is a dynamic/prefix key.

**Step 3 — Sweep `runtime/compiled_execution.go`.** Apply the same conversion.

**Step 4 — Sweep `relurpicabilities/*/` production files.** Files affected (non-test):
- `relurpicabilities/bkc/bkc.go`
- `relurpicabilities/chat/behavior.go`, `chat/routines.go`
- `relurpicabilities/archaeology/behavior.go`
- `relurpicabilities/debug/behavior.go`, `debug/behavior_simple.go`, `debug/pipeline_stages.go`, `debug/routines.go`, `debug/investigate_regression.go`
- `relurpicabilities/local/deferrals_resolve.go`, `local/deferrals_surface.go`, `local/learning_promote.go`, `local/trace_analyze.go`, `local/verification_execution.go`, `local/failed_verification_repair.go`, `local/migration_execute.go`, `local/refactor_api_compatible.go`, `local/review_family.go`, `local/tdd_red_green_refactor.go`

For each: run `grep -n '\.Set("euclo\.\|\.Get("euclo\.' <file>` to enumerate write sites, then replace with typed accessors. If a key is used but not in the registry, add it.

**Missing keys discovered during sweep.** Any key found during step 4 that does not yet exist in `runtime/state/keys.go` must be added before the accessor is written. Keep a running list during the sweep. At phase exit, verify that no new raw strings remain.

### File Dependencies

**Modified:**
- `named/euclo/runtime/state/keys.go` — two new entries + any discovered during sweep
- `named/euclo/runtime/state/accessors.go` — new accessors for `ORSelectedCapability` + any discovered
- `named/euclo/runtime/dispatch/dispatcher.go` — two raw writes converted
- `named/euclo/runtime/orchestrate/interactive.go`
- `named/euclo/runtime/orchestrate/observability.go`
- `named/euclo/runtime/orchestrate/profile_engine.go`
- `named/euclo/runtime/compiled_execution.go`
- All 20 `relurpicabilities/*/` production files listed above

### Unit Tests to Write

**`runtime/dispatch/dispatcher_sequence_test.go`** (new)
- After `executeANDSequence` completes two steps, state contains `euclo.sequence_step_completed.1` and `euclo.sequence_step_completed.2` with the correct capability IDs
- After `executeORSequence`, state contains `euclo.or_selected_capability` with the selected ID
- Typed accessor `GetORSelectedCapability` reads back what `SetORSelectedCapability` wrote

**`runtime/state/accessors_test.go`** (extend)
- `GetORSelectedCapability` on empty context returns `""` and `false`
- `SetORSelectedCapability` → `GetORSelectedCapability` round-trips correctly

### Exit Criteria

1. `go build ./...` and `go test ./...` pass.
2. `scripts/check-euclo-state-keys.sh` passes: no raw `.Set("euclo.` or `.Get("euclo.` or `.Set("pipeline.` or `.Get("pipeline.` in any non-test `.go` file under `named/euclo/`.
3. `KeySequenceStepCompleted` and `KeyORSelectedCapability` exist in `runtime/state/keys.go`.
4. `SetORSelectedCapability` and `GetORSelectedCapability` exist in `runtime/state/accessors.go`.

---

## Cleanup Phase 3 — Deprecated Interface Elimination

### Goal

Delete `execution.Behavior`, `euclorelurpic.SupportingRoutine`, and `relurpicabilities/planning.PlanningBehavior`. Remove all wrapping invocables that delegate to an internal `Behavior` and replace them with direct `Invocable` implementations. Delete all copies of `convertInvokeSupportingToRunSupporting`, `convertInvokeInputToExecuteInput`, and `convertInvokeInputToRoutineInput`. Remove `ExecuteInput.RunSupportingRoutine`. Remove `BehaviorAsInvocable` from `execution/invocable.go`.

### Problem in Detail

The wrapping pattern is present in all four capability packages:

```go
// current pattern in chat/invocable.go (repeated in debug, archaeology, local)
type askInvocable struct {
    behavior execution.Behavior  // internal behavior, still alive
}
func (a *askInvocable) Invoke(ctx context.Context, in execution.InvokeInput) (*core.Result, error) {
    execInput := convertInvokeInputToExecuteInput(in)  // adapter call
    return a.behavior.Execute(ctx, execInput)           // delegates to old interface
}
```

`convertInvokeSupportingToRunSupporting` exists in four places:
1. `execution/invocable.go` (as `convertInvokeSupportingToRunSupporting`)
2. `runtime/dispatch/dispatcher.go` (as `convertInvokeSupportingToRunSupporting`)
3. `relurpicabilities/chat/invocable.go` (as local `convertInvokeSupportingToRunSupporting`)
4. `relurpicabilities/debug/invocable.go` (likely same)

`ExecuteInput.RunSupportingRoutine` has the signature:
```go
RunSupportingRoutine func(context.Context, string, *core.Task, *core.Context, eucloruntime.UnitOfWork, agentenv.AgentEnvironment, ServiceBundle) ([]euclotypes.Artifact, error)
```
It is populated only through the `convertInvokeSupportingToRunSupporting` adapter. Once all behaviors implement `Invocable` directly and use `in.InvokeSupporting` (from `InvokeInput`), this field is unused and can be deleted.

`PlanningBehavior` in `relurpicabilities/planning/behavior.go` wraps an `EucloCodingCapability` as a `Behavior`. It was originally used to register BKC capabilities in the behaviors map; the new dispatcher no longer uses it. Its tests (`behavior_test.go`) must be migrated to test the BKC invocables directly.

### What to Build

**Step 1 — Flatten each behavior into a direct Invocable.**

For each capability package (`chat`, `debug`, `archaeology`, `local`), the pattern is:

*Before* (`chat/invocable.go`, `askInvocable`):
```go
type askInvocable struct {
    behavior execution.Behavior
}
func (a *askInvocable) Invoke(ctx context.Context, in execution.InvokeInput) (*core.Result, error) {
    execInput := convertInvokeInputToExecuteInput(in)
    return a.behavior.Execute(ctx, execInput)
}
```

*After* — merge the invocable and behavior into one type, `AskInvocable`, that directly holds what the old behavior held:
```go
type AskInvocable struct {
    // fields previously on askBehavior, e.g.:
    // registry capability.Registry
}
func (a *AskInvocable) ID() string       { return euclorelurpic.CapabilityChatAsk }
func (a *AskInvocable) IsPrimary() bool  { return true }
func (a *AskInvocable) Invoke(ctx context.Context, in execution.InvokeInput) (*core.Result, error) {
    // body of old askBehavior.Execute, using in.InvokeSupporting directly
    // instead of in.RunSupportingRoutine
}
```

The old `askBehavior` type and its `Execute` method are deleted. `NewAskInvocable` returns `*AskInvocable`.

For supporting routines (`directEditExecutionInvocable`, etc.), the same pattern applies: merge the routine struct into the invocable, delete `convertInvokeInputToRoutineInput`, and call `in.InvokeSupporting` directly where needed.

**Step 2 — Delete `convertInvokeSupportingToRunSupporting` from all four locations.**

Once `ExecuteInput.RunSupportingRoutine` is no longer populated by anything, delete the field from `ExecuteInput`. Then delete the four copies of the adapter function. Build will fail on any remaining caller — fix each caller to use `in.InvokeSupporting` directly.

**Step 3 — Delete `BehaviorAsInvocable` from `execution/invocable.go`.**

After all behaviors implement `Invocable` directly, `BehaviorAsInvocable` has no callers. Delete it and its constructor `NewBehaviorAsInvocable`.

**Step 4 — Delete `execution.Behavior` interface.**

After step 1 and 3, `execution.Behavior` has no implementors and no callers. Delete the `Behavior` interface from `execution/behavior.go`. If other types or tests reference it, they are now broken and must be updated. `ExecuteInput` is also cleaned up: with `RunSupportingRoutine` gone, `ExecuteInput` may or may not still be needed (the dispatcher already converts to `InvokeInput`). Check if `ExecuteInput` itself is still used at call sites and, if not, delete it too — its only role was as the `Behavior.Execute` parameter.

**Step 5 — Delete `euclorelurpic.SupportingRoutine` interface.**

`relurpicabilities/routines.go` defines `SupportingRoutine` and `RoutineInput`. After step 1, no type implements `SupportingRoutine` (the routine logic is folded into the invocable directly). Delete `SupportingRoutine`, `RoutineInput`, and `WorkContext` from `relurpicabilities/routines.go`. Remove `convertInvokeInputToRoutineInput` from all invocable files.

**Step 6 — Delete `PlanningBehavior`.**

Delete `relurpicabilities/planning/behavior.go`. Migrate `relurpicabilities/planning/behavior_test.go` to test BKC invocables directly (e.g., `bkccap.NewCompileInvocable(env).Invoke(...)` with a mock env). Delete the `planning` sub-package if it only contained `PlanningBehavior`.

**Step 7 — Clean up `dispatcher.go`.**

Remove `convertInvokeSupportingToRunSupporting` from `dispatcher.go`. Remove `convertExecuteInputToInvokeInput` — if `ExecuteInput` is deleted (step 4), this adapter is also deleted. If `ExecuteInput` is kept for other callers, keep the converter.

### File Dependencies

**Deleted:**
- `named/euclo/relurpicabilities/planning/behavior.go`
- `named/euclo/relurpicabilities/planning/behavior_test.go` (replaced by bkc invocable tests)
- `named/euclo/relurpicabilities/planning/` (if empty after deletions)
- `BehaviorAsInvocable` type from `execution/invocable.go`
- `convertInvokeSupportingToRunSupporting` from `execution/invocable.go`, `dispatcher.go`, `chat/invocable.go`, `debug/invocable.go`
- `convertInvokeInputToExecuteInput` from all invocable files
- `convertInvokeInputToRoutineInput` from all invocable files
- `SupportingRoutine` interface, `RoutineInput`, `WorkContext` from `relurpicabilities/routines.go`
- `Behavior` interface from `execution/behavior.go`
- `ExecuteInput.RunSupportingRoutine` field

**Modified:**
- `named/euclo/relurpicabilities/chat/invocable.go` — all invocables flattened
- `named/euclo/relurpicabilities/debug/invocable.go` — all invocables flattened
- `named/euclo/relurpicabilities/archaeology/invocable.go` — all invocables flattened
- `named/euclo/relurpicabilities/local/invocable.go` — all invocables flattened
- `named/euclo/relurpicabilities/bkc/invocable.go` — verify already direct (BKC never had the wrapping pattern)
- `named/euclo/execution/behavior.go` — `Behavior` deleted; `ExecuteInput` updated or deleted
- `named/euclo/execution/invocable.go` — `BehaviorAsInvocable` deleted
- `named/euclo/runtime/dispatch/dispatcher.go` — adapter functions deleted

**Existing tests to update:**
- `relurpicabilities/chat/behavior_test.go` — tests of `askBehavior`, `inspectBehavior`, etc. migrate to test the flattened invocable directly
- `relurpicabilities/archaeology/behavior_test.go` — same migration
- `relurpicabilities/debug/behavior_test.go` — same migration
- `execution/behavior_test.go` — tests of `Behavior` contract migrate to `Invocable` contract

### Unit Tests to Write

**`relurpicabilities/chat/invocable_test.go`** (replace behavior_test.go)
- `NewAskInvocable()`: `ID()` returns `"euclo:chat.ask"`, `IsPrimary()` returns `true`
- `AskInvocable.Invoke` with a nil task returns a structured error, not a panic
- `AskInvocable.Invoke` calls `InvokeSupporting` when it needs a supporting artifact (mock the function)

**`relurpicabilities/debug/invocable_test.go`** (replace behavior_test.go)
- `NewInvestigateRepairInvocable()`: `ID()` returns `"euclo:debug.investigate-repair"`, `IsPrimary()` returns `true`
- `NewSimpleRepairInvocable()`: `IsPrimary()` returns `true`

**`relurpicabilities/archaeology/invocable_test.go`** (replace behavior_test.go)
- All three primary archaeology invocables: correct IDs, `IsPrimary() = true`

**`relurpicabilities/bkc/invocable_test.go`** (extend existing if present)
- All four BKC invocables: `IsPrimary() = false`
- `NewCompileInvocable` with nil env: `Invoke` returns structured error, not panic

**`execution/invocable_test.go`** (update)
- `BehaviorAsInvocable` does not exist (compile-time check — import the package and confirm the symbol is absent by attempting a build)
- `Invocable` interface is satisfied by `RecipeInvocable` (compile-time)

### Exit Criteria

1. `go build ./...` and `go test ./...` pass.
2. `execution.Behavior` interface does not exist. (CI grep: `type Behavior interface` not in `execution/behavior.go`.)
3. `euclorelurpic.SupportingRoutine` interface does not exist. (CI grep: `type SupportingRoutine interface` not in `relurpicabilities/routines.go`.)
4. `relurpicabilities/planning.PlanningBehavior` does not exist. (CI grep.)
5. `BehaviorAsInvocable` does not exist. (CI grep.)
6. `convertInvokeSupportingToRunSupporting` does not exist in any file under `named/euclo/`. (CI grep.)
7. `convertInvokeInputToRoutineInput` does not exist in any file under `named/euclo/`. (CI grep.)
8. `ExecuteInput.RunSupportingRoutine` field does not exist. (CI grep: `RunSupportingRoutine` not in `execution/behavior.go`.)

---

## Cleanup Phase 4 — Classifier Deletion, Recipe Completion, Stale Comments

### Goal

Delete `runtime/capability_classifier.go`. Inline `recipeResultToCoreResult` into `RecipeInvocable.Invoke`. Fix `ExecutionPlan.ToDescriptor()` to set `RecipePath` from the actual file path. Update stale dispatch model comments in `agent.go` and `managed_execution.go`.

### Problem in Detail

**Old classifier still exists.** `runtime/capability_classifier.go` defines `CapabilityIntentClassifier` with the old Tier-1/Tier-2/Tier-3 classification logic. Phase 2 moved classification into `runtime/intake/enrich.go` via the `CapabilityClassifier` interface. The old file was supposed to be deleted; instead, `intake/pipeline.go` wraps it with `capabilityClassifierAdapter`. The adapter exists so existing callers that construct a `*CapabilityIntentClassifier` still work. The fix is to replace all constructors of `CapabilityIntentClassifier` with constructors of a type that implements the `intake.CapabilityClassifier` interface directly, then delete `capability_classifier.go`.

**`recipeResultToCoreResult` private function.** `thoughtrecipes/invocable.go` has:
```go
func recipeResultToCoreResult(recipeResult *RecipeResult) *core.Result { ... }
```
Called only from `RecipeInvocable.Invoke`. The plan's exit criteria says this function must not exist; the intent was for the conversion to be inlined into `Invoke`. The fix is one move: inline the function body directly into `Invoke` and delete the function declaration.

**`RecipePath` set to plan name.** `ExecutionPlan.ToDescriptor()` sets `RecipePath: p.Name`. If `ExecutionPlan` has a `FilePath` field (from loading the YAML), use it. If not, add `FilePath string` to `ExecutionPlan` and populate it during `LoadPlans`. The descriptor then becomes `RecipePath: p.FilePath`.

**Stale comments.** Two locations contain the old `runtimeState(...)` reference:
- `agent.go` line 66: `//	  -> runtimeState(...)` in the dispatch model block comment
- `managed_execution.go` line 41: `// Apply session resume context if present, before runtimeState.`
- `managed_execution.go` line 62: `// Single-pass enrichment: replaces the double runtimeState() call pattern`
- `managed_execution.go` line 237: `// and state before runtimeState is called.`

The line 66 comment should reference `intake.RunEnrichment`. The managed_execution.go comments at lines 62 and 237 can be simplified to drop the historical "replaces" framing now that the pattern is established.

### What to Build

**Step 1 — Replace `CapabilityIntentClassifier` with a direct interface implementation.**

Identify all callers that construct a `*runtime.CapabilityIntentClassifier` (grep for `CapabilityIntentClassifier{` and `NewCapabilityIntentClassifier`). For each caller, replace with a construction of a type that directly implements `intake.CapabilityClassifier`. This type should live in `runtime/intake/enrich.go` as:

```go
// TieredCapabilityClassifier implements the CapabilityClassifier interface using
// Tier-1 keyword matching, Tier-2 LLM semantic classification, and Tier-3 mode default.
type TieredCapabilityClassifier struct {
    Registry        *euclorelurpic.Registry
    Model           core.LanguageModel
    ExtraKeywords   map[string][]string
}

func (c *TieredCapabilityClassifier) Classify(ctx context.Context, instruction, modeID string) ([]string, string, error) {
    // Tier-1: keyword matching (moved from CapabilityIntentClassifier.classifyTier1)
    // Tier-2: LLM call (moved from CapabilityIntentClassifier.classifyTier2)
    // Tier-3: mode default fallback (moved from CapabilityIntentClassifier.classifyTier3)
}
```

Delete `capabilityClassifierAdapter` from `intake/pipeline.go`. Delete `NewCapabilityClassifier` adapter constructor from `intake/pipeline.go`. Delete `runtime/capability_classifier.go`.

**Step 2 — Inline `recipeResultToCoreResult`.**

In `thoughtrecipes/invocable.go`:
```go
// Before
result, err := r.Executor.ExecuteWithInput(ctx, r.Plan, in)
if err != nil { ... }
return recipeResultToCoreResult(result), nil

// After
result, err := r.Executor.ExecuteWithInput(ctx, r.Plan, in)
if err != nil { ... }
if result == nil {
    return &core.Result{Success: false, Error: fmt.Errorf("nil recipe result")}, nil
}
data := map[string]any{
    "recipe_id":      result.RecipeID,
    "artifacts":      result.Artifacts,
    "warnings":       result.Warnings,
    "final_captures": result.FinalCaptures,
    "step_results":   result.StepResults,
}
if result.FinalResult != nil && result.FinalResult.Data != nil {
    for k, v := range result.FinalResult.Data {
        if _, exists := data[k]; !exists {
            data[k] = v
        }
    }
}
return &core.Result{Success: result.Success, Data: data}, nil
```

Delete `recipeResultToCoreResult`.

**Step 3 — Fix `RecipePath` in `ExecutionPlan.ToDescriptor()`.**

Check if `ExecutionPlan` has a `FilePath string` field. If not, add it to the struct definition and populate it in `thoughtrecipes.LoadPlans` (or wherever plans are parsed from YAML). Update `ToDescriptor()`:

```go
RecipePath: p.FilePath,
```

**Step 4 — Update stale comments.**

In `agent.go`, update the dispatch model block comment:
```go
//	  -> intake.RunEnrichment(...)          // single-pass classification
```

In `managed_execution.go`, simplify the three stale `runtimeState` comment references to drop the historical context (they no longer need to explain what they replaced).

### File Dependencies

**Deleted:**
- `named/euclo/runtime/capability_classifier.go`
- `named/euclo/runtime/capability_classifier_test.go`
- `recipeResultToCoreResult` function from `thoughtrecipes/invocable.go`
- `capabilityClassifierAdapter` and `NewCapabilityClassifier` from `intake/pipeline.go`

**Modified:**
- `named/euclo/runtime/intake/enrich.go` — `TieredCapabilityClassifier` added
- `named/euclo/thoughtrecipes/invocable.go` — `recipeResultToCoreResult` inlined into `Invoke`
- `named/euclo/thoughtrecipes/plan.go` (or executor.go) — `ExecutionPlan.FilePath` added, populated in `LoadPlans`
- `named/euclo/agent.go` — comment updated (line 66)
- `named/euclo/managed_execution.go` — three stale comments cleaned

**Existing tests to update/delete:**
- `runtime/capability_classifier_test.go` — deleted (tests move to `intake/enrich_test.go`)

### Unit Tests to Write

**`runtime/intake/enrich_test.go`** (new or extend)
- `TieredCapabilityClassifier.Classify` with a nil model falls back to Tier-1 keyword matching
- `TieredCapabilityClassifier.Classify` with an instruction matching a Tier-1 keyword returns the correct capability sequence without calling the model
- `TieredCapabilityClassifier.Classify` with no keyword match and a nil model falls back to Tier-3 mode default
- `TieredCapabilityClassifier.Classify` with a mock model returns the model's classification when Tier-1 yields no match (Tier-2 path)

**`thoughtrecipes/invocable_test.go`** (extend)
- `recipeResultToCoreResult` function does not exist (compile-time check by importing the package)
- `RecipeInvocable.Invoke` with a recipe that has a non-empty `RecipeID` includes it in `result.Data["recipe_id"]`
- `RecipeInvocable.Invoke` with a recipe that returns `Warnings` includes them in `result.Data["warnings"]`

**`thoughtrecipes/plan_test.go`** (extend)
- `ToDescriptor()` sets `RecipePath` to `FilePath`, not to `Name`
- Plans loaded by `LoadPlans` have non-empty `FilePath`

### Exit Criteria

1. `go build ./...` and `go test ./...` pass.
2. `runtime/capability_classifier.go` does not exist. (CI grep.)
3. `recipeResultToCoreResult` does not appear in any file under `named/euclo/`. (CI grep.)
4. `capabilityClassifierAdapter` does not appear in `intake/pipeline.go`. (CI grep.)
5. `ExecutionPlan.FilePath` is populated for all plans loaded from YAML. (Verified by `thoughtrecipes/plan_test.go`.)
6. No comment in `agent.go` or `managed_execution.go` references `runtimeState`. (CI grep.)

---

## Cleanup Phase 5 — CI Enforcement Scripts

### Goal

Create all five CI enforcement scripts from the original `euclo-runtime-rework.md` plan plus the two new checks introduced by this cleanup plan. All scripts run after `go test ./...` in CI.

### Scripts to Create

**`scripts/check-euclo-state-keys.sh`**

Checks that no production `.go` file under `named/euclo/` contains a raw state key string literal. Test files (ending in `_test.go`) are exempt.

```bash
#!/usr/bin/env bash
# Fails if any non-test Go file in named/euclo/ contains a raw euclo/pipeline state key string.
set -euo pipefail

VIOLATIONS=$(grep -r --include='*.go' \
    -l '\.Set("euclo\.\|\.Get("euclo\.\|\.Set("pipeline\.\|\.Get("pipeline\.' \
    named/euclo/ | grep -v '_test\.go' || true)

if [ -n "$VIOLATIONS" ]; then
    echo "ERROR: Raw state key strings found in production files:"
    echo "$VIOLATIONS"
    echo "Use typed accessors from named/euclo/runtime/state/accessors.go instead."
    exit 1
fi
echo "check-euclo-state-keys: PASS"
```

**`scripts/check-euclo-deprecated-symbols.sh`**

Checks that Phase 2 deletions (`runtimeState`, `seedRuntimeState`, `classifyCapabilityIntent` methods on Agent) are gone, and that `capability_classifier.go` is deleted (Phase 4).

```bash
#!/usr/bin/env bash
set -euo pipefail
FAIL=0

check_absent() {
    local pattern="$1" description="$2"
    if grep -r --include='*.go' -l "$pattern" named/euclo/ | grep -v '_test\.go' > /dev/null 2>&1; then
        echo "ERROR: $description still exists"
        FAIL=1
    fi
}

check_absent 'func.*Agent.*runtimeState\b'          "Agent.runtimeState method"
check_absent 'func.*Agent.*seedRuntimeState\b'      "Agent.seedRuntimeState method"
check_absent 'func.*Agent.*classifyCapabilityIntent' "Agent.classifyCapabilityIntent method"
check_absent 'capability_classifier\.go'             "runtime/capability_classifier.go"
check_absent 'capabilityClassifierAdapter'           "capabilityClassifierAdapter in intake"

[ $FAIL -eq 0 ] && echo "check-euclo-deprecated-symbols: PASS" || exit 1
```

**`scripts/check-assurance-boundaries.sh`**

Checks Phase 3 assurance decomposition exit criteria.

```bash
#!/usr/bin/env bash
set -euo pipefail
FAIL=0

check_absent() {
    local pattern="$1" description="$2"
    if grep -r --include='*.go' "$pattern" named/euclo/runtime/assurance/ > /dev/null 2>&1; then
        echo "ERROR: $description still present in assurance package"
        FAIL=1
    fi
}

check_absent 'applyVerificationAndArtifacts' "applyVerificationAndArtifacts method"
check_absent 'CollectArtifactsFromState'     "direct CollectArtifactsFromState call in ShortCircuit"

# Check assurance.go line count
LINES=$(wc -l < named/euclo/runtime/assurance/assurance.go)
if [ "$LINES" -gt 150 ]; then
    echo "ERROR: assurance.go has $LINES lines; expected ≤150 (coordination shell only)"
    FAIL=1
fi

[ $FAIL -eq 0 ] && echo "check-assurance-boundaries: PASS" || exit 1
```

**`scripts/check-dispatch-registry.sh`**

Checks Phase 4 registry unification exit criteria plus the Cleanup Phase 3 deletions.

```bash
#!/usr/bin/env bash
set -euo pipefail
FAIL=0

check_absent_in() {
    local pattern="$1" path="$2" description="$3"
    if grep -r --include='*.go' "$pattern" "$path" > /dev/null 2>&1; then
        echo "ERROR: $description found in $path"
        FAIL=1
    fi
}

check_absent_in 'behaviorRoutineAdapter'          named/euclo/              "behaviorRoutineAdapter type"
check_absent_in 'type PlanningBehavior'           named/euclo/              "PlanningBehavior type"
check_absent_in 'behaviors\s*map\['               named/euclo/runtime/dispatch/ "Dispatcher.behaviors map field"
check_absent_in 'routines\s*map\['                named/euclo/runtime/dispatch/ "Dispatcher.routines map field"
check_absent_in '\.ServiceBundle\.('              named/euclo/runtime/dispatch/ "ServiceBundle type assertion in dispatch"
check_absent_in 'type Behavior interface'         named/euclo/execution/        "Behavior interface"
check_absent_in 'type SupportingRoutine interface' named/euclo/relurpicabilities/ "SupportingRoutine interface"
check_absent_in 'BehaviorAsInvocable'             named/euclo/              "BehaviorAsInvocable type"
check_absent_in 'convertInvokeSupportingToRunSupporting' named/euclo/       "convertInvokeSupportingToRunSupporting"
check_absent_in 'convertInvokeInputToRoutineInput' named/euclo/             "convertInvokeInputToRoutineInput"
check_absent_in 'RunSupportingRoutine'            named/euclo/execution/behavior.go "RunSupportingRoutine field"

[ $FAIL -eq 0 ] && echo "check-dispatch-registry: PASS" || exit 1
```

**`scripts/check-recipe-dispatch.sh`**

Checks Phase 5 recipe dispatch exit criteria plus Cleanup Phase 4.

```bash
#!/usr/bin/env bash
set -euo pipefail
FAIL=0

check_absent_in() {
    local pattern="$1" path="$2" description="$3"
    if grep -r --include='*.go' "$pattern" "$path" > /dev/null 2>&1; then
        echo "ERROR: $description"
        FAIL=1
    fi
}

check_absent_in 'HasPrefix.*euclo:recipe'        named/euclo/runtime/dispatch/ "recipe HasPrefix bypass in dispatch"
check_absent_in 'SetRecipeRegistry'              named/euclo/                  "SetRecipeRegistry method"
check_absent_in 'recipeResultToCoreResult'       named/euclo/                  "recipeResultToCoreResult function"
check_absent_in 'capabilityClassifierAdapter'    named/euclo/                  "capabilityClassifierAdapter"
check_absent_in 'runtime/capability_classifier'  named/euclo/                  "old capability_classifier package reference"

[ $FAIL -eq 0 ] && echo "check-recipe-dispatch: PASS" || exit 1
```

**`scripts/check-statekeys-removed.sh`** (new, from this cleanup plan)

```bash
#!/usr/bin/env bash
set -euo pipefail
FAIL=0

if [ -d "named/euclo/runtime/statekeys" ]; then
    echo "ERROR: named/euclo/runtime/statekeys/ still exists; should be deleted after consolidation into runtime/state"
    FAIL=1
fi

if grep -r --include='*.go' '".*runtime/statekeys"' named/euclo/ > /dev/null 2>&1; then
    echo "ERROR: imports of runtime/statekeys found; update to import runtime/state instead"
    FAIL=1
fi

[ $FAIL -eq 0 ] && echo "check-statekeys-removed: PASS" || exit 1
```

### Integration into CI

Add to whatever CI entry point runs the boundary scripts (alongside `check-framework-boundaries.sh` and `check-deprecated-agent-wrappers.sh`):

```bash
scripts/check-euclo-state-keys.sh
scripts/check-euclo-deprecated-symbols.sh
scripts/check-assurance-boundaries.sh
scripts/check-dispatch-registry.sh
scripts/check-recipe-dispatch.sh
scripts/check-statekeys-removed.sh
```

### File Dependencies

**Produces:**
- `scripts/check-euclo-state-keys.sh`
- `scripts/check-euclo-deprecated-symbols.sh`
- `scripts/check-assurance-boundaries.sh`
- `scripts/check-dispatch-registry.sh`
- `scripts/check-recipe-dispatch.sh`
- `scripts/check-statekeys-removed.sh`

**No source code modified in this phase.**

### Exit Criteria

1. All six scripts are executable (`chmod +x`).
2. All six scripts pass when run against the codebase as it exists after Cleanup Phases 1–4.
3. Scripts are integrated into the CI pipeline (same entry point as existing boundary scripts).
4. All scripts fail with a non-zero exit code when their target pattern is intentionally introduced into the codebase (verified by a quick local inversion test before merging).

---

## Cross-Phase Summary

| Cleanup Phase | Resolves | Can start after | Parallel with |
|---------------|---------|----------------|---------------|
| C1 — State consolidation + typing | Gap 1, Gap 3 | — | — |
| C2 — Write-site migration | Gap 2 | C1 | C3, C4 |
| C3 — Deprecated interface elimination | Gap 4 | C1 | C2, C4 |
| C4 — Classifier deletion + recipe + comments | Gaps 5, 6 | C1 | C2, C3 |
| C5 — CI enforcement | All | C2 + C3 + C4 | — |

**Total deletions this plan drives:** `runtime/statekeys/` package, `runtime/capability_classifier.go`, `relurpicabilities/planning/behavior.go`, `execution.Behavior` interface, `euclorelurpic.SupportingRoutine` interface, `BehaviorAsInvocable`, four copies of `convertInvokeSupportingToRunSupporting`, `convertInvokeInputToExecuteInput`, `convertInvokeInputToRoutineInput`, `capabilityClassifierAdapter`, `recipeResultToCoreResult`, `ExecuteInput.RunSupportingRoutine`.

**Total files added:** 6 CI scripts, `runtime/intake/enrich.go` (extended with `TieredCapabilityClassifier`).
