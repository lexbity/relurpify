// Package recipetemplates provides embedded YAML thought-recipe templates for Euclo.
//
// Recipe templates are pre-defined multi-step execution plans that combine
// relurpic capabilities with /agents execution paradigms (react, reflection,
// etc.) to accomplish common coding tasks.
//
// Templates are embedded via go:embed and loaded into a RecipeRegistry during
// agent initialization. The recipe executor then uses these templates to build
// agentgraph execution graphs for tasks routed to the recipe execution path.
//
// Template families:
// - debug/repair: debug_tdd_repair
// - review: code_review
// - investigation: investigation
// - refactor: extract_func
// - implementation/verification: test_synthesis
// - migration: dep_upgrade
package recipetemplates
