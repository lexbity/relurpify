# Contextmgr Strategy Consolidation Plan

## Objective

Collapse the three concrete context strategy types (`AggressiveStrategy`,
`ConservativeStrategy`, `AdaptiveStrategy`) into a single `ProfiledStrategy`
implementation backed by a `StrategyProfile` data type. The goal is to eliminate
threshold drift between strategy personalities, reduce the implementation surface
that tests must cover, and create a first-class profile vocabulary that euclo's
named strategy IDs can eventually resolve against directly.

This is a behaviour-preserving refactor. No agent-facing API changes, no
functional regressions.

## Motivation

Captured in `docs/research/codesmell-framework-contextmgr.md`. Key points:

- `ShouldCompress`, `DetermineDetailLevel`, and `PrioritizeContext` each encode
  the same decision shape three times with different thresholds. A change to one
  personality does not propagate.
- `AdaptiveStrategy.DetermineDetailLevel` re-encodes the Conservative and
  Aggressive thresholds inline in a switch block — two sources of truth for the
  same values.
- The three-type model requires test authors to keep coverage aligned across all
  three files. A missing branch can hide in one strategy even when the others are
  well covered.

## Scope

**In scope**

- `framework/contextmgr/aggressive_strategy.go` — deleted
- `framework/contextmgr/conservative_strategy.go` — deleted
- `framework/contextmgr/adaptive_strategy.go` — reduced to mode-switching logic only
- `framework/contextmgr/strategy_profile.go` — created
- `framework/contextmgr/profiled_strategy.go` — created
- `agents/context_strategy.go` — constructor return type updates only
- `named/euclo/runtime/context/runtime_impl.go` — named strategy ID resolution

**Out of scope**

- `framework/contextmgr/pruning_strategy.go` — separate concern, not targeted
- `framework/contextmgr/context_policy.go` — `ContextPolicy` complexity is a
  separate codesmell item; do not touch here
- `app/relurpish/**` — excluded per standing constraint
- Any agent behaviour changes

## Constraints

- `go build ./...` must pass at the end of every phase.
- `go test ./...` must pass at the end of every phase.
- The `ContextStrategy` interface signature is frozen — no method additions or
  removals.
- Constructor names `NewAggressiveStrategy`, `NewConservativeStrategy`,
  `NewAdaptiveStrategy` must remain available and callable by existing callers.
- `AdaptiveStrategy` stays as a named type because callers may hold it by
  concrete type (tests write to `adaptive.contextLoadHistory`,
  `adaptive.currentMode` directly).

---

## Phase 1: `StrategyProfile` Foundation

**Goal**: define the profile type and supporting enumerations. No behaviour
change. No deletions. Existing code compiles unchanged alongside the new types.

### New file: `framework/contextmgr/strategy_profile.go`

Define the following types:

```go
// DetailBand maps a minimum relevance threshold to a detail level.
// Bands are evaluated high-to-low; the first matching band wins.
type DetailBand struct {
    MinRelevance float64
    Level        DetailLevel
}

// PrioritizationMode controls how PrioritizeContext ranks items.
type PrioritizationMode int

const (
    PrioritizationRelevance PrioritizationMode = iota // sort by RelevanceScore descending
    PrioritizationRecency                             // sort by Age ascending
    PrioritizationWeighted                            // weighted mix of relevance and recency
)

// ExpandTriggerKind controls which runtime signals cause ShouldExpandContext
// to return true.
type ExpandTriggerKind int

const (
    ExpandOnErrorType           ExpandTriggerKind = iota // expand on specific error_type values
    ExpandOnToolUse                                      // expand after specific tool_used values
    ExpandOnFailureOrUncertainty                         // expand on failure or uncertainty markers
)

// StrategyProfile holds all parameterisable decisions for a context strategy
// personality. A zero-value profile is valid (no compression, no expansion, no
// detail banding).
type StrategyProfile struct {
    Name string

    // ShouldCompress threshold: compress when history length exceeds this value.
    // Zero means never compress via this strategy.
    CompressThreshold int

    // DetermineDetailLevel: bands evaluated high-to-low by MinRelevance.
    // If no band matches, DetailSignatureOnly is returned.
    DetailBands []DetailBand

    // PrioritizeContext ordering.
    PrioritizationMode PrioritizationMode
    RelevanceWeight    float64 // used when PrioritizationWeighted
    RecencyWeight      float64 // used when PrioritizationWeighted

    // SelectContext: fraction of budget.AvailableForContext assigned to MaxTokens.
    TokenBudgetFraction float64

    // AST listing filter for the initial symbol query.
    ASTExportedOnly bool

    // File loading behaviour for explicitly referenced files.
    FileDetailLevel DetailLevel
    FilePinned      bool // pin all referenced files
    PinFirstN       int  // pin only the first N files (overrides FilePinned when > 0)

    // Load AST dependency queries for each referenced file.
    LoadDependencies bool

    // Keyword/full-instruction search. Zero SearchMaxResults disables search.
    SearchMaxResults         int
    SearchUseFullInstruction bool // true = full instruction text; false = keyword extract

    // Memory loading. Zero MemoryMaxResults disables memory queries.
    LoadMemory       bool
    MemoryMaxResults int

    // ShouldExpandContext trigger.
    ExpandTrigger    ExpandTriggerKind
    ExpandErrorTypes []string // matched against result.Data["error_type"]
    ExpandToolTypes  []string // matched against result.Data["tool_used"]
}
```

Define the three preset profile variables. Their field values must exactly
reproduce the behaviour of the concrete strategy types they replace:

```go
var AggressiveProfile = StrategyProfile{
    Name:                "aggressive",
    CompressThreshold:   5,
    DetailBands: []DetailBand{
        {MinRelevance: 0.9, Level: DetailDetailed},
        {MinRelevance: 0.7, Level: DetailConcise},
        {MinRelevance: 0.5, Level: DetailMinimal},
        {MinRelevance: 0.0, Level: DetailSignatureOnly},
    },
    PrioritizationMode:  PrioritizationRecency,
    TokenBudgetFraction: 0.25,
    ASTExportedOnly:     true,
    FileDetailLevel:     DetailSignatureOnly,
    FilePinned:          false,
    SearchMaxResults:    0,
    LoadMemory:          false,
    ExpandTrigger:       ExpandOnErrorType,
    ExpandErrorTypes:    []string{"insufficient_context", "file_not_found"},
}

var ConservativeProfile = StrategyProfile{
    Name:                     "conservative",
    CompressThreshold:        15,
    DetailBands: []DetailBand{
        {MinRelevance: 0.8, Level: DetailFull},
        {MinRelevance: 0.5, Level: DetailDetailed},
        {MinRelevance: 0.0, Level: DetailConcise},
    },
    PrioritizationMode:       PrioritizationRelevance,
    TokenBudgetFraction:      0.75,
    ASTExportedOnly:          false,
    FileDetailLevel:          DetailDetailed,
    FilePinned:               true,
    LoadDependencies:         true,
    SearchMaxResults:         20,
    SearchUseFullInstruction: true,
    LoadMemory:               true,
    MemoryMaxResults:         10,
    ExpandTrigger:            ExpandOnToolUse,
    ExpandToolTypes:          []string{"search", "query_ast"},
}

var BalancedProfile = StrategyProfile{
    Name:                "balanced",
    CompressThreshold:   10,
    DetailBands: []DetailBand{
        {MinRelevance: 0.85, Level: DetailFull},
        {MinRelevance: 0.6,  Level: DetailDetailed},
        {MinRelevance: 0.0,  Level: DetailConcise},
    },
    PrioritizationMode:  PrioritizationWeighted,
    RelevanceWeight:     0.6,
    RecencyWeight:       0.4,
    TokenBudgetFraction: 0.5,
    ASTExportedOnly:     true,
    FileDetailLevel:     DetailConcise,
    PinFirstN:           2,
    SearchMaxResults:    10,
    LoadMemory:          false,
    ExpandTrigger:       ExpandOnFailureOrUncertainty,
}
```

### Tests: `strategy_profile_test.go`

- Verify each preset profile has the expected field values (regression guard
  against accidental threshold drift).
- Verify `DetailBand` slices are ordered correctly (highest `MinRelevance`
  first) — add a helper `ValidateProfileBands` and test it against all presets.
- Verify zero-value `StrategyProfile` is representable without panicking.

---

## Phase 2: `ProfiledStrategy` Implementation

**Goal**: a single `ContextStrategy` implementation that is fully driven by
`StrategyProfile`. All five interface methods implemented here. At this point
the original concrete types still exist — nothing is deleted.

### New file: `framework/contextmgr/profiled_strategy.go`

```go
type ProfiledStrategy struct {
    Profile StrategyProfile
}

func NewStrategyFromProfile(p StrategyProfile) *ProfiledStrategy {
    return &ProfiledStrategy{Profile: p}
}
```

Implement all five methods:

**`SelectContext`**

1. Compute `MaxTokens = int(float64(budget.AvailableForContext) * Profile.TokenBudgetFraction)`.
2. Always add one `ASTQueryListSymbols` with `ExportedOnly = Profile.ASTExportedOnly`.
3. For each file from `ExtractFileReferences`:
   - Determine `pinned`: if `Profile.PinFirstN > 0`, pin only the first N; else
     use `Profile.FilePinned`.
   - Set `Priority = 0` for pinned files, `Priority = 1` otherwise.
   - Use `Profile.FileDetailLevel`.
4. If `Profile.LoadDependencies` and files were found, append
   `ASTQueryGetDependencies` per file.
5. If `Profile.LoadDependencies` and no files were found, or if
   `Profile.SearchMaxResults > 0`, append a `SearchQuery`:
   - Text: full instruction if `Profile.SearchUseFullInstruction`, else
     `ExtractKeywords`.
   - MaxResults: `Profile.SearchMaxResults`.
   - Mode: `search.SearchHybrid`.
   - Skip search when `Profile.SearchMaxResults == 0`.
6. If `Profile.LoadMemory`, append a `MemoryQuery` with
   `Scope = memory.MemoryScopeProject`, `Query = task.Instruction`,
   `MaxResults = Profile.MemoryMaxResults`.
7. Call `AppendContextFiles(request, task, DetailFull)`.

**`ShouldCompress`**

```go
if Profile.CompressThreshold == 0 { return false }
return len(ctx.History()) > Profile.CompressThreshold
```

**`DetermineDetailLevel`**

Walk `Profile.DetailBands` (expected high-to-low). Return the level of the
first band whose `MinRelevance <= relevance`. If no band matches return
`DetailSignatureOnly`.

**`ShouldExpandContext`**

Dispatch on `Profile.ExpandTrigger`:

- `ExpandOnErrorType`: return false on success; check
  `result.Data["error_type"]` against `Profile.ExpandErrorTypes`.
- `ExpandOnToolUse`: check `result.Data["tool_used"]` against
  `Profile.ExpandToolTypes`.
- `ExpandOnFailureOrUncertainty`: return true on failure; on success check
  `result.Data["llm_output"]` for uncertainty markers (same set used by the
  current `AdaptiveStrategy`).

**`PrioritizeContext`**

Dispatch on `Profile.PrioritizationMode`:

- `PrioritizationRelevance`: sort by `RelevanceScore()` descending.
- `PrioritizationRecency`: sort by `Age()` ascending.
- `PrioritizationWeighted`: composite score
  `RelevanceScore()*RelevanceWeight + (1/(1+Age().Hours()))*RecencyWeight`,
  sort descending.

### Tests: `profiled_strategy_test.go`

Write table-driven tests that cover all three preset profiles against each
method. For each `(profile, input) → expected` case, verify that
`ProfiledStrategy` produces identical output to what the corresponding concrete
strategy would have produced. This is the behavioural equivalence gate.

Specific cases required:

- `SelectContext`: aggressive (files found), conservative (files found),
  conservative (no files), balanced (files found).
- `ShouldCompress`: below threshold, at threshold, above threshold for each profile.
- `DetermineDetailLevel`: all relevance breakpoints for each profile's band
  definitions.
- `ShouldExpandContext`: success/failure/nil cases; tool-use trigger; error-type
  trigger; uncertainty-marker trigger.
- `PrioritizeContext`: all three prioritization modes with a mixed item set.
- `NewStrategyFromProfile` with zero-value profile: verify no panic.

---

## Phase 3: `AdaptiveStrategy` Delegation

**Goal**: strip the duplicate threshold logic out of `AdaptiveStrategy`. It
becomes a profile-switcher that holds three `ProfiledStrategy` instances and
delegates all five `ContextStrategy` methods to the active one.

### Modify `adaptive_strategy.go`

**Keep intact**:

- `StrategyMode` type and `ModeAggressive`, `ModeBalanced`, `ModeConservative`
  constants.
- `AdaptiveStrategy` struct fields: `contextLoadHistory`, `successRate`,
  `currentMode`, `lowSuccessThreshold`, `highSuccessThreshold`.
- `analyzeTaskComplexity` — no change.
- `adjustMode` — no change.
- `ShouldExpandContext` — keep as-is; it has unique logic (uncertainty marker
  detection + history recording) that is not expressed by `ExpandTrigger`.

**Remove**:

- The inline `switch as.currentMode` blocks inside `ShouldCompress` and
  `DetermineDetailLevel`.
- `selectBalancedContext` — replaced by `BalancedProfile`.
- The `SelectContext` delegation calls to `NewAggressiveStrategy()` and
  `NewConservativeStrategy()` inside a switch.

**Add**:

```go
type AdaptiveStrategy struct {
    // ... existing fields ...
    aggressive   *ProfiledStrategy
    balanced     *ProfiledStrategy
    conservative *ProfiledStrategy
}

func NewAdaptiveStrategy() *AdaptiveStrategy {
    return &AdaptiveStrategy{
        contextLoadHistory:   make([]ContextLoadEvent, 0),
        successRate:          make(map[string]float64),
        currentMode:          ModeBalanced,
        lowSuccessThreshold:  0.6,
        highSuccessThreshold: 0.85,
        aggressive:           NewStrategyFromProfile(AggressiveProfile),
        balanced:             NewStrategyFromProfile(BalancedProfile),
        conservative:         NewStrategyFromProfile(ConservativeProfile),
    }
}

func (as *AdaptiveStrategy) activeStrategy() *ProfiledStrategy {
    switch as.currentMode {
    case ModeAggressive:
        return as.aggressive
    case ModeConservative:
        return as.conservative
    default:
        return as.balanced
    }
}
```

Rewrite `SelectContext`, `ShouldCompress`, `DetermineDetailLevel`, and
`PrioritizeContext` to call through `as.activeStrategy()`.

### Tests

All existing `AdaptiveStrategy` tests in `contextmgr_phase2_test.go` must
continue to pass without modification. Add targeted tests for:

- `activeStrategy()` returns the correct `ProfiledStrategy` for each mode.
- `NewAdaptiveStrategy()` initialises all three inner strategies.
- After `adjustMode` transitions mode, the delegated method produces the
  behaviour of the newly active profile.

---

## Phase 4: Constructor Consolidation and Dead Code Removal

**Goal**: redirect the public constructors to return `ProfiledStrategy`, delete
the two concrete strategy files, and clean up the `agents` shim layer.

### Changes

**`framework/contextmgr`**

Replace `aggressive_strategy.go` and `conservative_strategy.go` with
constructor redirects kept in `profiled_strategy.go`:

```go
func NewAggressiveStrategy() *ProfiledStrategy {
    return NewStrategyFromProfile(AggressiveProfile)
}

func NewConservativeStrategy() *ProfiledStrategy {
    return NewStrategyFromProfile(ConservativeProfile)
}
```

Delete `aggressive_strategy.go` and `conservative_strategy.go`.

**`agents/context_strategy.go`**

Update the constructor wrappers to reflect the new return types:

```go
func NewAggressiveStrategy() *contextmgr.ProfiledStrategy {
    return contextmgr.NewAggressiveStrategy()
}
func NewConservativeStrategy() *contextmgr.ProfiledStrategy {
    return contextmgr.NewConservativeStrategy()
}
```

`NewAdaptiveStrategy` return type is unchanged (`*contextmgr.AdaptiveStrategy`).

All callers in `agents/react/react.go` and `agents/rewoo/initialize.go` assign
to `contextmgr.ContextStrategy` interface variables — no changes required.

### Verification

- `go build ./...` passes.
- `go test ./...` passes.
- No references to `AggressiveStrategy` or `ConservativeStrategy` as types
  remain outside of `profiled_strategy.go` (grep check).

---

## Phase 5: Euclo Named Strategy Integration

**Goal**: make euclo's named strategy IDs ("narrow_to_wide",
"localize_then_expand", "targeted", "read_heavy", "expand_carefully") resolve
to named `StrategyProfile` instances rather than falling through to one of the
three original concrete types.

### Profile registry

Add a `ProfileRegistry` in `strategy_profile.go`:

```go
// profileRegistry is the canonical map from named strategy ID to profile.
var profileRegistry = map[string]StrategyProfile{
    "aggressive":          AggressiveProfile,
    "balanced":            BalancedProfile,
    "conservative":        ConservativeProfile,
    "narrow_to_wide":      ConservativeProfile, // reads broadly up front
    "localize_then_expand": BalancedProfile,    // starts focused, expands
    "targeted":            AggressiveProfile,   // minimal footprint
    "read_heavy":          ConservativeProfile, // maximise preloaded context
    "expand_carefully":    BalancedProfile,     // expand on evidence only
}

// LookupProfile returns the profile registered under name, or false if unknown.
func LookupProfile(name string) (StrategyProfile, bool) {
    p, ok := profileRegistry[name]
    return p, ok
}
```

The euclo named IDs map to the closest-matching preset personality. If a future
mode requires a custom profile the registry entry can be updated independently
without changing the strategy implementation.

### Modify `named/euclo/runtime/context/runtime_impl.go`

Replace `selectContextStrategy` with a profile-based lookup:

```go
func selectContextStrategy(mode eucloruntime.ModeResolution, work eucloruntime.UnitOfWork) (contextmgr.ContextStrategy, string) {
    // Prefer explicit strategy ID on the work unit.
    if id := strings.TrimSpace(work.ContextStrategyID); id != "" {
        if profile, ok := contextmgr.LookupProfile(id); ok {
            return contextmgr.NewStrategyFromProfile(profile), id
        }
    }
    // Fall back to mode-derived strategy.
    switch mode.ModeID {
    case "review", "debug", "archaeology":
        p, _ := contextmgr.LookupProfile("conservative")
        return contextmgr.NewStrategyFromProfile(p), "conservative"
    case "planning":
        p, _ := contextmgr.LookupProfile("aggressive")
        return contextmgr.NewStrategyFromProfile(p), "aggressive"
    default:
        return contextmgr.NewAdaptiveStrategy(), "adaptive"
    }
}
```

### Tests

- `LookupProfile` returns a profile for each registered name.
- `LookupProfile` returns false for an unregistered name.
- `selectContextStrategy` resolves correctly for each mode branch and for an
  explicit `ContextStrategyID`.
- Existing `runtime_impl_more_test.go` tests continue to pass.

---

## Phase 6: Coverage and Codesmell Resolution

**Goal**: ensure the consolidated code is well-covered, and update the codesmell
note to reflect the resolved items.

### Coverage targets

| File | Target |
|---|---|
| `strategy_profile.go` | 100% (data + registry) |
| `profiled_strategy.go` | ≥ 90% |
| `adaptive_strategy.go` (post-refactor) | ≥ 85% |

### Test additions

- Table-driven `ProfiledStrategy` tests covering all `DetailBand` boundary
  conditions, including an empty `DetailBands` slice (fallback to
  `DetailSignatureOnly`).
- `ShouldCompress` with `CompressThreshold = 0` (never compress).
- `SelectContext` with a `TokenBudgetFraction` of 0.0 and 1.0.
- `PrioritizeContext` with an empty item slice.
- `PrioritizationWeighted` with zero weights.
- `LookupProfile` exhaustive: all registered names, a missing name.
- Regression tests asserting the existing `contextmgr_phase2_test.go`
  behavioural assertions still hold after type changes.

### Codesmell note update

Update `docs/research/codesmell-framework-contextmgr.md`:

- Mark the strategy duplication observation as resolved.
- Add a note that `ContextPolicy` complexity remains an open item if it
  becomes a future refactor target.

---

## File Change Summary

| File | Action |
|---|---|
| `framework/contextmgr/aggressive_strategy.go` | Deleted |
| `framework/contextmgr/conservative_strategy.go` | Deleted |
| `framework/contextmgr/strategy_profile.go` | Created |
| `framework/contextmgr/profiled_strategy.go` | Created |
| `framework/contextmgr/strategy_profile_test.go` | Created |
| `framework/contextmgr/profiled_strategy_test.go` | Created |
| `framework/contextmgr/adaptive_strategy.go` | Modified |
| `agents/context_strategy.go` | Modified (return types only) |
| `named/euclo/runtime/context/runtime_impl.go` | Modified |
| `docs/research/codesmell-framework-contextmgr.md` | Updated |

Net: 2 files deleted, 4 files created, 4 files modified.
