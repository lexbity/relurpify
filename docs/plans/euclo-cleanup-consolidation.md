# Euclo Cleanup and Consolidation Plan

## Context

This plan addresses architectural debt in the Euclo named agent: dead code left over from earlier
capability routing designs, dual execution paths for capability selection, a fragmented mode concept,
and stale dev-agent-cli references. The cleanup is a prerequisite for the thought-recipes
implementation described in `euclo-thought-recipes.md`.

All phases must leave the build clean and all tests passing at their exit point.

---

## Phase 1 — Dead Code Removal

**Goal:** Delete `routing.go` and all its downstream references. Remove the deprecated
`debugSimpleRepairIntent` function. Remove the dead `ExecutorRecipe` field from `Descriptor`.

### Files Affected

| File | Change |
|------|--------|
| `named/euclo/runtime/routing.go` | **Delete entire file** |
| `named/euclo/runtime/types.go` | Remove `CapabilityFamilyRouting` struct (line ~869) |
| `named/euclo/managed_execution.go` | Remove `RouteCapabilityFamilies` call (~line 177) and the state-set call that stores it |
| `named/euclo/runtime/reporting/observability.go` | Replace `euclo.capability_family_routing` read + `PrimaryFamilyID` extraction with a direct inline mapping from mode string to family string (one or two lines) |
| `named/euclo/runtime/workunit.go` | Delete `debugSimpleRepairIntent` function (and its call site; confirm `debugSimpleRepairIntentFromSignals` is already wired) |
| `named/euclo/relurpicabilities/types.go` | Remove `ExecutorRecipe string` field from `Descriptor`; remove any `executor_recipe` YAML tags and usages |

### Mode-to-Family Inline Mapping (replaces routing.go output)

`routing.go`'s only live output was deriving `PrimaryFamilyID` as a trivial mode rename for
observability. Replace with a package-local helper in `observability.go` or `reporting/`:

```go
func primaryFamilyForMode(mode string) string {
    switch mode {
    case "debug":   return "debug"
    case "review":  return "review"
    case "planning": return "planning"
    case "tdd":     return "tdd"
    case "chat":    return "chat"
    default:        return mode
    }
}
```

This is intentionally trivial — `routing.go` never did more than this.

### Test Cleanup

- Delete `TestRouteCapabilityFamilies_SingleMode`, `TestRouteCapabilityFamilies_MultiMode`,
  `TestRouteCapabilityFamilies_UnknownMode`, `TestRouteCapabilityFamilies_EmptyMode` from
  `named/euclo/runtime/classification_test.go` (lines ~604–641).
- Confirm no other test references `RouteCapabilityFamilies`, `CapabilityFamilyRouting`,
  `ExecutorRecipe`, or `debugSimpleRepairIntent`.
- Run `go test ./named/euclo/...` — must pass with no compilation errors.

### Exit Criteria

- `routing.go` does not exist.
- `CapabilityFamilyRouting` type does not appear in any `.go` file.
- `RouteCapabilityFamilies` does not appear in any `.go` file.
- `ExecutorRecipe` does not appear in any `.go` file.
- `debugSimpleRepairIntent` (non-signals variant) does not appear in any `.go` file.
- `go build ./...` passes.
- `go test ./named/euclo/...` passes.

---

## Phase 2 — Capability Selection Consolidation

**Goal:** Establish `CapabilityIntentClassifier` as the single, always-executed path for
capability selection. Eliminate the parallel keyword-scanning logic in `workunit.go`. Move the
inline keyword detection functions (`planningExploreIntent`, `planningCompileIntent`,
`planningImplementIntent`) out of workunit.go and into the `Descriptor` keyword lists where they
belong, so the classifier picks them up automatically.

### Problem Statement

Two parallel paths currently produce the same output:

1. **Authoritative path:** `CapabilityIntentClassifier.Classify()` in `capability_classifier.go`
   populates `envelope.CapabilitySequence`.
2. **Legacy fallback:** `primaryRelurpicCapabilityForWork` in `workunit.go` rescans keywords if
   `CapabilitySequence` is empty *and* contains its own internal keyword-detection functions.

These must converge. `CapabilityIntentClassifier` always runs; `primaryRelurpicCapabilityForWork`
reads from `CapabilitySequence` only.

### Changes

**`named/euclo/runtime/workunit.go`**

- Remove `planningExploreIntent`, `planningCompileIntent`, `planningImplementIntent` functions.
- Simplify `primaryRelurpicCapabilityForWork` to:
  1. If `envelope.CapabilitySequence` is non-empty, return `CapabilitySequence[0]` (existing behaviour).
  2. Return an error or the mode's fallback capability — do not re-scan keywords internally.
- Remove any internal `strings.Contains`/`hasKeyword` calls that duplicate classifier logic.
- Remove `debugSimpleRepairIntentFromSignals` call-site duplication if any remains after Phase 1.

**`named/euclo/relurpicabilities/types.go` (or the descriptors registration)**

Move the keyword intent logic from workunit.go functions into `Descriptor.Keywords` on the
corresponding capability descriptors:

| Removed function | Target Descriptor | Keywords to add |
|---|---|---|
| `planningExploreIntent` | `euclo:planning.explore` (or equivalent) | `"explore"`, `"understand codebase"`, `"map"`, `"navigate"`, etc. |
| `planningCompileIntent` | `euclo:planning.compile` | `"compile"`, `"build"`, `"package"`, etc. |
| `planningImplementIntent` | `euclo:planning.implement` | `"implement"`, `"write"`, `"create feature"`, etc. |

Exact keyword lists should be reviewed against the removed functions' logic to ensure no signal loss.

**`named/euclo/runtime/pretask.go` or the call site that invokes `CapabilityIntentClassifier`**

- Confirm `CapabilityIntentClassifier.Classify()` is unconditionally called and that its result
  is always written to `envelope.CapabilitySequence` before `primaryRelurpicCapabilityForWork`
  is reached.
- If there are early-exit code paths that skip classification, remove them.

### Test Cleanup

- Remove any unit tests that test the deleted `planningExploreIntent` / `planningCompileIntent` /
  `planningImplementIntent` as standalone functions.
- Add or update `TestCapabilityIntentClassifier_*` tests to cover the keyword cases previously
  handled by those functions (confirm planning mode keywords produce correct `CapabilitySequence`).
- Add a regression test: `primaryRelurpicCapabilityForWork` must not produce a different
  capability ID than what the classifier put in `CapabilitySequence` for the same instruction.

### Exit Criteria

- `planningExploreIntent`, `planningCompileIntent`, `planningImplementIntent` do not exist as
  standalone functions in workunit.go.
- `primaryRelurpicCapabilityForWork` contains no `strings.Contains` / keyword-scanning logic of
  its own.
- For every test input that previously routed through the legacy path, `CapabilityIntentClassifier`
  produces the same capability selection via Descriptor keywords.
- `go test ./named/euclo/...` passes.

---

## Phase 3 — Descriptor Multi-Mode Extension

**Goal:** Extend `Descriptor` to support multiple mode families per capability (required for
thought recipes and multi-mode capabilities). Deprecate the single `ModeFamily string` field.
Add `TriggerPriority int` for tie-breaking.

### Changes

**`named/euclo/relurpicabilities/types.go`**

```go
// Replace:
ModeFamily string

// With:
ModeFamilies []string   // ordered; first entry is the primary mode
TriggerPriority int     // higher = considered first during keyword tie-breaking (default 0)
```

- Keep `ModeFamily` as a computed accessor for backward compat within this package only:
  ```go
  func (d Descriptor) PrimaryMode() string {
      if len(d.ModeFamilies) == 0 { return "" }
      return d.ModeFamilies[0]
  }
  ```
- Update `DefaultRegistry()` — change all `ModeFamily: "x"` assignments to
  `ModeFamilies: []string{"x"}`.
- Update `MatchByKeywords` and `PrimaryCapabilitiesForMode` to filter against `ModeFamilies`
  (contains check, not equality).
- Update `FallbackCapabilityForMode` similarly.

**`named/euclo/relurpicabilities/registry.go`** (if it exists separately)

- Any range/filter over descriptors using `d.ModeFamily` must switch to `d.ModeFamilies`.

**`named/euclo/runtime/capability_classifier.go`**

- `staticKeywordMatch` calls `MatchByKeywords` — no signature change needed if the registry
  method is updated.
- `modeDefaultFallback` calls `FallbackCapabilityForMode` — no change needed at call site.

**`named/euclo/runtime/workunit.go`**

- Any remaining `d.ModeFamily` references → use `d.PrimaryMode()` or `d.ModeFamilies`.

**`app/dev-agent-cli/euclo_cmd.go`**

- `show capability` and `list capabilities` output: replace `ModeFamily` display with
  `ModeFamilies` (comma-joined or listed).
- No behavioral change — cosmetic output update only.

### Test Cleanup

- Update existing descriptor tests that assert `d.ModeFamily` to assert `d.ModeFamilies[0]` or
  `d.PrimaryMode()`.
- Add a test: a descriptor with `ModeFamilies: []string{"debug", "review"}` must appear in both
  `PrimaryCapabilitiesForMode("debug")` and `PrimaryCapabilitiesForMode("review")` results.

### Exit Criteria

- `ModeFamily string` field removed from `Descriptor` struct (replaced by `ModeFamilies []string`).
- All internal usages updated.
- `TriggerPriority int` field added to `Descriptor`.
- `go build ./...` passes.
- `go test ./named/euclo/...` and `go test ./app/dev-agent-cli/...` pass.

---

## Phase 4 — Mode Registration Consolidation

**Goal:** Establish a single canonical source for valid mode IDs that feeds `collectKeywordSignals`
(signals.go), `ModeMachineRegistry` (interaction/registry.go), and `Descriptor.ModeFamilies`
registration. Eliminate the hard-coded keyword-group mode strings in signals.go as an independent
definition point.

### Problem Statement

Mode IDs are currently defined in at least three independent places:

1. `signals.go` — `keywordGroup.mode` strings inside `collectKeywordSignals`
2. `interaction/registry.go` — keys in `ModeMachineRegistry`
3. `Descriptor.ModeFamily` (now `ModeFamilies`) on each registered capability

There is no single location that enumerates all valid modes. A typo or rename in one place is not
caught by the others. This also means `collectKeywordSignals` partially re-implements the
classifier's keyword→mode mapping with a different, non-extensible mechanism.

### Changes

**New file: `named/euclo/relurpicabilities/modes.go`** (or add to `types.go`)

Define canonical mode constants:

```go
const (
    ModeDebug    = "debug"
    ModeReview   = "review"
    ModePlanning = "planning"
    ModeTDD      = "tdd"
    ModeCode     = "code"
    ModeChat     = "chat"
)

// AllModes returns all registered mode IDs in priority order.
func AllModes() []string {
    return []string{ModeDebug, ModeReview, ModePlanning, ModeTDD, ModeCode, ModeChat}
}
```

**`named/euclo/runtime/signals.go`**

- Replace hard-coded mode string literals in `collectKeywordSignals` groups with the constants
  from `relurpicabilities`.
- **Do not remove `collectKeywordSignals`** — it serves a different purpose than the classifier
  (pre-classification signal weighting for mode routing), but it must use canonical mode names.
- Add `collectUserRecipeSignals` stub (returns nil / empty) to establish the extension point for
  thought recipes. Document with a TODO referencing `euclo-thought-recipes.md`.

**`named/euclo/interaction/registry.go`**

- Replace mode string literals in `ModeMachineRegistry` registration calls with the constants.

**`named/euclo/relurpicabilities/types.go`**

- Replace `ModeFamilies` string literals in `DefaultRegistry()` capability registrations with
  the new constants.

### Test Cleanup

- Add a test: every mode returned by `AllModes()` has at least one descriptor registered in
  `DefaultRegistry()` with that mode in `ModeFamilies`.
- Add a test: every mode returned by `AllModes()` has a registered factory in `ModeMachineRegistry`.
- Add a test: every `keywordGroup.mode` string in `collectKeywordSignals` appears in `AllModes()`.

### Exit Criteria

- No bare mode string literals (`"debug"`, `"review"`, `"planning"`, `"tdd"`, `"code"`, `"chat"`)
  outside of `modes.go` / the constants file.
- `AllModes()` function exists and is used by the new tests.
- `collectUserRecipeSignals` stub exists with a documented extension point.
- `go test ./named/euclo/...` passes including new canonical-mode coverage tests.

---

## Phase 5 — dev-agent-cli Updates

**Goal:** Update `app/dev-agent-cli` to reflect all structural changes from Phases 1–4. Remove
references to deleted types. Update capability display output for the new `Descriptor` shape.
Ensure `agenttest_cmd.go` is consistent with any `ExpectSpec` changes introduced by ongoing
OSB redesign work.

### Changes

**`app/dev-agent-cli/euclo_cmd.go`**

- Remove any references to `CapabilityFamilyRouting`, `RouteCapabilityFamilies`, or
  `ExecutorRecipe` (should be gone after Phase 1, but verify here).
- Update `show capability` output: display `ModeFamilies` (from Phase 3) instead of `ModeFamily`.
- Update `list capabilities` table/matrix output: use `d.PrimaryMode()` or `d.ModeFamilies[0]`
  where single-mode display is needed; use comma-joined `ModeFamilies` where full display is shown.
- Update `TriggerPriority` display in `show capability` if the field is user-visible.
- Remove any `--executor-recipe` flags or references if they exist.

**`app/dev-agent-cli/agenttest_cmd.go`**

- Verify `runCapabilityTargeted` references to `c.Expect.Euclo.PrimaryRelurpicCapability` and
  `c.Expect.Euclo.SupportingRelurpicCapabilities` still compile cleanly after any `EucloExpectSpec`
  changes from the OSB assertion redesign (tracked separately).
- If `EucloExpectSpec` has been updated (per the OSB plan), update `runCapabilityTargeted` to
  use the new field names / assertion structure.
- Confirm `--capability` flag filtering still works against capability IDs (no change expected,
  but verify).
- Confirm `tapes` and `migrate` subcommands still compile cleanly.

**`app/dev-agent-cli/`** (general)

- Run `go vet ./app/dev-agent-cli/...` and resolve all warnings.
- Confirm the CLI still builds as a standalone binary.

### Test Cleanup

- If any `euclo_cmd_test.go` or `agenttest_cmd_test.go` tests assert on `ModeFamily`,
  `ExecutorRecipe`, or routing output, update them.
- Run `go test ./app/dev-agent-cli/...` — must pass.

### Exit Criteria

- `app/dev-agent-cli` compiles cleanly with no references to deleted types.
- `list capabilities` and `show capability` commands display correct output for the updated
  `Descriptor` shape.
- `go test ./app/dev-agent-cli/...` passes.
- `go build ./app/dev-agent-cli/...` produces a working binary.

---

## Phase 6 — Final Cleanup and Verification

**Goal:** Full build, test, and boundary-script verification pass. Confirm coverage targets.
Update inline comments that reference deleted concepts. Remove any remaining stale TODO comments
from deleted code paths.

### Checklist

**Build and test:**
- `go build ./...` — clean
- `go test ./...` — all pass, no skipped tests introduced during cleanup
- `go vet ./...` — no warnings

**Boundary scripts:**
- `scripts/check-framework-boundaries.sh` — passes
- `scripts/check-deprecated-agent-wrappers.sh` — passes

**Dead-reference sweep (grep for):**
- `RouteCapabilityFamilies` — zero matches
- `CapabilityFamilyRouting` — zero matches
- `ExecutorRecipe` — zero matches
- `debugSimpleRepairIntent` (without `FromSignals`) — zero matches
- `ModeFamily` as a struct field — zero matches (only `ModeFamilies` and `PrimaryMode()` remain)
- `routing.go` as a filename — zero matches

**Comment and documentation cleanup:**
- Remove or update any inline comments in `workunit.go`, `managed_execution.go`,
  `capability_classifier.go`, and `observability.go` that reference the old routing design,
  the dual-path selection, or the deprecated functions.
- Update `CLAUDE.md` references if any describe the deleted routing mechanism (none expected,
  but check).

**Coverage:**
- Verify `go test -cover ./named/euclo/...` does not regress from the pre-cleanup baseline.
- Verify `go test -cover ./app/dev-agent-cli/...` does not regress.

**Optional: code size check**
Run `wc -l` on the key files before and after; verify net reduction. This is a cleanup plan —
the euclo package should be smaller at exit.

### Exit Criteria

- All items in the checklist above pass with no exceptions.
- The `named/euclo/` package compiles and all tests pass on a clean checkout.
- The `app/dev-agent-cli/` package compiles and all tests pass.
- No grep matches for deleted symbols across the entire repo.
- The `euclo-thought-recipes.md` implementation plan can be started from this clean baseline
  without any of its "groundwork required" items remaining open.

---

## Phase Dependency Order

```
Phase 1 (Dead Code)
    → Phase 2 (Selection Consolidation)
        → Phase 3 (Descriptor Multi-Mode)
            → Phase 4 (Mode Registration)
                → Phase 5 (dev-agent-cli)
                    → Phase 6 (Final Verification)
```

Phases 3 and 4 have no ordering dependency on each other and may be done concurrently if working
across multiple branches, but Phase 5 depends on both.

---

## Relationship to Thought Recipes

This plan is a prerequisite, not a parallel track. Thought recipes require:

- A single authoritative capability classifier (Phase 2)
- `Descriptor.ModeFamilies` for multi-mode recipe registration (Phase 3)
- Canonical mode constants for recipe YAML validation (Phase 4)
- `collectUserRecipeSignals` extension point (Phase 4)

After Phase 6 passes, the thought-recipes implementation plan in `euclo-thought-recipes.md`
can begin at Phase P0 (Dynamic Resolution Infrastructure) without any pre-existing debt.
