# relurpish UX Expansion — Engineering Specification

**Status:** Design approved  
**Scope:** `app/relurpish`, `named/euclo`, `ayenitd`  
**Date:** 2026-04-07

---

## Background

This document specifies the engineering work required to bring `app/relurpish` up
to date with three areas of recent backend development, and to introduce the
archaeology interaction surface that has no current TUI representation.

The work falls into four functional areas:

1. **Service control** — `ayenitd.ServiceManager` is fully implemented but
   invisible to the TUI. Operators have no way to inspect, stop, or restart
   services from within relurpish.

2. **Chat context sidebar** — euclo's context enrichment pipeline
   (`ContextProposalPhase`) can propose and confirm files, but the TUI has no
   persistent view of what is currently in context. The `add` and `remove` action
   stubs in `ContextProposalPhase` are unimplemented.

3. **Archaeology tab** — the archaeology relurpic capability (owned by euclo,
   backed by the archaeo memory/provenance subsystem) has no dedicated TUI
   surface. Tensions, patterns, and learning interactions produced by the
   capability are not browsable or actionable from the TUI.

4. **Enrichment wiring** — `ShowConfirmationFrame` is hardcoded in `agent.go`
   with a placeholder comment. The manifest spec override path is unimplemented.

---

## Architectural Constraints

- **Layout**: `ChromeLayout` remains single-column. All split-pane rendering is
  internal to individual pane renderers; chrome is not affected.
- **Input bar**: universal across all tabs and subtabs. All prompts and commands
  flow through the same input bar regardless of active tab.
- **Keybinds**: all keybinds introduced by this work must be configurable via the
  existing keybindings system. No hardcoded keys.
- **Emoji**: type badges use emoji (`⚡` / `🧩` / `💡`) with a configurable
  fallback to letter badges (`[T]` / `[P]` / `[L]`) for terminals without
  coverage. Controlled by workspace config.
- **Ownership**: the archaeo tab surfaces data from the archaeo memory/provenance
  subsystem. All agent behaviour — enrichment, exploration, plan amendment — is
  owned by euclo, which invokes its archaeology relurpic capabilities. The archaeo
  subsystem is not an agent and is not addressed directly from the TUI.

---

## Terminology

| Term | Definition |
|---|---|
| **archaeo** | Memory/provenance subsystem. Passive data layer. Stores and retrieves tensions, patterns, learning interactions. |
| **archaeology relurpic capability** | Euclo-owned relurpic capability (`named/euclo/relurpicabilities/archaeology/`) that reads/writes the archaeo subsystem. |
| **relurpic capability** | Framework-level (`core.CapabilityRuntimeFamilyRelurpic`) higher-order execution behaviour composed from capabilities, skills, and execution paradigms. |
| **blob** | Collective term for a tension, pattern, or learning item as it appears in the plan subtab sidebar — a unit of archaeo-sourced content that may be added to or removed from the live plan. |
| **session pin** | A file confirmed in a prior turn of the current session; always included in subsequent enrichment passes. |
| **live plan** | The currently active `frameworkplan.LivingPlan` with step status, produced and amended by euclo. |

---

## Feature Specifications

### 1. Service Control — "ayenitd services" Subtab

**Location:** `app/relurpish/tui/pane_session.go`, new subtab `SubTabSessionServices`

**What it does:**  
Exposes the `ayenitd.ServiceManager` registry to the operator. Lists all
registered services with their status. Allows stop, restart, and stop-all /
restart-all operations. The list is scrollable.

**Data source:**  
`Workspace.ListServices()` → service IDs  
`Workspace.GetService(id)` → `Service` interface (Start/Stop)  
`Workspace.Restart(ctx)` → restart all

**Runtime adapter additions:**
```go
// In RuntimeAdapter interface
ListServices() []ServiceInfo
StopService(id string) error
RestartService(ctx context.Context, id string) error
RestartAllServices(ctx context.Context) error
```

```go
type ServiceInfo struct {
    ID     string
    Status ServiceStatus // Running | Stopped | Error
}
```

The runtime polls `ListServices()` on a ticker (alongside the existing
diagnostics poll) and delivers a `ServicesUpdatedMsg` to the session pane.

**UI:**

```
1:live  2:tasks  3:services  4:settings
─────────────────────────────────────────
 ayenitd services

 scheduler          [running]
 custom-worker      [stopped]

 [s] stop  [r] restart  [R] restart-all
```

- `j`/`k` navigate the list
- `s` stops the focused service
- `r` restarts the focused service  
- `R` restarts all services (with confirmation prompt)
- Status badge: `[running]` / `[stopped]` / `[error]`
- Error state shows a one-line message beneath the service entry
- List is scrollable via standard viewport component

**Confirmation:** stop and restart-all require a `y/n` prompt inline (reuse
the existing notification bar confirmation pattern, not a modal).

---

### 2. Chat Context Sidebar

**Location:** `app/relurpish/tui/pane_chat.go`, new `ContextSidebar` component

**What it does:**  
Provides a persistent, toggleable panel in chat mode showing all files currently
in the enrichment context (confirmed files and session pins), with their
insertion-action classification. Files can be added via `/add` or `@` and
removed via `/drop` or sidebar navigation.

**Insertion-action badges:**

| Badge | Insertion action | Meaning |
|---|---|---|
| `[dir]` | `direct` | Full file content in context |
| `[sum]` | `summarized` | One-line summary only |
| `[ref]` | `metadata-only` | Reference/path only |

**State:**  
The TUI holds `[]ContextSidebarEntry` in `RootModel`. This is populated when a
`ContextProposalContent` frame is received (already rendered inline; the sidebar
state is populated from the same frame). Session pins are visually distinguished
from per-turn confirmed files with a `·` prefix.

```go
type ContextSidebarEntry struct {
    Path            string
    InsertionAction string // "direct" | "summarized" | "metadata-only"
    IsPin           bool   // session pin vs per-turn confirmed
}
```

**Commands:**

| Input | Action |
|---|---|
| `/add <path>` | Add file or directory to confirmed files; queued as session pin |
| `/drop <path>` | Remove file from confirmed files and session pins |
| `@<path>` | Existing file picker; selection appended to confirmed files |
| Sidebar `x` / `d` | Remove focused entry from confirmed files |

**Sidebar layout** (internal to pane, not chrome):

```
┌─ context ─────────────────┐
│ · framework/core/...  [dir]│
│   agents/pipeline/... [dir]│
│   docs/plans/...      [ref]│
│                            │
│ [a] add  [x] remove       │
└────────────────────────────┘
```

- Toggled by configurable keybind (default `ctrl+]`, configurable)
- When sidebar focused: `j`/`k` navigate, `x`/`d` remove focused entry, `a`
  opens file picker
- Width: 30 cols fixed; collapses to hidden below 90-col terminal width
  (toggle still works as overlay in that case)
- Scroll: standard viewport for long lists

**`ContextProposalPhase` stub completion:**  
The `add` and `remove` action slots in `ContextProposalPhase` (currently stubs
at `interaction/modes/chat.go:226-273`) must be replaced with real behaviour:
- `add` → opens file picker; selected paths are appended to `context.confirmed_files` and `context.pinned_files`, pipeline re-runs
- `remove` → returns the current list to the TUI so the user can navigate and deselect (response carries the full list; the TUI updates sidebar state)

**`ShowConfirmationFrame` wiring:**  
The placeholder at `named/euclo/agent.go:1640` must be replaced with a read
from `AgentSpec` (the `skill_config.context_enrichment.show_confirmation_frame`
field). Default remains `true`.

---

### 3. Archaeology Tab

**Location:** New tab `TabArchaeo = "archaeo"`, registered for `AgentFilter: []string{"euclo"}`.  
New pane: `app/relurpish/tui/pane_archaeo.go`  
New subtabs: `SubTabArchaeoPlan = "plan"` and `SubTabArchaeoExplore = "explore"`

#### 3a. Tab Registration

```go
reg.Register(TabDefinition{
    ID: TabArchaeo, Label: "archaeo", AgentFilter: []string{"euclo"},
    SubTabs: []SubTabDefinition{
        {SubTabArchaeoPlan,    "plan"},
        {SubTabArchaeoExplore, "explore"},
    },
})
```

#### 3b. `plan` Subtab — Live Plan + Blob Sidebar

The plan subtab renders as an internal horizontal split. `ChromeLayout` is
unchanged; `pane_archaeo` allocates its own width when rendering.

**Main area (left):** Live plan display

```
 live plan

 ✓  1  explore framework/capability structure
 ▶  2  resolve naming-conv tension        ← linked to blob
 ·  3  verify pattern anchors
 ·  4  (pending)
 ──────────────────────────────────
 euclo output stream …
```

- Steps rendered with status icons: `✓` complete, `▶` active, `·` pending, `✗` failed
- Steps linked to blobs show the blob title inline
- Below the plan: recent euclo output (scrollable, fixed height)
- When a blob is added to the plan, a new step appears and is highlighted briefly

**Sidebar (right): blobs to consider**

Single scrollable list. Groups (tensions → patterns → learning) separated by a
blank line. No section headers.

```
 ⚡ naming-conv         [+]
 ⚡ type-shadow         [+]
 ⚡ import-order       [in]

 🧩 error-handling      [+]
 🧩 context-propag     [in]

 💡 prefer-iface        [+]
```

- `⚡` tension · `🧩` pattern · `💡` learning interaction
- `[+]` not in plan (can add) · `[in]` referenced by a plan step (can remove)
- `j`/`k` navigate the full list
- `Enter` on `[+]` item: dispatches to euclo to add blob to plan (creates/amends
  plan step). Euclo invokes its archaeology relurpic capability to produce the
  step.
- `x`/`d` on `[in]` item: removes blob from plan (unlinks/removes associated
  step)
- `e` expands the focused item inline (shows description, anchor refs, evidence)
- Sidebar width: 28 cols; collapses to overlay toggle below 90-col terminal width

**Emoji fallback:**  
Controlled by workspace config key `relurpish.blob_emoji: true|false`. When
false, renders `[T]` / `[P]` / `[L]` instead.

**Runtime adapter additions for plan subtab:**
```go
LoadActivePlan(ctx context.Context, workflowID string) (*ActivePlanView, error)
LoadBlobs(ctx context.Context, workflowID string) ([]BlobEntry, error)
AddBlobToPlan(ctx context.Context, workflowID string, blobID string) error
RemoveBlobFromPlan(ctx context.Context, workflowID string, blobID string) error
```

```go
type BlobEntry struct {
    ID          string
    Kind        BlobKind   // BlobTension | BlobPattern | BlobLearning
    Title       string
    Description string
    Severity    string     // for tensions: high/med/low
    Status      string     // for tensions: active/accepted/resolved
    InPlan      bool
    AnchorRefs  []string
    StepID      string     // set when InPlan is true
}

type BlobKind string
const (
    BlobTension  BlobKind = "tension"
    BlobPattern  BlobKind = "pattern"
    BlobLearning BlobKind = "learning"
)
```

`LoadBlobs` aggregates `TensionsByWorkflow`, pattern retrieval, and
`LearningQueue` from `ArchaeoServiceAccess` into a single sorted list (tensions
first, patterns second, learning third).

**Live updates:**  
The plan subtab subscribes to plan change events (polled via ticker alongside
diagnostics). When the live plan updates (step added, status changed), a
`PlanUpdatedMsg` is delivered to the pane and the main area re-renders.

#### 3c. `explore` Subtab — Archaeology Exploration

Full-width feed. No sidebar. The input bar prompts euclo, which invokes the
archaeology relurpic capability.

**Interaction flow:**

1. User types a prompt: "find naming inconsistencies in framework/capability"
2. euclo dispatches archaeology relurpic capability
3. Capability runs: anchor extraction → code retrieval → archaeo query →
   tension/pattern proposals
4. Results stream into the explore feed as structured frames:
   - `FrameProposal` with `ContextProposalContent` for retrieved files
   - Custom `FrameArchaeoFindings` for proposed tensions/patterns
5. Each proposed blob is rendered with a `[stage]` / `[staged]` toggle action

**Blob staging (option B — explicit promotion):**

```
 ⚡ naming-conv                              [stage]
    Inconsistent naming between capability
    registry and provider interfaces.
    Anchors: framework/capability/registry.go:42

 🧩 error-handling                           [staged]
    Consistent pattern: errors wrapped at
    boundary, not internally.
```

- `[stage]` → user promotes the blob; badge changes to `[staged]`; blob appears
  in plan subtab sidebar as `[+]`
- `[staged]` → already promoted; can be un-staged with `x`
- `/promote-all` command promotes all proposed blobs in the current explore run
- Un-staged blobs are not persisted; they exist only in the explore feed until
  the session ends

**Staged blob persistence:**  
Staged blobs are held in `pane_archaeo` state as `[]StagedBlobEntry`. On subtab
switch from `explore` to `plan`, staged blobs appear immediately in the sidebar
as `[+]` entries. They are written to archaeo (via euclo's relurpic capability)
only when added to the plan — not at staging time.

---

### 4. Learning Interactions

Non-blocking learning interactions are currently notified but not browsable.
Blocking interactions already open the HITL panel.

**Additions:**

- `/learning` command: displays the full `LearningQueueView` as a scrollable
  list in a read-only overlay (same visual pattern as `/guidance` and `/deferred`)
- Each entry shows: title, kind, blocking/non-blocking indicator, evidence list
- Non-blocking entries can be dismissed (`d`) or deferred (`esc`)
- Blocking entries redirect to the HITL panel (already implemented)
- Learning blobs (💡) in the plan subtab sidebar come from `LearningQueue` — they
  represent significant pending interactions that may affect the plan

---

## Data Flow Summary

```
User prompt (explore subtab)
  │
  ▼
euclo (archaeology relurpic capability)
  │  reads/writes
  ▼
archaeo subsystem (tensions, patterns, learning)
  │
  ├─→ explore feed: proposed blobs + [stage] action
  │
  └─→ (on staging) plan subtab sidebar: [+] candidates
            │
            ├─→ user adds blob → euclo amends live plan → step appears
            │
            └─→ user removes blob → plan step unlinked
```

```
User submit (chat mode)
  │
  ▼
ContextProposalPhase (enrichment pipeline, automatic)
  │  produces ContextProposalContent frame
  ▼
Confirmation frame (ShowConfirmationFrame=true)
  │  user confirms / adds / removes files
  ▼
context.confirmed_files + context.pinned_files written to state
  │
  └─→ chat context sidebar updated (live view of active context)
```

---

## Implementation Plan

### Phase 1 — Runtime Layer: Expose New Data Surfaces

**Goal:** All new data surfaces are available through `RuntimeAdapter` before any
UI work begins. Subsequent phases are purely additive UI.

**Dependencies:** None beyond current codebase.

**Scope:**

- `app/relurpish/runtime/runtime.go`: expose `ServiceManager` methods
  (`ListServices`, `StopService`, `RestartAllServices`)
- `app/relurpish/tui/runtime_adapter.go`: extend `RuntimeAdapter` interface with
  `ListServices`, `StopService`, `RestartAllServices`, `LoadActivePlan`,
  `LoadBlobs`, `AddBlobToPlan`, `RemoveBlobFromPlan`, `PendingLearning`
  (already on runtime, not yet on adapter interface)
- Wire `ArchaeoServiceAccess` into `runtime.go` (currently used inside euclo but
  not surfaced through the relurpish runtime)
- `named/euclo/interaction/modes/chat.go`: replace `add`/`remove` action stubs
  with real implementations (file picker dispatch + per-file removal)
- `named/euclo/agent.go:1640`: parse `ShowConfirmationFrame` from manifest
  `AgentSpec` skill config; default `true`
- `app/relurpish/tui/runtime_adapter_test.go`: extend with tests for all new
  adapter methods against a stub runtime

**Tests:**
- `TestRuntimeAdapter_ListServices` — returns service IDs and statuses
- `TestRuntimeAdapter_StopService` — delegates to ServiceManager, returns error
- `TestRuntimeAdapter_RestartAllServices` — delegates to Workspace.Restart
- `TestRuntimeAdapter_LoadBlobs` — aggregates tensions + patterns + learning into
  sorted BlobEntry slice
- `TestRuntimeAdapter_LoadActivePlan` — maps VersionedPlanView to pane types
- `TestShowConfirmationFrame_ManifestOverride` — false in manifest disables frame
- `TestContextProposalPhase_AddAction` — add action opens file picker and
  re-queues pipeline
- `TestContextProposalPhase_RemoveAction` — remove action returns file list for
  sidebar update

**Done when:** `go test ./app/relurpish/...` and `go test ./named/euclo/...` pass
with all new adapter methods covered.

---

### Phase 2 — Service Control: "ayenitd services" Subtab

**Goal:** Operators can inspect and control services from within the session pane.

**Dependencies:** Phase 1 complete.

**Scope:**

- `app/relurpish/tui/types.go`: add `SubTabSessionServices = "services"`
- `app/relurpish/tui/pane_session.go`: new services subtab renderer
  - `ServicesView` struct holding `[]ServiceInfo` and cursor position
  - `ServicesUpdatedMsg` message type
  - Render: service ID, status badge, error line when applicable
  - Scrollable via `viewport` component
- `app/relurpish/tui/model.go`: route `ServicesUpdatedMsg` to session pane;
  add services poll alongside existing diagnostics poll ticker
- `app/relurpish/tui/commands.go` or `commands_session.go`: add stop/restart
  commands (`/service stop <id>`, `/service restart <id>`, `/service restart-all`)
- `app/relurpish/tui/keys.go`: `s`, `r`, `R` in services subtab focus; all
  configurable
- Inline confirmation for `R` (restart-all) via notification bar pattern

**Tests:**
- `TestServicesView_Render_EmptyList`
- `TestServicesView_Render_WithServices` — running, stopped, error states
- `TestServicesView_Navigation` — cursor movement, viewport scroll
- `TestServiceStopCommand_Dispatch` — `/service stop scheduler` calls adapter
- `TestServiceRestartAll_Confirmation` — `R` key triggers confirmation prompt,
  `y` dispatches, `n` cancels
- `TestServicesPolling_UpdatesView` — tick delivers `ServicesUpdatedMsg`,
  view re-renders

**Done when:** services subtab is visible, populated, and all stop/restart
operations work end-to-end against a stub `ServiceManager`.

---

### Phase 3 — Chat Context Sidebar

**Goal:** Confirmed files and session pins are always visible in chat mode.
Files can be added and removed without leaving the chat tab.

**Dependencies:** Phase 1 complete (stub implementations replaced, confirmed
files state wired through from `ContextProposalContent` frame).

**Scope:**

- `app/relurpish/tui/pane_chat.go`: new `ContextSidebar` component
  - Holds `[]ContextSidebarEntry` (populated on `ContextProposalContent` frame)
  - Toggled by configurable keybind
  - Internal split: sidebar takes fixed 30 cols, rest is feed
  - Collapses to overlay toggle below 90-col terminal width
  - Scroll via `viewport`
- `app/relurpish/tui/state.go`: add `ContextSidebarEntry` type and
  `contextSidebarEntries []ContextSidebarEntry` to chat pane state
- `app/relurpish/tui/model.go`: on receipt of `ContextProposalContent` frame,
  update `contextSidebarEntries` in addition to existing feed rendering
- `app/relurpish/tui/commands.go`: `/add <path>` and `/drop <path>` commands
  - `/add` calls `RuntimeAdapter.AddFileToContext(path)` (new method) which
    appends to session pins and signals euclo to re-run enrichment if in active
    turn
  - `/drop` calls `RuntimeAdapter.DropFileFromContext(path)` which removes from
    confirmed files and session pins
- `app/relurpish/tui/keys.go`: sidebar toggle, `j`/`k` navigate, `x`/`d`
  remove, `a` open file picker; all configurable
- `app/relurpish/tui/euclo_renderer.go`: insertion-action badge rendering
  (`[dir]` / `[sum]` / `[ref]`)

**Tests:**
- `TestContextSidebar_PopulatesOnProposalFrame` — frame received → sidebar
  entries updated
- `TestContextSidebar_Toggle` — keybind hides/shows sidebar; pane width adjusts
- `TestContextSidebar_CollapseNarrowTerminal` — below 90 cols, sidebar hidden by
  default, toggle opens as overlay
- `TestContextSidebar_RemoveEntry` — `x` on focused entry removes it and
  dispatches DropFileFromContext
- `TestContextSidebar_InsertionActionBadge` — direct/summarized/metadata-only
  map to correct badge strings
- `TestContextSidebar_PinDistinction` — session pins rendered with `·` prefix
- `TestAddCommand_AppendsToSidebar`
- `TestDropCommand_RemovesFromSidebar`
- `TestAtMention_AppendsToConfirmedFiles` — `@path` in input → file appears in
  sidebar

**Done when:** chat context sidebar is fully operational; confirmed files persist
across turns within a session; `/add`, `/drop`, `@` all work correctly.

---

### Phase 4 — Archaeology Tab: Structure, Explore, and Staged Propagation

**Goal:** The archaeo tab exists, the explore subtab is functional, and blobs can
be staged and propagated to the plan subtab sidebar.

**Dependencies:** Phase 1 complete. Euclo's archaeology relurpic capability is
invocable from the explore feed input path.

**Scope:**

- `app/relurpish/tui/types.go`: add `TabArchaeo`, `SubTabArchaeoPlan`,
  `SubTabArchaeoExplore`
- `app/relurpish/tui/types.go`: `registerEucloTabs` registers archaeo tab
- New file `app/relurpish/tui/pane_archaeo.go`:
  - `ArchaeoPaneState` struct: `exploreEntries []ExploreEntry`,
    `stagedBlobs []StagedBlobEntry`, `blobList []BlobEntry`, `livePlan *ActivePlanView`
  - `ExploreEntry`: rendered frames from explore feed
  - `StagedBlobEntry`: blob ID, kind, title, staging state
  - `BlobEntry`: as specified above
- `explore` subtab renderer: full-width feed, structured frame rendering for
  `FrameArchaeoFindings` (new frame kind, or reuse `FrameProposal` with new
  content type)
- `[stage]`/`[staged]` action slots on blob entries in explore feed
- On stage: blob appended to `stagedBlobs`; switching to plan subtab makes staged
  blobs visible as `[+]` entries
- `/promote-all` command stages all proposed blobs in current explore run
- Blob list sorting: tensions first, patterns second, learning third; blank line
  between groups
- Emoji rendering for blob kind badges with fallback (workspace config
  `relurpish.blob_emoji`)
- `app/relurpish/tui/commands.go`: route explore input to euclo archaeology mode

**Tests:**
- `TestArchaeoTab_Registration` — tab appears in tab registry for euclo agent
- `TestExploreSubtab_RendersFullWidth` — no sidebar split
- `TestExploreSubtab_FrameRendering` — `FrameArchaeoFindings` renders blob
  proposals with `[stage]` action
- `TestBlobStaging_StageAction` — stage action → blob in stagedBlobs, badge
  changes to `[staged]`
- `TestBlobStaging_Unstage` — `x` on staged blob removes from stagedBlobs
- `TestBlobStaging_PromoteAll` — `/promote-all` stages all proposals
- `TestBlobPropagation_ToSidebar` — staged blobs appear as `[+]` in plan subtab
  after subtab switch
- `TestBlobList_Sorting` — tensions before patterns before learning
- `TestBlobList_EmojiRendering` — correct emoji per kind
- `TestBlobList_FallbackRendering` — letter badges when blob_emoji=false
- `TestBlobList_BlankLineSeparation` — blank line between type groups

**Done when:** explore subtab accepts prompts, renders blob proposals, allows
staging, and staged blobs appear correctly in the plan subtab sidebar.

---

### Phase 5 — Archaeology Tab: Live Plan and Blob-to-Plan Operations

**Goal:** The plan subtab shows the live plan, the blob sidebar is fully
interactive, and blob-to-plan operations are end-to-end.

**Dependencies:** Phase 4 complete. `RuntimeAdapter.LoadActivePlan`,
`LoadBlobs`, `AddBlobToPlan`, `RemoveBlobFromPlan` all functional.

**Scope:**

- `pane_archaeo.go`: plan subtab renderer
  - Internal horizontal split: main area (live plan + euclo output stream) and
    sidebar (blob list)
  - Sidebar width 28 cols; collapses to overlay toggle below 90-col terminal width
  - Main area: plan step list with status icons (`✓` / `▶` / `·` / `✗`),
    linked blob title when step has a blob ref
  - Below plan: scrollable euclo output stream (last N frames from the
    archaeology capability)
  - Sidebar: `BlobEntry` list, `j`/`k` navigation, `Enter` adds `[+]` blob to
    plan, `x`/`d` removes `[in]` blob from plan, `e` expands detail inline
  - `Tab` toggles focus between main area and sidebar
  - On `AddBlobToPlan`: dispatch to runtime adapter → euclo invokes archaeology
    relurpic capability to create/amend plan step → `PlanUpdatedMsg` delivered →
    main area re-renders with new step highlighted briefly
  - On `RemoveBlobFromPlan`: dispatch → euclo unlinks blob from step → plan
    re-renders
- Live plan polling: ticker delivers `PlanUpdatedMsg` to pane; poll interval
  same as diagnostics ticker
- Blob list polling: ticker delivers `BlobsUpdatedMsg`; refreshes sidebar
  contents from `ArchaeoServiceAccess`

**Tests:**
- `TestPlanSubtab_InternalSplit` — plan area + sidebar within allotted width
- `TestPlanSubtab_SidebarCollapseNarrow` — below 90 cols, sidebar togglable
- `TestPlanSubtab_StepRendering` — status icons, linked blob title
- `TestPlanSubtab_TabFocusToggle` — Tab switches focus between main and sidebar
- `TestBlobSidebar_AddBlob` — Enter on `[+]` entry dispatches AddBlobToPlan,
  plan step appears, blob changes to `[in]`
- `TestBlobSidebar_RemoveBlob` — x on `[in]` entry dispatches RemoveBlobFromPlan,
  step removed, blob changes to `[+]`
- `TestBlobSidebar_ExpandDetail` — `e` renders description, anchor refs inline
- `TestBlobSidebar_Scroll` — viewport scrolls correctly for long lists
- `TestPlanPolling_UpdatesView` — `PlanUpdatedMsg` re-renders plan with new step
- `TestBlobPolling_UpdatesSidebar` — `BlobsUpdatedMsg` updates sidebar entries
- `TestAddBlobHighlight` — newly added plan step is highlighted for one render
  cycle

**Done when:** plan subtab shows live plan, blob sidebar is navigable and
actionable, add/remove operations round-trip through euclo end-to-end.

---

### Phase 6 — Learning Interactions, Configurability, and Completeness

**Goal:** Learning interactions are browsable. All keybinds are configurable.
Emoji fallback works. Titlebar badges surface blob counts. Plan history subtab
accessible.

**Dependencies:** Phases 1–5 complete.

**Scope:**

**Learning interactions:**
- `/learning` command: overlay showing `LearningQueueView` (full list with title,
  kind, blocking indicator, evidence)
- Non-blocking entries: `d` dismiss, `esc` defer
- Blocking entries: redirect to HITL panel
- Learning blobs (💡) in plan subtab sidebar are sourced from `LearningQueue`
  (already in `LoadBlobs` aggregate from Phase 1)
- `LearningUpdatedMsg`: delivered when `LearningBroker` fires an event;
  refreshes learning blobs in sidebar

**Configurable keybinds:**
- All keybinds introduced in Phases 2–5 registered in the keybindings system:
  - `sidebar.toggle` (chat context sidebar)
  - `sidebar.focus` (archaeology plan sidebar focus toggle)
  - `blob.add` (add blob to plan)
  - `blob.remove` (remove blob from plan)
  - `blob.expand` (expand blob detail)
  - `service.stop` (stop focused service)
  - `service.restart` (restart focused service)
  - `service.restart_all` (restart all services)
  - `explore.stage` (stage focused blob in explore feed)
  - `explore.promote_all` (promote all blobs in current explore run)
- Document new keybinds in help overlay (`help.go`)

**Emoji fallback:**
- Workspace config key `relurpish.blob_emoji: true` (default) / `false`
- Config pane (`pane_config.go`) exposes this toggle
- `BlobKindBadge(kind BlobKind, emojiEnabled bool) string` utility function

**Titlebar blob counts:**
- `comp_titlebar.go`: when in archaeo tab, append tension/pattern/learning counts
  from `TensionSummaryView` + learning queue size
  - Format: `⚡3  🧩2  💡1` (or `T:3 P:2 L:1` when emoji disabled)
- Updated by `BlobsUpdatedMsg`

**Plan history subtab** (`SubTabArchaeoHistory = "history"`):
- Lists `PlanVersions` from `ArchaeoServiceAccess`
- Each entry: version number, status (active/draft/archived), derived-from
  exploration ref
- `Enter` on a non-active version: prompts euclo to activate that version
  (dispatches `ActivatePlanVersion` via runtime adapter)
- Read-only diff view: selecting a version shows its step list in the main area

**Tests:**
- `TestLearningOverlay_Render` — `/learning` opens scrollable overlay
- `TestLearningOverlay_DismissNonBlocking`
- `TestLearningOverlay_BlockingRedirectsHITL`
- `TestKeybindConfig_AllNewBinds` — all Phase 2–5 keybinds appear in keybindings
  registry; can be overridden
- `TestBlobKindBadge_EmojiOn` — returns emoji for each kind
- `TestBlobKindBadge_EmojiOff` — returns letter badge for each kind
- `TestTitlebar_BlobCounts_ArchaeoTab` — counts rendered when archaeo tab active
- `TestTitlebar_BlobCounts_EmojiOff` — letter form when emoji disabled
- `TestPlanHistorySubtab_ListVersions`
- `TestPlanHistorySubtab_ActivateVersion` — Enter dispatches ActivatePlanVersion

**Done when:** all features are complete, all keybinds configurable, emoji
fallback functional, learning queue browsable, plan history accessible.

---

## Phase Summary

| Phase | Name | Key Deliverables | Depends On |
|---|---|---|---|
| 1 | Runtime Layer | All new data surfaces on RuntimeAdapter; action stubs replaced; ShowConfirmationFrame wired | — |
| 2 | Service Control | "ayenitd services" subtab; stop/restart operations; polling | Phase 1 |
| 3 | Chat Context Sidebar | Toggleable sidebar; `/add`, `/drop`, `@`; insertion-action badges; configurable toggle keybind | Phase 1 |
| 4 | Archaeo Tab + Explore | Tab registered; explore subtab; staged propagation; blob list rendering; emoji/fallback | Phase 1 |
| 5 | Archaeo Plan + Blob Operations | Live plan display; blob sidebar; add/remove to plan; plan polling | Phase 4 |
| 6 | Learning, Keybinds, Completeness | Learning overlay; all keybinds configurable; titlebar counts; plan history subtab | Phases 2–5 |

---

## Files Added or Significantly Modified

| File | Change |
|---|---|
| `app/relurpish/tui/pane_archaeo.go` | New — archaeology pane, both subtabs |
| `app/relurpish/tui/pane_session.go` | Add services subtab renderer |
| `app/relurpish/tui/pane_chat.go` | Add ContextSidebar component |
| `app/relurpish/tui/types.go` | New tab/subtab constants |
| `app/relurpish/tui/state.go` | New state types (ContextSidebarEntry, BlobEntry, ServiceInfo) |
| `app/relurpish/tui/runtime_adapter.go` | New adapter methods |
| `app/relurpish/tui/model.go` | Route new message types; services + plan + blob polling |
| `app/relurpish/tui/commands.go` | `/add`, `/drop`, `/learning`, `/service`, `/promote-all` |
| `app/relurpish/tui/keys.go` | New configurable keybind registrations |
| `app/relurpish/tui/euclo_renderer.go` | Insertion-action badges; `FrameArchaeoFindings` rendering |
| `app/relurpish/tui/comp_titlebar.go` | Blob count badges when archaeo tab active |
| `app/relurpish/tui/help.go` | Document new keybinds |
| `app/relurpish/runtime/runtime.go` | Expose ServiceManager; expose ArchaeoServiceAccess |
| `named/euclo/interaction/modes/chat.go` | Replace add/remove stubs |
| `named/euclo/agent.go` | Wire ShowConfirmationFrame from manifest spec |
