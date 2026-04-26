# Context Budget

## Synopsis

`framework/contextmgr` is the legacy compatibility surface for what goes into each
LLM call. Local models have finite context windows; without active management a
long session accumulates messages, file contents, and tool results until the
context overflows. The working-set layer tracks token usage and prunes
intelligently so the most relevant information is always present within the
budget.

---

## Why It Exists

A naive approach sends the entire conversation history on every LLM call. This
works until it doesn't: the context window fills, the call fails, and the agent
crashes or loses coherence. The working-set layer makes prompt sizing an
explicit, managed concern rather than an implicit failure mode.

---

## How It Works

### Context Items

`SharedContext` holds typed items. Each item has a priority, an estimated token cost, and a type:

| Type | Examples | Default Priority |
|------|----------|-----------------|
| System prompt | Agent instructions, skill fragments | Highest — never pruned |
| User message | The current instruction | High |
| Assistant message | Previous LLM turns | Medium |
| Tool result | File contents, command output, search results | Medium |
| Background context | Pre-loaded workspace files | Low — pruned first |

### Token Estimation

Token counts are estimated from character count (characters ÷ 4 as a conservative approximation). The effective context budget is derived from the manifest's `max_tokens` setting, with a portion reserved for the model's response:

```
effective_budget ≈ max_tokens × 0.75
```

### Pruning Strategies

Three built-in strategies determine what gets dropped when the budget tightens:

**Progressive** (default for interactive use)
Drops background context first, then older conversation turns from oldest to newest. Preserves recency — the most recent exchange is always kept. This is the right strategy for interactive sessions where the latest instruction is the most important context.

**Conservative**
Holds onto context as long as possible and only drops when strictly necessary. Better for long planning sessions where decisions made several turns ago are still relevant to the current step.

**Aggressive**
Drops early and maintains a lean context at all times. Suited for small models with tight context limits, or when latency is more important than context completeness.

### Compression

When a single item is too large to fit in the remaining budget even after
pruning, the working-set layer can compress it: the LLM is called with a
summarisation prompt, and the original item is replaced with the shorter
summary. A `[compressed]` marker is added to the summary so the agent knows it
is working from a condensed version.

---

## Progressive Loader

`ProgressiveLoader` is a lazy-loading extension used by ReActAgent. Instead of
loading all potentially-relevant files upfront, it loads file content on demand
as tool calls reference them:

1. Agent emits a `file_read` tool call
2. ProgressiveLoader checks if the file content is already in context
3. If not, it estimates the token cost of loading it
4. If adding it would exceed the budget, a pruning pass runs first
5. File content is added to context for this turn and subsequent ones

This keeps the working set lean - only files the agent actually reads are
included, not everything that might be relevant.

> **Status:** The `IndexManager` integration (which would allow smarter symbol-level loading rather than full-file loading) is not yet wired. Progressive loading currently operates on full file contents.

---

## Context Policies

`ContextPolicy` objects sit above the pruning strategies and express
higher-level rules:

- **MaxFilesInContext** — cap on how many file contents can be held simultaneously
- **MaxToolResultSize** — truncate tool results larger than N tokens
- **PreserveSystemPrompt** — always keep the system prompt regardless of budget pressure

These are configured per-agent and composed with the pruning strategy.

---

## Integration Points

The compatibility working-set layer is used by:

- **ReActAgent** — wraps the shared context and calls `Prune()` before each LLM node execution
- **Graph runtime** — passes the shared context through every node; the LLM node triggers pruning
- **LLM client** — the Ollama client receives a pruned message list, not the raw shared context

---

## Debugging Budget Issues

If an agent seems to "forget" earlier context, or if tool results are being
truncated:

1. Check `max_tokens` in the manifest — increase it if the model supports a larger context window
2. Switch pruning strategy to `Conservative`
3. Check telemetry output (`relurpify_cfg/telemetry/telemetry.jsonl`) for context snapshot sizes at each LLM node

---
