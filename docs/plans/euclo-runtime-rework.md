# Euclo Runtime Rework — Engineering Specification

**Status:** Draft  
**Scope:** `named/euclo/` and immediate cross-package seams  
**Constraint:** Each phase must leave `go build ./...` and `go test ./...` green at its exit

---

## Background and Motivation

Five structural issues discovered during architecture review:

1. **Stringly-typed state bus** — `*core.Context` (`map[string]any`) is the shared state medium for the entire execution pipeline. All stages store and retrieve data through untyped string keys. There are no compile-time guarantees on types, key names, or the presence of required values. Every consumer does raw type assertions.

2. **Double `runtimeState` call** — `initializeManagedExecution` calls `runtimeState()` twice: once before capability classification to seed initial mode/profile, and again after to pick up the capability sequence written into state. The function is not idempotent, and its ordering dependencies on pre-populated state are implicit.

3. **Assurance layer overload** — `assurance.Execute` owns: context expansion, interaction seeding, interactive mode-machine execution, three mutation checkpoints, primary behavior dispatch, verification policy evaluation, waiver/degradation detection, artifact collection, action log and proof surface assembly, telemetry emission, and artifact persistence. The `ShortCircuit` path duplicates the observability/persistence tail almost exactly.

4. **Dual behavior/routine registry** — BKC capabilities are registered as `execution.Behavior` entries, wrapped in `planningbehavior.New()`, and also registered as `behaviorRoutineAdapter` instances in the routines map so `DirectCapabilityRun` can reach them. The adapter's `ServiceBundle` dependency is type-asserted from `any`, silently producing a zero-value bundle if the type changes.

5. **Recipe dispatch bypass** — Thought recipes are dispatched via a `strings.HasPrefix(behaviorID, "euclo:recipe.")` branch in the Dispatcher, before the behavior registry lookup. They do not implement `execution.Behavior`, have their own result type (`RecipeResult`) that is converted ad-hoc, and cannot participate in capability sequencing, verification gates, or assurance guarantees.

---

## Guiding Constraints

- **Layering rules are hard constraints.** `framework/` must not import `agents/` or `named/`. `agents/` must not import `named/`. All changes stay within `named/euclo/` unless a framework type genuinely belongs there.
- **No flag-driven back-compat.** Each phase is a clean cut-over at its exit. Feature flags, dual-paths, and adapter shims may exist within a phase to keep tests green but must be removed before the phase exits.
- **State key inventory is authoritative.** Phase 1 produces a canonical state key registry; all subsequent phases must use it. No new raw string key accesses after Phase 1 exits.
- **Small-model survivability.** Nothing in this rework increases the minimum LLM capability required. Typed state must not add prompt complexity.

---

## Phase 1 — Typed Execution State

### Goal

Replace the stringly-typed `map[string]any` state access pattern with a typed `EucloExecutionState` overlay. The `*core.Context` remains the serialization and wire format; the overlay provides typed accessors that all euclo packages use internally. No package outside `named/euclo/` is affected.

### Problem in Detail

The current anti-pattern appears in at least three forms:

**Form A — direct state write with raw key:**
```go
in.State.Set("euclo.verification_policy", policy)
```

**Form B — raw type assertion on read:**
```go
if raw, ok := in.State.Get("euclo.recovery_trace"); ok && raw != nil {
    if trace, ok := raw.(map[string]any); ok {
        switch fmt.Sprint(trace["status"]) { ... }
    }
}
```

**Form C — chained assertions through nested maps (the worst case, `changedPathsFromPipelineCode`):**
```go
payload, ok := raw.(map[string]any)
finalOutput, ok := payload["final_output"].(map[string]any)
result, ok := finalOutput["result"].(map[string]any)
for _, item := range result {
    entry, ok := item.(map[string]any)
    data, ok := entry["data"].(map[string]any)
    ...
}
```

Each site is a latent panic or silent wrong-value bug. `EnsureRoutineArtifacts` in `execution/behavior.go` has a 40-line switch statement that writes typed payloads under specific keys per routine — this is a symptom of the state bus being untyped at every layer.

### What to Build

**`named/euclo/runtime/state/` — new package**

```
state/
  keys.go        — canonical key constants (EucloStateKey type alias for string)
  state.go       — EucloExecutionState struct with typed accessor methods
  accessors.go   — per-field Get/Set/Clear methods
  migration.go   — LoadFromContext / FlushToContext for round-tripping with core.Context
```

`EucloExecutionState` is a struct, not a map. Every field corresponds to a state key. The type of each field is the authoritative type for that key. Reading an unknown type from context returns the zero value plus `false`, never panics.

**Canonical key registry in `keys.go`:**

All `"euclo.*"` and `"pipeline.*"` key strings used anywhere in `named/euclo/` become typed constants:

```go
type Key = string

const (
    KeyVerificationPolicy          Key = "euclo.verification_policy"
    KeyVerification                Key = "euclo.verification"
    KeySuccessGate                 Key = "euclo.success_gate"
    KeyAssuranceClass              Key = "euclo.assurance_class"
    KeyEditExecution               Key = "euclo.edit_execution"
    KeyEditIntent                  Key = "euclo.edit_intent"
    KeyExecutionWaiver             Key = "euclo.execution_waiver"
    KeyWaiver                      Key = "euclo.waiver"
    KeyRecoveryTrace               Key = "euclo.recovery_trace"
    KeyBehaviorTrace               Key = "euclo.relurpic_behavior_trace"
    KeyArtifacts                   Key = "euclo.artifacts"
    KeyActionLog                   Key = "euclo.action_log"
    KeyProofSurface                Key = "euclo.proof_surface"
    KeyFinalReport                 Key = "euclo.final_report"
    KeyContextRuntime              Key = "euclo.context_runtime"
    KeySecurityRuntime             Key = "euclo.security_runtime"
    KeySharedContextRuntime        Key = "euclo.shared_context_runtime"
    KeyProviderRestore             Key = "euclo.provider_restore"
    KeyVerificationSummary         Key = "euclo.verification_summary"
    KeyReviewFindings              Key = "euclo.review_findings"
    KeyRootCause                   Key = "euclo.root_cause"
    KeyRootCauseCandidates         Key = "euclo.root_cause_candidates"
    KeyRegressionAnalysis          Key = "euclo.regression_analysis"
    KeyPlanCandidates              Key = "euclo.plan_candidates"
    KeyWorkflowID                  Key = "euclo.workflow_id"
    KeyClassificationSource        Key = "euclo.capability_classification_source"
    KeyClassificationMeta          Key = "euclo.capability_classification_meta"
    KeyPreClassifiedCapSeq         Key = "euclo.pre_classified_capability_sequence"
    KeyCapabilitySequenceOperator  Key = "euclo.capability_sequence_operator"
    KeyUserRecipeSignals           Key = "euclo.user_recipe_signals"
    KeyRetrievalPolicy             Key = "euclo.retrieval_policy"
    KeyContextLifecycle            Key = "euclo.context_lifecycle"
    KeySessionID                   Key = "euclo.session_id"
    KeyDeferredIssues              Key = "euclo.deferred_execution_issues"
    KeyInteractionScript           Key = "euclo.interaction_script"
    KeyRequiresEvidencePreMutation Key = "euclo.requires_evidence_before_mutation"

    KeyPipelineExplore      Key = "pipeline.explore"
    KeyPipelineAnalyze      Key = "pipeline.analyze"
    KeyPipelinePlan         Key = "pipeline.plan"
    KeyPipelineCode         Key = "pipeline.code"
    KeyPipelineVerify       Key = "pipeline.verify"
    KeyPipelineFinalOutput  Key = "pipeline.final_output"
)
```

**Typed accessors in `accessors.go`:**

For each key, a pair of functions:

```go
func GetVerificationPolicy(s *core.Context) (eucloruntime.VerificationPolicy, bool)
func SetVerificationPolicy(s *core.Context, v eucloruntime.VerificationPolicy)

func GetRecoveryTrace(s *core.Context) (RecoveryTrace, bool)
func SetRecoveryTrace(s *core.Context, v RecoveryTrace)
```

Where payloads are currently `map[string]any`, they get typed struct replacements. For example:

```go
// replaces the raw map[string]any used for recovery_trace
type RecoveryTrace struct {
    Status       string `json:"status"`
    AttemptCount int    `json:"attempt_count"`
}
```

**Migration helpers for `core.Context` round-tripping:**

```go
// LoadFromContext reads all known euclo keys from ctx into an EucloExecutionState.
// Unknown keys and type mismatches are logged and skipped; they do not panic.
func LoadFromContext(ctx *core.Context) *EucloExecutionState

// FlushToContext writes all non-zero fields of s back to ctx.
func (s *EucloExecutionState) FlushToContext(ctx *core.Context)
```

This migration layer allows a phased cutover: existing code that still writes raw state keys continues to work because `LoadFromContext` reads the same keys. Once all write sites are migrated, the raw writes disappear.

### File Dependencies

**Produces:**
- `named/euclo/runtime/state/` (new package: `keys.go`, `state.go`, `accessors.go`, `migration.go`)

**Modified (write sites converted to typed accessors):**
- `named/euclo/runtime/assurance/assurance.go` — 15+ raw state reads/writes
- `named/euclo/execution/behavior.go` — `mergeStateArtifacts`, `EnsureRoutineArtifacts`, `changedPathsFromPipelineCode`, `readBehaviorTrace`
- `named/euclo/runtime/policy/` — all `ResolveVerificationPolicy`, `ResolveRetrievalPolicy` callers
- `named/euclo/runtime/reporting/` — `BuildActionLog`, `BuildProofSurface`
- `named/euclo/runtime/context/` — `ApplyEditIntentArtifacts`
- `named/euclo/runtime/session/` — session context reads
- `named/euclo/runtime/pretask/` — context enrichment reads
- `named/euclo/runtime/archaeomem/` — semantic bundle enrichment
- `named/euclo/internal/agentstate/helpers.go` — all raw state reads
- `named/euclo/agent_state_helpers.go` — `seedRuntimeState`
- `named/euclo/managed_execution.go` — session scoping reads
- `named/euclo/euclotypes/artifacts.go` — `CollectArtifactsFromState`, `StateKeyForArtifactKind`

**Not modified in this phase:**
- `framework/`, `agents/`, `archaeo/`, `platform/` — no changes cross the package boundary

### Unit Tests to Write

**`runtime/state/keys_test.go`**
- All key constants are non-empty strings
- No two key constants share the same value (deduplication test)
- Key format invariant: all euclo keys start with `"euclo."`, all pipeline keys start with `"pipeline."`

**`runtime/state/accessors_test.go`**
- `GetVerificationPolicy` on empty context returns zero value and `false`
- `SetVerificationPolicy` then `GetVerificationPolicy` round-trips without loss
- `GetRecoveryTrace` on context with a map written under the raw key returns the typed struct correctly via migration
- Type mismatch on read returns zero value and `false` (no panic)
- Nil context returns zero value and `false` on all getters

**`runtime/state/migration_test.go`**
- `LoadFromContext` on an empty context returns an EucloExecutionState with all fields at zero
- `LoadFromContext` followed by `FlushToContext` produces a context that returns the same values for all canonical keys
- A context written with raw keys (simulating legacy code) is correctly read by `LoadFromContext`
- A context with unknown keys is not disturbed by `FlushToContext`

**`execution/behavior_state_test.go`** (new)
- `mergeStateArtifacts` propagates all expected keys using typed accessors
- `EnsureRoutineArtifacts` for each routine ID sets the expected typed payload (not a `map[string]any`)
- `changedPathsFromPipelineCode` returns correct paths for a well-formed and a malformed pipeline.code state

### Exit Criteria

1. `go build ./...` passes with zero new warnings.
2. `go test ./...` passes; no existing tests regressed.
3. No raw `state.Get("euclo.` or `state.Set("euclo.` calls remain anywhere in `named/euclo/`. (CI grep check added to `scripts/check-euclo-state-keys.sh`.)
4. No raw `state.Get("pipeline.` or `state.Set("pipeline.` calls remain in `named/euclo/`. (Same script.)
5. `named/euclo/runtime/state` package has >90% statement coverage from the tests written in this phase.
6. `EnsureRoutineArtifacts` switch statement in `execution/behavior.go` is replaced by per-routine typed setters using `accessors.go`.

---

## Phase 2 — Single-Pass Classification

### Goal

Eliminate the double `runtimeState()` call in `initializeManagedExecution`. Classification becomes an explicit, single-pass enrichment step that produces a fully-resolved `ClassifiedEnvelope`. `runtimeState` is called once, after enrichment, with complete inputs. The ordering dependency on pre-populated state becomes explicit and compile-time visible.

### Problem in Detail

Current flow in `managed_execution.go`:

```
// Pass 1: initial envelope with no capability sequence
envelope, classification, mode, profile, work = a.runtimeState(task, state)
a.seedRuntimeState(state, ...)

// Side effects written to state:
a.classifyCapabilityIntent(ctx, task, state)  // writes euclo.pre_classified_capability_sequence to state

a.ensureDeferralPlan(...)
a.ensureWorkflowRun(...)
a.restoreExecutionContinuity(...)

// Pass 2: re-reads state to pick up capability sequence
envelope, classification, mode, profile, work = a.runtimeState(task, state)
```

`runtimeState` on pass 2 calls `NormalizeTaskEnvelope` which reads `"euclo.pre_classified_capability_sequence"` from state. This creates an implicit ordering contract: classification must run between the two `runtimeState` calls. Adding any code between them that also calls `runtimeState` would silently produce stale results.

Additionally, `runtimeState` is doing four things: normalize envelope, classify task, resolve mode, select profile. These are distinct transformation steps that should be composable.

### What to Build

**`named/euclo/runtime/intake/` — new package**

```
intake/
  pipeline.go    — EnrichmentPipeline, ClassifiedEnvelope, RunEnrichment()
  normalize.go   — envelope normalization (extracted from NormalizeTaskEnvelope)
  classify.go    — task-level classification (extracted from ClassifyTask)
  resolve.go     — mode and profile resolution (extracted from runtimeState)
  enrich.go      — capability intent classification (extracted from classifyCapabilityIntent)
```

**`ClassifiedEnvelope` struct:**

```go
type ClassifiedEnvelope struct {
    Envelope       runtime.TaskEnvelope
    Classification runtime.TaskClassification
    Mode           euclotypes.ModeResolution
    Profile        euclotypes.ExecutionProfileSelection
    Work           runtime.UnitOfWork
}
```

**`RunEnrichment` function:**

```go
// RunEnrichment runs the full single-pass classification pipeline.
// It does not read from or write to state. Callers are responsible for
// persisting the result to state via SeedClassifiedEnvelope.
func RunEnrichment(
    ctx context.Context,
    task *core.Task,
    state *core.Context,
    env agentenv.AgentEnvironment,
    modeRegistry euclotypes.ModeRegistry,
    profileRegistry euclotypes.ExecutionProfileRegistry,
    capabilityClassifier CapabilityClassifier,
) (ClassifiedEnvelope, error)
```

The pipeline runs in order:
1. `normalize.NormalizeEnvelope(task, state, env.Registry)` → `TaskEnvelope`
2. `classify.ClassifyTask(envelope)` → `TaskClassification`
3. `resolve.ResolveMode(envelope, classification, modeRegistry)` → `ModeResolution`
4. `resolve.ResolveProfile(envelope, classification, mode, profileRegistry)` → `ExecutionProfileSelection`
5. `enrich.ClassifyCapabilityIntent(ctx, envelope, classification, mode, capabilityClassifier)` → enriched `TaskEnvelope` (with `CapabilitySequence` set)
6. `resolve.BuildUnitOfWork(task, state, envelope, classification, mode, profile, modeRegistry, semanticInputs, policy, descriptor)` → `UnitOfWork`

Steps 1–4 are pure functions (no context, no IO, deterministic). Step 5 may call the LLM (Tier 2 classifier). Step 6 is pure.

**`CapabilityClassifier` interface** (replaces the ad-hoc classifier in `runtime/capability_classifier.go`):

```go
type CapabilityClassifier interface {
    // Classify returns the recommended capability sequence for the given instruction
    // and mode. It must not write to state.
    Classify(ctx context.Context, instruction, modeID string) ([]string, string, error)
    // operator: "AND" | "OR"
}
```

**Modified `initializeManagedExecution`:**

```go
func (a *Agent) initializeManagedExecution(...) (*managedExecutionFlow, *core.Result, error) {
    // session resume, react delegate init, guidance wiring, session scoping — unchanged

    // Single enrichment pass
    classified, err := intake.RunEnrichment(ctx, task, state, a.Environment,
        a.ModeRegistry, a.ProfileRegistry, a.capabilityClassifier)
    if err != nil { ... }

    // Persist classified envelope to state (replaces seedRuntimeState + two runtimeState calls)
    intake.SeedClassifiedEnvelope(state, classified)

    a.ensureDeferralPlan(task, state)
    a.ensureWorkflowRun(ctx, task, state)
    if restoreErr := a.restoreExecutionContinuity(...); restoreErr != nil { ... }

    // Rebuild work after restore (only if restore changed state)
    if stateChanged(state) {
        classified.Work = intake.RebuildUnitOfWork(task, state, classified, a.ModeRegistry)
    }

    flow := &managedExecutionFlow{..., classified: classified}
    return flow, nil, nil
}
```

`runtimeState` is deprecated and removed after this phase. `seedRuntimeState` is removed. `classifyCapabilityIntent` is removed (its logic moved into `intake.enrich`).

**Extraction of restore-rebuild:**

The one case where `runtimeState` legitimately needs to re-run is after a failed restore (which writes additional state). This is handled explicitly as `intake.RebuildUnitOfWork`, which takes the already-classified envelope and only rebuilds the `UnitOfWork` after restore state changes — it does not re-normalize or re-classify.

### File Dependencies

**Produces:**
- `named/euclo/runtime/intake/` (new package)

**Deleted:**
- `runtimeState` function in `named/euclo/agent_state_helpers.go`
- `seedRuntimeState` function in `named/euclo/agent_state_helpers.go`
- `classifyCapabilityIntent` function in `named/euclo/agent.go` / `agent_state_helpers.go`
- `runtime/capability_classifier.go` — logic moved to `intake/enrich.go`

**Modified:**
- `named/euclo/managed_execution.go` — replace two `runtimeState` calls with `intake.RunEnrichment`
- `named/euclo/execute.go` — `BuildGraph` uses `intake.RunEnrichment` instead of `runtimeState`
- `named/euclo/runtime/classification.go` — `NormalizeTaskEnvelope` becomes `intake/normalize.NormalizeEnvelope`; `ClassifyTask` moves to `intake/classify.ClassifyTask`
- `named/euclo/runtime/workunit.go` — `BuildUnitOfWork` becomes `intake/resolve.BuildUnitOfWork`

### Unit Tests to Write

**`runtime/intake/normalize_test.go`**
- Nil task returns envelope with `EditPermitted` derived from capability snapshot
- Task with explicit `euclo.mode` context key sets `ModeHint` correctly
- Task with `euclo.pre_classified_capability_sequence` in state correctly populates `CapabilitySequence` in the normalized envelope
- Instruction trimming (leading/trailing whitespace removed)
- `ResumedMode` is populated from state when present

**`runtime/intake/classify_test.go`**
- Empty instruction → classification with default intent family
- Instruction matching a known intent keyword → correct `IntentFamilies`
- `MixedIntent` is true when instruction matches keywords from two distinct mode families
- `RiskLevel` is elevated correctly for known high-risk patterns
- Deterministic: same input always produces same output

**`runtime/intake/resolve_test.go`**
- `ResolveMode` with no mode hint selects the mode recommended by classification
- `ResolveMode` with an explicit mode hint overrides the classification recommendation
- `ResolveProfile` selects the correct profile for a given mode
- `BuildUnitOfWork` returns a `UnitOfWork` with `PrimaryRelurpicCapabilityID` from the envelope's `CapabilitySequence[0]` when sequence length is 1

**`runtime/intake/pipeline_test.go`**
- `RunEnrichment` with a mock `CapabilityClassifier` (Tier 2) produces a `ClassifiedEnvelope` with the classifier's sequence
- `RunEnrichment` with a classifier that returns an error falls back to Tier 3 (default for mode)
- `RunEnrichment` is called once and produces the same result as the previous double-`runtimeState` path (regression test using a golden fixture)
- `SeedClassifiedEnvelope` followed by reading back all state keys returns the expected values

**`managed_execution_test.go`** (extension of existing)
- `initializeManagedExecution` no longer calls `runtimeState` at all (verified by absence of the symbol after refactor)
- After restore failure, the returned result has `result_class = "restore_failed"` and classification is not re-run

### Exit Criteria

1. `go build ./...` and `go test ./...` pass.
2. `runtimeState`, `seedRuntimeState`, `classifyCapabilityIntent` symbols no longer exist in `named/euclo/`. (CI check added.)
3. `initializeManagedExecution` contains exactly one call to `intake.RunEnrichment` and zero calls to `NormalizeTaskEnvelope`, `ClassifyTask`, or `BuildUnitOfWork` directly.
4. `runtime/intake` package has >85% statement coverage.
5. A determinism test passes: given identical inputs, `RunEnrichment` with a deterministic classifier produces byte-identical `ClassifiedEnvelope` across 100 invocations.

---

## Phase 3 — Assurance Layer Decomposition

### Goal

Split `runtime/assurance/assurance.go` into four focused services with a thin coordination shell. Eliminate the near-duplicate code between `Execute` and `ShortCircuit`. Each service is independently testable.

### Problem in Detail

`assurance.Execute` (400 lines) does, in order:

1. Context expansion (load workflow artifacts, apply retrieval policy)
2. Interaction seeding (SeedInteraction hook)
3. Interactive mode-machine execution (runInteractive)
4. PreDispatch checkpoint
5. Primary behavior dispatch
6. PostExecution checkpoint
7. PreVerification checkpoint
8. BeforeVerification hook
9. Verification policy evaluation
10. Edit intent application
11. Success gate evaluation (waiver, degradation, repair trace)
12. PreFinalization checkpoint
13. Artifact collection
14. Action log assembly
15. Proof surface assembly
16. Artifact persistence
17. Final report assembly
18. Telemetry emission

`ShortCircuit` repeats steps 10, 13–18. The verification policy is resolved twice (once in `ShortCircuit`, once in `applyVerificationAndArtifacts`). The final report assembly in both paths has identical structure but slightly different field inclusions.

### What to Build

Decompose into four service types, each in its own file within `runtime/assurance/`:

**`context_expander.go` — `ContextExpander`**

```go
type ContextExpander struct {
    Memory memory.MemoryStore
}

type ContextExpansionResult struct {
    ExecutionTask *core.Task
    Err           error
}

func (e ContextExpander) Expand(ctx context.Context, in Input) ContextExpansionResult
```

Owns: loading workflow artifacts, applying retrieval policy, producing the final `executionTask`. Currently `expandContext` in `assurance.go`.

**`interaction_runner.go` — `InteractionRunner`**

```go
type InteractionRunner struct {
    ProfileCtrl         *orchestrate.ProfileController
    InteractionRegistry *interaction.ModeMachineRegistry
    ResolveEmitter      EmitterResolver
    Emitter             interaction.FrameEmitter
    SeedInteraction     PrepassSeeder
    ResetDoomLoop       func()
}

func (r InteractionRunner) Run(ctx context.Context, executionTask *core.Task, in Input) error
```

Owns: interaction seeding, mode-machine execution, doom-loop reset. Currently `runInteractive` plus the `SeedInteraction` call inline in `Execute`.

**`verification_gate.go` — `VerificationGate`**

```go
type VerificationGate struct {
    Environment agentenv.AgentEnvironment
}

type GateResult struct {
    Evidence    eucloruntime.VerificationEvidence
    SuccessGate eucloruntime.SuccessGate
    Err         error
}

func (g VerificationGate) Evaluate(ctx context.Context, in Input, mutationAllowed bool) GateResult
```

Owns: `ApplyEditIntentArtifacts`, `NormalizeVerificationEvidence`, `EvaluateSuccessGate`, waiver application, automatic degradation detection, repair trace folding. Currently `applyVerificationAndArtifacts` (the verification half).

**`execution_recorder.go` — `ExecutionRecorder`**

```go
type ExecutionRecorder struct {
    PersistArtifacts ArtifactPersister
    Telemetry        core.Telemetry
}

type RecordResult struct {
    Artifacts    []euclotypes.Artifact
    ActionLog    []eucloruntime.ActionLogEntry
    ProofSurface eucloruntime.ProofSurface
    FinalReport  map[string]any
    Err          error
}

func (r ExecutionRecorder) Record(ctx context.Context, task *core.Task, state *core.Context,
    gateResult GateResult, result *core.Result) RecordResult
```

Owns: artifact collection, action log assembly, proof surface assembly, artifact persistence, final report assembly, telemetry emission. Currently the second half of `applyVerificationAndArtifacts` and the identical tail of `ShortCircuit`.

**Unified coordination shell in `assurance.go`:**

```go
func Execute(s Runtime, ctx context.Context, in Input) Output {
    // 1. Expand context
    expansion := s.Expander.Expand(ctx, in)
    executionTask := expansion.ExecutionTask

    // 2. Run interaction
    if err := s.Interaction.Run(ctx, executionTask, in); err != nil { ... }

    // 3. Pre-dispatch checkpoint
    if err := s.runCheckpoint(ctx, MutationCheckpointPreDispatch, in.Task, in.State); err != nil { ... }

    // 4. Dispatch
    result, execErr := s.dispatch(ctx, executionTask, in)

    // 5. Post-execution + pre-verification checkpoints
    ...

    // 6. Verify
    gateResult := s.Gate.Evaluate(ctx, in, in.Profile.MutationAllowed)

    // 7. Record
    recorded := s.Recorder.Record(ctx, in.Task, in.State, gateResult, result)

    return buildOutput(result, gateResult, recorded, execErr)
}

func ShortCircuit(s Runtime, ctx context.Context, in ShortCircuitInput) Output {
    // Minimal path: skip expansion, interaction, dispatch, checkpoints
    policy := euclopolicy.ResolveVerificationPolicy(in.Mode, in.Profile)
    state.SetVerificationPolicy(in.State, policy)

    recorded := s.Recorder.Record(ctx, in.Task, in.State,
        GateResult{SuccessGate: eucloruntime.SuccessGate{Allowed: true}}, in.Result)

    return buildShortCircuitOutput(in.Result, recorded)
}
```

`ShortCircuit` now calls `Recorder.Record` directly — no duplication with `Execute`.

**`Runtime` struct cleanup:**

Remove hooks that are now services:
- `ResolveEmitter EmitterResolver` → moves to `InteractionRunner`
- `SeedInteraction PrepassSeeder` → moves to `InteractionRunner`
- `PersistArtifacts ArtifactPersister` → moves to `ExecutionRecorder`
- `BeforeVerification BeforeVerificationHook` → absorbed into `VerificationGate.Evaluate` as an optional pre-evaluate hook

The new `Runtime`:

```go
type Runtime struct {
    Memory      memory.MemoryStore
    Environment agentenv.AgentEnvironment
    Dispatcher  *euclodispatch.Dispatcher
    Expander    ContextExpander
    Interaction InteractionRunner
    Gate        VerificationGate
    Recorder    ExecutionRecorder
    Checkpoint  MutationCheckpointHook
    ResetDoomLoop func()
}
```

Wire-up of `Runtime` moves to `agent.go` where it is constructed.

### File Dependencies

**Produces:**
- `named/euclo/runtime/assurance/context_expander.go`
- `named/euclo/runtime/assurance/interaction_runner.go`
- `named/euclo/runtime/assurance/verification_gate.go`
- `named/euclo/runtime/assurance/execution_recorder.go`

**Modified:**
- `named/euclo/runtime/assurance/assurance.go` — becomes the thin coordination shell; `applyVerificationAndArtifacts` is removed
- `named/euclo/agent.go` — constructs `Runtime` with the four service types

**Unchanged:**
- All callers of `assurance.Execute` and `assurance.ShortCircuit` — signatures are preserved

### Unit Tests to Write

**`runtime/assurance/context_expander_test.go`**
- `Expand` with nil workflow surfaces returns the input task unchanged
- `Expand` with a workflow store correctly applies retrieval policy to the task context
- `Expand` error is surfaced in `ContextExpansionResult.Err`, not panicked

**`runtime/assurance/interaction_runner_test.go`**
- `Run` with nil `ProfileCtrl` is a no-op and returns nil
- `Run` calls `SeedInteraction` before `runInteractive`
- `Run` calls `ResetDoomLoop` after `runInteractive` completes
- `Run` with a scripted interaction (using `interactionScript` from agentstate) drives through expected phases

**`runtime/assurance/verification_gate_test.go`**
- `Evaluate` with a waiver in state sets `AssuranceClass = operator_deferred` and `Allowed = true` regardless of evidence
- `Evaluate` with a `repair_exhausted` recovery trace sets `AssuranceClass = repair_exhausted`
- `Evaluate` with no evidence and `MutationAllowed = true` returns `UnverifiedSuccess`
- `Evaluate` with passing verification evidence returns `VerifiedSuccess`
- `Evaluate` degradation detection: when automatic degradation triggers, `DegradationMode` and `DegradationReason` are non-empty

**`runtime/assurance/execution_recorder_test.go`**
- `Record` with no artifacts produces empty `ActionLog` and `ProofSurface`
- `Record` with artifacts produces an `ActionLog` with one entry per capability invocation
- `Record` calls `PersistArtifacts` when non-nil
- `Record` on persistence failure sets `Err` and does not panic
- `Record` emits telemetry with non-nil `Telemetry`
- `Record` with a waiver in state includes `"waiver"` in `FinalReport`
- `Record` and `ShortCircuit`'s recorder call produce identical `FinalReport` structures given the same inputs (regression test)

**`runtime/assurance/assurance_test.go`** (extension)
- `ShortCircuit` now uses `ExecutionRecorder.Record` — verify no duplicate artifact collection calls (mock recorder)
- `Execute` and `ShortCircuit` produce structurally identical `FinalReport` keys when given equivalent state

### Exit Criteria

1. `go build ./...` and `go test ./...` pass.
2. `applyVerificationAndArtifacts` method no longer exists. (CI check.)
3. `ShortCircuit` contains no direct calls to `CollectArtifactsFromState`, `BuildActionLog`, `BuildProofSurface`, or `EmitObservabilityTelemetry` — all delegated to `ExecutionRecorder`.
4. Each service file has >85% statement coverage independently.
5. `assurance.go` coordination shell is ≤100 lines.

---

## Phase 4 — Behavior/Routine Registry Unification

### Goal

Eliminate the dual-registry pattern in the Dispatcher. Behaviors and routines implement the same `Invocable` interface. The BKC dual-registration and the `behaviorRoutineAdapter` type assertion hack are removed. `DirectCapabilityRun` uses the unified registry directly.

### Problem in Detail

The Dispatcher currently maintains two separate maps:

```go
type Dispatcher struct {
    behaviors map[string]execution.Behavior
    routines  map[string]euclorelurpic.SupportingRoutine
    ...
}
```

BKC capabilities are in both:

```go
// In behaviors (for Execute):
planningbehavior.New(euclorelurpic.CapabilityBKCCompile, bkccap.NewCompileCapability(env))

// Also in routines (via adapter for ExecuteRoutine):
d.routines[id] = &behaviorRoutineAdapter{id: id, behavior: b}
```

The `behaviorRoutineAdapter` adapts `execution.Behavior` to `euclorelurpic.SupportingRoutine` via:

```go
if sb, ok := in.ServiceBundle.(execution.ServiceBundle); ok {
    execInput.ServiceBundle = sb
}
```

If `in.ServiceBundle` is not an `execution.ServiceBundle`, the assertion silently fails and `execInput.ServiceBundle` is the zero value. There is no error, no log.

Additionally, `euclorelurpic.SupportingRoutine` and `execution.Behavior` are structurally nearly identical:

```go
// SupportingRoutine
Execute(ctx context.Context, in euclorelurpic.RoutineInput) ([]euclotypes.Artifact, error)

// Behavior
Execute(ctx context.Context, in execution.ExecuteInput) (*core.Result, error)
```

The input types differ, but the information they carry is equivalent. The return types differ, but `Behavior.Execute` returning `*core.Result` where `Data["artifacts"]` is `[]euclotypes.Artifact` is functionally the same as returning `[]euclotypes.Artifact` directly.

### What to Build

**`named/euclo/execution/invocable.go` — new file**

```go
// Invocable is the unified interface for all euclo capabilities.
// It replaces the separate Behavior and SupportingRoutine interfaces.
// Primary behaviors return a *core.Result with full execution context.
// Supporting invocables return only artifacts.
type Invocable interface {
    ID() string
    // Invoke executes the capability. Primary behaviors return a full result;
    // supporting invocables embed artifacts in result.Data["artifacts"].
    Invoke(context.Context, InvokeInput) (*core.Result, error)
    // IsPrimary returns true if this invocable can be a primary dispatch target.
    IsPrimary() bool
}

type InvokeInput struct {
    Task             *core.Task
    ExecutionTask    *core.Task
    State            *core.Context
    Mode             euclotypes.ModeResolution
    Profile          euclotypes.ExecutionProfileSelection
    Work             eucloruntime.UnitOfWork
    Environment      agentenv.AgentEnvironment
    ServiceBundle    ServiceBundle
    WorkflowExecutor graph.WorkflowExecutor
    Telemetry        core.Telemetry
    InvokeSupporting func(context.Context, string, InvokeInput) ([]euclotypes.Artifact, error)
}
```

**Adapter constructors (backward compat within phase):**

```go
// BehaviorAsInvocable wraps an execution.Behavior as an Invocable.
// Removed after all call sites are migrated to the unified interface.
func BehaviorAsInvocable(b Behavior) Invocable

// RoutineAsInvocable wraps a euclorelurpic.SupportingRoutine as an Invocable.
// Removed after all call sites are migrated to the unified interface.
func RoutineAsInvocable(r euclorelurpic.SupportingRoutine) Invocable
```

**Unified `InvocableRegistry` in `named/euclo/runtime/dispatch/registry.go`:**

```go
type InvocableRegistry struct {
    entries map[string]execution.Invocable
}

func (r *InvocableRegistry) Register(inv execution.Invocable) error
func (r *InvocableRegistry) Lookup(id string) (execution.Invocable, bool)
func (r *InvocableRegistry) Primary(id string) (execution.Invocable, bool)   // returns only if IsPrimary()
func (r *InvocableRegistry) Supporting(id string) (execution.Invocable, bool) // returns only if !IsPrimary()
```

**Dispatcher rewrite (`runtime/dispatch/dispatcher.go`):**

```go
type Dispatcher struct {
    env      agentenv.AgentEnvironment
    registry *InvocableRegistry
    recipes  *thoughtrecipes.PlanRegistry
}

func NewDispatcher(env agentenv.AgentEnvironment) *Dispatcher {
    d := &Dispatcher{
        env:      env,
        registry: newInvocableRegistry(),
    }
    // All capabilities registered uniformly — no separate behaviors/routines maps
    for _, inv := range buildBuiltinInvocables(env) {
        _ = d.registry.Register(inv)
    }
    return d
}
```

BKC capabilities are registered once:

```go
func buildBuiltinInvocables(env agentenv.AgentEnvironment) []execution.Invocable {
    return []execution.Invocable{
        chatbehavior.NewAskInvocable(),
        chatbehavior.NewInspectInvocable(),
        chatbehavior.NewImplementInvocable(),
        debugbehavior.NewInvestigateRepairInvocable(),
        debugbehavior.NewSimpleRepairInvocable(),
        archaeologybehavior.NewExploreInvocable(),
        archaeologybehavior.NewCompilePlanInvocable(),
        archaeologybehavior.NewImplementPlanInvocable(),
        bkccap.NewCompileInvocable(env),
        bkccap.NewStreamInvocable(env),
        bkccap.NewCheckpointInvocable(env),
        bkccap.NewInvalidateInvocable(env),
    }
}
```

No `behaviorRoutineAdapter`. No `planningbehavior.New` wrapper. Each BKC capability reports `IsPrimary() = false`, so it is only reachable as a supporting invocable. The `PlanningBehavior` wrapper is deleted.

**`ExecuteRoutine` replacement:**

```go
func (d *Dispatcher) InvokeSupporting(ctx context.Context, id string, in execution.InvokeInput) ([]euclotypes.Artifact, error) {
    inv, ok := d.registry.Supporting(id)
    if !ok {
        return nil, fmt.Errorf("supporting invocable %q not registered", id)
    }
    result, err := inv.Invoke(ctx, in)
    if err != nil {
        return nil, err
    }
    if result != nil && result.Data != nil {
        if artifacts, ok := result.Data["artifacts"].([]euclotypes.Artifact); ok {
            return artifacts, nil
        }
    }
    return nil, nil
}
```

No type assertion on `ServiceBundle` from `any`.

**Migration of existing behaviors:**

Each existing behavior (`chatbehavior.AskBehavior`, etc.) implements the new `Invocable` interface by wrapping its current `Execute` in `Invoke`. The old `Behavior` interface is kept as a deprecated alias until all callers are migrated, then removed at phase exit.

Similarly, each existing supporting routine wraps its `Execute` in `Invoke` and reports `IsPrimary() = false`.

### File Dependencies

**Produces:**
- `named/euclo/execution/invocable.go`
- `named/euclo/runtime/dispatch/registry.go`

**Deleted:**
- `named/euclo/runtime/dispatch/dispatcher.go` — replaced by new Dispatcher
- `named/euclo/relurpicabilities/planning/planning_behavior.go` (the PlanningBehavior wrapper)
- `behaviorRoutineAdapter` struct

**Modified:**
- All files in `named/euclo/relurpicabilities/*/` — each behavior adds `Invoke` and `IsPrimary` methods
- `named/euclo/relurpicabilities/bkc/` — capabilities implement `Invocable` directly (not wrapped)
- `named/euclo/agent.go` — constructs `Dispatcher` with unified registry
- `named/euclo/runtime/assurance/assurance.go` — `BehaviorDispatcher.Execute` becomes `Dispatcher.Invoke`

**Deprecated (removed at phase exit):**
- `execution.Behavior` interface
- `euclorelurpic.SupportingRoutine` interface

### Unit Tests to Write

**`execution/invocable_test.go`**
- `BehaviorAsInvocable` correctly routes `Invoke` to the wrapped `Behavior.Execute`
- `RoutineAsInvocable` correctly routes `Invoke` to the wrapped `SupportingRoutine.Execute`
- `IsPrimary()` returns correct value for each adapter type

**`runtime/dispatch/registry_test.go`**
- `Register` then `Lookup` returns the registered invocable
- `Register` with duplicate ID returns an error
- `Primary` returns only invocables where `IsPrimary() = true`
- `Supporting` returns only invocables where `IsPrimary() = false`
- `Lookup` for unknown ID returns `false`

**`runtime/dispatch/dispatcher_test.go`** (rewrite)
- `Execute` routes to the correct primary invocable by ID
- `Execute` with a BKC capability ID returns an error (BKC is supporting-only)
- `InvokeSupporting` routes to the correct supporting invocable by ID
- `InvokeSupporting` with a primary capability ID returns an error
- No `ServiceBundle` type assertion occurs anywhere in the Dispatcher (verified by absence of `.ServiceBundle.(` in the file)
- All 12 built-in invocables are registered in `NewDispatcher`

**`relurpicabilities/bkc/*_test.go`** (new)
- Each BKC invocable: `IsPrimary() = false`
- Each BKC invocable: `Invoke` with a nil environment returns a structured error, not a panic
- `NewCompileInvocable` with a valid env registers the correct ID

### Exit Criteria

1. `go build ./...` and `go test ./...` pass.
2. `behaviorRoutineAdapter` type does not exist. (CI check.)
3. `PlanningBehavior` type in `relurpicabilities/planning/` does not exist. (CI check.)
4. `Dispatcher.behaviors` and `Dispatcher.routines` fields do not exist. (CI check.)
5. No type assertion of the form `.ServiceBundle.(` occurs in `runtime/dispatch/`. (CI grep check.)
6. All built-in behaviors and routines implement `execution.Invocable`.
7. `execution.Behavior` interface is deleted. `euclorelurpic.SupportingRoutine` interface is deleted.

---

## Phase 5 — Thought Recipes as First-Class Behaviors

### Goal

Thought recipes implement `execution.Invocable` and are registered in the unified `InvocableRegistry`. The `strings.HasPrefix(behaviorID, "euclo:recipe.")` dispatch bypass in the Dispatcher is removed. Recipes participate in capability sequencing, assurance guarantees, and verification gates identically to built-in behaviors.

### Problem in Detail

Current recipe dispatch path in `Dispatcher.Execute`:

```go
if strings.HasPrefix(behaviorID, "euclo:recipe.") && d.recipeRegistry != nil && d.recipeExecutor != nil {
    plan, ok := d.recipeRegistry.Get(behaviorID)
    if ok {
        recipeResult, err := d.recipeExecutor.Execute(ctx, plan, in.Task, in.Environment)
        ...
        return recipeResultToCoreResult(recipeResult), nil
    }
    return nil, fmt.Errorf("thought recipe %q not found", behaviorID)
}
```

Problems:
- Recipe execution bypasses the `InvokeSupporting` wiring — recipes cannot call supporting routines
- `RecipeResult` → `core.Result` conversion loses recipe-specific metadata (warnings, step artifacts)
- Recipes do not go through the assurance `BeforeVerification` hook
- `d.SetRecipeRegistry(registry, executor)` is a setter mutation pattern on a constructed Dispatcher — fragile if called after first use
- Recipe keywords for Tier-1 classification are loaded separately from the built-in capability descriptor keywords

### What to Build

**`named/euclo/thoughtrecipes/invocable.go` — new file**

```go
// RecipeInvocable wraps a thoughtrecipes.Plan as an execution.Invocable.
// It implements the full Invocable interface so recipes participate in the
// unified registry and dispatch path.
type RecipeInvocable struct {
    Plan     Plan
    Executor *Executor
}

func (r *RecipeInvocable) ID() string       { return r.Plan.CapabilityID }
func (r *RecipeInvocable) IsPrimary() bool  { return true }

func (r *RecipeInvocable) Invoke(ctx context.Context, in execution.InvokeInput) (*core.Result, error) {
    result, err := r.Executor.ExecuteWithInput(ctx, r.Plan, in)
    if err != nil {
        return &core.Result{Success: false, Error: err}, err
    }
    return recipeResultToResult(result), nil
}
```

**`thoughtrecipes.Executor.ExecuteWithInput`** — new method that takes `execution.InvokeInput` instead of `(*core.Task, agentenv.AgentEnvironment)`:

```go
func (e *Executor) ExecuteWithInput(ctx context.Context, plan Plan, in execution.InvokeInput) (*RecipeResult, error)
```

This gives the recipe executor access to `in.InvokeSupporting`, `in.State`, `in.Work`, and `in.ServiceBundle` — enabling recipes to call supporting routines and accumulate artifacts into the canonical state.

**Recipe registration via `capabilities/registry.go`:**

```go
func (r *EucloCapabilityRegistry) LoadAndRegisterRecipes(
    recipeDir string,
    invocableRegistry *dispatch.InvocableRegistry,
    env agentenv.AgentEnvironment,
) error {
    plans, err := thoughtrecipes.LoadPlans(recipeDir)
    if err != nil {
        return err
    }
    executor := thoughtrecipes.NewExecutor(env)
    for _, plan := range plans {
        inv := &thoughtrecipes.RecipeInvocable{Plan: plan, Executor: executor}
        if err := invocableRegistry.Register(inv); err != nil {
            return fmt.Errorf("registering recipe %s: %w", plan.CapabilityID, err)
        }
        // Also register the descriptor for keyword matching in the Tier-1 classifier
        _ = r.descriptorRegistry.Register(plan.ToDescriptor())
    }
    return nil
}
```

**`Plan.ToDescriptor()` — new method on `thoughtrecipes.Plan`:**

```go
func (p Plan) ToDescriptor() euclorelurpic.Descriptor {
    return euclorelurpic.Descriptor{
        ID:                     p.CapabilityID,
        DisplayName:            p.Name,
        ModeFamilies:           p.Modes,
        PrimaryCapable:         true,
        AllowDynamicResolution: true,
        IsUserDefined:          true,
        RecipePath:             p.FilePath,
        Keywords:               p.Keywords,
        Summary:                p.Description,
    }
}
```

This means recipe keywords are registered in the same `euclorelurpic.Registry` as built-in keywords, and the Tier-1 classifier in `intake/enrich.go` sees them automatically.

**Dispatcher cleanup:**

After this phase, `Dispatcher` no longer has:
- `recipeRegistry *thoughtrecipes.PlanRegistry`
- `recipeExecutor *thoughtrecipes.Executor`
- `SetRecipeRegistry` method
- The `strings.HasPrefix("euclo:recipe.")` branch

Recipes are in the `InvocableRegistry` and dispatched identically to built-in behaviors.

**`recipeResultToCoreResult` removal:**

The conversion function is replaced by `thoughtrecipes.RecipeInvocable.Invoke` which constructs a `*core.Result` directly, preserving all fields including `Warnings` and step-level artifacts in `Data`.

**Hot-reload foundation:**

Add `InvocableRegistry.Deregister(id string) bool` and `InvocableRegistry.Replace(inv Invocable) error`. These enable a future watch-and-reload mechanism for the recipe directory without re-initializing the full agent. (The watcher itself is out of scope for this phase — only the registry primitives are added.)

### File Dependencies

**Produces:**
- `named/euclo/thoughtrecipes/invocable.go`

**Modified:**
- `named/euclo/thoughtrecipes/executor.go` — adds `ExecuteWithInput`
- `named/euclo/thoughtrecipes/plan.go` — adds `ToDescriptor()`
- `named/euclo/capabilities/registry.go` — `LoadAndRegisterRecipes` registers into `InvocableRegistry`
- `named/euclo/runtime/dispatch/registry.go` — adds `Deregister`, `Replace`
- `named/euclo/runtime/dispatch/dispatcher.go` — removes recipe bypass branch and `SetRecipeRegistry`
- `named/euclo/agent.go` — removes `initializeThoughtRecipes` call and `SetRecipeRegistry` call; recipes registered in `capabilities/registry.go`

**Deleted:**
- `recipeResultToCoreResult` function in `runtime/dispatch/dispatcher.go`
- `SetRecipeRegistry` method

**Unchanged:**
- `named/euclo/thoughtrecipes/*.go` except additions above
- All `relurpicabilities/` files

### Unit Tests to Write

**`thoughtrecipes/invocable_test.go`**
- `RecipeInvocable.ID()` returns the plan's `CapabilityID`
- `RecipeInvocable.IsPrimary()` returns `true`
- `RecipeInvocable.Invoke` with a plan that has no steps returns a successful result with empty artifacts
- `RecipeInvocable.Invoke` with a plan that calls a supporting routine invokes `in.InvokeSupporting`
- `RecipeInvocable.Invoke` propagates executor errors as `*core.Result{Success: false}`
- `RecipeInvocable.Invoke` preserves `Warnings` in `result.Data["warnings"]`

**`thoughtrecipes/plan_test.go`**
- `Plan.ToDescriptor()` returns a descriptor with `IsUserDefined = true`
- `Plan.ToDescriptor()` copies all keywords from the plan
- `Plan.ToDescriptor()` sets `AllowDynamicResolution = true`
- `Plan.ToDescriptor()` sets `PrimaryCapable = true`

**`capabilities/registry_test.go`**
- `LoadAndRegisterRecipes` with a directory containing two recipe files registers two invocables and two descriptors
- `LoadAndRegisterRecipes` with a malformed YAML file returns an error
- After `LoadAndRegisterRecipes`, `invocableRegistry.Primary("euclo:recipe.my-recipe")` returns the recipe invocable

**`runtime/dispatch/dispatcher_test.go`** (extension)
- Dispatcher with a registered `RecipeInvocable` dispatches to it via `Execute` without any prefix check
- `SetRecipeRegistry` method does not exist (compile-time check via test file importing the package)
- Dispatcher with no recipe registered for `"euclo:recipe.unknown"` returns a `not registered` error, not a `not found` error

**`runtime/dispatch/registry_test.go`** (extension)
- `Deregister` removes a previously registered invocable
- `Deregister` on an unknown ID returns `false`
- `Replace` overwrites an existing invocable without error
- `Replace` for an unknown ID returns an error

### Exit Criteria

1. `go build ./...` and `go test ./...` pass.
2. `strings.HasPrefix(behaviorID, "euclo:recipe.")` does not appear in `runtime/dispatch/`. (CI grep check.)
3. `SetRecipeRegistry` method does not exist. (CI check.)
4. `recipeResultToCoreResult` does not exist. (CI check.)
5. A round-trip test verifies: loading a fixture recipe YAML, registering it, dispatching to it via `Dispatcher.Execute`, and confirming the result contains `recipe_id` in `Data`.
6. The Tier-1 classifier (`intake/enrich.go`) matches a recipe keyword in an instruction and routes to the recipe invocable (integration test with a fixture recipe that has a unique keyword).

---

## Cross-Phase CI Additions

All new CI checks are added at the phase that introduces them. They are cumulative:

| Script | Added in | Checks |
|--------|----------|--------|
| `scripts/check-euclo-state-keys.sh` | Phase 1 | No raw `state.Get("euclo.` or `state.Set("euclo.` or `state.Get("pipeline.` in `named/euclo/` |
| `scripts/check-euclo-deprecated-symbols.sh` | Phase 2 | `runtimeState`, `seedRuntimeState`, `classifyCapabilityIntent` do not exist |
| `scripts/check-assurance-boundaries.sh` | Phase 3 | `applyVerificationAndArtifacts` does not exist; `ShortCircuit` does not call `CollectArtifactsFromState` directly |
| `scripts/check-dispatch-registry.sh` | Phase 4 | `behaviorRoutineAdapter`, `Dispatcher.behaviors`, `Dispatcher.routines`, `PlanningBehavior` do not exist; no `.ServiceBundle.(` in `runtime/dispatch/` |
| `scripts/check-recipe-dispatch.sh` | Phase 5 | No `HasPrefix.*euclo:recipe` in `runtime/dispatch/`; no `SetRecipeRegistry`; no `recipeResultToCoreResult` |

These join the existing boundary scripts and run in CI after `go test ./...`.

---

## Sequencing and Dependencies

```
Phase 1 (Typed State)
    ↓  [state/keys.go is the foundation all later phases use]
Phase 2 (Single-Pass Classification)
    ↓  [ClassifiedEnvelope carries typed state from Phase 1]
Phase 3 (Assurance Decomposition)
    ↓  [services use typed state accessors from Phase 1]
Phase 4 (Registry Unification)
    ↓  [Invocable uses InvokeInput with typed state fields from Phase 1]
Phase 5 (Recipe First-Class)
        [uses unified InvocableRegistry from Phase 4]
```

Phases 2 and 3 are independent of each other (both depend only on Phase 1) and could be worked in parallel by two engineers. Phases 4 and 5 each depend on the previous phase completing cleanly.

---

## Risk Register

| Risk | Phase | Mitigation |
|------|-------|------------|
| State key inventory is incomplete — a key used only in tests or rarely executed paths is missed | 1 | Grep for all `.Get("` and `.Set("` across `named/euclo/` before writing `keys.go`; make CI check fail on any new raw key added |
| `LoadFromContext` migration helper silently drops values for unknown types, masking bugs | 1 | Log (with structured logger when available) every type mismatch in `LoadFromContext`; add a test that verifies no drops occur for any known key |
| Double-`runtimeState` elimination breaks a subtlety in the restore path | 2 | Preserve the restore-rebuild as `intake.RebuildUnitOfWork` with an explicit comment; add a golden fixture test for the restore path |
| Assurance `Runtime` struct change breaks callers in `agent.go` and test harnesses | 3 | Provide a `NewRuntime(...)` constructor that assembles the services; update agent.go in the same PR |
| BKC `IsPrimary() = false` changes routing behavior for any caller that dispatches BKC directly | 4 | Audit all `Dispatcher.Execute` calls with a BKC capability ID before phase exit; add a test that asserts the error behavior |
| Recipe executor `ExecuteWithInput` must not break existing recipe YAML files | 5 | All existing recipe fixture files are run through the new executor in a test; output is compared to the old path's output on the same fixtures |
