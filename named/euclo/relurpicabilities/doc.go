// Package relurpicabilities implements Euclo's relurpic capability handlers.
//
// Each handler is constructed with explicit family-specific dependency contracts
// (CommandDeps, IndexDeps, WorkspaceFiles+IndexRefresher, SymbolQuerier,
// model.LanguageModel). Registration uses RegistrationDeps{Registry, Declared}.
//
// Capability families:
// - command: test_run, diff_summary, bisect, api_compat, coverage_check, blame_trace
// - index read: ast_query, symbol_trace, call_graph, layer_check, boundary_report
// - workspace mutation: targeted_refactor, rename_symbol
// - model synthesis: code_review
package relurpicabilities
