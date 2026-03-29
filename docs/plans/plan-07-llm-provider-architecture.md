# Plan 07: LLM Provider Architecture

## Goals

This plan restructures `platform/llm` from a single Ollama implementation into a
durable, provider-oriented inference architecture. It introduces a native in-process
inference engine path alongside transport-backed providers, decouples retrieval
embeddings from Ollama, generalises all provider-specific assumptions out of the
runtime, config, probe, and TUI layers, and integrates native engine capabilities
directly into the framework's context management and execution graph.

---

## Why

### Vendor lock-in

The current system has Ollama assumptions in six distinct layers: the framework core
spec type (`OllamaToolCalling`), the platform LLM package (single implementation),
the runtime config (`OllamaEndpoint`, `OllamaModel`), the bootstrap options struct,
the probe/doctor layer (`OllamaReport`, `detectOllama`), and the retrieval embedder
(`ollama_embedder.go`). Adding any second provider today would require touching all
six layers simultaneously with no stable interface to guide what changes are safe.
The framework deserves a provider facade with a stable contract, not a hardwired
client.

### Native inference engine

The framework's design philosophy is enabling small models to complete tasks they
otherwise could not. Transport-backed providers (Ollama, LM Studio) add HTTP
framing, JSON serialization, and round-trip overhead to every inference call. A
native in-process engine eliminates all of that. More importantly, it opens
capabilities that HTTP transports cannot expose: KV cache session affinity across
agent iterations, direct token-level streaming, batch coalescing of parallel graph
branches, and real-time VRAM pressure feedback to the context manager. These are not
optimisations — they are structural advantages for small-model agent loops.

### Mesh resource sharing without inference routing

The Nexus mesh is designed to share agent tasks, sessions, and context across nodes,
not to route raw inference calls. Each node owns its own inference backend — whatever
model and hardware it has — and the mesh scheduler uses advertised inference
capability to route work to the right node. This means `platform/llm` is always a
local concern. Inference never crosses a node boundary. What does cross node
boundaries is the agent session state (context, history, workflow position), and that
requires a clean session portability contract at the runtime level.

### Local only support

Nodes may be local-only and permanently offline from the mesh. The inference backend
must work with no network access other than whatever the local backend requires.
Transport-backed backends (Ollama running locally, LM Studio running locally) satisfy
this naturally. The native in-process engine satisfies it definitively — it is a local
file and a CGo call. The Nexus mesh is optional infrastructure, not a dependency for
inference.

### In-process native engine with principled failsafes

A managed-worker subprocess was considered and rejected for the native engine. With
the mesh routing work at the session level (not the inference level), there is no
distribution use case for a networked worker. The remaining benefit of a worker —
crash isolation — is real but does not outweigh the latency cost and deployment
complexity for the local inference scenario this engine targets. Instead, the plan
invests in a layered failsafe architecture: pre-flight validation prevents the most
common crash causes, an abort callback provides Go-to-C cancellation, a watchdog
goroutine handles liveness failures, and a health state machine degrades gracefully
for recoverable errors. The honest constraint is documented: a true SIGSEGV from C
code will terminate the process. The failsafes protect against everything short of
that.

---

## Design Principles

1. `core.LanguageModel` remains the agent-facing execution contract. Agents never
   see provider names, endpoints, or backend-specific config.

2. `ManagedBackend` is the runtime-facing ownership contract. The runtime boots,
   warms, and closes backends through this interface. All provider-specific behaviour
   lives below it.

3. Optional interfaces (`SessionAwareBackend`, `NativeTokenStream`,
   `BatchInferenceBackend`, `BackendResourceReporter`, `ModelController`,
   `BackendRecovery`) allow native engines to expose capabilities the framework can
   use without requiring every backend to implement everything. Framework code
   type-asserts these at the point of use.

4. Framework-native capability calling is the mandatory fallback for all backends.
   Backend-native tool calling is an optional optimisation. The handoff contract is
   owned by `framework/capability`, not by any provider package.

5. `platform/llm` is a local concern. It never speaks to another node. Inference
   capability is advertised to Nexus as metadata; Nexus never calls into the inference
   backend.

6. Transport-backed backends include distributed inference engines (vLLM, TGI) that
   present an HTTP endpoint. From the framework's perspective these are identical to
   single-server transports — one endpoint, one `ManagedBackend`.

7. Backward compatibility is preserved through a single normalisation layer. Old
   config field names (`OllamaEndpoint`, `OllamaModel`, `OllamaToolCalling`) are
   accepted and mapped internally. Direct reads of deprecated fields outside that
   layer are forbidden.

---

## Dependency Order

```
Phase 1 → Phase 2 → Phase 3
                  → Phase 4
          Phase 3 → Phase 5 → Phase 6
                             → Phase 7
                             → Phase 8
          Phase 2 → Phase 9
          Phase 4 → Phase 10
          Phase 2 → Phase 11 → Phase 12
          Phase 5 → Phase 13
          Phase 2 → Phase 14
          Phase 5 → Phase 15
          Phase 5 → Phase 16
  Phases 3,10,12 → Phase 17
        All above → Phase 18
```

Phases 6, 7, 8, 9 may proceed in parallel after Phase 5.
Phase 10 may proceed in parallel with Phase 5 after Phase 4.
Phases 11 and 12 may proceed in parallel with Phase 5 after Phase 2.
Phases 13 and 14 may proceed in parallel after their respective prerequisites.

---

## Phase 1: framework/core — Spec and manifest neutralisation

### Objective

Remove the only provider name that currently exists in the framework type system and
replace it with a provider-neutral equivalent.

### Context

`AgentRuntimeSpec.OllamaToolCalling *bool` embeds a provider name in
`framework/core`, the deepest shared layer in the codebase. Every agent manifest that
sets `ollama_tool_calling` depends on it. This must be resolved before any other
layer can be cleanly abstracted because the spec type is used throughout bootstrap,
agent construction, and capability calling decisions.

### Work

**`framework/core/agent_spec.go`**
- Add `NativeToolCalling *bool` field with yaml/json tag `native_tool_calling`.
- Retain `OllamaToolCalling *bool` tagged `ollama_tool_calling,omitempty` as a
  deprecated alias (no removal yet).
- Replace `ToolCallingEnabled()` logic: return `*NativeToolCalling` if set, else
  fall back to `*OllamaToolCalling` if set, else return `true`.
- Add `NativeToolCallingEnabled() bool` as the canonical accessor going forward.
  `ToolCallingEnabled()` becomes an alias for backward compatibility.

**`framework/core/agent_spec_overlay.go`** (if it exists)
- Ensure overlay merging handles both field names without double-applying.

### Tests

- `TestNativeToolCallingEnabled_NewField`: manifest with `native_tool_calling: false`
  → returns false.
- `TestNativeToolCallingEnabled_LegacyField`: manifest with `ollama_tool_calling:
  false`, no `native_tool_calling` → returns false via compat mapping.
- `TestNativeToolCallingEnabled_Default`: manifest with neither field → returns true.
- `TestNativeToolCallingEnabled_NewFieldWins`: both fields set with conflicting values
  → `native_tool_calling` takes precedence.
- `TestAgentSpecValidate_BothFieldsCoexist`: spec with both fields set validates
  without error.

### Exit criteria

- A manifest with `ollama_tool_calling: false` behaves identically to before.
- A manifest with `native_tool_calling: false` disables native tool calling.
- `framework/core` contains no other provider names.
- `go test ./framework/core/...` passes.

---

## Phase 2: platform/llm — ManagedBackend interface and optional contracts

### Objective

Define the complete interface surface for all inference backends before any
implementation moves.

### Context

Without stable interfaces, every provider implementation will be written to different
shapes and the factory layer will have no contract to enforce. All interface
definitions must exist before subpackages are created so that implementations are
written to the contract from the start, not retrofitted later.

### Work

**`platform/llm/backend.go`** (new file)

```go
type ManagedBackend interface {
    Model() core.LanguageModel
    Embedder() retrieval.Embedder      // nil if not supported
    Capabilities() BackendCapabilities
    Health(ctx context.Context) (*HealthReport, error)
    ListModels(ctx context.Context) ([]ModelInfo, error)
    Warm(ctx context.Context) error
    Close() error
    SetDebugLogging(enabled bool)
}

type BackendCapabilities struct {
    NativeToolCalling bool
    Streaming         bool
    Embeddings        bool
    ModelListing      bool
    BackendClass      BackendClass
}

type BackendClass string
const (
    BackendClassTransport BackendClass = "transport"
    BackendClassNative    BackendClass = "native"
)

type BackendHealthState string
const (
    BackendHealthReady      BackendHealthState = "ready"
    BackendHealthDegraded   BackendHealthState = "degraded"
    BackendHealthUnhealthy  BackendHealthState = "unhealthy"
    BackendHealthRecovering BackendHealthState = "recovering"
)

type HealthReport struct {
    State       BackendHealthState
    Message     string
    LastError   string
    LastErrorAt time.Time
    ErrorCount  int64
    UptimeSince time.Time
    Resources   *ResourceSnapshot
}

type ModelInfo struct {
    Name         string
    Family       string
    ParameterSize string
    ContextSize  int
    Quantization string
    HasGPU       bool
}

type ResourceSnapshot struct {
    VRAMUsedMB      int64
    VRAMTotalMB     int64
    SystemRAMUsedMB int64
    ThreadsActive   int
    KVCacheSlots    int
    KVCacheUsed     int
    ModelLoaded     bool
}
```

**`platform/llm/backendext.go`** (new file — optional interfaces)

```go
// SessionAwareBackend: implemented by native backends that maintain KV cache.
type SessionAwareBackend interface {
    WithSession(sessionID string) core.LanguageModel
    EvictSession(ctx context.Context, sessionID string) error
    ActiveSessions(ctx context.Context) ([]string, error)
}

// NativeTokenStream: token-level streaming with metadata.
type NativeTokenStream interface {
    ChatTokenStream(ctx context.Context, messages []core.Message,
        opts *core.LLMOptions) (<-chan Token, error)
}

type Token struct {
    Text    string
    ID      int32
    Logprob float32
    Final   bool
}

// BatchInferenceBackend: coalesced batch inference for parallel graph branches.
type BatchInferenceBackend interface {
    ChatBatch(ctx context.Context, requests []BatchRequest,
        opts *core.LLMOptions) ([]*core.LLMResponse, error)
}

type BatchRequest struct {
    Messages  []core.Message
    Tools     []core.LLMToolSpec
    SessionID string
}

// BackendResourceReporter: VRAM, KV cache, thread metrics.
type BackendResourceReporter interface {
    ResourceSnapshot(ctx context.Context) (*ResourceSnapshot, error)
}

// ModelController: explicit model load/unload for native backends.
type ModelController interface {
    LoadModel(ctx context.Context, path string, opts ModelLoadOptions) error
    UnloadModel(ctx context.Context) error
    ModelInfo(ctx context.Context) (*LoadedModelInfo, error)
}

type ModelLoadOptions struct {
    ContextSize int
    Threads     int
    GPULayers   int
    BatchSize   int
    FlashAttn   bool
    Config      map[string]any
}

type LoadedModelInfo struct {
    Path           string
    Architecture   string
    ParameterCount int64
    ContextLength  int
    Quantization   string
    VRAMEstimateMB int64
}

// BackendRecovery: non-fatal restart without process restart.
type BackendRecovery interface {
    Restart(ctx context.Context) error
    ErrorHistory() []BackendError
}

type BackendError struct {
    Err        error
    OccurredAt time.Time
    Stack      string
}
```

**`platform/llm/config.go`** (new file)

```go
type ProviderConfig struct {
    Provider          string
    Endpoint          string
    Model             string
    ModelPath         string
    APIKey            string
    Timeout           time.Duration
    NativeToolCalling bool
    Debug             bool
    Config            map[string]any  // backend-specific extension block
}
```

**`framework/core/telemetry_types.go`**
- Add new event types:
  `EventInferenceError`, `EventInferenceTimeout`, `EventInferenceAbort`,
  `EventBackendStateChange`, `EventBackendWarm`, `EventBackendClose`,
  `EventBackendRestart`.

### Tests

**`platform/llm/backend_test.go`**
- `TestManagedBackendInterfaceCompleteness`: compile-time check that a stub
  implementing `ManagedBackend` satisfies the interface.
- `TestOptionalInterfaceTypeAssertions`: verify that a stub implementing each
  optional interface can be type-asserted from a `ManagedBackend` variable.
- `TestBackendCapabilities_Defaults`: zero value `BackendCapabilities` has expected
  field states.
- `TestProviderConfig_Validate`: missing provider, empty endpoint for transport
  backends, invalid timeout values each return descriptive errors.
- `TestHealthReport_Serialisation`: `HealthReport` round-trips through JSON without
  data loss.

### Exit criteria

- All interface and type definitions compile cleanly.
- No implementation code exists yet; this phase is contracts only.
- `go build ./platform/llm/...` passes.
- `go test ./platform/llm/...` passes (stub-only tests).

---

## Phase 3: platform/llm — Facade split and Ollama subpackage

### Objective

Move all Ollama wire-format code into `platform/llm/ollama/`, implement
`ManagedBackend` for Ollama, and establish the factory as the only construction path.

### Context

`platform/llm/ollama.go` currently contains Ollama wire types, HTTP transport, option
translation, endpoint normalisation, response parsing, and usage normalisation — all
in one file. `InstrumentedModel` and `TapeModel` are already backend-agnostic and
must stay in the root package. The split makes Ollama one implementation among
several rather than the implicit definition of what an LLM backend is.

### Work

**`platform/llm/ollama/` (new subpackage)**
- Move all wire-format struct types (`ollamaResponse`, `ollamaMessage`,
  `ollamaToolCall`, `toolDef`, `toolFunction`) from the root package here.
- Move `Client`, `NewClient`, `doRequest`, `doRequestStream`, `applyOptions`,
  `ollamaAPIEndpoint`, `convertMessages`, `convertLLMToolSpecs`,
  `schemaToOllamaParameters`, `decodeLLMResponse`, `parseToolCalls`,
  `normalizeUsage`, and all helpers here.
- Wrap `Client` in an `OllamaBackend` struct that implements `ManagedBackend`:
  - `Warm()`: issue a `GET /api/tags` connectivity check; fail if unreachable.
  - `Close()`: drain idle HTTP connections.
  - `Health()`: issue `GET /api/tags`; populate `HealthReport` with model list and
    endpoint reachability.
  - `ListModels()`: parse `/api/tags` response into `[]ModelInfo`.
  - `Capabilities()`: return `BackendClass: Transport`, `NativeToolCalling` from
    config, `Streaming: true`, `Embeddings: true` (Ollama supports embeddings),
    `ModelListing: true`.
  - `Embedder()`: return an `OllamaEmbedder` instance (moved from
    `framework/retrieval` in Phase 8; stub reference for now).
  - `Model()`: return an `InstrumentedModel`-wrapped `Client`.
  - `SetDebugLogging()`: delegate to `Client.Debug`.

**`platform/llm/factory.go`** (new file)
```go
func New(cfg ProviderConfig) (ManagedBackend, error) {
    switch strings.ToLower(cfg.Provider) {
    case "ollama", "":
        return ollama.NewBackend(cfg)
    default:
        return nil, fmt.Errorf("unknown provider %q", cfg.Provider)
    }
}
```
Additional cases added in Phases 10 and 12.

**`platform/llm/ollama.go`** (root package)
- Remove all wire-format and HTTP code; the file either becomes empty or is deleted.
- Add a compatibility shim `NewClient(endpoint, model string) *ollama.Client` that
  delegates to `platform/llm/ollama` for any callers that have not yet migrated.

**`platform/llm/instrumented_model.go`** — unchanged; stays in root package.

**`platform/llm/tape_model.go`** — unchanged; stays in root package.

**`platform/llm/tape_model.go`** — add provider metadata field to tape headers so
replay validation can confirm which provider was used during capture.

### Tests

**`platform/llm/ollama/ollama_test.go`**
- Move and update all existing tests from `platform/llm/ollama_test.go`.
- `TestOllamaBackend_Warm_Reachable`: mock HTTP server returns valid `/api/tags` →
  `Warm()` succeeds.
- `TestOllamaBackend_Warm_Unreachable`: mock server returns 500 → `Warm()` returns
  error.
- `TestOllamaBackend_Health_Healthy`: mock server with models → `HealthReport.State`
  is `ready`.
- `TestOllamaBackend_Health_Unhealthy`: mock server returns 503 → `HealthReport.State`
  is `unhealthy`.
- `TestOllamaBackend_ListModels`: mock `/api/tags` response → correct `[]ModelInfo`.
- `TestOllamaBackend_Capabilities`: returned `BackendCapabilities` matches transport
  class expectations.
- `TestOllamaBackend_Chat_RoundTrip`: mock chat endpoint → `LLMResponse` populated
  correctly.
- `TestOllamaBackend_ChatWithTools_NativeEnabled`: tools sent in payload when
  `NativeToolCalling: true`.
- `TestOllamaBackend_ChatWithTools_NativeDisabled`: tools omitted from payload when
  `NativeToolCalling: false`.
- `TestOllamaBackend_Streaming`: streaming chat request → `StreamCallback` receives
  tokens.

**`platform/llm/factory_test.go`**
- `TestFactory_OllamaDefault`: provider `""` resolves to Ollama backend.
- `TestFactory_OllamaExplicit`: provider `"ollama"` resolves to Ollama backend.
- `TestFactory_UnknownProvider`: unknown provider string → error with provider name
  in message.

**Compatibility**
- `TestNewClientShim_BackwardCompat`: `llm.NewClient()` returns a working client
  through the shim.

### Exit criteria

- No Ollama wire-format types remain in root `platform/llm`.
- Existing `ollama_test.go` tests pass under their new location.
- `go test ./platform/llm/...` passes.
- `go build ./...` passes.

---

## Phase 4: platform/llm/openaicompat — Shared OpenAI-compatible transport

### Objective

Implement a reusable HTTP transport for OpenAI-API-compatible backends so that LM
Studio (Phase 10) and any future OpenAI-compat backends are thin adapters over a
single, well-tested transport rather than independent reimplementations.

### Context

LM Studio, vLLM, TGI, llama-server, and many other inference servers expose an
OpenAI-compatible API (`/v1/chat/completions`, `/v1/embeddings`, `/v1/models`). The
wire format is identical across all of them. Implementing this transport once and
reusing it prevents duplicated streaming chunk assembly, tool-call parsing, and auth
header logic across every compatible backend.

### Work

**`platform/llm/openaicompat/` (new subpackage)**

- `client.go`: HTTP client for `/v1/chat/completions` (sync and streaming),
  `/v1/embeddings`, `/v1/models`.
  - Implements `core.LanguageModel`.
  - Bearer token auth via `Authorization: Bearer <token>` header.
  - Streaming: SSE chunk assembly into `core.LLMResponse`; incremental tool-call
    fragment accumulation before converting to `core.ToolCall`.
  - `applyOptions()`: maps `LLMOptions` to OpenAI request fields
    (`temperature`, `max_tokens`, `stop`, `top_p`, `stream`).
  - `convertMessages()`: maps `core.Message` to OpenAI message format including
    `tool_calls` and `tool_call_id` fields.
  - `convertTools()`: maps `[]core.LLMToolSpec` to OpenAI `tools` array format.
  - `parseToolCalls()`: maps OpenAI tool call response back to `[]core.ToolCall`.
  - `normalizeUsage()`: maps OpenAI `usage` object to `map[string]int`.

- `embedder.go`: implements `retrieval.Embedder` using `/v1/embeddings`.

- `models.go`: `ListModels(ctx)` via `/v1/models` → `[]ModelInfo`.

- `config.go`: `OpenAICompatConfig` with `Endpoint`, `APIKey`, `Timeout`,
  `NativeToolCalling`.

**Important invariants:**
- OpenAI-compat request/response structs are entirely internal to this package.
- No OpenAI-specific type names leak into `platform/llm` root or `framework/`.
- Tool-call streaming fragment accumulation is complete before any `core.ToolCall`
  is constructed; callers never see partial tool calls.

### Tests

**`platform/llm/openaicompat/client_test.go`** (canned HTTP fixtures via
`httptest.NewServer`)

- `TestChat_Sync`: non-streaming chat completion → `LLMResponse.Text` correct.
- `TestChat_Streaming`: streaming chat → `StreamCallback` receives tokens in order;
  final `LLMResponse.Text` is accumulated correctly.
- `TestChatWithTools_NativeEnabled_Sync`: tools in request payload; response with
  `tool_calls` → `LLMResponse.ToolCalls` populated.
- `TestChatWithTools_NativeEnabled_Streaming`: streaming response with incremental
  tool-call fragments → assembled into a single `core.ToolCall` on completion.
- `TestChatWithTools_NativeDisabled`: `NativeToolCalling: false` → tools absent from
  request payload.
- `TestBearerAuth`: request headers contain correct `Authorization: Bearer <token>`
  when `APIKey` is set.
- `TestBearerAuth_NoKey`: no `Authorization` header when `APIKey` is empty.
- `TestListModels`: `/v1/models` response → correct `[]ModelInfo`.
- `TestEmbedder_Single`: single text → correct embedding vector dimensions.
- `TestEmbedder_Batch`: multiple texts → correct count of embedding vectors.
- `TestHTTPError_500`: server returns 500 → error message contains status code.
- `TestStreamingCancel`: context cancelled mid-stream → stream goroutine exits,
  channel closed without panic.

### Exit criteria

- All canned-fixture tests pass without a live inference server.
- No OpenAI-specific type names in any exported symbol.
- `go test ./platform/llm/openaicompat/...` passes.

---

## Phase 5: Runtime config and bootstrap neutralisation

### Objective

Replace all `Ollama*` field names in the runtime config and bootstrap options with
provider-neutral equivalents, and change the bootstrap path to accept a
`ManagedBackend` rather than raw connection strings.

### Context

`app/relurpish/runtime/config.go` has `OllamaEndpoint` and `OllamaModel` as named
fields. `AgentBootstrapOptions` in `bootstrap.go` has the same. `Normalize()` hardcodes
the Ollama default endpoint. `WorkspaceConfig.Model` has no provider association.
These are the last structural coupling points outside `platform/llm`; resolving them
lets every caller above bootstrap become provider-agnostic.

### Work

**`app/relurpish/runtime/config.go`**
- Add neutral fields:
  ```go
  InferenceProvider          string
  InferenceEndpoint          string
  InferenceModel             string
  InferenceAPIKey            string
  InferenceNativeToolCalling bool
  EmbeddingProvider          string
  EmbeddingEndpoint          string
  EmbeddingModel             string
  ```
- Retain `OllamaEndpoint string` and `OllamaModel string` tagged
  `deprecated_ollama_endpoint` / `deprecated_ollama_model` (kept for compat window,
  not removed).
- In `Normalize()`: if `InferenceProvider == ""` and `OllamaEndpoint == ""`, set
  `InferenceProvider = "ollama"` and `InferenceEndpoint = "http://localhost:11434"`.
  If `OllamaEndpoint != ""` and `InferenceEndpoint == ""`, copy across. Same for
  model name.
- `WorkspaceConfig`: add `Provider string` field alongside existing `Model string`.

**`app/relurpish/runtime/bootstrap.go`**
- `AgentBootstrapOptions`: replace `OllamaEndpoint string` and `OllamaModel string`
  with `Backend llm.ManagedBackend`.
- All callers of `BootstrapAgentRuntime` that previously passed endpoint/model now
  construct a `ManagedBackend` via `llm.New(ProviderConfig{...})` before calling
  bootstrap.
- `BootstrappedAgentRuntime`: add `Backend llm.ManagedBackend` field so downstream
  code (context manager, session lifecycle) can access the backend after bootstrap.

**`platform/llm/config.go`**
- Add `ProviderConfigFromRuntimeConfig(cfg runtime.Config) llm.ProviderConfig`
  normalisation helper that reads the neutral fields and produces a `ProviderConfig`.

### Tests

**`app/relurpish/runtime/config_test.go`**
- `TestNormalize_DefaultsToOllama`: empty config normalises to
  `InferenceProvider="ollama"`, `InferenceEndpoint="http://localhost:11434"`.
- `TestNormalize_OllamaCompatAlias_Endpoint`: `OllamaEndpoint` set,
  `InferenceEndpoint` empty → `InferenceEndpoint` receives the value.
- `TestNormalize_OllamaCompatAlias_Model`: same for model name.
- `TestNormalize_ExplicitProvider_NoOllamaAlias`: `InferenceProvider="lmstudio"`,
  `InferenceEndpoint` set → `OllamaEndpoint` remains empty; no cross-contamination.
- `TestNormalize_BothFieldsSet_NeutralWins`: both neutral and legacy fields set →
  neutral field takes precedence.
- `TestWorkspaceConfig_ProviderPersistence`: `SaveWorkspaceConfig` then
  `LoadWorkspaceConfig` round-trips `Provider` field correctly.

**`app/relurpish/runtime/bootstrap_test.go`**
- `TestBootstrap_AcceptsBackend`: `BootstrapAgentRuntime` with a mock
  `ManagedBackend` succeeds and stores backend in returned runtime.
- `TestBootstrap_BackendRequired`: nil backend → error.
- `TestProviderConfigFromRuntimeConfig_Ollama`: default-normalised config produces
  `ProviderConfig{Provider: "ollama", ...}`.
- `TestProviderConfigFromRuntimeConfig_Custom`: neutral fields produce correct
  `ProviderConfig`.

### Exit criteria

- No code path in `app/relurpish/runtime/` reads `OllamaEndpoint` or `OllamaModel`
  for any purpose other than the compat-alias normalisation in `Normalize()`.
- `go test ./app/relurpish/runtime/...` passes.
- `go build ./...` passes.

---

## Phase 6: Probe and doctor generalisation

### Objective

Replace `OllamaReport` and `detectOllama` with a provider-neutral inference backend
report populated via `ManagedBackend`.

### Context

`probe.go` currently contains `OllamaReport`, `EnvironmentReport.Ollama OllamaReport`,
and `detectOllama()` which directly issues `GET /api/tags` against
`cfg.OllamaEndpoint`. After Phase 5 the runtime has a `ManagedBackend`; the probe
layer should delegate to it rather than reimplementing health checks for a specific
provider.

### Work

**`app/relurpish/runtime/probe.go`**
- Replace `OllamaReport` with:
  ```go
  type InferenceBackendReport struct {
      Provider      string
      Endpoint      string
      State         llm.BackendHealthState
      Models        []string
      SelectedModel string
      Error         string
      Resources     *llm.ResourceSnapshot
  }
  ```
- Replace `EnvironmentReport.Ollama OllamaReport` with
  `Inference InferenceBackendReport`.
- Replace `detectOllama(ctx, cfg)` with `detectInferenceBackend(ctx, backend)` that
  calls `backend.Health(ctx)` and `backend.ListModels(ctx)`.
- `ProbeEnvironment` signature gains `backend llm.ManagedBackend` parameter. Callers
  in the runtime pass the bootstrapped backend.
- For the case where no backend is constructed yet (pre-bootstrap doctor invocation),
  fall back to constructing a temporary backend from config, probing it, and closing
  it.

**`app/relurpish/runtime/doctor.go`** (if separate from probe)
- Update doctor output formatting to display `InferenceBackendReport` fields:
  provider name, state, endpoint (for transport backends), model list.
- Native backends: show `Resources.VRAMUsedMB`, `Resources.KVCacheUsed`, model path.

### Tests

**`app/relurpish/runtime/probe_test.go`** (update existing)
- `TestProbeEnvironment_HealthyBackend`: mock backend returning healthy `HealthReport`
  → `InferenceBackendReport.State` is `ready`.
- `TestProbeEnvironment_UnhealthyBackend`: mock backend returning unhealthy report →
  `InferenceBackendReport.State` is `unhealthy`, `Error` populated.
- `TestProbeEnvironment_ModelsListed`: mock backend returning `[]ModelInfo` →
  `InferenceBackendReport.Models` contains correct names.
- `TestProbeEnvironment_WithResources`: mock `BackendResourceReporter` → resources
  included in report.
- `TestProbeEnvironment_NoBackend_FallbackConstruct`: nil backend, config present →
  falls back to temporary construction; no panic.

### Exit criteria

- `OllamaReport`, `detectOllama`, and `EnvironmentReport.Ollama` are removed.
- `go test ./app/relurpish/runtime/...` passes.

---

## Phase 7: TUI and runtime_adapter generalisation

### Objective

Replace Ollama-specific model listing and provider display in the TUI with
provider-neutral equivalents backed by `ManagedBackend.ListModels()`.

### Context

The TUI settings pane currently calls something equivalent to `OllamaModels()` to
populate the model selector. The session info display shows Ollama-specific
strings. `WorkspaceConfig.Model` now carries a `Provider` field (Phase 5) but the
TUI doesn't use it yet.

### Work

**`app/relurpish/tui/runtime_adapter.go`**
- Replace `OllamaModels(ctx)` (or equivalent) with `InferenceModels(ctx)` that
  calls `runtime.Backend.ListModels(ctx)`.
- Session info display: show `Provider` alongside model name where model name is
  currently shown alone.
- Model selection: when a model is selected from the TUI list, persist both model
  name and provider name to `WorkspaceConfig`.

**`app/relurpish/tui/pane_settings.go`** (or equivalent settings view)
- Update model selector to display provider + model pairs.
- Show backend health state indicator (ready / degraded / unhealthy) alongside
  provider name.

### Tests

**`app/relurpish/tui/runtime_adapter_test.go`** (new or update)
- `TestInferenceModels_PopulatesFromBackend`: mock backend returning `[]ModelInfo`
  → adapter returns correct model names.
- `TestInferenceModels_BackendError`: mock backend returning error → adapter returns
  empty list, does not panic.
- `TestModelSelection_PersistsProvider`: selecting a model from the list persists
  both `Provider` and `Model` in workspace config.
- `TestSessionInfo_ShowsProvider`: session info string contains provider name.

### Exit criteria

- No TUI code reads `cfg.OllamaModel` or `cfg.OllamaEndpoint` directly.
- `go test ./app/relurpish/tui/...` passes.

---

## Phase 8: Retrieval and embedder decoupling

### Objective

Move the Ollama embedder into the Ollama subpackage, introduce an embedder factory,
and establish the priority order: backend-native embeddings first, then
separately-configured embedding provider, then no embeddings.

### Context

`framework/retrieval/ollama_embedder.go` is the last Ollama-specific file outside
`platform/llm`. The retrieval bootstrap currently constructs an `OllamaEmbedder`
directly using `cfg.OllamaEndpoint`. After this phase, bootstrap checks
`backend.Embedder()` first; the separately-configured embedding provider is a
fallback for cases where the chat backend does not support embeddings or where a
dedicated embedding model is preferred.

### Work

**Move `framework/retrieval/ollama_embedder.go`**
- Relocate implementation to `platform/llm/ollama/embedder.go` (within the Ollama
  subpackage). The `OllamaEmbedder` type becomes `ollama.Embedder`.
- `OllamaBackend.Embedder()` returns an instance of `ollama.Embedder` configured
  with the same endpoint but with `EmbeddingModel` from config (defaults to the chat
  model if not set).

**`framework/retrieval/embedder_factory.go`** (new file)
- `NewEmbedder(backend llm.ManagedBackend, cfg EmbedderConfig) (Embedder, error)`:
  1. If `backend.Embedder() != nil`, return it.
  2. If `cfg.EmbeddingProvider != ""`, construct via `llm.New(EmbeddingProviderConfig)`.
  3. Return `nil, nil` (no embeddings available; caller handles gracefully).

**`EmbedderConfig`** (new type in `framework/retrieval`):
```go
type EmbedderConfig struct {
    Provider string
    Endpoint string
    Model    string
    APIKey   string
}
```

**`app/relurpish/runtime/retrieval_semantic.go`** (or bootstrap path)
- Replace `retrieval.NewOllamaEmbedder(cfg.OllamaEndpoint, cfg.OllamaModel)` with
  `retrieval.NewEmbedder(backend, embedderCfgFromRuntimeConfig(cfg))`.
- If returned embedder is nil, log that semantic indexing is disabled and proceed
  with keyword-only search.

**`framework/retrieval/ollama_embedder.go`** — delete after relocation.

### Tests

**`platform/llm/ollama/embedder_test.go`**
- `TestOllamaEmbedder_Embed_Single`: mock `/api/embeddings` → correct vector.
- `TestOllamaEmbedder_Embed_Batch`: multiple texts → correct batch output.
- `TestOllamaEmbedder_ModelID`: returns configured model name.

**`framework/retrieval/embedder_factory_test.go`**
- `TestNewEmbedder_BackendHasEmbedder`: backend returns non-nil embedder → factory
  returns it directly.
- `TestNewEmbedder_BackendNilEmbedder_ConfigPresent`: backend returns nil, config has
  provider → factory constructs separate embedder.
- `TestNewEmbedder_BackendNilEmbedder_NoConfig`: backend returns nil, no config →
  factory returns nil, nil.
- `TestNewEmbedder_BackendNilEmbedder_OllamaConfig`: config with Ollama embedding
  provider → returns working Ollama embedder.

**`app/relurpish/runtime/retrieval_semantic_test.go`** (update)
- `TestRetrievalBootstrap_UsesBackendEmbedder`: backend with embedder → semantic
  indexing initialised; `OllamaEmbedder` not constructed.
- `TestRetrievalBootstrap_NilEmbedder_GracefulFallback`: backend without embedder,
  no embedding config → semantic indexing skipped, keyword search still works.

### Exit criteria

- `framework/retrieval/ollama_embedder.go` deleted.
- No `retrieval` package code references Ollama directly.
- `go test ./framework/retrieval/...` passes.
- `go test ./platform/llm/ollama/...` passes.

---

## Phase 9: Framework-native capability calling hardening

### Objective

Make the framework-native capability calling fallback the explicit, tested, and owned
contract for all backends. Establish the handoff boundary between
`framework/capability` and `platform/llm`.

### Context

The framework can render callable capabilities into model-visible instructions and
parse structured tool-call output back into framework tool invocations when a backend
does not support or has disabled native tool calling. This fallback path exists but
is not explicitly documented as the contract, is not tested against a parity
requirement, and has no defined handoff with the platform layer. Any backend added in
future phases must be able to rely on this path.

### Work

**`framework/capability/` (existing package)**
- Audit `RenderToolsToPrompt` and `ParseToolCallsFromText`: ensure they are exported,
  documented, and testable in isolation.
- Add `CapabilityCallingMode` type:
  ```go
  type CapabilityCallingMode string
  const (
      CapabilityCallingNative   CapabilityCallingMode = "native"
      CapabilityCallingFallback CapabilityCallingMode = "fallback"
  )
  ```
- Add `ResolveCallingMode(spec *core.AgentRuntimeSpec, backend llm.ManagedBackend)
  CapabilityCallingMode`: returns `Native` if `spec.NativeToolCallingEnabled()` and
  `backend.Capabilities().NativeToolCalling`, else `Fallback`.
- Ensure no duplicated fallback render/parse logic exists in any provider subpackage.
  Provider packages translate native schemas only; they never re-implement the
  text-render fallback.

**`framework/capability/write_path_precheck.go`** — ensure fallback path is invoked
correctly when `CapabilityCallingFallback` is resolved.

### Tests

**`framework/capability/capability_calling_test.go`** (new)
- `TestResolveCallingMode_NativeEnabled_BackendSupports`: spec native enabled, backend
  native capable → `Native`.
- `TestResolveCallingMode_NativeEnabled_BackendLacks`: spec native enabled, backend
  `NativeToolCalling: false` → `Fallback`.
- `TestResolveCallingMode_NativeDisabled`: spec native disabled → `Fallback`
  regardless of backend.
- `TestFallbackParity_TextMatchesNative`: agent issues two identical tool calls — one
  via native path (mock backend), one via fallback text render/parse. The resulting
  `core.ToolCall` structs are equal (name and args match).
- `TestRenderToolsToPrompt_RoundTrip`: render capabilities to prompt, then parse the
  model's simulated response → all tool calls recovered.
- `TestParseToolCallsFromText_MalformedJSON`: model returns syntactically invalid JSON
  in response → parser returns partial results without panic.
- `TestParseToolCallsFromText_EmptyResponse`: empty model response → empty tool call
  list, no error.

### Exit criteria

- `ResolveCallingMode` is the single decision point for native vs fallback.
- No provider subpackage contains any version of `RenderToolsToPrompt` or
  `ParseToolCallsFromText`.
- `go test ./framework/capability/...` passes.

---

## Phase 10: platform/llm/lmstudio — LM Studio backend

### Objective

Add LM Studio as a selectable provider using the `openaicompat` transport from Phase 4.

### Context

LM Studio exposes OpenAI-compatible endpoints. The implementation is a thin adapter
over `openaicompat` — configuring auth, endpoint defaulting, and model listing — not
a new transport layer. This phase validates that the `openaicompat` transport is
genuinely reusable.

### Work

**`platform/llm/lmstudio/` (new subpackage)**
- `backend.go`: `LMStudioBackend` wrapping `openaicompat.Client`, implementing
  `ManagedBackend`.
  - Default endpoint: `http://localhost:1234`.
  - `Warm()`: `GET /v1/models` connectivity check.
  - `Health()`: `GET /v1/models`; report healthy/unhealthy.
  - `ListModels()`: from `/v1/models`.
  - `Capabilities()`: `BackendClass: Transport`, `NativeToolCalling` from config,
    `Streaming: true`, `Embeddings: true`, `ModelListing: true`.
  - `Embedder()`: `openaicompat.Embedder` instance with LM Studio endpoint.
  - `Model()`: instrumented `openaicompat.Client`.
- `config.go`: `LMStudioConfig` with endpoint, API key, timeout, native tool calling.

**`platform/llm/factory.go`**
- Add `"lmstudio"` case.

**`framework/retrieval/`** — add `lmstudio` to the embedder factory mapping.

### Tests

**`platform/llm/lmstudio/backend_test.go`** (canned HTTP fixtures)
- `TestLMStudioBackend_Warm_Reachable`: mock `/v1/models` → `Warm()` succeeds.
- `TestLMStudioBackend_Warm_Unreachable`: mock returns 503 → `Warm()` error.
- `TestLMStudioBackend_Chat`: mock chat completion → correct `LLMResponse`.
- `TestLMStudioBackend_Streaming`: streaming response → tokens delivered in order.
- `TestLMStudioBackend_ChatWithTools_Native`: tools in payload, tool call in response.
- `TestLMStudioBackend_BearerAuth`: API key present → `Authorization` header set.
- `TestLMStudioBackend_NoAuth`: no API key → no `Authorization` header.
- `TestLMStudioBackend_Embeddings`: `/v1/embeddings` → correct vector.
- `TestLMStudioBackend_ListModels`: `/v1/models` → correct `[]ModelInfo`.
- `TestFactory_LMStudio`: `llm.New(ProviderConfig{Provider: "lmstudio"})` →
  `LMStudioBackend`.

### Exit criteria

- Agent manifest with `provider: lmstudio` constructs an LM Studio backend via the
  factory.
- `go test ./platform/llm/lmstudio/...` passes.
- `openaicompat` package is unchanged; no LM Studio-specific code was added to it.

---

## Phase 11: platform/llm/llamago — In-process native engine failsafe foundation

### Objective

Build the complete failsafe and lifecycle infrastructure for the in-process native
engine before writing any inference code. This phase defines what "safe enough" means
for in-process CGo and establishes the patterns every subsequent inference call uses.

### Context

A true SIGSEGV or SIGABRT from C code will terminate the process regardless of any
Go-level failsafe. This is the documented hard constraint. Everything else — bad model
files, insufficient memory, hanging inference, recoverable GPU errors, Go panics from
CGo wrappers — can be handled. This phase builds those defences first so that the
actual inference implementation (Phase 12) is written within a safety envelope from
the start.

### Work

**`platform/llm/llamago/` (new subpackage)**

`config.go`:
```go
type InProcessConfig struct {
    ModelPath       string
    ContextSize     int
    Threads         int
    GPULayers       int
    BatchSize       int
    FlashAttn       bool
    InferenceTimeout time.Duration
    ErrorThreshold  int     // errors before transitioning to Unhealthy
    CrashReportDir  string
    Config          map[string]any
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
  `SIGABRT` handler (not SIGSEGV — that is too dangerous to handle); writes crash
  report JSON to `CrashReportDir` on signal receipt.
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

### Exit criteria

- All failsafe infrastructure compiles and tests pass with no CGo dependency yet.
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
  `[]BatchRequest`; call `ChatBatch()`; distribute responses back to waiting
  branches.
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
of these interfaces at the agent runtime level. The KV cache is explicitly excluded
from migration — it is a local acceleration that is rebuilt from message history on
the destination node. Permission policies migrate with the session but may be
restricted at the trust boundary of the destination node.

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
      PermissionPolicy PolicySnapshot
      SchemaVersion   string
  }
  ```
- `ExportSession(sessionID string, rt *BootstrappedAgentRuntime) (*PortableSessionEnvelope, error)`:
  serialises all migratable fields. Calls `EvictSession` on backend after export.
- `ImportSession(env *PortableSessionEnvelope, backend llm.ManagedBackend, ...) (*BootstrappedAgentRuntime, error)`:
  reconstructs runtime from envelope. Permission policy applied with trust-class
  reduction where source trust exceeds destination node's allowed trust level.
- Implement `ProviderSnapshotter` and `ProviderRestorer` on the runtime struct using
  the above.

**`app/relurpish/runtime/session_export_test.go`** (new)
- `TestExportSession_RoundTrip`: export then import → `core.ContextSnapshot` fields
  equal; message history equal.
- `TestExportSession_KVCacheNotIncluded`: export with native backend → envelope
  contains no KV cache data; `EvictSession` called on source backend.
- `TestImportSession_FreshKVCache`: import with native `SessionAwareBackend` →
  `WithSession` called; slot starts empty (nCached = 0).
- `TestImportSession_PermissionPolicyReduction`: envelope with `allow` on a
  restricted capability; destination has stricter trust boundary → capability
  requires re-approval on destination.
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

## Phase 17: Ollama finalization and provider conformance

### Objective

Stabilise Ollama as the reference provider, audit all remaining `Ollama*` references,
add tape recording provider metadata, and establish a shared conformance test suite
that all providers must pass.

### Context

After Phases 3–16, residual `Ollama*` references in agents, templates, manifests, and
tests must be audited and cleaned up. The tape model must record provider metadata so
replays can validate they are using the same provider that captured the tape. A
conformance suite ensures that future provider additions do not regress the contract.

### Work

**Audit**
- `grep -r "Ollama\|ollama_endpoint\|ollama_model\|ollama_tool_calling"
  --include="*.go" --include="*.yaml"` across the full repository.
- For each remaining reference: either migrate to neutral equivalent, confirm it is
  inside the compat normalisation layer (the only allowed location for legacy names),
  or document why it is intentionally Ollama-specific (e.g. an Ollama-subpackage
  comment).

**`platform/llm/tape_model.go`**
- Add `ProviderID string` and `ModelName string` to the tape header.
- On replay: if tape header has provider metadata and active provider differs, emit a
  warning (not an error — replay may intentionally switch providers).

**`platform/llm/conformance_test.go`** (new, shared suite)
- A `BackendConformanceSuite` function that accepts a `ManagedBackend` factory and
  runs a standard set of assertions. Each provider's test file calls this suite with
  its own mock server.
- Conformance tests:
  - `Generate` returns non-empty text.
  - `Chat` with a system + user message returns response.
  - `ChatWithTools` with `NativeToolCalling: true` sends tools in payload.
  - `ChatWithTools` with `NativeToolCalling: false` omits tools from payload.
  - `Warm()` succeeds against a responsive mock; fails against an unresponsive mock.
  - `Health()` returns `ready` for a healthy mock.
  - `ListModels()` returns non-empty list from a mock with models.
  - `Close()` is idempotent (safe to call twice).
  - `SetDebugLogging(true)` does not panic.

**Default-path tests**
- `TestDefaultConfig_ResolvesToOllama`: empty `InferenceProvider` in config →
  factory produces Ollama backend.
- `TestManifest_NoProvider_ResolvesToOllama`: agent manifest with no `provider` field
  → bootstrap uses Ollama.

### Tests

- All conformance tests passing for `platform/llm/ollama`.
- All conformance tests passing for `platform/llm/lmstudio`.
- Conformance tests for `platform/llm/llamago` run under `integration` tag with a
  real model, or via a mock `core.LanguageModel` for the protocol-level assertions.
- `TestTapeModel_ProviderMetadata_Captured`: tape header includes provider and model.
- `TestTapeModel_ProviderMetadata_ReplayWarning`: replay with different provider →
  warning emitted; replay proceeds.

### Exit criteria

- No `Ollama*` field names remain outside the compat normalisation layer in
  `config.go` and the `platform/llm/ollama` subpackage.
- All three provider implementations pass the conformance suite.
- `go test ./platform/llm/...` passes.

---

## Phase 18: Testsuite verification and documentation

### Objective

Confirm that the `/testsuite` agent test workflows still function against a live
Ollama instance through the refactored provider architecture, update documentation,
and formalise the deprecation window for legacy config fields.

### Work

**`/testsuite` audit**
- Identify all helpers, fixtures, and test setups that still reference
  `OllamaEndpoint`, `OllamaModel`, or direct `llm.NewClient` construction.
- Migrate to `runtime.Config` neutral fields and `llm.New(ProviderConfig{...})`.
- Ensure live-Ollama preflight validation still works (endpoint reachability, model
  availability check).
- Ensure tape capture and replay remain functional with the updated provider metadata
  in tape headers.

**Documentation**
- `docs/platform.md`: describe the `platform/llm` facade, subpackage layout, factory,
  and provider selection.
- `docs/framework.md`: document `ManagedBackend`, optional interfaces, and where each
  is used.
- `docs/providers.md` (new): per-provider setup guide for Ollama, LM Studio, and
  llama.go (in-process). Includes: configuration fields, default endpoint, capability
  flags, known limitations.
- `docs/migration.md` (new): migration guide from `OllamaEndpoint`/`OllamaModel`
  config fields to `InferenceEndpoint`/`InferenceModel`. States the deprecation
  window (old fields accepted but will be removed in a future release).

**Deprecation warnings**
- In `Normalize()`: if `OllamaEndpoint` or `OllamaModel` is set, emit a log warning
  once per process start naming the replacement fields.
- In manifest loading: if `ollama_tool_calling` is present, emit a log warning naming
  `native_tool_calling` as the replacement.

### Tests

**`/testsuite` integration (requires live Ollama)**
- Verify that at least one end-to-end agent test run completes successfully with:
  - `InferenceProvider: "ollama"` (explicit neutral config)
  - A live Ollama instance at the configured endpoint
  - The refactored `platform/llm` facade and `platform/llm/ollama` subpackage
- Verify tape capture produces a tape with provider metadata.
- Verify tape replay succeeds when provider metadata matches.

**Unit**
- `TestDeprecationWarning_OllamaEndpoint`: `Normalize()` with `OllamaEndpoint` set
  → deprecation warning logged.
- `TestDeprecationWarning_OllamaModel`: same for model field.
- `TestDeprecationWarning_OllamaToolCalling`: manifest with `ollama_tool_calling` →
  deprecation warning on load.
- `TestNoDeprecationWarning_NeutralFields`: only neutral fields set → no deprecation
  warning.

### Exit criteria

- `/testsuite` passes against a live Ollama instance using the neutral config path.
- Documentation is internally consistent.
- No test helper references `llm.NewClient` directly; all use `llm.New(ProviderConfig{...})`.
- Deprecation warnings fire correctly and reference the correct replacement field names.
- `go build ./...` and `go test ./...` both pass cleanly.

---

## Cross-Cutting Testing Strategy

### Shared provider conformance

`platform/llm/conformance_test.go` defines `BackendConformanceSuite(factory func()
ManagedBackend)`. Every provider test file calls it. This ensures no provider silently
drops a contract method or changes semantics across refactors.

### Compatibility regression

A dedicated `compatibility_test.go` in `app/relurpish/runtime/` runs for every
phase from Phase 5 onward:
- Old manifests with `ollama_tool_calling` still produce correct `NativeToolCallingEnabled()` behaviour.
- Old CLI flags (`--ollama-endpoint`, `--ollama-model`) still resolve to correct
  `InferenceEndpoint` and `InferenceModel`.
- `go build ./...` with no new config set produces a working Ollama-default runtime.

### Native engine safety regression

`platform/llm/llamago/safety_test.go` runs after any change to the backend:
- Pre-flight validation tests (no CGo required).
- Watchdog timeout tests (no CGo required).
- Health state machine transitions (no CGo required).
- Panic recovery (no CGo required).
These must always pass; they are the first line of defence against regressions in the
failsafe layer.

### Offline-first verification

A subset of runtime bootstrap tests runs with Nexus client set to nil:
- `Warm()` succeeds.
- `ProbeEnvironment()` completes.
- Capability advertisement skipped without error.
- Session export completes and envelope is valid.

---

## Key Risks and Mitigations

**Risk: Partial config migration leaves two config systems alive.**
Mitigation: Phase 5 normalisation layer is the single read point for legacy fields.
The compatibility regression suite (above) runs after every phase to catch divergence.

**Risk: OpenAI-compat transport assumptions leak into the facade.**
Mitigation: `platform/llm/openaicompat` exports no type names that appear in root
`platform/llm` or `framework/`. LM Studio test confirms it delegates to
`openaicompat` without adding its own copies of request/response types.

**Risk: CGo crash in native backend.**
Mitigation: Documented hard constraint. Pre-flight validation, watchdog, and health
state machine reduce probability of reaching C code in a bad state. Crash report
infrastructure captures context before the process dies.

**Risk: KV cache/context budget feedback creates unexpected compression cycles.**
Mitigation: `OnBudgetExceeded` marks sessions stale without forcing eviction; eviction
is triggered only by `OnBudgetWarning` when KV pressure threshold is also exceeded.
The budget adaptor tests verify this threshold logic explicitly.

**Risk: Session migration permission policy reduction silently drops capabilities.**
Mitigation: `ImportSession` logs each capability downgrade at warn level. The
`TestImportSession_PermissionPolicyReduction` test explicitly verifies that
downgraded capabilities require re-approval rather than silently inheriting the
source node's grant.
