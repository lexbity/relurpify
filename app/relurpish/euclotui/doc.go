// Package euclotui provides the Euclo TUI surface — interaction panes,
// event routing, and projection snapshots for the relurpish terminal UI.
//
// # Two-tier stepper
//
// Progress is rendered in two tiers:
//
//	Tier 1 — Macro lifecycle rail: rendered by Stepper.Render using the
//	         surface.MacroPhase from the router snapshot.
//	Tier 2 — Dynamic recipe-step graph: one node per projected step with
//	         paradigm glyph (from theme.ParadigmGlyph), runtime status glyph,
//	         and group topology.
//
// The old static Phase enum (PhaseIdle..PhaseDone) has been removed. The
// Stepper is now purely data-driven from surface.RecipeProjection +
// map[string]surface.StepRuntime + surface.MacroPhase.
//
// # Event-driven router
//
// EucloEventRouter ingests a stream of ExecutionEvent values derived from
// reporting events and interaction frames. It projects them into three views:
//
//	ChatProjection       — human-readable milestone / output / frame feed
//	DiffProjection       — causal code-change hunks grouped by step and file
//	RecipeProjection     — full recipe definition with per-step runtime status
//	StepRuntime map      — live per-step status (active, done, failed, skipped)
//	MacroPhase           — coarse lifecycle phase derived from lifecycle events
//
// The router's Snapshot() returns an immutable EucloProjectionSnapshot that
// panes consume for rendering. Deep-copy semantics are maintained in the
// clone paths of all nested projections.
//
// Resume / durability
//
// RecipeResumeData persists only the recipe ID, per-step statuses, and macro
// phase (never the full recipe structure — see DEC-6). On session resume,
// the recipe is rehydrated from the ThoughtRecipeRegistry via
// RecipeRegistryLookup.
//
// See devdocs/plans/relurpish-rework-spec.md for the full engineering spec.
package euclotui
