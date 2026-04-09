# dev-agent-cli: Euclo Completion Plan

## Objective

Transform `app/dev-agent-cli` from a generic, partially-wired agent runner into a
fully-functional euclo CLI tool. The current `start.go` manually reimplements a
subset of `ayenitd.Open()`, strips workspace-specific stores via `envToWorkspace`,
and leaves archaeology mode, BKC background services, and the full
`WorkspaceEnvironment` completely unavailable.

The end state is a CLI that has feature parity with the relurpish runtime path for
euclo execution, adds administrative commands for service and archaeology state
inspection, and has comprehensive unit test coverage.

## Reference Implementation

`app/relurpish/runtime/runtime.go` is the canonical euclo wiring reference. Every
wiring decision in this plan mirrors its approach. Key functions to reference
throughout:
- `runtime.New()` — `ayenitd.Open()` call, relurpic capability registration, euclo
  instantiation
- `wireRuntimeAgentDependencies()` — the exact set of nil-checks and store
  injections needed to fully wire a `*euclo.Agent`
- `RegisterBuiltinRelurpicCapabilitiesWithOptions()` — the full option set required
  for archaeology-aware capability registration

---

## Phase 1 — Replace Manual Bootstrap with `ayenitd.Open()`

**Goal:** Make `start.go` use `ayenitd.Open()` as the single composition root,
eliminating the manual `BootstrapAgentRuntime` assembly. This unlocks the full
`WorkspaceEnvironment` including all stores, the ServiceManager, and BKC background
services.

### 1.1 — Introduce `openWorkspaceFn` injectable

Add a package-level var in `start.go` alongside the existing injectable vars:

```go
var openWorkspaceFn = ayenitd.Open
```

This mirrors the existing pattern (`bootstrapAgentRuntimeFn`, `newLLMClientFn`,
etc.) and makes Phase 6 tests straightforward.

### 1.2 — Replace `BootstrapAgentRuntime` call

Replace the entire manual assembly block in `newStartCmd` (from `bootstrapAgentRuntimeFn`
through the `boot.*` field extractions) with a call to `openWorkspaceFn`. The
`ayenitd.WorkspaceConfig` fields map directly from what `start.go` already resolves:

```
Workspace        ← ensureWorkspace()
ManifestPath     ← runtimeCfg.ManifestPath
OllamaEndpoint   ← defaultEndpoint()
OllamaModel      ← modelName (resolved from spec / globalCfg)
LogPath          ← cfg paths (new --log flag, or derive from config.New(ws).LogsDir())
MemoryPath       ← paths.MemoryDir()
MaxIterations    ← 8 (or from spec)
HITLTimeout      ← runtimeCfg.HITLTimeout
AuditLimit       ← runtimeCfg.AuditLimit
Sandbox          ← resolved from flags / runtimeCfg
SkipASTIndex     ← new --skip-ast-index flag (default true for CLI, matching agenttest)
DebugLLM/Agent   ← from spec.Logging
```

`ayenitd.Open()` handles:
- telemetry setup, log file, permission event logger
- store initialization (WorkflowStore, PlanStore, PatternStore, CommentStore, KnowledgeStore)
- sandbox runner construction
- memory store + vector store
- BootstrapAgentRuntime internally
- compiled policy engine
- ServiceManager creation
- `bkc.workspace_bootstrap`, `bkc.invalidation`, `bkc.git_watcher`, `scheduler`
  all registered as services

### 1.3 — Service lifecycle around execution

After `openWorkspaceFn`, call `ws.ServiceManager.StartAll(ctx)` before agent
execution and `ws.Close()` in a deferred cleanup. This is a one-liner since
`Workspace.Close()` already calls `ServiceManager.Clear()` + store closes.

### 1.4 — HITL wiring

The existing interactive HITL handler (stdin scanner goroutine) and `--yes`
auto-approve path are preserved as-is. They reference `registration.HITL` which
is now obtained from `ws.Registration.HITL`.

### 1.5 — Remove redundant injectable vars

Once `openWorkspaceFn` is the composition root, these package-level vars in
`start.go` become dead code and should be removed:
- `bootstrapAgentRuntimeFn`
- `registerBuiltinProvidersFn`
- `newHybridMemoryFn`
- `newLLMClientFn`
- `newInstrumentedModelFn`
- `newLocalCommandRunnerFn`
- `newSandboxCommandRunnerFn`

`registerBuiltinRelurpicCapabilitiesFn` and `registerAgentCapabilitiesFn` are
**retained** — they are called after Open() in the same pattern as
`relurpish/runtime.New()`.

### 1.6 — New flags

Add the following flags to `newStartCmd` to expose `ayenitd.WorkspaceConfig` knobs
that had no CLI surface before:
- `--skip-ast-index` (bool, default `true`) — avoids expensive indexing for quick CLI
  runs; set `false` for dedicated end-to-end sessions
- `--log` (string) — override log file path
- `--events-log` (string) — optional SQLite event log (mirrors relurpish)
- `--telemetry` (string) — optional JSON telemetry file path

### Files changed
- `app/dev-agent-cli/start.go` — primary change
- `app/dev-agent-cli/cli_coverage_test.go` — update mocks (see Phase 6)

---

## Phase 2 — Full Euclo Agent Wiring

**Goal:** When the resolved agent is `coding`/`euclo`, wire the full
`WorkspaceEnvironment` into `*euclo.Agent` — matching what
`runtime.wireRuntimeAgentDependencies()` does — plus the LearningBroker and
ConvVerifier that currently only exist in the relurpish runtime path.

### 2.1 — Extract `buildAndWireEucloAgent` into a new file

Create `app/dev-agent-cli/euclo_wiring.go`. This keeps the euclo-specific
dependency surface isolated from the general `start.go` logic.

```go
// buildAndWireEucloAgent constructs a *euclo.Agent from the full workspace
// environment and wires all dependency stores that the factory envToWorkspace
// shim cannot carry. Mirrors runtime.wireRuntimeAgentDependencies.
func buildAndWireEucloAgent(
    ws *ayenitd.Workspace,
    learningBroker *archaeolearning.Broker,
) *euclo.Agent {
    agent := euclo.New(ws.Environment)
    env := ws.Environment
    if agent.RetrievalDB == nil {
        if sqlStore, ok := env.WorkflowStore.(*memorydb.SQLiteWorkflowStateStore); ok {
            agent.RetrievalDB = sqlStore.DB()
        }
    }
    if agent.ConvVerifier == nil && env.PatternStore != nil {
        var td relurpic.TensionDetector
        if env.WorkflowStore != nil {
            td = archaeotensions.Service{Store: env.WorkflowStore}
        }
        agent.ConvVerifier = &relurpic.PatternCoherenceVerifier{
            PatternStore:    env.PatternStore,
            TensionDetector: td,
        }
    }
    if agent.LearningBroker == nil && learningBroker != nil {
        agent.LearningBroker = learningBroker
    }
    if agent.DeferralPolicy.MaxBlastRadiusForDefer == 0 {
        agent.DeferralPolicy = guidance.DefaultDeferralPolicy()
    }
    return agent
}
```

### 2.2 — LearningBroker construction

Create `archaeolearning.NewBroker(0)` after `openWorkspaceFn` returns — exactly
as `relurpish/runtime.New()` does (line 225). Pass it to `buildAndWireEucloAgent`
and also to `RegisterBuiltinRelurpicCapabilitiesWithOptions` (not currently
passed there from dev-agent-cli — check whether it should be via a future
`WithLearningBroker` option or direct wiring on the agent).

### 2.3 — Relurpic capability registration with full options

Update the `registerBuiltinRelurpicCapabilitiesFn` call in `start.go` to use the
full option set (matching `relurpish/runtime.go:234–249`):

```go
agents.RegisterBuiltinRelurpicCapabilitiesWithOptions(
    env.Registry, env.Model, env.Config,
    agents.WithIndexManager(env.IndexManager),
    agents.WithGraphDB(graphDBFromEnv(env)),
    agents.WithPatternStore(env.PatternStore),
    agents.WithCommentStore(env.CommentStore),
    agents.WithRetrievalDB(retrievalDBFromEnv(env)),
    agents.WithPlanStore(env.PlanStore),
    agents.WithGuidanceBroker(env.GuidanceBroker),
    agents.WithWorkflowStore(env.WorkflowStore),
)
```

Helper functions `graphDBFromEnv` and `retrievalDBFromEnv` are small private
helpers analogous to `graphDBFromIndexManager` in the runtime package.

### 2.4 — Agent selection in `start.go`

Replace the single `buildFromSpecFn(env, *spec)` call with a two-path dispatch:

```go
if isEucloAgent(spec) {
    agent = buildAndWireEucloAgent(ws, learningBroker)
} else {
    agent, err = buildFromSpecFn(agentEnv, *spec)
    // existing fallback to react
}
```

`isEucloAgent` checks `strings.ToLower(spec.Implementation) == "coding"` or
`agentName == "euclo"`. This eliminates the `envToWorkspace` shim entirely for
the euclo case.

### 2.5 — Compile policy and registry wire-up

After `openWorkspaceFn`, the compiled policy comes from `ws.CompiledPolicy`.
Wire it the same way as `relurpish/runtime.New()`:
```go
registration.Policy = ws.CompiledPolicy.Engine
env.Registry.SetPolicyEngine(ws.CompiledPolicy.Engine)
env.Registry.UseAgentSpec(registration.ID, env.Config.AgentSpec)
```

Currently `start.go` only does the first of these three.

### Files changed
- `app/dev-agent-cli/euclo_wiring.go` — new file
- `app/dev-agent-cli/start.go` — dispatch update, relurpic options, policy wire-up

---

## Phase 3 — Euclo Mode Surface and Output

**Goal:** Expose euclo's mode system properly at the CLI level — validate modes
against `euclotypes.DefaultModeRegistry()`, output euclo-specific result fields,
and produce useful exit information for scripting.

### 3.1 — Mode validation at startup

After building the euclo agent and before executing the task, validate that the
requested `--mode` value is a registered euclo mode:

```go
if !agent.ModeRegistry.IsRegistered(core.AgentMode(mode)) {
    return fmt.Errorf("unknown euclo mode %q; valid modes: %s",
        mode, strings.Join(agent.ModeRegistry.Names(), ", "))
}
```

This surfaces a clear error instead of silently falling back to the default mode
or panicking mid-execution.

### 3.2 — Mode auto-detection hint

If `--mode` is not set and no default is found in the spec, print a hint listing
available modes rather than using "default":

```
Agent euclo ready. Available modes: code, debug, review, planning, chat, archaeology
Use --mode <name> or set spec.agent.mode in the manifest.
```

### 3.3 — Structured euclo result output

Replace the generic `"Agent complete (node=%s): %+v\n"` output with euclo-aware
fields extracted from the result context state:

```go
mode        := state.GetString("euclo.mode_resolution.resolved_mode")
artifacts   := state.GetString("euclo.artifacts")   // summary
recording   := state.GetBool("euclo.interaction_recording.recorded")
fmt.Fprintf(stdout, "euclo complete · mode=%s · artifacts=%s · recorded=%v\n",
    mode, artifacts, recording)
```

Add a `--json` flag to `start` that emits a machine-readable JSON summary
(task ID, mode, result node, elapsed, artifact paths) for integration with
scripts and CI pipelines.

### 3.4 — Signal handling

Wrap the `agent.Execute` call with a context that is cancelled on `SIGINT`/`SIGTERM`.
Currently there is a 10-minute `context.WithTimeout` but no signal handling; a
user pressing Ctrl-C leaves services and stores in an undefined cleanup state.

```go
ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
defer cancel()
ctx, timeoutCancel := context.WithTimeout(ctx, 10*time.Minute)
defer timeoutCancel()
```

`ws.Close()` already deferred above ensures clean store shutdown on any exit path.

### Files changed
- `app/dev-agent-cli/start.go` — mode validation, output, signal handling
- `app/dev-agent-cli/euclo_wiring.go` — mode validation helper

---

## Phase 4 — Service and Workspace Inspection Commands

**Goal:** Expose the `ayenitd.ServiceManager` and workspace probe results as CLI
subcommands. These are administrative/observability tools that use `ayenitd.Open()`
under the hood but do not execute any agent task.

### 4.1 — `dev-agent workspace` subcommand group

New file: `app/dev-agent-cli/workspace.go`

```
dev-agent workspace probe      -- run ProbeWorkspace checks and report
dev-agent workspace status     -- show workspace config summary (paths, model, manifest)
dev-agent workspace services   -- list registered services and their health
```

**`probe`** calls `ayenitd.ProbeWorkspace(cfg)` and prints a table of check names,
OK/FAIL status, and messages. Exits non-zero if any required check fails.

**`status`** prints the resolved `WorkspaceConfig` fields (workspace path, manifest,
model, agent name, log paths) without opening stores — useful for diagnosing
misconfiguration without a full Open() call.

**`services`** calls `openWorkspaceFn` + `sm.StartAll` + `ws.ListServices()` and
prints the service IDs. Extends to show health via a `HealthSnapshot`-like
interface if services implement it.

### 4.2 — `dev-agent service` subcommand group

New file: `app/dev-agent-cli/service.go`

```
dev-agent service list         -- list registered service IDs
dev-agent service restart <id> -- restart a specific service by ID
```

These require a running workspace session, so they open one (via `openWorkspaceFn`),
interact with the ServiceManager, then close it. Useful for diagnosing stalled
indexing or forcing a BKC invalidation pass from the CLI.

### 4.3 — `dev-agent workspace init`

Scaffold a minimal `relurpify.yaml` workspace config if none exists:

```
dev-agent workspace init [--model <name>] [--agent <name>]
```

Writes `relurpify_cfg/relurpify.yaml` with sensible defaults. Prevents the common
"workspace is required" / "ManifestPath is required" errors for new users.

### Files changed
- `app/dev-agent-cli/workspace.go` — new file
- `app/dev-agent-cli/service.go` — new file
- `app/dev-agent-cli/root.go` — register the two new subcommand groups

---

## Phase 5 — Archaeology CLI Inspection Commands

**Goal:** Surface the archaeo domain (tensions, plans, request history, learning
queue) as read-only CLI inspection commands. These consume `WorkflowStore`,
`PlanStore`, `RetrievalDB`, and the archaeo projection layer — all of which are
only available after Phase 1 and 2.

### 5.1 — `dev-agent archaeo` subcommand group

New file: `app/dev-agent-cli/archaeo.go`

All subcommands accept `--workflow <id>` to scope to a specific workflow, and
`--json` for machine-readable output.

```
dev-agent archaeo plan      [--workflow <id>]  -- active plan + step status
dev-agent archaeo tensions  [--workflow <id>]  -- list tensions (kind, severity, status)
dev-agent archaeo history   [--workflow <id>]  -- request history projection
dev-agent archaeo learning  [--workflow <id>]  -- learning queue (pending, blocking)
dev-agent archaeo workflows                    -- list all known workflow IDs
```

### 5.2 — Implementation approach

Each subcommand:
1. Calls `openWorkspaceFn` (same config resolution as `start`)
2. Calls `ws.ServiceManager.StartAll(ctx)` to ensure stores are initialized
3. Accesses the relevant store directly (WorkflowStore for tensions/history,
   PlanStore for plan, etc.) through the archaeo projection layer
4. Prints results as formatted text or JSON
5. Defers `ws.Close()`

Reuse the view types already defined in `named/euclo/archaeo_access.go`
(`TensionView`, `ActivePlanView`, `RequestHistoryView`, etc.) — they are already
the right level of abstraction for CLI output.

### 5.3 — `dev-agent archaeo workflows`

Uses `WorkflowStore.ListWorkflows(ctx)` (or equivalent) to enumerate known
workflow IDs with their creation time and last-updated time. Enables users to
find the workflow ID to pass to other subcommands.

### 5.4 — Shared open helper

Extract a `openWorkspaceForInspection(ctx, ws string) (*ayenitd.Workspace, error)`
helper used by both Phase 4 and Phase 5 commands. It calls `openWorkspaceFn`
with `SkipASTIndex: true` so inspection commands don't trigger a full index pass.

### Files changed
- `app/dev-agent-cli/archaeo.go` — new file
- `app/dev-agent-cli/workspace.go` — `openWorkspaceForInspection` helper
- `app/dev-agent-cli/root.go` — register `archaeo` subcommand group

---

## Phase 6 — Tests and Testsuite Coverage

**Goal:** Comprehensive unit tests for all new and modified code in dev-agent-cli,
plus testsuite additions for euclo archaeology mode. This is the final phase
because it depends on stable APIs from Phases 1–5.

### 6.1 — Update existing mocks in `start.go` / `cli_coverage_test.go`

The existing `bootstrapAgentRuntimeFn` mock is removed in Phase 1. Replace all
tests that inject it with `openWorkspaceFn` injection:

```go
var capturedOpenCfg ayenitd.WorkspaceConfig

openWorkspaceFn = func(ctx context.Context, cfg ayenitd.WorkspaceConfig) (*ayenitd.Workspace, error) {
    capturedOpenCfg = cfg
    return stubWorkspace(t), nil
}
```

Provide a `stubWorkspace(t)` constructor (in a new `testutil_test.go` within the
package) that returns a minimal `*ayenitd.Workspace` with in-memory stores and a
no-op ServiceManager. This replaces the ~20 lines of mock wiring currently spread
across `cli_coverage_test.go`.

### 6.2 — `start.go` tests (`start_test.go`)

New file covering the refactored `newStartCmd`:

| Test | What it checks |
|---|---|
| `TestStartPassesWorkspaceToOpen` | Verifies the `WorkspaceConfig` fields sent to `openWorkspaceFn` match CLI flags |
| `TestStartWithEucloAgentUsesWiring` | When `implementation: coding`, verifies `buildAndWireEucloAgent` is called (not factory) |
| `TestStartServicesStartedBeforeExecute` | Verifies `ServiceManager.StartAll` is called before agent.Execute |
| `TestStartServicesStoppedOnExit` | Verifies `ws.Close()` is called even when agent returns error |
| `TestStartSignalCancelsContext` | Sends SIGINT, verifies ctx is cancelled and ws.Close() runs |
| `TestStartModeValidation` | Passing an unknown `--mode` returns an error before execute |
| `TestStartAutoApprovePolicy` | `--yes` sets default policy to Allow |
| `TestStartJSONOutput` | `--json` flag produces parseable JSON result |
| `TestStartDryRunSkipsOpen` | `--dry-run` skips `openWorkspaceFn` entirely |
| `TestStartNoInstructionSkipsOpen` | Missing `--instruction` exits early with a hint, no Open() call |

### 6.3 — `euclo_wiring.go` tests (`euclo_wiring_test.go`)

New file:

| Test | What it checks |
|---|---|
| `TestBuildAndWireEucloAgent_RetrievalDB` | RetrievalDB is wired from WorkflowStore.DB() when nil |
| `TestBuildAndWireEucloAgent_ConvVerifier` | ConvVerifier is constructed when PatternStore is present |
| `TestBuildAndWireEucloAgent_LearningBroker` | LearningBroker is injected when non-nil |
| `TestBuildAndWireEucloAgent_DeferralPolicy` | DefaultDeferralPolicy set when zero |
| `TestBuildAndWireEucloAgent_NilWorkflowStore` | No panic when WorkflowStore is nil |
| `TestBuildAndWireEucloAgent_NilPatternStore` | ConvVerifier is nil when PatternStore is nil |

### 6.4 — `workspace.go` tests (`workspace_test.go`)

| Test | What it checks |
|---|---|
| `TestWorkspaceProbeReturnsCheckResults` | `probe` calls ProbeWorkspace and prints results |
| `TestWorkspaceProbeExitsNonZeroOnRequiredFail` | Required check failure → non-zero exit |
| `TestWorkspaceServicesListsIDs` | `services` opens workspace and prints ServiceManager IDs |
| `TestWorkspaceInitCreatesConfig` | `init` writes relurpify.yaml in a temp workspace |
| `TestWorkspaceInitIdempotent` | Second `init` call doesn't overwrite existing config |

### 6.5 — `service.go` tests (`service_test.go`)

| Test | What it checks |
|---|---|
| `TestServiceListPrintsIDs` | Lists service IDs from workspace ServiceManager |
| `TestServiceRestartCallsRestart` | Calls `ws.Restart(ctx)` for matching service |
| `TestServiceRestartUnknownID` | Returns error for unknown service name |

### 6.6 — `archaeo.go` tests (`archaeo_test.go`)

Uses stub stores implementing `memory.WorkflowStateStore` and `plan.PlanStore`.

| Test | What it checks |
|---|---|
| `TestArchaeoPlanPrintsActiveStepID` | plan command extracts and prints active step |
| `TestArchaeoPlanNoWorkflowID` | Omitting `--workflow` returns an error |
| `TestArchaeoTensionsPrintsKindSeverity` | tensions command prints TensionView fields |
| `TestArchaeoTensionsEmptyList` | No tensions → prints "no tensions" message |
| `TestArchaeoHistoryPrintsRequestCounts` | history shows pending/running/completed counts |
| `TestArchaeoLearningPrintsPendingQueue` | learning command prints pending interaction IDs |
| `TestArchaeoWorkflowsListsIDs` | workflows command lists all workflow IDs |
| `TestArchaeoJSONFlag` | `--json` on any subcommand produces valid JSON |
| `TestArchaeoOpenWorkspaceWithSkipASTIndex` | Inspection open uses SkipASTIndex=true |

### 6.7 — Testsuite additions (`testsuite/agenttests/`)

Add two new YAML testsuite files that exercise the euclo agent through the
agenttest runner (which already uses the full ayenitd bootstrap path):

**`euclo-archaeology-smoke.testsuite.yaml`** — `tier: smoke`
- Case: `archaeology-explore` — requests archaeology mode on a small codebase
  fixture, asserts the agent completes without error and produces an artifact
- Case: `archaeology-plan-projection` — verifies a plan version is created and
  the active plan projection is non-empty
- Case: `archaeology-tension-detect` — introduces a known pattern inconsistency
  and asserts at least one tension is surfaced

**`euclo-modes-smoke.testsuite.yaml`** — `tier: smoke`
- Cases for each non-default euclo mode (`code`, `debug`, `review`, `planning`,
  `chat`) to verify mode dispatch reaches the correct interaction handler without
  crashing. Assertions are permissive (`must_succeed: true` only — no content
  assertion) since these are smoke tests.

### 6.8 — Testsuite impact notes

The existing `testsuite/agenttest` package drives agent execution via
`ayenitd.BootstrapAgentRuntime` internally (not via dev-agent-cli as a subprocess).
The Phase 1–2 changes to dev-agent-cli **do not break** the agenttest bootstrap
path. However:

- The `agenttest.Runner.RunSuite` path should be audited to confirm it calls
  `RegisterBuiltinRelurpicCapabilitiesWithOptions` with the same full option set
  that Phases 1–2 introduce for dev-agent-cli. If it doesn't, archaeology-mode
  testsuite cases will fail at the capability layer, not in agent logic.
- `skill.go:newSkillTestCmd` calls `newAgentTestRunnerFn()` directly; this is
  unaffected by the `openWorkspaceFn` refactor.

### Files changed / created in Phase 6

| File | Action |
|---|---|
| `app/dev-agent-cli/start_test.go` | new |
| `app/dev-agent-cli/euclo_wiring_test.go` | new |
| `app/dev-agent-cli/workspace_test.go` | new |
| `app/dev-agent-cli/service_test.go` | new |
| `app/dev-agent-cli/archaeo_test.go` | new |
| `app/dev-agent-cli/testutil_test.go` | new (stubWorkspace + stub stores) |
| `app/dev-agent-cli/cli_coverage_test.go` | update (swap bootstrapAgentRuntimeFn → openWorkspaceFn mock) |
| `testsuite/agenttests/euclo-archaeology-smoke.testsuite.yaml` | new |
| `testsuite/agenttests/euclo-modes-smoke.testsuite.yaml` | new |

---

## Dependency Order

```
Phase 1 (Open())
  └─► Phase 2 (euclo wiring)
        └─► Phase 3 (mode surface + output)
              └─► Phase 4 (workspace/service commands)
                    └─► Phase 5 (archaeo commands)
                          └─► Phase 6 (tests — covers all prior phases)
```

Phases 4 and 5 share the `openWorkspaceForInspection` helper and can be
developed in parallel once Phase 2 is complete. Phase 6 is last because the
test APIs stabilize only after the implementation is complete.

## Risk Notes

- **`envToWorkspace` shim**: `named/factory/factory.go:envToWorkspace` is used by
  the `coding` and `rex` paths in `BuildFromSpec`. After Phase 2, dev-agent-cli
  bypasses it for euclo. The shim remains for other callers (agenttest, unit tests
  that call `BuildFromSpec` directly). Do not delete it.

- **Relurpish-only imports**: `app/relurpish/runtime` imports `app/nexus/db` for
  the SQLite event log. Dev-agent-cli should not import `app/relurpish/runtime`
  for wiring — it should replicate only what it needs directly. The event log
  extension in Phase 1 flags (`--events-log`) should use `app/nexus/db` directly,
  not via the relurpish runtime wrapper.

- **Circular import check**: Verify `app/dev-agent-cli` does not create a cycle
  by importing `named/euclo` (for `euclo_wiring.go`). Currently `start.go` does
  not import it directly; the factory does. The wiring file adds a direct import
  — run `go build ./app/dev-agent-cli/...` after Phase 2 to confirm no cycle.

- **Testsuite agenttest bootstrap path**: As noted in 6.8, if
  `agenttest.Runner.RunSuite` does not use the full relurpic option set, the new
  archaeology testsuites will fail. Audit and fix in Phase 6 before writing the
  suite YAML.
