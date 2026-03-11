// Package contextmgr manages the LLM context window for long-running agent
// sessions, keeping token usage within model limits through progressive
// compression and selective pruning.
//
// # Strategies
//
// Four built-in compression strategies are provided, selected by ContextPolicy:
//
//   - Conservative: retains as much content as possible; drops only stale items.
//   - Adaptive: adjusts compression aggressiveness based on remaining budget.
//   - Pruning: removes the least-recently-accessed items first.
//   - Aggressive: maximum compression; reduces all non-essential items to metadata.
//
// # ContextManager
//
// ContextManager is the primary entry point. It tracks Context items against
// a token budget derived from the model's max_tokens setting, rebuilds the
// live prompt from compact state each iteration, and signals the agent loop
// when budget is critically low.
//
// # ProgressiveLoader
//
// ProgressiveLoader defers loading file contents until the agent actually
// needs them, reducing the upfront token cost of workspace exploration. Files
// are promoted from path-only stubs to full content on first access.
package contextmgr
