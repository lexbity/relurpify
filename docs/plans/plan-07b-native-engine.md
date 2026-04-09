# Plan 07b: Native In-Process Inference Engine

## Goals

Implement a native in-process LLM inference engine (`platform/llm/llamago`) using
llama.go CGo bindings. This engine eliminates HTTP round-trip overhead for local
inference and opens capabilities that transport backends cannot expose: KV cache
session affinity across agent iterations, direct token-level streaming, batch
coalescing of parallel graph branches, and real-time VRAM pressure feedback to the
context manager.

**Dependency:** Requires plan-07a to be complete. The `ManagedBackend` interface,
`BackendCapabilities`, optional interface definitions (`SessionAwareBackend`,
`NativeTokenStream`, `BatchInferenceBackend`, `BackendResourceReporter`,
`ModelController`, `BackendRecovery`), and the `platform/llm` factory must all exist
before this plan begins.

**Split from plan-07:** This plan covers Phases 11–12 from the original design.
Provider facade and transport backend work is plan-07a. Advanced framework
integrations that consume this engine (KV cache wiring, batch graph coalescing, Nexus
advertisement, session portability) are plan-07c.

---

## Why

### Transport overhead and small-model constraints

The framework's design philosophy is enabling small models to complete tasks they
otherwise could not. Transport-backed providers (Ollama, LM Studio) add HTTP framing,
JSON serialisation, and round-trip latency to every inference call. For an agent loop
that issues dozens of inference calls per task, this overhead accumulates. More
importantly, HTTP transports cannot expose: KV cache slot affinity across calls,
direct sampler-level token streaming, multi-request batch coalescing, or real-time
VRAM pressure signalling. These are structural advantages for small-model agent loops,
not optimisations.

### In-process with principled failsafes

A managed-worker subprocess was considered and rejected. With the mesh routing work
at the session level (not the inference level), there is no distribution use case for
a networked worker. The remaining benefit of a worker — crash isolation — is real but
does not outweigh the latency cost and deployment complexity for the local inference
scenario this engine targets.

Instead, the plan invests in a layered failsafe architecture: pre-flight validation
prevents the most common crash causes, an abort callback provides Go-to-C
cancellation, a watchdog goroutine handles liveness failures, and a health state
machine degrades gracefully for recoverable errors.

**Hard constraint:** A true SIGSEGV or SIGABRT from C code will terminate the
process. This is documented and accepted. The failsafes protect against everything
short of that.

---

## Dependency Order

```
Phase 11 → Phase 12
```

Phase 12 has a CGo dependency; Phase 11 does not. Phase 11 can be completed and
merged independently, giving the safety infrastructure test coverage before the
hard-to-test CGo inference code is added.

---

## Phase 11: platform/llm/llamago — Failsafe foundation

### Objective

Build the complete failsafe and lifecycle infrastructure for the in-process native
engine before writing any inference code. This phase defines what "safe enough" means
for in-process CGo and establishes the patterns every subsequent inference call uses.

### Context

Everything short of a true SIGSEGV/SIGABRT can be handled. This phase builds those
defences first so that the actual inference implementation (Phase 12) is written
within a safety envelope from the start.

### Work

**`platform/llm/llamago/` (new subpackage)**

`config.go`:
```go
type InProcessConfig struct {
    ModelPath        string
    ContextSize      int
    Threads          int
    GPULayers        int
    BatchSize        int
    FlashAttn        bool
    InferenceTimeout time.Duration
    ErrorThreshold   int     // errors before transitioning to Unhealthy
    CrashReportDir   string
    Config           map[string]any
}

func (c *InProcessConfig) validate() error { /* param range checks */ }
```

`preflight.go`:
- `validateBeforeLoad(cfg InProcessConfig) error`:
  1. Open model file; read 4 magic bytes; check for GGUF magic (`0x47 0x47 0x55 0x46`).
  2. Call `cfg.validate()` for parameter range checks.
  3. Estimate memory requirement (file size × 1.3); check against `runtime.MemStats.Sys`.
  4. Return a descriptive error for each failure mode.

`watchdog.go`:
- `inferenceWatchdog` struct with `timeout`, `abortCh chan<- struct{}`, `activeCh`,
  `stopCh`.
- `run()` goroutine: polls active requests every 250ms; signals `abortCh` and calls
  the request's fail function when timeout exceeded.
- `startRequest(id string, failFn func(error))` / `completeRequest()`.

`health.go`:
- `backendHealth` struct holding `atomic.Value` for `BackendHealthState`, error
  count, last error, uptime.
- `setState(s BackendHealthState)` emits `EventBackendStateChange` via telemetry.
- `onError(err error, stack []byte)` increments error count, stores last error, and
  transitions state.
- `toHealthReport() *llm.HealthReport`.

`recovery.go`:
- `BackendRecovery` implementation:
  - `Restart(ctx) error`: calls `closeInternal()`, `validateBeforeLoad()`,
    `loadModel(ctx)` in sequence; resets error count on success; updates state.
  - `ErrorHistory() []BackendError`: returns last N errors from ring buffer.

`crashreport.go`:
- `installCrashHandlers(cfg InProcessConfig, health *backendHealth)`: installs
  `SIGABRT` handler (not SIGSEGV — too dangerous to handle); writes crash report
  JSON to `CrashReportDir` on signal receipt.
- `writeCrashReport(reason string, health *backendHealth)`: writes model path, error
  history, goroutine stacks, timestamp.

`backend.go` (skeleton — no CGo yet):
```go
type InProcessBackend struct {
    cfg       InProcessConfig
    health    *backendHealth
    watchdog  *inferenceWatchdog
    abortCh   chan struct{}
    requestCh chan inferRequest
    telemetry core.Telemetry
    // model and context fields added in Phase 12
}

func NewBackend(cfg InProcessConfig, tel core.Telemetry) (*InProcessBackend, error) {
    if err := cfg.validate(); err != nil {
        return nil, err
    }
    // ...
}

func (b *InProcessBackend) Warm(ctx context.Context) error {
    if err := validateBeforeLoad(b.cfg); err != nil {
        return err
    }
    b.health.setState(BackendHealthReady) // placeholder until Phase 12
    return nil
}
```

### Tests

**`platform/llm/llamago/preflight_test.go`**
- `TestValidateBeforeLoad_ValidGGUF`: temp file with GGUF magic bytes, valid params,
  sufficient memory → nil error.
- `TestValidateBeforeLoad_WrongMagic`: file with wrong magic bytes → error mentioning
  "GGUF".
- `TestValidateBeforeLoad_FileNotFound`: non-existent path → error mentioning path.
- `TestValidateBeforeLoad_InsufficientMemory`: artificially low memory threshold →
  error mentioning memory.
- `TestValidateConfig_ContextSizeZero`: context_size 0 → error.
- `TestValidateConfig_ContextSizeTooLarge`: context_size > 1<<20 → error.
- `TestValidateConfig_ThreadsZero`: threads 0 → error.
- `TestValidateConfig_GPULayersNegative`: gpu_layers -1 → error.
- `TestValidateConfig_ValidParams`: all in-range → nil error.

**`platform/llm/llamago/watchdog_test.go`**
- `TestWatchdog_TimeoutTriggersAbortAndFail`: request started; no completion within
  timeout → `abortCh` receives signal, fail function called with `ErrInferenceTimeout`.
- `TestWatchdog_CompleteBeforeTimeout`: request started; completed before timeout →
  no abort, no fail.
- `TestWatchdog_Stop`: stop channel closed → `run()` goroutine exits cleanly.

**`platform/llm/llamago/health_test.go`**
- `TestHealth_InitialState_Ready`: new backend → `State` is `ready`.
- `TestHealth_OnError_BelowThreshold`: single error → `State` is `degraded`.
- `TestHealth_OnError_AtThreshold`: errors equal threshold → `State` is `unhealthy`.
- `TestHealth_TransitionEmitsTelemetry`: state change → telemetry event emitted with
  correct `EventBackendStateChange`.
- `TestHealth_ToHealthReport`: health struct → `HealthReport` populated correctly.

**`platform/llm/llamago/recovery_test.go`**
- `TestRestart_FromDegraded_Succeeds`: mock load function succeeds → state resets to
  `ready`, error count cleared.
- `TestRestart_LoadFails`: mock load function returns error → state remains `unhealthy`.
- `TestErrorHistory_LimitedRingBuffer`: more errors than ring buffer capacity → oldest
  errors evicted.

**`platform/llm/llamago/backend_test.go`**
- `TestNewBackend_InvalidConfig`: invalid config → error before construction.
- `TestWarm_PreflightFails`: GGUF validation fails → `Warm()` returns error, state
  remains uninitialized.
- `TestInProcessBackend_ImplementsManagedBackend`: compile-time interface check.
- `TestInProcessBackend_ImplementsBackendRecovery`: compile-time interface check.

**`platform/llm/llamago/safety_test.go`** (permanent safety regression suite)
- All of the above must pass after any future change to this package. These are the
  first line of defence against regressions in the failsafe layer; none require CGo.

### Exit criteria

- All failsafe infrastructure compiles and tests pass with no CGo dependency.
- `go test ./platform/llm/llamago/...` passes.
- The inference goroutine pattern, watchdog, health state machine, and crash reporter
  are all implemented and tested independently of the llama.cpp binding.

---

## Phase 12: platform/llm/llamago — Full inference implementation

### Objective

Implement the complete in-process inference path using llama.go CGo bindings,
including all optional interfaces.

### Context

This phase adds the CGo dependency. All tests that require a real model file use a
build tag (`//go:build integration`) and are skipped in CI unless a model is present.
Unit-testable portions (token channel plumbing, session slot management, abort
callback wiring) use mocks or are structured to be callable from tests without a live
model.

### Work

**`platform/llm/llamago/backend.go`** (complete)
- Add `model *llama.Model`, `lctx *llama.Context`, `sessions map[string]*sessionSlot`,
  `nextSeqID int`, `mu sync.Mutex`.
- `inferenceLoop()`: `runtime.LockOSThread()`, `debug.SetPanicOnFault(true)`,
  `defer recover()` calling `b.onError()`, processes `requestCh`.
- `processWithRecovery()`: deferred panic recovery; checks health state before
  processing; calls `processInferRequest()`.

**`platform/llm/llamago/model.go`**
- `Warm()`: calls `validateBeforeLoad`, then `llama.LoadModel`, then
  `llama.NewContext`, then starts inference goroutine and watchdog, then sets abort
  callback via `lctx.SetAbortCallback`.
- `Close()`: signals abort channel, closes `requestCh`, waits for inference goroutine,
  calls `lctx.Free()` then `model.Free()`.

**`platform/llm/llamago/infer.go`**
- `processInferRequest()`: tokenize messages, find longest cached prefix in session
  slot, call `llama_decode` for new suffix only, sample tokens until EOS or stop.
- Token sampling loop: signal to `abortCh` if context cancelled; write to token
  channel for streaming requests; accumulate text for non-streaming.

**`platform/llm/llamago/session.go`** — implements `SessionAwareBackend`:
- `sessionSlot` struct: `seqID int`, `cachedToks []int32`, `nCached int`.
- `WithSession(sessionID)`: allocates slot if absent, returns `sessionBoundModel`.
- `EvictSession(ctx, sessionID)`: sends eviction request to inference goroutine;
  goroutine calls `llama_kv_cache_seq_rm`.
- `ActiveSessions(ctx)`: returns keys of sessions map.

**`platform/llm/llamago/stream.go`** — implements `NativeTokenStream`:
- `ChatTokenStream()`: sends a streaming inference request; inference goroutine writes
  `Token` values to the returned channel directly from the sampler.

**`platform/llm/llamago/batch.go`** — implements `BatchInferenceBackend`:
- `ChatBatch()`: builds combined `llama_batch` for all requests; processes in one
  `llama_decode` call; demultiplexes results.

**`platform/llm/llamago/resources.go`** — implements `BackendResourceReporter`:
- `ResourceSnapshot()`: calls `lctx.KVCacheUsedCells()`,
  `lctx.KVCacheTokenCount()`, model size from `model.SizeBytes()`.

**`platform/llm/llamago/embedder.go`** — implements `retrieval.Embedder`:
- `Embed()`: inference in embedding mode (no causal mask, pool hidden state).

**`platform/llm/llamago/modelcontroller.go`** — implements `ModelController`:
- `LoadModel()` / `UnloadModel()` / `ModelInfo()`.

**`platform/llm/factory.go`** — add `"llamago"` / `"native"` case.

### Tests

**Unit tests (no model required)**
- `TestSessionSlot_PrefixMatching`: given cached tokens [1,2,3] and new sequence
  [1,2,3,4,5], prefix match returns 3; only [4,5] need eval.
- `TestSessionSlot_PrefixMismatch`: new sequence diverges at position 2; match
  returns 2; slots 2..end evicted.
- `TestAbortChannel_SignaledOnContextCancel`: inference request with already-cancelled
  context → abort channel receives signal immediately.
- `TestProcessWithRecovery_PanicRecovered`: processInferRequest replaced by a function
  that panics → `recover()` catches it, `onError()` called, request fails with error.
- `TestTokenChannel_BufferedDelivery`: inference goroutine writes to buffered channel;
  consumer reads after goroutine completes; no deadlock.

**Integration tests (require model file, build tag `integration`)**
- `TestWarm_RealModel`: loads a small GGUF model → `Warm()` succeeds, state is ready.
- `TestChat_RealModel`: single turn chat → non-empty `LLMResponse.Text`.
- `TestChatStream_RealModel`: streaming → tokens received before final event.
- `TestSessionAffinity_KVCacheReuse`: two consecutive calls with same session ID and
  shared prefix → second call is measurably faster (timing-based).
- `TestBatchInference_RealModel`: two requests in one batch → both responses correct.
- `TestEmbedder_RealModel`: embed a short text → vector with correct dimensions.
- `TestClose_ReleaseVRAM`: `Warm()` then `Close()` → model freed without panic.

### Exit criteria

- All unit tests pass without a model file.
- Integration tests pass when run with `go test -tags integration` and a model path
  env var set.
- `InProcessBackend` satisfies `ManagedBackend`, `SessionAwareBackend`,
  `NativeTokenStream`, `BatchInferenceBackend`, `BackendResourceReporter`,
  `ModelController`, `BackendRecovery`.
- `go test ./platform/llm/llamago/...` passes (unit only by default).

---

## Cross-Cutting Testing Strategy

### Native engine safety regression

`platform/llm/llamago/safety_test.go` runs after any change to the backend:
- Pre-flight validation tests (no CGo required).
- Watchdog timeout tests (no CGo required).
- Health state machine transitions (no CGo required).
- Panic recovery (no CGo required).
These must always pass; they are the first line of defence against regressions in the
failsafe layer.

### Integration test gating

Integration tests use build tag `integration` and are skipped in standard CI. A
model path is supplied via environment variable. These are run manually or in a
dedicated CI job that has a model file available.

---

## Key Risks and Mitigations

**Risk: CGo crash in native backend.**
Mitigation: Documented hard constraint (SIGSEGV terminates the process). Pre-flight
validation, watchdog, and health state machine reduce probability of reaching C code
in a bad state. Crash report infrastructure captures context on SIGABRT before the
process dies.

**Risk: KV cache/context budget feedback creates unexpected compression cycles.**
Mitigation: This risk is addressed in plan-07c where the budget adaptor is wired in.
The native engine itself makes no assumptions about how callers use KV cache state.

**Risk: Integration tests require a specific model file format/size.**
Mitigation: Tests accept any GGUF model via env var. A small quantised model (e.g.
1B–3B parameter) is sufficient for functional correctness tests. Timing-based tests
(KV cache reuse) are allowed to be flaky on very fast hardware.
