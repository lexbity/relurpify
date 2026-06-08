// Package relurpicabilities implements Euclo's relurpic capability handlers.
//
// Relurpic capabilities are Euclo-owned capabilities with opinionated execution
// behavior that may compose cognition paradigms, ordinary capabilities, and
// workflow-like reasoning.
//
// This package contains the concrete implementations of Euclo's capability
// handlers, which are registered with the capability registry during agent
// initialization.
//
// Capability families implemented:
// - verification: test_run, coverage_check
// - code_understanding: ast_query, symbol_trace, call_graph
// - regression_localization: blame_trace, bisect
// - review_synthesis: code_review, diff_summary
// - architecture: layer_check, boundary_report
// - refactor_patch: targeted_refactor, rename_symbol
// - migration_support: api_compat
package relurpicabilities
