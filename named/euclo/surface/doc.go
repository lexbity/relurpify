// Package surface provides UX-agnostic types shared between Euclo and its
// frontends (TUI, LLM prompt providers). It is a leaf package: it imports
// only the Go standard library and must never import thoughtrecipes, reporting,
// interaction, orchestrate, intentcontext, framework/graphdb,
// framework/retrieval, or any app/ package (enforced by arch_test.go).
//
// Canonical types
//
//	Paradigm, MacroPhase, StepStatus      — vocabularies shared by all consumers
//	ThoughtRecipe, ThoughtRecipeStep, …   — DSL structural types (moved here from
//	                                         thoughtrecipes in the rework)
//	RecipeProjection, ProjectedStep, …    — UX-agnostic flattened recipe view,
//	                                         built by BuildRecipeProjection
//	StateView                             — structured runtime state used by both
//	                                         prompt providers and the UI
//	RecipeRegistryLookup                  — minimal interface for recipe rehydration
//	                                         on session resume
//
// # Two-tier UI model
//
// The TUI renders progress in two tiers:
//
//	Tier 1 — Macro lifecycle rail: MacroPhase (idle → intake → route → execute
//	         → verify → done), driven by lifecycle reporting events.
//	Tier 2 — Dynamic recipe-step graph: one node per real recipe step with
//	         paradigm glyph, runtime status (active/done/failed/skipped), and
//	         group topology (parallel/conditional/pipeline).
//
// # Boundary protocol
//
// The runtime delivers state to frontends through three channels:
//  1. reporting events (live stream — recipe.selected, step.started,
//     step.completed, branch.resolved, parallel.fanout, verify.*)
//  2. interaction frames (durable user-facing clarifications / HITL)
//  3. RecipeProjection (reconstructed from the ThoughtRecipeRegistry on
//     session resume, never serialized in full — see DEC-6).
//
// See devdocs/plans/relurpish-rework-spec.md for the full engineering spec.
package surface
