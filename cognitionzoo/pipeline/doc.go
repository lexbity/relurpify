// Package pipeline provides a deterministic staged LLM execution model for
// agents that need structured, type-safe multi-step workflows.
//
// # Stages
//
// A Stage is a self-contained unit of work with a declared contract:
//
//   - BuildPrompt: constructs the LLM prompt for this stage.
//   - Decode: parses the raw LLM response into a typed result.
//   - Validate: checks the result against the stage's schema contract.
//   - Apply: writes the result into downstream runtime state for later stages.
//
// # ContractDescriptor
//
// Every stage declares a ContractDescriptor naming its input key, output key,
// schema version, and retry policy. The runner enforces these contracts at
// runtime and persists stage results to the lifecycle repository so
// interrupted pipelines resume from the last completed stage.
//
// # Runner
//
// Runner executes stages sequentially, threading a shared runtime-state map through
// each step. On validation failure it retries according to the stage's retry
// policy before propagating an error.
package pipeline
