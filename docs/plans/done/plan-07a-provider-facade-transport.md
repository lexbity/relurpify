# Plan 07a: Provider Facade and Transport Backends

## Goals

Restructure `platform/llm` from a single Ollama implementation into a durable,
provider-oriented inference architecture by introducing a stable `ManagedBackend`
facade, moving Ollama wire-format code into a proper subpackage, adding a shared
OpenAI-compatible transport, adding LM Studio as a second transport provider, and
sweeping all `Ollama*` assumptions out of the config, bootstrap, probe, TUI,
retrieval, and `ayenitd` layers.

**Ollama remains the default inference backend throughout and after this plan.**
Nothing about how an existing Ollama user configures or uses the system changes
at the surface level — old config fields are accepted via a compat normalisation
layer, old manifests continue to work. What changes is that Ollama is now one
implementation of a stable interface rather than the implicit definition of what an
LLM backend is.

**Split from plan-07:** This plan covers Phases 1–10, 17, and 18 from the original
design. Native in-process engine work (Phases 11–12) is plan-07b. Advanced framework
integrations that depend on the native engine (Phases 13–16) are plan-07c.

---

## Why

### Vendor lock-in

The current system has Ollama assumptions in six distinct layers: the framework core
spec type (`OllamaToolCalling`), the platform LLM package (single implementation),
the runtime config (`OllamaEndpoint`, `OllamaModel`), the bootstrap options struct,
the probe/doctor layer (`OllamaReport`, `detectOllama`), and the retrieval embedder
(`ollama_embedder.go`). Adding any second provider today requires touching all six
layers simultaneously with no stable interface to guide what changes are safe.

### Mesh resource sharing without inference routing

The Nexus mesh shares agent tasks, sessions, and context across nodes — not raw
inference calls. Each node owns its own inference backend. `platform/llm` is always a
local concern. Inference never crosses a node boundary. This requires a clean session
portability contract at the runtime level, which in turn requires the bootstrap layer
to speak `ManagedBackend` rather than endpoint strings.

### Local-only support

Nodes may be permanently offline from the mesh. Transport-backed backends (Ollama,
LM Studio) running locally satisfy this naturally. No network access beyond the local
backend is required.

---

## Design Principles

1. `core.LanguageModel` remains the agent-facing execution contract. Agents never
   see provider names, endpoints, or backend-specific config.

2. `ManagedBackend` is the runtime-facing ownership contract. The runtime boots,
   warms, and closes backends through this interface. All provider-specific behaviour
   lives below it.

3. `BackendCapabilities` and `BackendClass` live in `framework/core` so that
   `framework/capability` and other framework packages can reason about backend
   capabilities without importing `platform/llm`. Dependency direction:
   `platform/llm` imports `framework/core`; never the reverse.

4. Optional interfaces (`SessionAwareBackend`, `NativeTokenStream`,
   `BatchInferenceBackend`, `BackendResourceReporter`, `ModelController`,
   `BackendRecovery`) are defined in this plan for completeness but are only
   implemented by the native engine (plan-07b). Transport backends return `false`
   or nil for capabilities they do not support.

5. Framework-native capability calling is the mandatory fallback for all backends.
   Backend-native tool calling is an optional optimisation. The handoff contract is
   owned by `framework/capability`, not by any provider package.

6. `platform/llm` is a local concern. It never speaks to another node.

7. Transport-backed backends include distributed inference engines (vLLM, TGI) that
   present an HTTP endpoint. From the framework's perspective these are identical to
   single-server transports.

8. Backward compatibility is preserved through a single normalisation layer. Old
   config field names (`OllamaEndpoint`, `OllamaModel`, `OllamaToolCalling`) are
   accepted and mapped internally. Direct reads of deprecated fields outside that
   layer are forbidden.

---

## Dependency Order

```
Phase 1 → Phase 2 → Phase 3 → Phase 5 → Phase 6
                             → Phase 7
                             → Phase 8
                  → Phase 4 → Phase 10
                  → Phase 9
  Phases 3,10 → Phase 17
  All above   → Phase 18
```

Phases 6, 7, 8, 9 may proceed in parallel after their respective prerequisites.
Phase 10 may proceed in parallel with Phase 5 after Phase 4.

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

## Phase 2: framework/core + platform/llm — Interface definitions and contracts

### Objective

Define `BackendCapabilities` in `framework/core` and the complete `ManagedBackend`
interface surface in `platform/llm` before any implementation moves.

### Context

`BackendCapabilities` lives in `framework/core` so that `framework/capability` and
other framework packages can reason about capabilities without importing `platform/llm`.
All interface definitions must exist before subpackages are created so implementations
are written to the contract from the start.

### Work

**`framework/core/backend_capabilities.go`** (new file)

```go
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
```

**`platform/llm/backend.go`** (new file)

```go
type ManagedBackend interface {
    Model() core.LanguageModel
    Embedder() retrieval.Embedder      // nil if not supported
    Capabilities() core.BackendCapabilities
    Health(ctx context.Context) (*HealthReport, error)
    ListModels(ctx context.Context) ([]ModelInfo, error)
    Warm(ctx context.Context) error
    Close() error
    SetDebugLogging(enabled bool)
}

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
    Name          string
    Family        string
    ParameterSize string
    ContextSize   int
    Quantization  string
    HasGPU        bool
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

**`platform/llm/backendext.go`** (new file — optional interfaces, implemented by
native engine in plan-07b; defined here so the factory and callers can type-assert
them from day one)

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
    config, `Streaming: true`, `Embeddings: true`, `ModelListing: true`.
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
Additional cases added in Phases 4 and 10.

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
  - `applyOptions()`: maps `LLMOptions` to OpenAI request fields.
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
- `TestChat_Streaming`: streaming chat → `StreamCallback` receives tokens in order.
- `TestChatWithTools_NativeEnabled_Sync`: tools in request; response with
  `tool_calls` → `LLMResponse.ToolCalls` populated.
- `TestChatWithTools_NativeEnabled_Streaming`: streaming response with incremental
  tool-call fragments → assembled into a single `core.ToolCall` on completion.
- `TestChatWithTools_NativeDisabled`: `NativeToolCalling: false` → tools absent from
  request payload.
- `TestBearerAuth`: correct `Authorization: Bearer <token>` header when `APIKey` set.
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
`ManagedBackend` rather than raw connection strings. Apply the same neutralisation to
`ayenitd`, which was added after the original plan was written.

### Context

`app/relurpish/runtime/config.go` has `OllamaEndpoint` and `OllamaModel` as named
fields. `AgentBootstrapOptions` in `bootstrap.go` has the same. `Normalize()` hardcodes
the Ollama default endpoint. `WorkspaceConfig.Model` has no provider association.
`ayenitd/config.go`, `ayenitd/bootstrap_extract.go`, `ayenitd/capability_bundle.go`,
and `ayenitd/open.go` carry the same coupling independently.

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

**`ayenitd/config.go`**
- Replace `OllamaEndpoint string` and `OllamaModel string` with
  `InferenceProvider string`, `InferenceEndpoint string`, `InferenceModel string`.
- Fix all call sites.

**`ayenitd/bootstrap_extract.go`**
- Replace `OllamaEndpoint string` and `OllamaModel string` in the opts struct with
  `Backend llm.ManagedBackend`.
- `OllamaToolCalling` reference migrates to `NativeToolCallingEnabled()` via the spec
  accessor (Phase 1).
- Update internal construction paths that previously called `llm.NewClient(...)` to
  use `llm.New(ProviderConfig{...})`.

**`ayenitd/capability_bundle.go`**
- Replace `OllamaEndpoint string` and `OllamaModel string` fields with
  `InferenceEndpoint string`, `InferenceModel string` (or accept `ManagedBackend`
  directly if construction happens post-bootstrap).

**`ayenitd/open.go`**
- Replace `llm.NewClient(cfg.OllamaEndpoint, ollamaModel)` with
  `llm.New(ProviderConfig{...})` followed by `backend.Model()`.
- Replace `OllamaEndpoint`/`OllamaModel` validation guards with provider-neutral
  validation (non-empty `InferenceEndpoint`, non-empty `InferenceModel`).
- Embedder construction is updated in Phase 8.

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
- No code path in `ayenitd/` reads `OllamaEndpoint` or `OllamaModel` directly; all
  provider-specific strings flow through neutral fields or `ManagedBackend`.
- `go test ./app/relurpish/runtime/...` passes.
- `go test ./ayenitd/...` passes.
- `go build ./...` passes.

---

## Phase 6: Probe and doctor generalisation

### Objective

Replace `OllamaReport` and `detectOllama` with a provider-neutral inference backend
report populated via `ManagedBackend`. Apply the same to `ayenitd/probe.go`.

### Context

`probe.go` currently contains `OllamaReport`, `EnvironmentReport.Ollama OllamaReport`,
and `detectOllama()` which directly issues `GET /api/tags` against
`cfg.OllamaEndpoint`. After Phase 5 the runtime has a `ManagedBackend`; the probe
layer should delegate to it rather than reimplementing health checks for a specific
provider. `ayenitd/probe.go` independently calls `checkOllamaReachable` and
`checkOllamaModel` via direct HTTP.

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
- `ProbeEnvironment` signature gains `backend llm.ManagedBackend` parameter.
- For the case where no backend is constructed yet (pre-bootstrap doctor invocation),
  fall back to constructing a temporary backend from config, probing it, and closing
  it.

**`app/relurpish/runtime/doctor.go`** (if separate from probe)
- Update doctor output formatting to display `InferenceBackendReport` fields:
  provider name, state, endpoint (for transport backends), model list.

**`ayenitd/probe.go`**
- Replace `checkOllamaReachable(endpoint)` and `checkOllamaModel(endpoint, model)`
  with a single `checkInferenceBackend(backend llm.ManagedBackend)` that delegates to
  `backend.Health(ctx)` and `backend.ListModels(ctx)`.
- `ProbeWorkspace` (or equivalent entry point) gains a `backend llm.ManagedBackend`
  parameter. Callers in `ayenitd/open.go` pass the bootstrapped backend.
- Remove all direct HTTP calls against `cfg.OllamaEndpoint` from this package.

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
- `checkOllamaReachable` and `checkOllamaModel` in `ayenitd/probe.go` are removed;
  no direct HTTP calls against `OllamaEndpoint` remain in `ayenitd/`.
- `go test ./app/relurpish/runtime/...` passes.
- `go test ./ayenitd/...` passes.

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
- `TestModelSelection_PersistsProvider`: selecting a model persists both `Provider`
  and `Model` in workspace config.
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
directly using `cfg.OllamaEndpoint`. `ayenitd/open.go` does the same independently.
After this phase, bootstrap checks `backend.Embedder()` first.

### Work

**Move `framework/retrieval/ollama_embedder.go`**
- Relocate implementation to `platform/llm/ollama/embedder.go`. The `OllamaEmbedder`
  type becomes `ollama.Embedder`.
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

**`ayenitd/open.go`** (embedder portion)
- Replace `retrieval.NewOllamaEmbedder(cfg.OllamaEndpoint, ollamaModel)` with
  `retrieval.NewEmbedder(backend, embedderCfgFromConfig(cfg))`.
- `embedderCfgFromConfig` maps `ayenitd.Config` neutral fields to
  `retrieval.EmbedderConfig`; if embedding fields are empty, falls back to the
  inference endpoint/model as the embedding source (matching existing behaviour).
- If returned embedder is nil, log and proceed.

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
- `ayenitd/open.go` embedder construction uses `retrieval.NewEmbedder`; no direct
  `NewOllamaEmbedder` call remains in `ayenitd/`.
- `go test ./framework/retrieval/...` passes.
- `go test ./platform/llm/ollama/...` passes.
- `go test ./ayenitd/...` passes.

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
is not explicitly documented as the contract and is not tested against a parity
requirement.

`ResolveCallingMode` takes `core.BackendCapabilities` (not `llm.ManagedBackend`) so
that `framework/capability` does not import `platform/llm`. Callers obtain
capabilities via `backend.Capabilities()` before passing.

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
- Add `ResolveCallingMode(spec *core.AgentRuntimeSpec, caps core.BackendCapabilities)
  CapabilityCallingMode`: returns `Native` if `spec.NativeToolCallingEnabled()` and
  `caps.NativeToolCalling`, else `Fallback`.
- Ensure no duplicated fallback render/parse logic exists in any provider subpackage.
  Provider packages translate native schemas only; they never re-implement the
  text-render fallback.

**`framework/capability/write_path_precheck.go`** — ensure fallback path is invoked
correctly when `CapabilityCallingFallback` is resolved.

### Tests

**`framework/capability/capability_calling_test.go`** (new)
- `TestResolveCallingMode_NativeEnabled_CapsSupports`: spec native enabled,
  `BackendCapabilities{NativeToolCalling: true}` → `Native`.
- `TestResolveCallingMode_NativeEnabled_CapsLacks`: spec native enabled,
  `BackendCapabilities{NativeToolCalling: false}` → `Fallback`.
- `TestResolveCallingMode_NativeDisabled`: spec native disabled → `Fallback`
  regardless of capabilities.
- `TestFallbackParity_TextMatchesNative`: agent issues two identical tool calls — one
  via native path (mock backend), one via fallback text render/parse. The resulting
  `core.ToolCall` structs are equal (name and args match).
- `TestRenderToolsToPrompt_RoundTrip`: render capabilities to prompt, then parse the
  model's simulated response → all tool calls recovered.
- `TestParseToolCallsFromText_MalformedJSON`: model returns syntactically invalid JSON
  → parser returns partial results without panic.
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

## Phase 17: Ollama finalization and provider conformance

### Objective

Stabilise Ollama as the reference provider, audit all remaining `Ollama*` references,
add tape recording provider metadata, and establish a shared conformance test suite
that all providers must pass.

### Work

**Audit**
- `grep -r "Ollama\|ollama_endpoint\|ollama_model\|ollama_tool_calling"
  --include="*.go" --include="*.yaml"` across the full repository, including
  `ayenitd/`.
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

### Exit criteria

- No `Ollama*` field names remain outside the compat normalisation layer in
  `config.go` and the `platform/llm/ollama` subpackage.
- Both transport provider implementations (Ollama, LM Studio) pass the conformance
  suite.
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
- `docs/providers.md` (new): per-provider setup guide for Ollama and LM Studio.
  Includes: configuration fields, default endpoint, capability flags, known
  limitations.
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
- Documentation is internally consistent and covers Ollama and LM Studio.
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

---

## Key Risks and Mitigations

**Risk: Partial config migration leaves two config systems alive.**
Mitigation: Phase 5 normalisation layer is the single read point for legacy fields.
The compatibility regression suite runs after every phase to catch divergence.

**Risk: OpenAI-compat transport assumptions leak into the facade.**
Mitigation: `platform/llm/openaicompat` exports no type names that appear in root
`platform/llm` or `framework/`. LM Studio test confirms it delegates to
`openaicompat` without adding its own copies of request/response types.
