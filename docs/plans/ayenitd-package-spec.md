# ayenitd — Engineering Specification

**Status**: Approved, not yet implemented  
**Depends on**: Nothing (foundational)  
**Required by**: `docs/plans/chat-context-enrichment-pipeline.md` (cannot implement without this)  
**Affects**: `app/relurpish/runtime/`, `framework/agentenv/`, `agents/`, all named agents

---

## 1. Purpose

`ayenitd` is the **composition root and service lifecycle manager** for Relurpify. It is analogous to systemd/init: it starts services in dependency order, holds them alive, and shuts them down cleanly on exit. Nothing in `agents/`, `named/`, or `app/` should be responsible for constructing or wiring platform services — that is `ayenitd`'s job.

Currently this responsibility is scattered across:
- `app/relurpish/runtime/runtime.go:New()` — opens stores, builds model, calls bootstrap
- `app/relurpish/runtime/bootstrap.go:BootstrapAgentRuntime()` — builds capabilities, wires `AgentEnvironment`
- `app/relurpish/runtime/runtime.go:openRuntimeStores()` — opens SQLite stores
- `app/dev-agent-cli/start.go` — separate init path with duplicated wiring

`ayenitd` centralises all of this into a single `Open()` call that returns a live, health-checked `*Workspace`.

### Position in package hierarchy

```
app/relurpish/          ← TUI, CLI, HTTP — uses ayenitd.Open()
app/dev-agent-cli/      ← CLI entry point — uses ayenitd.Open()
named/euclo/            ← receives ayenitd.WorkspaceEnvironment
named/rex/              ← receives ayenitd.WorkspaceEnvironment
named/testfu/           ← receives ayenitd.WorkspaceEnvironment
------------------------------------------------------------
ayenitd/                  ← NEW: composition root + service lifecycle
------------------------------------------------------------
agents/                 ← implementation layer, framework imports only
framework/              ← pure abstractions: interfaces, types, algorithms
platform/               ← OS-level adapters: llm/, fs/, git/, shell/, ast/
```

`ayenitd` imports from `framework/`, `platform/`, and `archaeo/`. Nothing in `framework/` or `platform/` imports `ayenitd`.

---

## 2. Package Structure

```
ayenitd/
  doc.go                 — package overview
  open.go                — Open() entry point
  workspace.go           — Workspace struct + Close()
  environment.go         — WorkspaceEnvironment type
  config.go              — WorkspaceConfig (resolved from flags + YAML)
  probe.go               — platform runtime checks
  services.go            — service graph + initialization ordering
  scheduler.go           — ServiceScheduler (cron-from-memory)
  stores.go              — SQLite store opening helpers
  aliases.go             — backward-compat type aliases (temporary)
  workspace_test.go      — integration tests
  probe_test.go          — platform probe tests
  scheduler_test.go      — scheduler tests
```

---

## 3. Core Types

### 3.1 WorkspaceEnvironment

This is the shared dependency container that agents receive. It replaces `framework/agentenv.AgentEnvironment`.

```go
// WorkspaceEnvironment is the set of pre-initialized services shared across all
// agents in a workspace session. It is produced by ayenitd.Open() and passed
// directly to agent constructors. It is shallow-copyable; agents may narrow
// scope (e.g. replace Registry for a child execution) without rebuilding.
type WorkspaceEnvironment struct {
    // Identity + model
    Config  *core.Config
    Model   core.LanguageModel

    // Capability + permission
    Registry          *capability.Registry
    PermissionManager *authorization.PermissionManager

    // Code intelligence
    IndexManager *ast.IndexManager
    SearchEngine *search.SearchEngine

    // Memory + storage
    Memory          memory.MemoryStore
    WorkflowStore   memory.WorkflowStateStore
    CheckpointStore memory.CheckpointStore   // nil until implemented in framework
    PlanStore       frameworkplan.PlanStore
    PatternStore    patterns.PatternStore
    CommentStore    patterns.CommentStore
    GuidanceBroker  *guidance.GuidanceBroker

    // Retrieval
    Embedder    retrieval.Embedder    // generic interface, not Ollama-specific
    RetrievalDB *sql.DB               // shared DB for retrieval index tables

    // Agents that verify or extract compatibility surface (optional)
    VerificationPlanner           agentenv.VerificationPlanner
    CompatibilitySurfaceExtractor agentenv.CompatibilitySurfaceExtractor

    // Scheduler
    Scheduler *ServiceScheduler
}

// WithRegistry returns a shallow copy with Registry replaced.
// Agents use this to scope capability access for child executions.
func (e WorkspaceEnvironment) WithRegistry(r *capability.Registry) WorkspaceEnvironment {
    e.Registry = r
    return e
}

// WithMemory returns a shallow copy with Memory replaced.
func (e WorkspaceEnvironment) WithMemory(m memory.MemoryStore) WorkspaceEnvironment {
    e.Memory = m
    return e
}
```

**Note on Embedder**: The concrete implementation is `retrieval.OllamaEmbedder` constructed from the workspace config's Ollama endpoint. When a non-Ollama embedder backend is added, this is the only wiring point that changes.

**Note on CheckpointStore**: Not yet implemented in `framework/memory`. The field is included now; it will be nil until the store is added. This avoids a second migration.

### 3.2 WorkspaceConfig (input)

```go
// WorkspaceConfig is the resolved configuration produced from CLI flags, YAML
// workspace config, and environment. It is the input to ayenitd.Open().
type WorkspaceConfig struct {
    // Required
    Workspace      string   // absolute path to workspace root
    ManifestPath   string   // agent manifest YAML
    OllamaEndpoint string
    OllamaModel    string   // overrides manifest if non-empty

    // Optional
    ConfigPath          string   // workspace config YAML (relurpify.yaml etc)
    AgentsDir           string   // named agent definition overlay directory
    AgentName           string   // initial agent to load
    LogPath             string
    TelemetryPath       string
    EventsPath          string
    MemoryPath          string
    MaxIterations       int
    SkipASTIndex        bool
    HITLTimeout         time.Duration
    AuditLimit          int
    Sandbox             bool
    DebugLLM            bool
    DebugAgent          bool
    AllowedCapabilities []core.CapabilitySelector
}
```

### 3.3 Workspace (output)

```go
// Workspace is a live, initialized workspace session. It holds all open
// resources. Close() must be called when the session ends.
type Workspace struct {
    Environment WorkspaceEnvironment
    Registration *authorization.AgentRegistration

    // Internals held for Close()
    logFile     io.Closer
    eventLog    io.Closer
    patternDB   io.Closer

    // Derived fields for callers that need them
    AgentSpec         *core.AgentRuntimeSpec
    AgentDefinitions  map[string]*core.AgentDefinition
    CompiledPolicy    *policybundle.CompiledPolicyBundle
    EffectiveContract *contractpkg.EffectiveAgentContract
}

func (w *Workspace) Close() error { ... }
```

---

## 4. Open() — Entry Point

```go
// Open initializes a complete workspace session: platform checks, store
// opening, service graph construction, agent registration, and background
// indexing. The returned *Workspace is ready for agent construction.
//
// Open is the single composition root for all Relurpify entry points.
// app/relurpish, app/dev-agent-cli, and integration tests all call Open().
func Open(ctx context.Context, cfg WorkspaceConfig) (*Workspace, error)
```

`Open()` executes the following phases in order:

### Phase A: Configuration Validation

1. Validate required fields: `Workspace`, `ManifestPath`, `OllamaEndpoint`, `OllamaModel`
2. Resolve `WorkspaceConfig` from workspace YAML if `ConfigPath` is set (model override, agent name override, allowed capabilities)
3. Normalise paths (absolute, mkdir log directory)

### Phase B: Platform Runtime Checks (via `probe.go`)

Run before opening any resources. If any required check fails, `Open()` returns an error immediately.

| Check | Required | Error message |
|---|---|---|
| `Workspace` directory exists and is readable | yes | `"workspace not found: %s"` |
| SQLite file locations are writable | yes | `"sessions dir not writable: %s"` |
| Ollama endpoint reachable (`/api/tags` HEAD) | yes | `"ollama not reachable at %s; is it running?"` |
| Ollama model present | yes | `"model %s not found in ollama; run: ollama pull %s"` |
| Disk space ≥ 256 MB on workspace volume | no (warn only) | log warning |

The platform checks are exposed as `ProbeWorkspace(cfg WorkspaceConfig) []ProbeResult` so `relurpish doctor` can call them independently (replacing the current `app/relurpish/runtime/doctor.go` with calls to this).

```go
type ProbeResult struct {
    Name     string
    Required bool
    OK       bool
    Message  string
}
```

### Phase C: Log and Telemetry Setup

Open log file → build telemetry sinks (logger, JSON file, event log). Same logic as current `runtime.go:New()` lines 119–231.

### Phase D: Store Initialization (dependency order)

```
SQLite workflow DB  ←── WorkflowStateStore
                    ←── PlanStore (shares same DB connection)

patterns DB         ←── PatternStore
                    ←── CommentStore

workflow DB         ←── RetrievalDB (same *sql.DB, retrieval tables created on first use)

memory path         ←── HybridMemoryStore + InMemoryVectorStore
```

Each store failure propagates upward with cleanup of already-opened handles.

### Phase E: Agent Registration + Authorization

`authorization.RegisterAgent(ctx, ...)` — sandbox, manifest, HITL setup. Same as current `runtime.go` lines 150–181.

### Phase F: Capability Bundle + Agent Environment

Build `CapabilityBundle` (existing `BuildBuiltinCapabilityBundle` in `bootstrap.go`):
- AST IndexManager (starts background indexing)
- SearchEngine
- CapabilityRegistry

Construct `WorkspaceEnvironment` from all services built above.

### Phase G: Relurpic Capability Registration

`agents.RegisterBuiltinRelurpicCapabilitiesWithOptions(...)` — same as current `bootstrap.go` lines 171–184.

### Phase H: Embedder Initialization

Construct `retrieval.OllamaEmbedder` from `cfg.OllamaEndpoint`. Assign to `WorkspaceEnvironment.Embedder`.

Future: config-driven embedder backend selection.

### Phase I: Scheduler Start

Start `ServiceScheduler` if cron-from-memory entries exist. Assign to `WorkspaceEnvironment.Scheduler`.

---

## 5. Service Initialization Dependency Graph

```
┌──────────────────────────────────────────────────┐
│ Platform Checks                                  │
└───────────────────────┬──────────────────────────┘
                        │
           ┌────────────▼────────────┐
           │ SQLite workflow DB      │
           └──┬──────────┬──────────┘
              │          │
     ┌────────▼──┐  ┌────▼──────────┐
     │ WorkflowStore│  │ PlanStore   │
     └────────┬──┘  └───────────────┘
              │
     ┌────────▼──────────────────────┐
     │ RetrievalDB (shared DB conn)  │
     └────────┬──────────────────────┘
              │
     ┌────────▼──┐   ┌───────────────┐
     │ PatternDB │   │ MemoryStore   │
     └──┬───┬───┘   └───────────────┘
        │   │
 ┌──────▼┐ ┌▼──────────┐
 │Pattern│ │Comment    │
 │Store  │ │Store      │
 └───────┘ └───────────┘
              │
     ┌────────▼──────────────────────┐
     │ AgentRegistration             │
     │ (sandbox + manifest + HITL)   │
     └────────┬──────────────────────┘
              │
     ┌────────▼──────────────────────┐
     │ CapabilityBundle              │
     │  ├── IndexManager (+ Start)   │
     │  ├── SearchEngine             │
     │  └── CapabilityRegistry       │
     └────────┬──────────────────────┘
              │
     ┌────────▼──────────────────────┐
     │ WorkspaceEnvironment          │
     │  (all services assembled)     │
     └────────┬──────────────────────┘
              │
     ┌────────▼──────────────────────┐
     │ Embedder init                 │
     └────────┬──────────────────────┘
              │
     ┌────────▼──────────────────────┐
     │ Scheduler start               │
     └───────────────────────────────┘
```

---

## 6. ServiceScheduler

`ServiceScheduler` handles time-based and memory-triggered service invocations. Initial scope (Phase 1 implementation):

```go
type ScheduledJob struct {
    ID       string
    CronExpr string          // standard 5-field cron expression
    Action   func(context.Context) error
    Source   string          // "memory" | "config" | "internal"
}

type ServiceScheduler struct {
    jobs   []ScheduledJob
    cancel context.CancelFunc
    wg     sync.WaitGroup
}

func NewServiceScheduler() *ServiceScheduler
func (s *ServiceScheduler) Register(job ScheduledJob)
func (s *ServiceScheduler) Start(ctx context.Context)
func (s *ServiceScheduler) Stop()
```

**Cron-from-memory**: On startup, `ayenitd.Open()` queries the pattern/memory store for persisted job definitions (stored under a well-known key prefix, e.g. `ayenitd.cron.*`). Each entry is deserialized as a `ScheduledJob` and registered with the scheduler.

**Phase 1 jobs** (hardcoded, not from memory yet):
- Re-index workspace (re-call `IndexManager.StartIndexing`) on a configurable interval (default: none, opt-in via workspace config)

**Phase 2**: Load jobs from memory store records. Job definitions are saved by agents that want recurring background work (e.g. a planning agent that wants to pre-warm the retrieval index nightly).

---

## 7. Moving framework/agentenv → ayenitd

### 7.1 What lives in framework/agentenv today

- `AgentEnvironment` struct — 8 fields (Model, Registry, IndexManager, SearchEngine, Memory, Config, VerificationPlanner, CompatibilitySurfaceExtractor)
- `VerificationPlan`, `VerificationPlanRequest`, `VerificationPlanner` interface
- `CompatibilitySurface`, `CompatibilitySurfaceRequest`, `CompatibilitySurfaceExtractor` interface
- `WithRegistry()` / `WithMemory()` shallow-copy helpers

### 7.2 What moves where

| Item | Moves to |
|---|---|
| `AgentEnvironment` struct + helpers | **replaced** by `ayenitd.WorkspaceEnvironment` |
| `VerificationPlan` / `VerificationPlanRequest` / `VerificationPlanner` | `ayenitd/verification.go` (or `framework/agentenv` stays as a thin interface-only package — see note) |
| `CompatibilitySurface*` / `CompatibilitySurfaceExtractor` | same as above |

**Design note**: `VerificationPlanner` and `CompatibilitySurfaceExtractor` are interfaces used by `named/euclo`. They don't belong in `framework/` (they're agent-level contracts, not framework-level abstractions), but they also shouldn't live in `ayenitd/` if `ayenitd` is imported by `framework/`. Since `framework/` never imports `ayenitd`, there is no cycle risk — these interfaces can move to `ayenitd/verification.go`. The concrete implementations (if any) stay in `named/euclo/`.

### 7.3 Migration plan (backward-compat first, cleanup later)

**Step 1** — Add `ayenitd.WorkspaceEnvironment` with all fields. Do not touch `framework/agentenv` yet.

**Step 2** — In `agents/environment.go`, add a second type alias:
```go
// WorkspaceEnvironment is the new composition-root-supplied environment.
// Use this in new code. AgentEnvironment is kept for compatibility.
type WorkspaceEnvironment = ayenitd.WorkspaceEnvironment
```

**Step 3** — Change `app/relurpish/runtime/runtime.go` to call `ayenitd.Open()` instead of inline construction. The `BootstrappedAgentRuntime.Environment` field stays as `AgentEnvironment` at this point (it's populated from `WorkspaceEnvironment` by a conversion function).

**Step 4** — Migrate `named/euclo/agent.go`, `named/rex/agent.go`, and other named agents to accept `ayenitd.WorkspaceEnvironment` instead of `agents.AgentEnvironment`. This is a field rename — the struct fields are a superset.

**Step 5** (cleanup, later session) — Delete `framework/agentenv/environment.go`. Update all import paths. Remove `agents/environment.go` alias.

### 7.4 Files that reference AgentEnvironment (callers to update in Step 4)

The following files use `agents.AgentEnvironment` or `agentenv.AgentEnvironment` as a struct type (not just import):

- `app/relurpish/runtime/bootstrap.go` — constructs and returns it (line 186)
- `app/relurpish/runtime/runtime.go` — stores as `agentEnv`, passes to `instantiateAgent` (lines 288, 554)
- `named/euclo/agent.go` — `a.Environment agents.AgentEnvironment`
- `named/rex/agent.go` — same pattern
- `named/testfu/agent.go` — same pattern
- `named/eternal/agent.go` — same pattern
- `agents/builder.go` — builds and returns it
- `testutil/euclotestutil/integration.go` — test helper constructs it
- `testutil/agenttestscenario/fixture.go` — fixture builds it

These are the files that need updating in Step 4. All other files that import `agentenv` just use the interface types (`VerificationPlanner` etc) — those are unaffected until Step 5.

---

## 8. app/relurpish/runtime migration

After `ayenitd.Open()` exists, `app/relurpish/runtime/runtime.go:New()` becomes:

```go
func New(ctx context.Context, cfg Config) (*Runtime, error) {
    wcfg := ayenitd.WorkspaceConfig{
        Workspace:      cfg.Workspace,
        ManifestPath:   cfg.ManifestPath,
        OllamaEndpoint: cfg.OllamaEndpoint,
        OllamaModel:    cfg.OllamaModel,
        // ... map remaining fields
    }
    ws, err := ayenitd.Open(ctx, wcfg)
    if err != nil {
        return nil, err
    }
    rt := &Runtime{
        Environment: ws.Environment,
        // ... populate from ws.*
    }
    // Register providers, wire agent, etc.
    return rt, nil
}
```

The `Runtime` struct keeps its existing fields for now (no structural change to `Runtime`). The composition logic moves; the struct's public surface stays stable.

`app/relurpish/runtime/bootstrap.go` becomes an internal detail of `ayenitd` — its exported `BootstrapAgentRuntime` and `AgentBootstrapOptions` types are deprecated (kept for `dev-agent-cli` until it's also migrated).

---

## 9. Architectural Cleanup Notes (future work)

The following are **not** in scope for the initial `ayenitd` implementation. They are listed here so they are not forgotten.

| Item | Current location | Future location | Reason |
|---|---|---|---|
| SQLite store implementations | `framework/memory/db/` | `ayenitd/db/` or keep in framework | SQLite is an impl detail, not a framework abstraction |
| `IndexManager` (concrete) | `framework/ast/` | `ayenitd/` or `platform/ast/` | Depends on filesystem + goroutines, not a pure abstraction |
| `PermissionManager` | `framework/authorization/` | `ayenitd/` | Stateful service, not an algorithm |
| `CapabilityRegistry` | `framework/capability/` | `ayenitd/` | Stateful registry, depends on service lifecycle |
| `OllamaEmbedder` | `framework/retrieval/` | `platform/llm/` | Platform implementation |
| `openRuntimeStores` | `app/relurpish/runtime/` | `ayenitd/stores.go` | Already handled in initial scope |
| `doctor.go` | `app/relurpish/runtime/` | delegates to `ayenitd.ProbeWorkspace()` | ayenitd owns checks |

**Rule**: `framework/` should contain zero SQLite/HTTP/filesystem dependencies. It should be buildable with `GOOS=js` or in a WASM sandbox. Move everything that violates this rule to `ayenitd/` or `platform/` in a future cleanup pass.

---

## 10. Testing Infrastructure

### 10.1 Unit tests (no Ollama, no SQLite needed)

- `probe_test.go` — test each `ProbeResult` against mock HTTP server and temp directories
- `scheduler_test.go` — register jobs, assert firing at correct intervals using fake clock
- `environment_test.go` — test `WithRegistry`, `WithMemory` shallow-copy helpers

### 10.2 Integration test

```go
// workspace_test.go
func TestOpenWorkspace(t *testing.T) {
    // Requires: Ollama running, test model present
    // 1. Create temp directory with a few .go files
    // 2. Copy test manifest
    // 3. ayenitd.Open(ctx, cfg)
    // 4. Assert ws.Environment.IndexManager != nil
    // 5. Assert ws.Environment.WorkflowStore != nil
    // 6. Assert ws.Environment.Registry != nil
    // 7. Wait for IndexManager ready (with timeout)
    // 8. Assert IndexManager.Ready() == true
    // 9. ws.Close() — no error
}
```

This is the integration test that would have caught the IndexManager wiring gap described in `docs/issues/recurring-indexmanager-wiring-gap.md`. Once this test exists, the gap cannot silently recur.

### 10.3 Probe test

```go
func TestProbeWorkspace_OllamaUnreachable(t *testing.T) {
    cfg := WorkspaceConfig{
        Workspace:      t.TempDir(),
        OllamaEndpoint: "http://127.0.0.1:11435", // wrong port
        OllamaModel:    "qwen2.5-coder:14b",
    }
    results := ProbeWorkspace(cfg)
    var ollamaResult ProbeResult
    for _, r := range results {
        if strings.Contains(r.Name, "ollama") {
            ollamaResult = r
        }
    }
    assert.False(t, ollamaResult.OK)
    assert.True(t, ollamaResult.Required)
}
```

---

## 11. Implementation Phases

### Phase 1 — Package skeleton + WorkspaceEnvironment type

- Create `ayenitd/` directory
- Define `WorkspaceEnvironment`, `WorkspaceConfig`, `Workspace` types
- Implement `Open()` by **extracting** the existing logic from `runtime.go:New()` + `bootstrap.go:BootstrapAgentRuntime()` verbatim — no new logic yet
- Add `ProbeWorkspace()` extracted from `runtime/doctor.go`
- Add `stores.go` extracted from `openRuntimeStores()`
- Add `ayenitd.WorkspaceEnvironment` to `agents/environment.go` as second alias
- All existing tests must pass unchanged

### Phase 2 — Platform probe improvements

- Implement full probe suite (Ollama, disk space, workspace dir, SQLite writability)
- Make `app/relurpish/runtime/doctor.go` delegate to `ayenitd.ProbeWorkspace()`
- Add probe unit tests

### Phase 3 — Migrate app/relurpish to use ayenitd.Open()

- Rewrite `runtime.go:New()` to call `ayenitd.Open()`
- Delete or internalize `bootstrap.go` logic (keep exported symbols as deprecated stubs for dev-agent-cli)
- Add integration test `TestOpenWorkspace`

### Phase 4 — Migrate named agents to WorkspaceEnvironment

- Update `named/euclo/agent.go` — `Environment ayenitd.WorkspaceEnvironment`
- Update `named/rex/agent.go`, `named/testfu/agent.go`, `named/eternal/agent.go`
- Update `testutil/euclotestutil/` helpers
- Confirm all tests pass

### Phase 5 — ServiceScheduler

- Implement `scheduler.go` with cron-from-memory loading
- Start scheduler in `Open()`
- Add scheduler tests

### Phase 6 — framework/agentenv cleanup (separate session)

- After all named agents use `ayenitd.WorkspaceEnvironment`
- Delete `framework/agentenv/environment.go`
- Move `VerificationPlanner` / `CompatibilitySurfaceExtractor` interfaces to `ayenitd/verification.go`
- Update all import paths
- Delete `agents/environment.go`

---

## 12. Open Questions

1. **Embedder in WorkspaceEnvironment vs CapabilityBundle**: The current `BuildBuiltinCapabilityBundle` doesn't build an Embedder. Should Phase 1 add Embedder to `CapabilityRegistryOptions` and wire it there, or should `ayenitd.Open()` construct it separately after `CapabilityBundle`? Recommendation: construct separately in Phase 1 (simpler), add to bundle in Phase 5 when the context enrichment pipeline needs it.

2. **dev-agent-cli migration**: `app/dev-agent-cli/start.go` has its own wiring path. It should call `ayenitd.Open()` in Phase 3, but it currently uses different CLI flags. Flag mapping should be explicit; no silent defaults.

3. **WorkspaceEnvironment in framework/contextmgr**: `NewProgressiveLoader` currently takes `*ast.IndexManager` directly. After agents receive `WorkspaceEnvironment`, should `contextmgr` accept the full environment or continue with the specific field? Continue with specific field — `contextmgr` is a framework package and must not import `ayenitd`.
