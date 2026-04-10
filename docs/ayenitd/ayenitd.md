# ayenitd — Composition Root and Service Lifecycle Manager

## Overview

`ayenitd` is the composition root and service lifecycle manager for Relurpify. It is analogous to systemd/init: it starts services in dependency order, holds them alive, and shuts them down cleanly on exit. Nothing in `agents/`, `named/`, or `app/` is responsible for constructing or wiring platform services — that is `ayenitd`'s job.

The single entry point is `Open()`, which accepts a `WorkspaceConfig` and returns a live, health-checked `*Workspace` ready for agent construction.

---

## Position in Package Hierarchy

```
app/relurpish/          ← TUI, CLI, HTTP — calls ayenitd.Open()
app/dev-agent-cli/      ← CLI entry point — calls ayenitd.Open()
named/euclo/            ← receives ayenitd.WorkspaceEnvironment
named/rex/              ← receives ayenitd.WorkspaceEnvironment
named/testfu/           ← receives ayenitd.WorkspaceEnvironment
------------------------------------------------------------
ayenitd/                  ← composition root + service lifecycle
------------------------------------------------------------
agents/                 ← implementation layer, framework imports only
framework/              ← pure abstractions: interfaces, types, algorithms
platform/               ← OS-level adapters: llm/, fs/, git/, shell/, ast/
```

`ayenitd` imports from `framework/`, `platform/`. Nothing in `framework/` or `platform/` imports `ayenitd`.

---

## Package Structure

```
ayenitd/
  doc.go                 — package overview
  open.go                — Open() entry point + telemetry setup
  workspace.go           — Workspace struct, Close(), StealClosers()
  environment.go         — WorkspaceEnvironment type + WithRegistry/WithMemory
  config.go              — WorkspaceConfig + AgentLabel() helper
  probe.go               — ProbeWorkspace() + ProbeResult
  capability_bundle.go   — CapabilityBundle + BuildBuiltinCapabilityBundle
  bootstrap_extract.go   — BootstrapAgentRuntime (extracted from app/relurpish)
  browser_service.go     — browser service wiring helper for Open()
  stores.go              — openRuntimeStores() (SQLite store opening helpers)
  scheduler.go           — ServiceScheduler, ScheduledJob, SaveJobToMemory
  agentenv_interfaces.go — re-exports VerificationPlanner, CompatibilitySurface*
  aliases.go             — backward-compat type aliases (WorkspaceEnvironmentAlias, etc.)
  authorization.go       — placeholder (authorization logic in framework/authorization)
  services.go            — placeholder (future service graph)
  workspace_test.go      — integration tests
  workspace_test_manifest_test.go — shared integration manifest helper
  probe_test.go          — platform probe tests
  scheduler_test.go      — scheduler tests
  environment_test.go    — WithRegistry/WithMemory tests
  scheduler_export_test.go — exported test helpers for scheduler
```

---

## Core Types

### WorkspaceConfig (input)

```go
type WorkspaceConfig struct {
    // Required
    Workspace      string            // absolute path to workspace root
    ManifestPath   string            // agent manifest YAML
    InferenceEndpoint string
    InferenceModel    string            // overrides manifest if non-empty

    // Optional
    ConfigPath          string
    AgentsDir           string        // named agent definition overlay directory
    AgentName           string        // initial agent to load
    LogPath             string
    TelemetryPath       string
    EventsPath          string
    MemoryPath          string
    MaxIterations       int
    SkipASTIndex        bool
    HITLTimeout         time.Duration
    AuditLimit          int
    Sandbox             fsandbox.SandboxConfig
    DebugLLM            bool
    DebugAgent          bool
    AllowedCapabilities []core.CapabilitySelector
    ReindexInterval     time.Duration // non-zero enables periodic AST re-indexing
}

func (cfg WorkspaceConfig) AgentLabel() string
```

`AgentLabel()` returns `AgentName` if set, otherwise `"default"`. Used for configuration lookup.

**Config override**: If `ConfigPath` is set, `Open()` loads the workspace YAML before running platform probes and merges `model` and `agents[0]` into the config (JSON-parsed; empty-only override semantics).

### WorkspaceEnvironment

The shared dependency container passed directly to agent constructors. Shallow-copyable — agents may narrow scope without rebuilding.

```go
type WorkspaceEnvironment struct {
    // Identity + model
    Config *core.Config
    Model  core.LanguageModel

    // Capability + permission
    Registry          *capability.Registry
    PermissionManager *fauthorization.PermissionManager

    // Code intelligence
    IndexManager *ast.IndexManager
    SearchEngine *search.SearchEngine

    // Memory + storage
    Memory          memory.MemoryStore
    WorkflowStore   memory.WorkflowStateStore
    CheckpointStore *memory.CheckpointStore  // nil (not yet implemented)
    PlanStore       plan.PlanStore
    PatternStore    patterns.PatternStore
    CommentStore    patterns.CommentStore
    GuidanceBroker  *guidance.GuidanceBroker

    // Retrieval
    Embedder    retrieval.Embedder  // OllamaEmbedder; interface for future backends
    RetrievalDB *sql.DB             // shared DB for retrieval index tables

    // Agent contracts (optional, set by named agents)
    VerificationPlanner           VerificationPlanner
    CompatibilitySurfaceExtractor CompatibilitySurfaceExtractor

    // Scheduler
    Scheduler *ServiceScheduler
}

func (e WorkspaceEnvironment) WithRegistry(r *capability.Registry) WorkspaceEnvironment
func (e WorkspaceEnvironment) WithMemory(m memory.MemoryStore) WorkspaceEnvironment
```

`WithRegistry` and `WithMemory` return a shallow copy with the named field replaced. Used by ArchitectAgent and similar patterns to scope capabilities for child executions without rebuilding the full environment.

**Note on CheckpointStore**: The field exists but is always `nil`. It will be populated when `framework/memory` gains a checkpoint store implementation.

**Note on VerificationPlanner / CompatibilitySurfaceExtractor**: These are `nil` after `Open()`. Named agents (euclo) set them on their copy of the environment.

### Workspace (output)

```go
type Workspace struct {
    Environment  WorkspaceEnvironment
    Registration *fauthorization.AgentRegistration

    // Derived fields
    AgentSpec            *core.AgentRuntimeSpec
    AgentDefinitions     map[string]*core.AgentDefinition
    CompiledPolicy       *policybundle.CompiledPolicyBundle
    EffectiveContract    *contractpkg.EffectiveAgentContract
    CapabilityAdmissions []capabilityplan.AdmissionResult
    SkillResults         []frameworkskills.SkillResolution

    // Observability
    Telemetry core.Telemetry
    Logger    *log.Logger
}

func (w *Workspace) Close() error
func (w *Workspace) StealClosers() (logFile, patternDB, eventLog io.Closer)
```

`Close()` stops the scheduler, closes `WorkflowStore`, `patternDB`, `eventLog`, and `logFile` in order.

`StealClosers()` transfers ownership of raw `io.Closer` handles to the caller and nils them on the `Workspace`. Used by `app/relurpish/runtime` so that `Runtime.Close()` manages the lifecycle directly and avoids double-close.

---

## Open() — Entry Point

```go
func Open(ctx context.Context, cfg WorkspaceConfig) (*Workspace, error)
```

`Open()` executes the following phases in order. On any phase failure, all already-opened resources are cleaned up before returning the error.

### Phase A: Configuration Validation

Validates required fields: `Workspace`, `ManifestPath`, `InferenceEndpoint`, `InferenceModel`. Loads workspace YAML overrides via `resolveWorkspaceConfigOverrides`.

### Phase B: Platform Runtime Checks

Calls `ProbeWorkspace(cfg)`. If any required check returns `OK: false`, `Open()` returns immediately with the check's `Message` as the error.

### Phase C: Log and Telemetry Setup

Opens `LogPath` (defaults to `relurpify_cfg/logs/ayenitd.log`). Builds a `log.Logger` and assembles a `telemetry.MultiplexTelemetry` sink chain (logger + optional JSON file if `TelemetryPath` is set).

### Phase D: Store Initialization

```
openRuntimeStores(workspace):
  SQLite workflow DB  → WorkflowStateStore (SQLiteWorkflowStateStore)
                      → PlanStore (shares same *sql.DB)
  patterns.db         → PatternStore (SQLitePatternStore)
                      → CommentStore (SQLiteCommentStore)
```

`RetrievalDB` is the `WorkflowStore.DB()` connection — retrieval index tables are created on first use, sharing the workflow database.

### Phase E: Agent Registration + Authorization

`fauthorization.RegisterAgent(ctx, RuntimeConfig{...})` — sandbox initialization, manifest loading, HITL setup, permission manager construction.

### Phase F: Capability Bundle + Agent Environment

1. `fsandbox.NewSandboxCommandRunner` — builds the sandboxed command runner.
2. `memory.NewHybridMemory` + `WithVectorStore(InMemoryVectorStore)` — memory store.
3. Model resolution: `cfg.InferenceModel` overrides manifest `spec.agent.model.name`; manifest wins if config is empty.
4. `llm.NewInstrumentedModel` — wraps the Ollama client with telemetry.
5. `guidance.NewGuidanceBroker(0)` — guidance broker.
6. Permission event logger wired to telemetry emit function.
7. `BootstrapAgentRuntime(workspace, opts)` — resolves effective contract, builds capability bundle, admits skill capabilities, compiles policy bundle, constructs `WorkspaceEnvironment`.

**Relurpic capability registration is intentionally NOT done in ayenitd.** Each named agent (euclo, rex, etc.) registers its own relurpic capabilities after receiving `WorkspaceEnvironment`. Registering in `ayenitd` would require importing `named/`, creating a cycle.

### Browser service bootstrap

If the agent manifest enables browser support, `Open()` also constructs the
workspace-owned browser service via [`browser_service.go`](browser_service.go)
and registers it in `ServiceManager`.

That helper:

- derives browser defaults from the agent spec
- builds workspace-scoped file-scope policy
- installs the shared permission manager and command policy
- starts the service before `Open()` returns

This keeps browser wiring out of the rest of the workspace bootstrap flow.

### Phase G: Policy Application

If `boot.CompiledPolicy != nil`, sets `registration.Policy` and installs the engine in `Registry` via `SetPolicyEngine`.

### Phase H: Embedder Initialization

Constructs the configured backend embedder from `cfg.InferenceEndpoint` and `cfg.InferenceModel`. Assigned to `env.Embedder`. Future: config-driven backend selection via the `retrieval.Embedder` interface.

### Phase I: Scheduler Start

Creates `ServiceScheduler`, loads persisted cron jobs from memory store (under `ayenitd.cron.*` keys), optionally registers a `reindex-workspace` interval job if `cfg.ReindexInterval > 0`, then starts the scheduler.

---

## Platform Probes

```go
func ProbeWorkspace(cfg WorkspaceConfig) []ProbeResult

type ProbeResult struct {
    Name     string
    Required bool
    OK       bool
    Message  string
}
```

`ProbeWorkspace` is exposed so `relurpish doctor` can call it independently without triggering `Open()`.

| Check | Name | Required | What it does |
|---|---|---|---|
| Workspace directory readable | `workspace_directory` | yes | `os.Stat` + `os.Open` |
| Sessions dir writable | `sqlite_writable` | yes | `mkdir` + write probe file |
| Ollama endpoint reachable | `ollama_reachable` | yes | `HEAD /api/tags` with 5s timeout |
| Ollama model present | `ollama_model` | yes | `GET /api/tags`, checks name in list |
| Disk space ≥ 256 MB | `disk_space` | no (warn) | `syscall.Statfs` |

---

## Capability Bundle

```go
type CapabilityBundle struct {
    Registry     *capability.Registry
    IndexManager *ast.IndexManager
    SearchEngine *search.SearchEngine
}

type CapabilityRegistryOptions struct {
    Context           context.Context
    AgentID           string
    PermissionManager *fauthorization.PermissionManager
    AgentSpec         *core.AgentRuntimeSpec
    InferenceEndpoint string
    InferenceModel    string
    SkipASTIndex      bool
}

func BuildBuiltinCapabilityBundle(workspace string, runner fsandbox.CommandRunner, opts ...CapabilityRegistryOptions) (*CapabilityBundle, error)
```

`BuildBuiltinCapabilityBundle` registers all built-in capabilities into a fresh `capability.Registry`:
- **Filesystem** — `platformfs.FileOperations(workspace)` (read, write, list, etc.)
- **Search** — `SimilarityTool`, `SemanticSearchTool`
- **Git** — `GitCommandTool` for diff, history, branch, commit, blame
- **Shell** — `platformshell.CommandLineTools(workspace, runner)`
- **AST** — `ASTTool`, `AttachASTSymbolProvider`
- **Code index** — `memory.NewCodeIndex`
- **Graph DB** — opened at `relurpify_cfg/memory/graphdb`; attached to `IndexManager.GraphDB`

If `SkipASTIndex` is true, `BuildIndex` and `StartIndexing` are skipped. The registry and manager are still returned; indexing simply doesn't run.

---

## ServiceScheduler

```go
type ScheduledJob struct {
    ID       string
    Interval time.Duration  // fixed-period; runs immediately on start then repeats
    CronExpr string         // 5-field cron expression (checked every minute)
    Action   func(context.Context) error
    Source   string         // "memory" | "config" | "internal"
}
```

If both `Interval` and `CronExpr` are set, `Interval` takes precedence.

```go
func NewServiceScheduler() *ServiceScheduler
func (s *ServiceScheduler) Register(job ScheduledJob)
func (s *ServiceScheduler) LoadJobsFromMemory(ctx context.Context, mem memory.MemoryStore) error
func (s *ServiceScheduler) Start(ctx context.Context)
func (s *ServiceScheduler) Stop()

func SaveJobToMemory(ctx context.Context, mem memory.MemoryStore, job ScheduledJob) error
```

**Interval jobs** (`Interval > 0`): fires immediately on `Start()`, then repeats on the interval using a `time.Ticker`. Each job runs in its own goroutine.

**Cron jobs** (`CronExpr` set): polls once per minute with a `time.Ticker`. Fires `Action` when the expression matches the current wall-clock time.

**Cron expression syntax** — standard 5-field format (`minute hour day month weekday`). Supported: wildcard (`*`), single value (`5`), range (`1-5`), step on wildcard (`*/2`), step on range (`1-10/3`), comma list (`1,3,5`).

**`LoadJobsFromMemory`**: searches the memory store for keys matching `ayenitd.cron.*`. Each record is deserialized as a `ScheduledJob`. Currently, loaded jobs have inert `Action` functions — full action dispatch via the capability registry with provenance tracking is Phase 2 work. Jobs are loaded and registered but take no effect until then.

**`SaveJobToMemory`**: serializes a job definition to the memory store under `ayenitd.cron.<id>`. `Action` is not serialized (closures are not persistable).

**`Start()` is idempotent** — no-op if already started or no jobs registered.

---

## Store Layout

```
relurpify_cfg/sessions/workflow_state.db   ← WorkflowStateStore + PlanStore + RetrievalDB
relurpify_cfg/patterns.db                  ← PatternStore + CommentStore
relurpify_cfg/memory/code_index.json       ← CodeIndex
relurpify_cfg/memory/graphdb/              ← graph database (attached to IndexManager)
relurpify_cfg/memory/ast_index.db          ← AST symbol store
relurpify_cfg/logs/ayenitd.log             ← default log path
```

---

## BootstrapAgentRuntime

`BootstrapAgentRuntime` is the extracted form of `app/relurpish/runtime/bootstrap.go:BootstrapAgentRuntime`. It is public in `ayenitd` so that the agent test runner (`testsuite/agenttest`) can call it directly without going through `Open()` — tests need controlled HITL and deterministic permission behavior.

`Open()` calls `BootstrapAgentRuntime` internally as part of Phase F.

```go
func BootstrapAgentRuntime(workspace string, opts AgentBootstrapOptions) (*BootstrappedAgentRuntime, error)
```

The returned `BootstrappedAgentRuntime` carries the fully resolved `WorkspaceEnvironment`, compiled policy, effective contract, skill results, and capability admissions.

---

## Relationship to app/relurpish/runtime

`app/relurpish/runtime/bootstrap.go:BootstrapAgentRuntime` is now a thin wrapper over `ayenitd.BootstrapAgentRuntime`. After delegating, it registers relurpic capabilities and agent capabilities — the parts ayenitd intentionally omits. Named agents register their own relurpic capabilities; `app/relurpish` and `app/dev-agent-cli` use the generic `agents.BuildFromSpec` path, so they need relurpic capabilities registered at the bootstrap layer.

`app/dev-agent-cli/start.go` calls `ayenitd.BootstrapAgentRuntime` directly and registers relurpic/agent capabilities inline. The `appruntime` import is kept only for config management, `Runtime` struct construction, and `RegisterBuiltinProviders`.

`app/relurpish/runtime/runtime.go:New()` calls `ayenitd.Open()` (which internally calls `BootstrapAgentRuntime`) and then registers relurpic/agent capabilities itself after receiving the `WorkspaceEnvironment`. It calls `StealClosers()` to take ownership of the log and database handles. The `Runtime` struct's public surface is unchanged.
