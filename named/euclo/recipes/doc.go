// Package recipes implements the ThoughtRecipe execution framework.
//
// ThoughtRecipes are YAML-defined execution plans that specify how to accomplish
// a task through a sequence of steps. Each recipe has:
// - Global configuration shared across all steps
// - A sequence of steps (ordered execution)
// - Optional parallel execution groups
// - Optional conditional execution logic
//
// Step Types:
//   - llm: Invoke an LLM for reasoning or generation
//   - retrieve: Query the knowledge store
//   - ingest: Run the ingestion pipeline
//   - transform: Transform data between formats
//   - emit: Emit an interaction frame to the user
//   - gate: Human-in-the-loop approval gate
//   - branch: Conditional branching
//   - capture: Capture intermediate results
//   - verify: Run verification checks
//   - policy_check: Enforce policy constraints
//   - telemetry: Emit telemetry events
//   - custom: Custom step implementation
//
// Recipe execution is managed by the RecipeExecutorNode in the orchestrate package.
package recipe
