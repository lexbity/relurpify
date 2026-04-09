# Plan 07c: Advanced Framework Integrations

## Goals

Wire the native in-process inference engine (plan-07b) into the broader framework:
KV cache session affinity through the agent session lifecycle, batch coalescing of
parallel graph branches, inference capability advertisement to Nexus, and a portable
session export/import contract for agent session migration between nodes.

**Dependencies:**
- Plan-07a must be complete: `ManagedBackend`, `BackendCapabilities`, and the full
  optional interface set must exist and be stable.
- Plan-07b must be complete: `InProcessBackend` implementing `SessionAwareBackend`,
  `BatchInferenceBackend`, `BackendResourceReporter` must exist.

**Split from plan-07:** This plan covers Phases 13–16 from the original design.

---

## Scope note: Session migration trust reduction

Phase 16 (session portability) includes a `PermissionPolicy` field in the portable
session envelope. **Trust-class reduction at import time** — where the source node's
granted permissions exceed the destination node's allowed trust level and capabilities
require re-approval — is **out of scope for this plan**. It is a non-trivial
cross-cutting concern that depends on the Nexus mesh trust topology being defined.
`ImportSession` in this plan applies the permission policy as-is without reduction.
Trust reduction will be addressed in a dedicated plan once the mesh trust model is
stable.

---

## Dependency Order

```
Phase 13 (independent after 07b)
Phase 14 (independent after 07b)
Phase 15 (independent after 07b)
Phase 16 (independent after 07b)
```

All four phases depend on plan-07b being done. They are independent of each other
and may proceed in any order or in parallel.

---

## Phase 13: Context manager and SessionAwareBackend integration

### Objective

Wire `SessionAwareBackend` into the agent session lifecycle so that native backends
reuse KV cache across iterations, and connect `BackendResourceReporter` to the
context budget so KV cache pressure informs compression decisions.

### Context

`SessionMeta.ID` in `tui/session_store.go` is the correct session ID anchor — it
spans the full TUI session across multiple task iterations. The context budget has a
`BudgetListener` interface with `OnBudgetWarning` and `OnBudgetExceeded`. The native
backend can implement `BudgetListener` to receive advance notice before the context
manager prunes, allowing it to pre-emptively mark cached token sequences as partially
stale rather than discovering the mismatch on the next inference call.

### Work

**`app/relurpish/runtime/bootstrap.go`**
- After constructing the agent, if `opts.Backend` implements `SessionAwareBackend`,
  call `backend.WithSession(sessionID)` and store the session-bound model in the
  bootstrapped runtime.
- Add `SessionID string` to `BootstrappedAgentRuntime`.

**`app/relurpish/runtime/runtime.go`** (session lifecycle)
- On session start: if backend implements `SessionAwareBackend`, bind session.
- On session end / shutdown: call `backend.EvictSession(ctx, sessionID)`.

**`framework/contextmgr/context_policy.go`**
- Add optional `BackendListener BackendBudgetAdaptor` field to
  `ContextPolicyConfig`.
- If set, register it as a `BudgetListener` on the `ContextBudget`.

**`platform/llm/llamago/budget_adaptor.go`** (new)
- `BackendBudgetAdaptor` struct holds a reference to `InProcessBackend`.
- Implements `core.BudgetListener`:
  - `OnBudgetWarning(usage float64)`: queries `ResourceSnapshot()`; if KV cache
    pressure > 0.85, calls `evictLRUSessions(1)`.
  - `OnBudgetExceeded(category string, requested, available int)`: marks session slot
    `nCached` values down to `available` so next inference call knows the prefix cache
    is stale.

### Tests

**`app/relurpish/runtime/bootstrap_test.go`** (additions)
- `TestBootstrap_SessionBinding_SessionAwareBackend`: mock `SessionAwareBackend` →
  `WithSession` called with non-empty session ID during bootstrap.
- `TestBootstrap_NoSessionBinding_NonSessionAware`: backend without
  `SessionAwareBackend` → no panic, no binding attempt.
- `TestSessionEviction_OnShutdown`: runtime shutdown → `EvictSession` called on
  session-aware backend.

**`platform/llm/llamago/budget_adaptor_test.go`**
- `TestOnBudgetWarning_LowKVPressure`: KV cache at 50% → no eviction.
- `TestOnBudgetWarning_HighKVPressure`: KV cache at 90% → `evictLRUSessions` called.
- `TestOnBudgetExceeded_StaltsSessionCache`: two sessions with 500 cached tokens each;
  `available=200` → both sessions' `nCached` reduced to 200.

**`framework/contextmgr/context_policy_test.go`** (additions)
- `TestContextPolicy_RegistersBackendListener`: config with `BackendListener` →
  listener registered on budget.

### Exit criteria

- Native backend KV cache is evicted when session ends.
- Context manager and native backend can communicate through the budget listener
  without circular imports.
- `go test ./app/relurpish/runtime/...` passes.
- `go test ./platform/llm/llamago/...` passes.

---

## Phase 14: Graph runtime and BatchInferenceBackend integration

### Objective

Allow parallel graph branches to coalesce their LLM calls into a single batch
request when the active backend supports it.

### Context

`framework/graph` currently executes parallel branches by dispatching goroutines that
each call `model.Chat()` independently. For a native backend with `llama_batch`,
these can be submitted as a single decode pass for better GPU utilisation. The graph
runtime should detect `BatchInferenceBackend` and coalesce when available; sequential
fallback must remain correct for backends that do not implement it.

### Work

**`framework/graph/` (LLM node execution)**
- Identify the point where parallel branch LLM calls are dispatched.
- Before dispatch: type-assert `BatchInferenceBackend` on the model/backend.
- If supported: collect pending LLM calls from all parallel branches into a
  `[]BatchRequest`; call `ChatBatch()`; distribute responses back to waiting branches.
- If not supported: existing sequential dispatch unchanged.
- The `ManagedBackend` must be accessible from the graph execution context. Pass it
  through the graph execution options or store it alongside the model reference.

### Tests

**`framework/graph/batch_test.go`** (new)
- `TestParallelBranches_BatchCoalescing`: graph with two parallel LLM nodes; backend
  implements mock `BatchInferenceBackend` that records call count → `ChatBatch`
  called once (not twice); both branches receive correct responses.
- `TestParallelBranches_SequentialFallback`: same graph; backend does NOT implement
  `BatchInferenceBackend` → `Chat` called twice; both branches correct; no panic.
- `TestBatchCoalescing_ContextCancellation`: context cancelled while batch in flight →
  both branches receive context error; no goroutine leak.
- `TestBatchCoalescing_PartialFailure`: `ChatBatch` returns error for one request,
  success for other → failing branch gets error, succeeding branch gets response.

### Exit criteria

- No change to existing single-branch graph behaviour.
- `go test ./framework/graph/...` passes.

---

## Phase 15: Inference capability advertisement

### Objective

Publish the node's inference capability to Nexus on connection so the scheduler can
route tasks based on what model and hardware each node has.

### Context

Nexus already has `UpdateNodeCapabilities` and `ApprovedCapabilities` on node
records. What is needed is a structured type for inference capability that the runtime
constructs after `Warm()` and publishes when connecting to Nexus. This is a one-way
publish from the runtime to Nexus; Nexus never calls back into the inference backend.

### Work

**`framework/core/` or `app/relurpish/runtime/`**
- Add `InferenceCapabilityAd`:
  ```go
  type InferenceCapabilityAd struct {
      Provider      string
      ModelName     string
      ModelFamily   string
      ParameterSize string
      ContextSize   int
      Quantization  string
      BackendClass  string
      HasEmbeddings bool
      HasGPU        bool
      VRAMTotalMB   int64
  }
  ```

**`app/relurpish/runtime/runtime.go`**
- After `backend.Warm()` succeeds: construct `InferenceCapabilityAd` from
  `backend.Capabilities()`, `backend.ListModels()`, and (if available)
  `BackendResourceReporter.ResourceSnapshot()`.
- If Nexus client is connected: call `UpdateNodeCapabilities` with the ad included.
- On model switch (TUI): re-construct and re-publish.
- If Nexus is not connected (offline node): skip publish silently; log at debug level.

### Tests

**`app/relurpish/runtime/capability_ad_test.go`** (new)
- `TestBuildCapabilityAd_TransportBackend`: mock Ollama backend → ad has correct
  provider, model name, BackendClass "transport".
- `TestBuildCapabilityAd_NativeBackend_WithResources`: mock native backend with
  `BackendResourceReporter` → ad includes VRAM total and HasGPU.
- `TestBuildCapabilityAd_NativeBackend_NoResources`: native backend without resource
  reporter → ad still populated with available fields; no panic.
- `TestPublishCapabilityAd_Connected`: Nexus client mock connected →
  `UpdateNodeCapabilities` called with ad.
- `TestPublishCapabilityAd_Disconnected`: Nexus client nil → no call, no error, no
  panic.
- `TestPublishCapabilityAd_OnModelSwitch`: model changed in TUI → re-publish called.

### Exit criteria

- Offline nodes complete `Warm()` without any Nexus call.
- Connected nodes publish a well-formed `InferenceCapabilityAd` after `Warm()`.
- `go test ./app/relurpish/runtime/...` passes.

---

## Phase 16: Context portability and session migration

### Objective

Implement a portable session export/import contract at the agent runtime level so
that agent sessions can migrate between Nexus nodes.

### Context

The existing `ProviderSnapshotter` / `ProviderRestorer` / `ProviderSessionSnapshotter`
/ `ProviderSessionRestorer` interfaces in `framework/core/provider_types.go` provide
the foundation. What is needed is a concrete session envelope type and implementations
of these interfaces at the agent runtime level.

The KV cache is explicitly excluded from migration — it is a local acceleration that
is rebuilt from message history on the destination node.

**Trust reduction is out of scope** — see plan scope note above. Permission policies
migrate with the session but are applied as-is on the destination node. A separate
plan will address trust-class reduction once the Nexus mesh trust topology is stable.

### Work

**`app/relurpish/runtime/session_export.go`** (new)
- `PortableSessionEnvelope`:
  ```go
  type PortableSessionEnvelope struct {
      SessionID       string
      AgentName       string
      ModelHint       string        // informational; destination uses its own model
      ProviderHint    string        // informational
      ExportedAt      time.Time
      Context         *core.ContextSnapshot
      MessageHistory  []core.Message
      WorkflowState   *WorkflowStateSnapshot  // if mid-task
      PermissionPolicy PolicySnapshot         // applied as-is; no trust reduction
      SchemaVersion   string
  }
  ```
- `ExportSession(sessionID string, rt *BootstrappedAgentRuntime) (*PortableSessionEnvelope, error)`:
  serialises all migratable fields. Calls `EvictSession` on backend after export.
- `ImportSession(env *PortableSessionEnvelope, backend llm.ManagedBackend, ...) (*BootstrappedAgentRuntime, error)`:
  reconstructs runtime from envelope. Permission policy applied as-is (trust-class
  reduction is out of scope for this plan).
- Implement `ProviderSnapshotter` and `ProviderRestorer` on the runtime struct using
  the above.

### Tests

**`app/relurpish/runtime/session_export_test.go`** (new)
- `TestExportSession_RoundTrip`: export then import → `core.ContextSnapshot` fields
  equal; message history equal.
- `TestExportSession_KVCacheNotIncluded`: export with native backend → envelope
  contains no KV cache data; `EvictSession` called on source backend.
- `TestImportSession_FreshKVCache`: import with native `SessionAwareBackend` →
  `WithSession` called; slot starts empty (nCached = 0).
- `TestImportSession_PermissionPolicyApplied`: envelope with a permission policy →
  imported runtime reflects that policy unchanged (trust-class reduction is out of
  scope; see plan scope note).
- `TestExportSession_MidTask_WorkflowStateIncluded`: session with active workflow →
  `WorkflowState` populated in envelope.
- `TestEnvelopeSerialisation_JSONRoundTrip`: envelope marshals and unmarshals without
  data loss.
- `TestExportSession_OfflineNode_Succeeds`: no Nexus connection → export succeeds
  locally; envelope can be stored for later transfer.

### Exit criteria

- A session can be exported to a `PortableSessionEnvelope`, serialised to JSON, and
  imported on a second runtime instance with no loss of context or message history.
- KV cache state is never present in the envelope.
- `go test ./app/relurpish/runtime/...` passes.

---

## Cross-Cutting Testing Strategy

### Offline-first verification

A subset of runtime bootstrap tests runs with Nexus client set to nil:
- `Warm()` succeeds.
- `ProbeEnvironment()` completes.
- Capability advertisement skipped without error.
- Session export completes and envelope is valid.

### Transport backend regression

All phases in this plan must not regress transport backend (Ollama, LM Studio)
behaviour. Mock `ManagedBackend` stubs that do not implement the optional interfaces
(`SessionAwareBackend`, `BatchInferenceBackend`, `BackendResourceReporter`) must
continue to work correctly in all code paths added by this plan.

---

## Key Risks and Mitigations

**Risk: KV cache / context budget feedback creates unexpected compression cycles.**
Mitigation: `OnBudgetExceeded` marks sessions stale without forcing eviction; eviction
is triggered only by `OnBudgetWarning` when KV pressure threshold is also exceeded.
The budget adaptor tests verify this threshold logic explicitly.

**Risk: Batch coalescing changes graph execution semantics.**
Mitigation: The sequential fallback path is tested explicitly (`TestParallelBranches_SequentialFallback`).
Batch coalescing is only activated via type assertion — if the backend does not
implement `BatchInferenceBackend`, the existing path is unchanged.

**Risk: Session migration permission policy reduction silently drops capabilities.**
Mitigation: Trust-class reduction is explicitly out of scope for this plan. The
envelope carries the policy as-is. The separate trust-reduction plan will introduce
explicit logging and re-approval flows when it lands.
