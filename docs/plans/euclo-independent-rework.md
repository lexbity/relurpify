# Euclo Independent Rework — Engineering Specification

**Status:** Draft  
**Scope:** Three independent workstreams; no inter-dependency between them or with `euclo-runtime-rework.md`  
**Constraint:** Each phase must leave `go build ./...` and `go test ./...` green at its exit  
**Parallelism:** Workstreams A, B, and C may be developed concurrently by separate engineers

---

## Workstream A — Language Platform Lazy Construction

### Background

`named/euclo/agent.go` unconditionally imports all four language platform packages and allocates all eight resolver instances during `InitializeEnvironment`, regardless of what language the workspace actually uses:

```go
// In InitializeEnvironment — always runs, for every workspace:
a.Environment.VerificationPlanner = frameworkplan.NewVerificationScopePlanner(
    golangpkg.NewVerificationResolver(),
    pythonpkg.NewVerificationResolver(),
    rustpkg.NewVerificationResolver(),
    jspkg.NewVerificationResolver(),
)
a.Environment.CompatibilitySurfaceExtractor = frameworkplan.NewCompatibilitySurfacePlanner(
    golangpkg.NewCompatibilitySurfaceResolver(),
    pythonpkg.NewCompatibilitySurfaceResolver(),
    rustpkg.NewCompatibilitySurfaceResolver(),
    jspkg.NewCompatibilitySurfaceResolver(),
)
```

This has three costs. First, language resolvers may perform filesystem probes at construction time (checking for toolchain binaries, config files). On a machine running Ollama and a small model, these are unnecessary startup taxes. Second, all resolver allocations are live for the agent's lifetime even if unused. Third, the coupling means a new language platform package must be imported at the `named/euclo` level even if only one workspace in one workspace ever uses it — it cannot be conditionally included.

The compile-time imports (`import` declarations) remain in this workstream; removing them requires a plugin or interface-registration pattern that is out of scope. The goal is lazy *instance construction*: only allocate resolver instances for the language(s) actually present in the workspace.

---

### Phase A1 — Workspace Language Detection

**Goal:** Produce a `WorkspaceLanguages` value that describes which language(s) are present in a workspace directory, based on well-known indicator files. This is a pure filesystem probe with no LLM involvement and no framework dependencies.

**What to Build**

New package: `named/euclo/langdetect/`

```
langdetect/
  detect.go     — Detect(), WorkspaceLanguages
  indicators.go — per-language indicator file lists
  detect_test.go
```

**`WorkspaceLanguages` struct:**

```go
type WorkspaceLanguages struct {
    Go     bool
    Python bool
    Rust   bool
    JS     bool // JavaScript or TypeScript
}

// IsEmpty returns true when no language was detected.
func (w WorkspaceLanguages) IsEmpty() bool {
    return !w.Go && !w.Python && !w.Rust && !w.JS
}

// Detected returns the IDs of detected languages in stable order.
func (w WorkspaceLanguages) Detected() []string
```

**`Detect` function:**

```go
// Detect probes workspaceDir for language indicator files and returns
// a WorkspaceLanguages value. It does not recurse more than maxDepth
// directories (default 2). It never returns an error; a missing or
// unreadable directory returns an empty WorkspaceLanguages.
func Detect(workspaceDir string) WorkspaceLanguages
```

Indicator files checked:

| Language | Indicators (any match → detected) |
|----------|-----------------------------------|
| Go | `go.mod`, `*.go` (top 2 levels) |
| Python | `pyproject.toml`, `setup.py`, `setup.cfg`, `requirements.txt`, `*.py` (top 2 levels) |
| Rust | `Cargo.toml` |
| JS | `package.json`, `tsconfig.json`, `deno.json` |

Detection is ordered: check project root files first (fast path), then recurse shallowly only if the root check produces no match. The function caps filesystem reads at 200 entries total.

**File Dependencies**

Produces:
- `named/euclo/langdetect/detect.go`
- `named/euclo/langdetect/indicators.go`
- `named/euclo/langdetect/detect_test.go`

No modifications to existing files.

**Unit Tests**

- `Detect` on an empty directory returns `WorkspaceLanguages{}` with all fields false
- `Detect` on a directory containing only `go.mod` returns `{Go: true}`
- `Detect` on a directory containing `package.json` returns `{JS: true}`
- `Detect` on a directory containing `go.mod` and `Cargo.toml` returns `{Go: true, Rust: true}`
- `Detect` on a directory containing a `*.go` file in a subdirectory (max depth) returns `{Go: true}`
- `Detect` with a non-existent path returns the zero value and does not panic
- `WorkspaceLanguages.Detected()` returns a stable sorted slice
- `WorkspaceLanguages.IsEmpty()` is true iff all fields are false

**Exit Criteria**

1. `go build ./...` and `go test ./...` pass.
2. `named/euclo/langdetect` has >90% statement coverage.
3. `Detect` completes in under 10ms on a real workspace directory (benchmark test).

---

### Phase A2 — Lazy Resolver Factory

**Goal:** Replace the unconditional four-resolver construction with a factory that only constructs resolvers for detected languages. The factory reads `WorkspaceLanguages` and returns a `frameworkplan.VerificationScopePlanner` and a `frameworkplan.CompatibilitySurfacePlanner` populated with only the relevant resolvers.

**What to Build**

New file: `named/euclo/langdetect/resolver_factory.go`

```go
// ResolverFactory builds framework planners from detected workspace languages.
// It is the single place in named/euclo that imports platform/lang/* packages.
type ResolverFactory struct {
    Languages WorkspaceLanguages
}

// VerificationPlanner builds a VerificationScopePlanner for the detected languages.
// If no language is detected, falls back to all languages (safe default).
func (f ResolverFactory) VerificationPlanner() *frameworkplan.VerificationScopePlanner

// CompatibilitySurfacePlanner builds a CompatibilitySurfacePlanner for the
// detected languages. Falls back to all languages if none detected.
func (f ResolverFactory) CompatibilitySurfacePlanner() *frameworkplan.CompatibilitySurfacePlanner
```

The factory methods use guarded construction:

```go
func (f ResolverFactory) VerificationPlanner() *frameworkplan.VerificationScopePlanner {
    langs := f.Languages
    if langs.IsEmpty() {
        langs = WorkspaceLanguages{Go: true, Python: true, Rust: true, JS: true}
    }
    var resolvers []frameworkplan.VerificationResolver
    if langs.Go {
        resolvers = append(resolvers, golangpkg.NewVerificationResolver())
    }
    if langs.Python {
        resolvers = append(resolvers, pythonpkg.NewVerificationResolver())
    }
    if langs.Rust {
        resolvers = append(resolvers, rustpkg.NewVerificationResolver())
    }
    if langs.JS {
        resolvers = append(resolvers, jspkg.NewVerificationResolver())
    }
    return frameworkplan.NewVerificationScopePlanner(resolvers...)
}
```

The platform/lang/* imports move from `agent.go` to `resolver_factory.go`. `agent.go` no longer imports them directly.

**File Dependencies**

Produces:
- `named/euclo/langdetect/resolver_factory.go`
- `named/euclo/langdetect/resolver_factory_test.go`

Modified:
- `named/euclo/agent.go` — removes `golangpkg`, `jspkg`, `pythonpkg`, `rustpkg` import declarations; uses `langdetect.ResolverFactory`

**Unit Tests**

- `ResolverFactory{Languages: WorkspaceLanguages{Go: true}}.VerificationPlanner()` returns a non-nil planner
- `ResolverFactory{Languages: WorkspaceLanguages{Go: true}}.CompatibilitySurfacePlanner()` returns a non-nil planner
- `ResolverFactory{Languages: WorkspaceLanguages{}}.VerificationPlanner()` falls back to all resolvers (non-nil result)
- `ResolverFactory{Languages: WorkspaceLanguages{JS: true}}.VerificationPlanner()` does not allocate a Go resolver (verified via a mock resolver counter)
- Factory does not call `golangpkg.NewVerificationResolver()` when `Languages.Go = false` (verified by replacing with a panicking stub in test)

**Exit Criteria**

1. `go build ./...` and `go test ./...` pass.
2. `named/euclo/agent.go` no longer contains import declarations for `platform/lang/go`, `platform/lang/js`, `platform/lang/python`, `platform/lang/rust`. (CI grep check added to `scripts/check-agent-lang-imports.sh`.)
3. All four language imports now appear only in `named/euclo/langdetect/resolver_factory.go`.

---

### Phase A3 — Agent Wiring Update

**Goal:** Wire the lazy resolver factory into `InitializeEnvironment`. Detection is triggered by the workspace path from the `WorkspaceEnvironment`. Construction only happens when the planner field is nil (preserving the existing guard pattern).

**What to Build**

Modified `InitializeEnvironment` in `named/euclo/agent.go`:

```go
// Before (current):
if a.Environment.VerificationPlanner == nil {
    a.Environment.VerificationPlanner = frameworkplan.NewVerificationScopePlanner(
        golangpkg.NewVerificationResolver(),
        pythonpkg.NewVerificationResolver(),
        rustpkg.NewVerificationResolver(),
        jspkg.NewVerificationResolver(),
    )
}

// After:
if a.Environment.VerificationPlanner == nil {
    workspace := strings.TrimSpace(fmt.Sprint(env.Config.GetString("workspace")))
    detected := langdetect.Detect(workspace)
    factory := langdetect.ResolverFactory{Languages: detected}
    a.Environment.VerificationPlanner = factory.VerificationPlanner()
}
if a.Environment.CompatibilitySurfaceExtractor == nil {
    // factory already constructed above; re-use
    a.Environment.CompatibilitySurfaceExtractor = factory.CompatibilitySurfacePlanner()
}
```

The workspace path resolution: `InitializeEnvironment` receives `ayenitd.WorkspaceEnvironment`. The workspace path is either in `env.Config` or can be extracted from the working directory if not set. Add a `workspacePathFromEnv(env ayenitd.WorkspaceEnvironment) string` helper that checks `env.Config` first, then falls back to `os.Getwd()`.

**Detection caching:** `Detect` is cheap (<10ms by Phase A1 criteria) so caching is not needed. If the workspace changes between calls (unusual), the next `InitializeEnvironment` will re-detect.

**File Dependencies**

Modified:
- `named/euclo/agent.go` — `InitializeEnvironment` updated; `workspacePathFromEnv` helper added

**Unit Tests**

- `InitializeEnvironment` with a mock `WorkspaceEnvironment` pointing to a Go-only temp directory allocates a non-nil `VerificationPlanner`
- `InitializeEnvironment` with an environment that already has `VerificationPlanner` set does not call `Detect` (detection is only triggered when the field is nil)
- `InitializeEnvironment` with an empty workspace path falls back to the four-language default
- After `InitializeEnvironment`, `a.Environment.VerificationPlanner` and `a.Environment.CompatibilitySurfaceExtractor` are both non-nil

**Exit Criteria**

1. `go build ./...` and `go test ./...` pass.
2. A Go-only workspace allocates exactly 1 verification resolver and 1 compatibility resolver (not 4+4).
3. An empty workspace allocates 4 verification resolvers and 4 compatibility resolvers (safe default).
4. `scripts/check-agent-lang-imports.sh` still passes (imports remain only in `resolver_factory.go`).

---

## Workstream B — Deferred Issue Lifecycle Completion

### Background

The deferral system is structurally complete at the data layer but has an incomplete lifecycle. Issues are accumulated, converted to typed structs, and persisted as markdown artifacts. The return path — user sees, decides, resolves — is entirely missing.

Current system state:

```
GuidanceBroker.Request() → timeout with GuidanceTimeoutDefer
  → handleTimeout() → recordDeferredObservation() → DeferralPlan.AddObservation()

refreshRuntimeExecutionArtifacts()
  → BuildDeferredExecutionIssues(DeferralPlan, ...) → []DeferredExecutionIssue
  → PersistDeferredExecutionIssuesToWorkspace() → .md files in relurpify_cfg/artifacts/euclo/deferred/
  → SeedDeferredIssueState() → "euclo.deferred_execution_issues" in state
```

Missing:

1. **Surface path**: no capability to list open deferrals to the user in a session
2. **Resolution path**: `DeferralPlan.ResolveObservation(id)` exists but nothing in the execution layer calls it; persisted markdown files are never updated to `resolved`
3. **Re-entry context**: starting a new session does not load prior deferred issues from disk; the next execution has no awareness of what was deferred in prior runs
4. **Next-action suggestion**: the final report mentions deferrals exist but doesn't surface a concrete actionable prompt for each one

The `guidance.GuidanceBroker.Subscribe()` event channel exists, the `DeferralPlan.PendingObservations()` method exists, and `DeferralPlan.ResolveObservation()` exists — these are the right primitives. The gap is the euclo-layer plumbing between them.

---

### Phase B1 — Deferral Surface Capability

**Goal:** A new euclo capability `euclo:deferrals.surface` that, when invoked (primarily in `chat` or `planning` mode), reads the persisted deferred issues for the current workflow and formats them for the user. This closes the observability gap: the user can ask "what's deferred?" and get a structured answer.

**What to Build**

New file: `named/euclo/relurpicabilities/local/deferrals_surface.go`

```go
// DeferralsSurfaceRoutine is a SupportingRoutine that loads persisted
// deferred issues for the current workflow and writes a structured summary
// to state. It does not mutate any deferred issue status.
type DeferralsSurfaceRoutine struct{}

func (r *DeferralsSurfaceRoutine) ID() string {
    return "euclo:deferrals.surface"
}

func (r *DeferralsSurfaceRoutine) Execute(ctx context.Context, in RoutineInput) ([]euclotypes.Artifact, error)
```

The routine:
1. Reads `euclo.deferred_execution_issues` from state (already populated by `SeedDeferredIssueState`)
2. Falls back to loading from the workspace artifact directory (`relurpify_cfg/artifacts/euclo/deferred/*.md`) if state is empty
3. Groups issues by `Kind` and `Severity`
4. Writes to state as `euclo.deferrals_surface` — a typed `DeferralsSurfaceSummary` struct
5. Produces a `euclotypes.Artifact` with kind `euclo.deferrals_surface`

New type in `named/euclo/runtime/` (or `euclotypes`):

```go
type DeferralsSurfaceSummary struct {
    TotalOpen   int                          `json:"total_open"`
    BySeverity  map[string]int               `json:"by_severity"`
    ByKind      map[string]int               `json:"by_kind"`
    Issues      []DeferredIssueSummaryEntry  `json:"issues"`
    WorkflowID  string                       `json:"workflow_id"`
}

type DeferredIssueSummaryEntry struct {
    IssueID             string `json:"issue_id"`
    Kind                string `json:"kind"`
    Severity            string `json:"severity"`
    Title               string `json:"title"`
    RecommendedNext     string `json:"recommended_next_action"`
    WorkspaceArtifactPath string `json:"workspace_artifact_path,omitempty"`
}
```

New loader in `named/euclo/runtime/deferrals.go`:

```go
// LoadDeferredIssuesFromWorkspace reads all persisted deferred issue markdown
// files from workspaceDir and returns the parsed issues. Malformed files are
// skipped with a warning; they never cause an error return.
func LoadDeferredIssuesFromWorkspace(workspaceDir string) []DeferredExecutionIssue
```

This function parses the YAML frontmatter from the markdown files written by `renderDeferredIssueMarkdown`. It uses only the frontmatter fields (not the body), which is sufficient for surface purposes.

**Registration:**

Add `DeferralsSurfaceRoutine` to the supporting routines registered in the dispatcher. Since this is a supporting routine, it is registered in `NewDispatcher` in `runtime/dispatch/dispatcher.go` alongside the existing `SupportingRoutines` calls.

**File Dependencies**

Produces:
- `named/euclo/relurpicabilities/local/deferrals_surface.go`
- `named/euclo/relurpicabilities/local/deferrals_surface_test.go`

Modified:
- `named/euclo/runtime/deferrals.go` — adds `LoadDeferredIssuesFromWorkspace`, `DeferralsSurfaceSummary`, `DeferredIssueSummaryEntry`
- `named/euclo/runtime/dispatch/dispatcher.go` — registers `DeferralsSurfaceRoutine`

**Unit Tests**

- `DeferralsSurfaceRoutine.Execute` with empty state and no workspace directory returns an artifact with `TotalOpen = 0`
- `DeferralsSurfaceRoutine.Execute` with two issues in state returns a `DeferralsSurfaceSummary` with `TotalOpen = 2`
- `DeferralsSurfaceRoutine.Execute` produces one artifact with kind `euclo.deferrals_surface`
- `LoadDeferredIssuesFromWorkspace` with a directory containing three well-formed markdown files returns three `DeferredExecutionIssue` values with correct field mappings
- `LoadDeferredIssuesFromWorkspace` with a malformed markdown file in the directory skips that file and returns the remaining valid ones
- `LoadDeferredIssuesFromWorkspace` with a non-existent directory returns nil without panicking
- `DeferralsSurfaceSummary.BySeverity` correctly groups issues by severity string
- `DeferralsSurfaceSummary.ByKind` correctly groups issues by kind string

**Exit Criteria**

1. `go build ./...` and `go test ./...` pass.
2. `euclo:deferrals.surface` is registered in the Dispatcher and reachable via `DirectCapabilityRun`.
3. `LoadDeferredIssuesFromWorkspace` round-trips correctly: a `DeferredExecutionIssue` written by `PersistDeferredExecutionIssuesToWorkspace` is correctly parsed back by `LoadDeferredIssuesFromWorkspace` (round-trip test).
4. `named/euclo/relurpicabilities/local/deferrals_surface.go` has >85% statement coverage.

---

### Phase B2 — Deferral Resolution Path

**Goal:** Complete the loop from deferred issue → user decision → resolved status. This requires: (a) a `euclo:deferrals.resolve` capability that accepts an issue ID and a resolution choice; (b) updating the persisted markdown file to mark the issue resolved; (c) calling `DeferralPlan.ResolveObservation(id)` so the in-memory plan reflects the resolution; (d) propagating the resolution to the `GuidanceBroker` event stream so subscribers (TUI, nexus) can observe it.

**What to Build**

New file: `named/euclo/relurpicabilities/local/deferrals_resolve.go`

```go
type DeferralsResolveRoutine struct {
    DeferralPlan   *guidance.DeferralPlan
    GuidanceBroker *guidance.GuidanceBroker
}

func (r *DeferralsResolveRoutine) ID() string { return "euclo:deferrals.resolve" }

func (r *DeferralsResolveRoutine) Execute(ctx context.Context, in RoutineInput) ([]euclotypes.Artifact, error)
```

Resolution input is read from state at key `"euclo.deferral_resolve_input"` (a typed struct):

```go
// DeferralResolveInput is written to state before invoking this routine.
type DeferralResolveInput struct {
    IssueID    string `json:"issue_id"`
    Resolution string `json:"resolution"` // "accept" | "reject" | "defer_again" | "escalate"
    Note       string `json:"note,omitempty"`
}
```

The routine:
1. Reads `DeferralResolveInput` from state
2. Validates `IssueID` is non-empty and `Resolution` is a known value
3. Calls `r.DeferralPlan.ResolveObservation(input.IssueID)` to mark in-memory
4. Loads the workspace artifact file for the issue (via `LoadDeferredIssuesFromWorkspace`)
5. Rewrites the markdown file with `status: resolved` in the YAML frontmatter, appending a resolution section with the note
6. Emits a `GuidanceEvent{Type: GuidanceEventResolved, ...}` via the broker's broadcast mechanism (if broker is non-nil)
7. Writes a `euclo.deferral_resolved` artifact to state
8. Updates `euclo.deferred_execution_issues` in state to mark the resolved issue

**Markdown resolution update:**

`renderDeferredIssueMarkdown` currently writes static frontmatter. Add `RewriteDeferredIssueMarkdown(path string, resolution DeferralResolveInput) error` in `runtime/deferrals.go` that:
- Reads the file
- Parses the YAML frontmatter
- Updates `status:` to `"resolved"`
- Appends a `## Resolution` section at the end of the file body
- Writes back atomically (write to `.tmp`, then rename)

**`GuidanceBroker` broadcast access:**

`GuidanceBroker.broadcast()` is currently unexported. Add a narrow exported method:

```go
// EmitResolution broadcasts a GuidanceEventResolved event for an observation
// that was resolved externally (not through the Request/Resolve flow).
// This is used when a deferred issue is resolved via the execution layer
// rather than through a live guidance request.
func (g *GuidanceBroker) EmitResolution(observationID, resolvedBy string)
```

This addition is in `framework/guidance/broker.go`. It is a minimal surface extension — it does not expose the internal `requests` map or the `waiters` map.

**Registration:**

`DeferralsResolveRoutine` requires `DeferralPlan` and `GuidanceBroker` fields that are on the `Agent` struct. Registration happens in `agent.go` via the dispatcher setup, where the agent injects its fields:

```go
// In agent.go, where the Dispatcher is constructed:
a.BehaviorDispatcher = euclodispatch.NewDispatcher(a.Environment)
a.BehaviorDispatcher.RegisterSupporting(&local.DeferralsResolveRoutine{
    DeferralPlan:   a.DeferralPlan,
    GuidanceBroker: a.GuidanceBroker,
})
```

This requires adding `RegisterSupporting(routine euclorelurpic.SupportingRoutine)` to the `Dispatcher` API.

**File Dependencies**

Produces:
- `named/euclo/relurpicabilities/local/deferrals_resolve.go`
- `named/euclo/relurpicabilities/local/deferrals_resolve_test.go`

Modified:
- `named/euclo/runtime/deferrals.go` — adds `RewriteDeferredIssueMarkdown`, `DeferralResolveInput`
- `framework/guidance/broker.go` — adds `EmitResolution`
- `named/euclo/runtime/dispatch/dispatcher.go` — adds `RegisterSupporting`
- `named/euclo/agent.go` — registers `DeferralsResolveRoutine` at dispatcher setup

**Unit Tests**

- `DeferralsResolveRoutine.Execute` with a valid `DeferralResolveInput` calls `DeferralPlan.ResolveObservation`
- `DeferralsResolveRoutine.Execute` with a valid input and a persisted markdown file rewrites the file with `status: resolved`
- `DeferralsResolveRoutine.Execute` with an unknown `IssueID` returns an error (issue not found)
- `DeferralsResolveRoutine.Execute` with an invalid `Resolution` value returns a validation error
- `DeferralsResolveRoutine.Execute` with `GuidanceBroker = nil` completes without error (broker is optional)
- `RewriteDeferredIssueMarkdown` updates `status:` in frontmatter and appends a `## Resolution` section
- `RewriteDeferredIssueMarkdown` is atomic: if write fails midway, the original file is not corrupted (test using a read-only target directory)
- `GuidanceBroker.EmitResolution` broadcasts a `GuidanceEventResolved` event to all subscribers
- `GuidanceBroker.EmitResolution` with no subscribers completes without blocking
- `Dispatcher.RegisterSupporting` adds the routine to the routines map; subsequent `ExecuteRoutine` calls reach it

**Exit Criteria**

1. `go build ./...` and `go test ./...` pass.
2. `DeferralPlan.ResolveObservation` is called during `DeferralsResolveRoutine.Execute` (verified by test mock).
3. A persisted markdown file written by `PersistDeferredExecutionIssuesToWorkspace` is correctly updated by `RewriteDeferredIssueMarkdown` (integration test using a temp directory).
4. `GuidanceBroker.EmitResolution` is exported and documented. Its addition is additive — no existing tests regress.

---

### Phase B3 — Re-entry Context Loading and Next-Action Suggestions

**Goal:** When a session starts, load prior deferred issues from the workspace into the pre-task enrichment pipeline, making them visible to the semantic context bundle. When a session ends with open deferrals, the final report includes one concrete next-action prompt per critical/high-severity issue.

**What to Build**

**Part 1: Pre-task deferral loading**

New step in `named/euclo/runtime/pretask/`:

```
pretask/
  deferral_loader.go    — DeferralLoader, implements PipelineStep
```

```go
// DeferralLoader is a pretask pipeline step that loads persisted deferred
// issues from the workspace and injects them as context knowledge items.
// It runs before the main context enrichment steps.
type DeferralLoader struct {
    WorkspaceDir string
}

func (d DeferralLoader) Run(ctx context.Context, state *core.Context) error
```

`DeferralLoader.Run`:
1. Calls `eucloruntime.LoadDeferredIssuesFromWorkspace(d.WorkspaceDir)` (from Phase B1)
2. Filters to non-resolved issues (status `"open"`)
3. Converts each to a `pretask.ContextKnowledgeItem` with `Source = "deferred_issue"`, `Content = issue.Summary`, `Tags = []string{string(issue.Kind), string(issue.Severity)}`
4. Writes the items to state via `pretask.AddContextKnowledgeItems(state, items)`
5. Also seeds `"euclo.prior_deferred_issues"` in state as `[]DeferredExecutionIssue` for downstream use by the behavior layer

The loader is registered as the first step in the pipeline in `named/euclo/agent.go` where `ContextPipeline` is constructed, conditional on `WorkspaceDir` being non-empty.

**Part 2: Final report next-action suggestions**

Modified `euclotypes.AssembleFinalReport` (or a new `AssembleDeferralNextActions` function called from `assurance.applyVerificationAndArtifacts`):

```go
// AssembleDeferralNextActions returns one suggested next-session prompt per
// critical or high-severity open deferred issue. The prompts are concrete
// and specific to the issue's kind and evidence.
func AssembleDeferralNextActions(issues []DeferredExecutionIssue) []DeferralNextAction

type DeferralNextAction struct {
    IssueID        string `json:"issue_id"`
    Title          string `json:"title"`
    Severity       string `json:"severity"`
    SuggestedPrompt string `json:"suggested_prompt"`
}
```

`SuggestedPrompt` is constructed per `DeferredIssueKind`:

| Kind | Prompt template |
|------|-----------------|
| `ambiguity` | `"Before continuing, clarify: {issue.Title}. Context: {issue.Summary}"` |
| `stale_assumption` | `"Review whether this assumption still holds: {issue.Title}"` |
| `pattern_tension` | `"Start a planning session to resolve the tension: {issue.Title}"` |
| `verification_concern` | `"Run verification with focus on: {issue.Title}"` |
| `nonfatal_failure` | `"Investigate the non-fatal failure logged in the prior run: {issue.Title}"` |
| (default) | `"Resume with archaeology to address: {issue.Title}"` |

The `DeferralNextAction` list is added to `FinalReport["deferred_next_actions"]` when non-empty. This is a pure data addition — no existing report field is changed.

**File Dependencies**

Produces:
- `named/euclo/runtime/pretask/deferral_loader.go`
- `named/euclo/runtime/pretask/deferral_loader_test.go`

Modified:
- `named/euclo/euclotypes/artifacts.go` — adds `AssembleDeferralNextActions`, `DeferralNextAction`
- `named/euclo/runtime/assurance/assurance.go` — calls `AssembleDeferralNextActions` in the final report tail
- `named/euclo/agent.go` — registers `DeferralLoader` as first pretask pipeline step

**Unit Tests**

- `DeferralLoader.Run` with a workspace containing two open deferred issues adds two `ContextKnowledgeItem` entries to state
- `DeferralLoader.Run` with a workspace containing one resolved and one open issue adds only the open one
- `DeferralLoader.Run` with no deferred issues adds nothing and does not error
- `DeferralLoader.Run` with an empty `WorkspaceDir` is a no-op
- `AssembleDeferralNextActions` with no critical/high issues returns an empty slice
- `AssembleDeferralNextActions` with one critical ambiguity issue returns one `DeferralNextAction` with a non-empty `SuggestedPrompt`
- `AssembleDeferralNextActions` prompts are deterministic given the same input (stability test)
- Final report assembled during `assurance.Execute` includes `"deferred_next_actions"` key when critical issues are present

**Exit Criteria**

1. `go build ./...` and `go test ./...` pass.
2. `DeferralLoader` is the first registered step in the `ContextPipeline` when workspace is non-empty (verified by inspecting pipeline step order in test).
3. A session that ends with one critical open deferred issue has `FinalReport["deferred_next_actions"]` as a non-empty slice.
4. A session with no open deferred issues has `FinalReport["deferred_next_actions"]` absent or empty.

---

## Workstream C — Cross-Session Learning Propagation

### Background

The `archaeo/learning` service has complete infrastructure for managing learning interactions (pattern proposals, anchor drifts, tension reviews). The session resume path loads BKC context and plan state, but does not load pending learning interactions. Patterns and tensions confirmed or rejected in prior sessions are applied to the `PatternStore` and `PlanStore` but the new session doesn't get told this happened. There is no user-facing capability to promote a session-local insight ("this function is always called with a non-nil context") to a workspace-level pattern.

The three missing pieces:

1. **Session-start learning sync**: at the start of a session for an existing workflow, `learning.Service.SyncAll()` should run to create learning interactions for any proposed patterns, drifted anchors, or active tensions that have accumulated since the last sync. The results should be visible in the semantic context bundle.

2. **"Promote" capability**: a user-invocable capability `euclo:learning.promote` that takes an insight described in natural language (or extracted from current session state) and creates a `learning.Interaction` via `learning.Service.Create()`, persisting it as a pending proposal for confirmation in a future session.

3. **Post-resolution change notification**: when a session starts and prior learning interactions were resolved since the last session, a brief summary ("2 patterns confirmed, 1 tension resolved since your last session") is added to the semantic context so the executing behavior knows the workspace semantics have evolved.

---

### Phase C1 — Session-Start Learning Sync

**Goal:** At the start of each session for an existing workflow, call `learning.Service.SyncAll()` to create learning interactions for current workspace state. The result (pending + blocking interactions) is seeded into the semantic context bundle so the executing behavior is aware of pending learning decisions before it starts.

**What to Build**

New file: `named/euclo/runtime/pretask/learning_sync.go`

```go
// LearningSyncStep is a pretask pipeline step that calls learning.SyncAll
// for the current workflow and injects pending interactions as context
// knowledge items.
type LearningSyncStep struct {
    LearningService  learning.Service
    WorkflowResolver WorkflowIDResolver // func(state *core.Context) string
}

// WorkflowIDResolver extracts the workflow ID from execution state.
type WorkflowIDResolver func(state *core.Context) string

func (s LearningSyncStep) Run(ctx context.Context, state *core.Context) error
```

`LearningSyncStep.Run`:
1. Calls `s.WorkflowResolver(state)` to get the workflow ID; skips if empty
2. Reads the current `ExplorationID` and `CorpusScope` from state (set during pretask enrichment)
3. Calls `s.LearningService.SyncAll(ctx, workflowID, explorationID, snapshotID, corpusScope, codeRevision)`
4. Writes `"euclo.pending_learning_interactions"` to state as `[]learning.Interaction` (the pending set)
5. Converts pending interactions to `ContextKnowledgeItem` entries via `learningInteractionToKnowledgeItem`
6. Adds items to the context knowledge pool via `pretask.AddContextKnowledgeItems`
7. Sets `"euclo.has_blocking_learning"` in state as `bool` if any blocking interactions exist

**`learningInteractionToKnowledgeItem` function:**

```go
func learningInteractionToKnowledgeItem(interaction learning.Interaction) pretask.ContextKnowledgeItem {
    return pretask.ContextKnowledgeItem{
        Source:  "learning_interaction",
        Content: fmt.Sprintf("[Pending: %s] %s — %s", interaction.Kind, interaction.Title, interaction.Description),
        Tags:    []string{string(interaction.Kind), string(interaction.SubjectType)},
        Priority: learningPriority(interaction),
    }
}
```

Priority is elevated for `Blocking: true` interactions.

**Blocking learning gate:**

If `"euclo.has_blocking_learning"` is true and the current mode is `planning` or `chat.implement`, the pretask pipeline emits a warning into the context:

```go
// AddBlockingLearningWarning writes a warning context item when
// blocking learning interactions are pending.
func AddBlockingLearningWarning(state *core.Context, count int)
```

This does not block execution — it informs the behavior but does not short-circuit.

**Registration in agent:**

In `named/euclo/agent.go`, where `ContextPipeline` is initialized:

```go
if a.WorkspaceEnv.PatternStore != nil || a.WorkspaceEnv.WorkflowStore != nil {
    a.ContextPipeline.AddStep(pretask.LearningSyncStep{
        LearningService: a.learningService(),
        WorkflowResolver: func(state *core.Context) string {
            return workflowIDFromState(state)
        },
    })
}
```

The step runs only when both a workflow ID is available in state and the `PatternStore` is non-nil (i.e., archaeology is wired up).

**File Dependencies**

Produces:
- `named/euclo/runtime/pretask/learning_sync.go`
- `named/euclo/runtime/pretask/learning_sync_test.go`

Modified:
- `named/euclo/agent.go` — registers `LearningSyncStep` in pipeline initialization

**Unit Tests**

- `LearningSyncStep.Run` with an empty workflow ID is a no-op (returns nil, touches no state)
- `LearningSyncStep.Run` calls `LearningService.SyncAll` exactly once with the correct `workflowID`
- `LearningSyncStep.Run` with two pending interactions (one blocking) sets `"euclo.has_blocking_learning" = true`
- `LearningSyncStep.Run` with zero pending interactions sets `"euclo.has_blocking_learning" = false`
- `LearningSyncStep.Run` adds one `ContextKnowledgeItem` per pending interaction
- `LearningSyncStep.Run` with a `LearningService` whose `Store` is nil completes without error (nil-safe)
- `learningInteractionToKnowledgeItem` produces a non-empty `Content` for all five `InteractionKind` values
- `learningInteractionToKnowledgeItem` produces elevated priority for `Blocking: true` interactions

**Exit Criteria**

1. `go build ./...` and `go test ./...` pass.
2. `LearningSyncStep` is registered in the pipeline when `PatternStore` is non-nil (verified by inspecting the agent's pipeline configuration in test).
3. `LearningSyncStep` with a mock `LearningService` and a non-empty `workflowID` calls `SyncAll` exactly once.
4. A session that already has `"euclo.pending_learning_interactions"` in state from a prior step does not lose it (the step appends, not replaces).

---

### Phase C2 — Promote-to-Archaeology Capability

**Goal:** A new `euclo:learning.promote` capability that creates a `learning.Interaction` from a user-described insight. This is the explicit user-action path: "remember this for future sessions." It creates a `InteractionKnowledgeProposal` or `InteractionPatternProposal` interaction and persists it via `learning.Service.Create()`. The interaction remains pending until confirmed in a future session's learning sync.

**What to Build**

New file: `named/euclo/relurpicabilities/local/learning_promote.go`

```go
// LearningPromoteRoutine is a SupportingRoutine that creates a learning
// interaction from a user-described insight.
type LearningPromoteRoutine struct {
    LearningService  learning.Service
    WorkflowResolver func(state *core.Context) (workflowID, explorationID string)
}

func (r *LearningPromoteRoutine) ID() string { return "euclo:learning.promote" }

func (r *LearningPromoteRoutine) Execute(ctx context.Context, in RoutineInput) ([]euclotypes.Artifact, error)
```

**Promote input** is read from state at key `"euclo.learning_promote_input"`:

```go
type LearningPromoteInput struct {
    Title       string `json:"title"`        // required
    Description string `json:"description"`  // required
    Kind        string `json:"kind"`         // "pattern_proposal" | "knowledge_proposal" | "tension_review"
    SubjectID   string `json:"subject_id,omitempty"`
    SubjectType string `json:"subject_type,omitempty"` // "pattern" | "tension" | "exploration"
    Blocking    bool   `json:"blocking"`
}
```

The routine:
1. Reads `LearningPromoteInput` from state
2. Validates `Title`, `Description`, and `Kind` are non-empty
3. Resolves `workflowID` and `explorationID` via `WorkflowResolver(state)`
4. Calls `learning.Service.Create(ctx, learning.CreateInput{...})` with the input fields
5. Writes the created `Interaction` to state as `"euclo.promoted_learning_interaction"`
6. Produces an artifact with kind `euclo.learning_promotion` and payload containing the interaction ID and title
7. On `Create` error, returns the error — no silent fallback

**Fallback when no workflow is active:**

If `workflowID == ""`, the routine returns an error:  
`"cannot promote learning: no active workflow (start a planning session first)"`

This is the correct behavior — learning interactions are workflow-scoped.

**Evidence extraction:**

If `SubjectID` is empty and the state contains a `euclo.relurpic_behavior_trace` with `PrimaryCapabilityID`, the routine uses the trace's touched symbols and pattern refs as evidence:

```go
func extractEvidenceFromState(state *core.Context, in LearningPromoteInput) []learning.EvidenceRef
```

This makes "remember this" more useful when called after a behavior has already populated the state with its work.

**Registration:**

Add to `agent.go` alongside other `RegisterSupporting` calls:

```go
a.BehaviorDispatcher.RegisterSupporting(&local.LearningPromoteRoutine{
    LearningService: a.learningService(),
    WorkflowResolver: func(state *core.Context) (string, string) {
        return workflowIDFromState(state), explorationIDFromState(state)
    },
})
```

**File Dependencies**

Produces:
- `named/euclo/relurpicabilities/local/learning_promote.go`
- `named/euclo/relurpicabilities/local/learning_promote_test.go`

Modified:
- `named/euclo/agent.go` — registers `LearningPromoteRoutine`

**Unit Tests**

- `LearningPromoteRoutine.Execute` with a valid input and a mock `LearningService` calls `Create` exactly once
- `LearningPromoteRoutine.Execute` produces one artifact with kind `euclo.learning_promotion`
- `LearningPromoteRoutine.Execute` with empty `Title` returns a validation error
- `LearningPromoteRoutine.Execute` with empty `workflowID` returns a "no active workflow" error
- `LearningPromoteRoutine.Execute` with `LearningService.Create` returning an error propagates the error
- `extractEvidenceFromState` with a non-empty behavior trace returns at least one `EvidenceRef`
- `extractEvidenceFromState` with an empty state returns nil (not an error)
- The created `Interaction` has `Status = StatusPending` (verified via mock capture)
- `LearningPromoteInput.Kind = "pattern_proposal"` maps to `learning.InteractionPatternProposal`
- `LearningPromoteInput.Kind = "knowledge_proposal"` maps to `learning.InteractionKnowledgeProposal`

**Exit Criteria**

1. `go build ./...` and `go test ./...` pass.
2. `euclo:learning.promote` is registered in the Dispatcher and reachable via `DirectCapabilityRun`.
3. `LearningPromoteRoutine.Execute` with a valid mock `LearningService` produces a `Interaction` with `Status = "pending"`.
4. `LearningPromoteRoutine.Execute` with `workflowID = ""` returns a non-nil error containing the string `"no active workflow"`.

---

### Phase C3 — Post-Resolution Change Notification

**Goal:** When a new session starts for a workflow that has had learning interactions resolved since the previous session, inject a "since your last session" summary into the semantic context. This closes the awareness gap: the behavior knows the workspace semantics have evolved and can reference the changes when making decisions.

**What to Build**

New file: `named/euclo/runtime/pretask/learning_delta.go`

```go
// LearningDeltaStep is a pretask pipeline step that computes a delta
// of learning interactions resolved since the last session and injects
// a summary into the context knowledge pool.
type LearningDeltaStep struct {
    LearningService  learning.Service
    WorkflowResolver WorkflowIDResolver
    SessionResolver  SessionRevisionResolver // resolves last known session revision
}

// SessionRevisionResolver returns the last code revision for this workflow
// from the session index, or empty string if unknown.
type SessionRevisionResolver func(state *core.Context) string

func (s LearningDeltaStep) Run(ctx context.Context, state *core.Context) error
```

`LearningDeltaStep.Run`:
1. Resolves `workflowID` and `lastRevision` via their resolvers; skips if `workflowID == ""`
2. Calls `s.LearningService.ListByWorkflow(ctx, workflowID)` to get all interactions
3. Filters to interactions with `Status = StatusResolved` and `UpdatedAt > lastSessionTime`
4. `lastSessionTime` is read from state as `"euclo.last_session_time"` (set by the session resume path when the session record is loaded); falls back to zero time (meaning all resolved interactions are considered "new")
5. Groups resolved interactions by `SubjectType` → counts: confirmed patterns, rejected patterns, resolved tensions, refined anchors
6. Constructs a `LearningDeltaSummary`:

```go
type LearningDeltaSummary struct {
    TotalResolved      int            `json:"total_resolved"`
    ByKind             map[string]int `json:"by_kind"`
    ConfirmedPatterns  []string       `json:"confirmed_pattern_ids,omitempty"`
    ResolvedTensions   []string       `json:"resolved_tension_ids,omitempty"`
    RefinedAnchors     []string       `json:"refined_anchor_ids,omitempty"`
    SinceSummary       string         `json:"since_summary"` // human-readable line
}
```

7. If `TotalResolved > 0`, writes `"euclo.learning_delta"` to state and adds one `ContextKnowledgeItem` with `Source = "learning_delta"` and `Content = delta.SinceSummary`
8. If `TotalResolved == 0`, is a complete no-op (no state writes)

**`SinceSummary` construction:**

```go
// Example outputs:
// "Since your last session: 2 patterns confirmed, 1 tension resolved."
// "Since your last session: 3 patterns confirmed."
// "Since your last session: 1 anchor refined."
```

The summary is constructed from the counts without naming specific IDs (those are in the full delta struct if a behavior needs them).

**`lastSessionTime` wiring:**

The session resume path already loads `SessionResumeContext` which has a `CodeRevision`. Add `SessionStartTime time.Time` to `SessionResumeContext` and populate it from the `SessionRecord.CreatedAt` during `applySessionResumeContext`. Then in the pretask pipeline, `LearningSyncStep` writes the session start time to state as `"euclo.last_session_time"`.

**Registration:**

In `named/euclo/agent.go`, register `LearningDeltaStep` immediately after `LearningSyncStep`:

```go
a.ContextPipeline.AddStep(pretask.LearningDeltaStep{
    LearningService: a.learningService(),
    WorkflowResolver: func(state *core.Context) string { return workflowIDFromState(state) },
    SessionResolver:  func(state *core.Context) string { return state.GetString("euclo.last_session_revision") },
})
```

**File Dependencies**

Produces:
- `named/euclo/runtime/pretask/learning_delta.go`
- `named/euclo/runtime/pretask/learning_delta_test.go`

Modified:
- `named/euclo/runtime/session/resume.go` — adds `SessionStartTime time.Time` to `SessionResumeContext`
- `named/euclo/session_scoping.go` (or `managed_execution.go`) — writes `"euclo.last_session_time"` to state during resume context application
- `named/euclo/agent.go` — registers `LearningDeltaStep` in pipeline

**Unit Tests**

- `LearningDeltaStep.Run` with an empty `workflowID` is a no-op (no state writes, no error)
- `LearningDeltaStep.Run` with two resolved interactions since `lastSessionTime` writes `"euclo.learning_delta"` with `TotalResolved = 2`
- `LearningDeltaStep.Run` with zero resolved interactions since `lastSessionTime` does not write `"euclo.learning_delta"`
- `LearningDeltaStep.Run` with a zero `lastSessionTime` treats all resolved interactions as new
- `LearningDeltaSummary.SinceSummary` with `ConfirmedPatterns = 2, ResolvedTensions = 1` produces the correct human-readable line
- `LearningDeltaSummary.SinceSummary` with `TotalResolved = 0` is empty string
- `LearningDeltaStep.Run` with a non-nil mock `LearningService` calls `ListByWorkflow` exactly once
- A `ContextKnowledgeItem` is added to state when `TotalResolved > 0`
- `SessionResumeContext.SessionStartTime` is populated correctly from `SessionRecord.CreatedAt` during resume (integration test via `applySessionResumeContext`)

**Exit Criteria**

1. `go build ./...` and `go test ./...` pass.
2. `LearningDeltaStep` is registered in the pipeline after `LearningSyncStep` (verified by inspecting step order in test).
3. A workflow with two resolved interactions produces a state entry `"euclo.learning_delta"` with `TotalResolved = 2`.
4. A workflow with no resolved interactions produces no `"euclo.learning_delta"` state entry.
5. `SessionResumeContext.SessionStartTime` is non-zero after session resume (verified by existing session resume tests extended with a `CreatedAt` fixture).

---

## Cross-Workstream CI Additions

| Script | Added in | Checks |
|--------|----------|--------|
| `scripts/check-agent-lang-imports.sh` | Workstream A, Phase A2 | `named/euclo/agent.go` does not import any `platform/lang/` package |
| `scripts/check-deferral-lifecycle.sh` | Workstream B, Phase B2 | `euclo:deferrals.surface` and `euclo:deferrals.resolve` are registered (grep for their ID strings in dispatcher) |
| `scripts/check-learning-promote.sh` | Workstream C, Phase C2 | `euclo:learning.promote` is registered |

---

## Workstream Sequencing

Within each workstream, phases are strictly sequential. Across workstreams, there are no dependencies:

```
Workstream A:  A1 → A2 → A3
Workstream B:  B1 → B2 → B3
Workstream C:  C1 → C2 → C3
```

All three workstreams may begin on the same day. The only shared file touched by more than one workstream is `named/euclo/agent.go` — specifically the `InitializeEnvironment` method (A3) and the pipeline setup block (B3, C1, C3). These edits are in different sections of the method and will not produce merge conflicts if worked in separate branches with clean rebase.

---

## Risk Register

| Risk | Workstream | Mitigation |
|------|-----------|------------|
| `Detect` reads too many directory entries on a large monorepo, causing slow startup | A1 | Hard cap at 200 entries; benchmark test must pass in <10ms |
| `ResolverFactory` falls back to all-languages when workspace path is empty, defeating the purpose | A3 | Log a warning (not an error) when falling back; add a metric for observability |
| `LoadDeferredIssuesFromWorkspace` parses YAML frontmatter incorrectly for edge-case issue IDs | B1 | Fuzz the YAML frontmatter round-trip test with malformed IDs (empty, with slashes, with quotes) |
| `RewriteDeferredIssueMarkdown` fails atomically but leaves a `.tmp` file | B2 | Add cleanup of `.tmp` on error; test with a temp directory that becomes read-only midway |
| `GuidanceBroker.EmitResolution` races with `broadcast` when called from multiple goroutines | B2 | `EmitResolution` acquires `g.mu` before calling `broadcast`; add a concurrent-call test |
| `learning.SyncAll` is slow (database read + pattern store scan) when called at every session start | C1 | Add a `last_sync_time` check: skip sync if last sync was within 60 seconds (configurable); add a timeout context of 5 seconds |
| `LearningDeltaStep` calls `ListByWorkflow` which returns all interactions (potentially large) | C3 | Add a `SinceTime` parameter to `learning.Service.ListByWorkflow` or filter in Go with early exit when the oldest resolved interaction predates `lastSessionTime` |
| `SessionResumeContext.SessionStartTime` is zero for first sessions (no prior session record) | C3 | `LearningDeltaStep` treats zero time as "all resolved interactions are new" — correct behavior for first session, potentially noisy; add a first-session guard that sets a 48-hour look-back cap |
