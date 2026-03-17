// Package stages provides pipeline.Stage implementations for chainer links.
//
// Each stage converts a chainer Link into the framework's typed Stage interface,
// enabling deterministic multi-step workflows with contract validation, automatic
// input isolation, and optional checkpointing/resumability.
//
// # BaseStage
//
// BaseStage is a helper struct for common Link→Stage logic:
//   - Template rendering with .Instruction and .Input context
//   - Input key filtering (isolation)
//   - Interaction recording for audit trail
//   - State isolation between links
//
// # LinkStage
//
// LinkStage wraps a chainer Link and implements pipeline.Stage:
//   - Contract() declares input key, output key, schema version, retry policy
//   - BuildPrompt() renders link system prompt with filtered inputs
//   - Decode() invokes Link.Parse (or returns raw text if nil)
//   - Validate() checks output against schema (stub, extended in Phase 5)
//   - Apply() writes result to state at output key
//
// Links are isolated: each stage receives only its declared InputKeys,
// preventing state leakage between sequential prompts.
//
// # Convenience Wrappers
//
// SummarizeStage and TransformStage are pre-built stages for common patterns:
//   - SummarizeStage: generic "summarize available input" stage
//   - TransformStage: parsing transform stage with custom parse function
package stages
