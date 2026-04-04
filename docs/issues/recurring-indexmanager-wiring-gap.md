# Recurring IndexManager Wiring Gap

**Status**: Resolved (currently wired) — but structurally at risk of recurrence  
**Severity**: High (silent degradation, no error surface)  
**First occurrence**: ~2026-03-09 (during coding_agent.go deletion / three-layer arch rework)  
**Second occurrence**: Acknowledged by author — pattern identified  
**Discovered via**: Code review + architecture discussion 2026-04-03

---

## Summary

The `IndexManager` has been silently unwired from the progressive loader at least once before. The current code is correctly wired, but the wiring chain is 7 layers deep with no enforcement, making it vulnerable to any future refactor of the agent init or bootstrap path.

---

## What IndexManager Enables

When wired, `ProgressiveLoader` can:
- Load files at graduated detail levels (Full → Detailed → Concise → Minimal → SignatureOnly)
- Execute AST queries: function/method signatures, symbol search, call graph expansion
- Load related files via dependency graph traversal
- Demote low-priority files to free token budget before pruning

When `IndexManager` is nil, all of the above silently degrades:
- `executeASTQuery` returns `"ast index unavailable"` (no error propagated)
- `LoadRelatedFiles` returns `nil` immediately
- `formatSignaturesOnly` / `formatDetailed` return empty strings
- `fileStats` returns empty strings
- The agent sees raw file content only, with no structural enrichment

**The agent appears to work normally. There is no visible failure.**

---

## Current Wiring Chain (as of 2026-04-03)

```
BuildBuiltinCapabilityBundle (runtime.go)
  → manager := ast.NewIndexManager(...)
  → manager.StartIndexing(buildCtx)          ← background workspace index
  → CapabilityBundle{IndexManager: manager}

BootstrapAgentRuntime (bootstrap.go:186-193)
  → AgentEnvironment{IndexManager: indexManager}
  → BootstrappedAgentRuntime{Environment: env}

New(runtime.go) / instantiateAgent
  → agentEnv := boot.Environment             ← full env, not extracted
  → instantiateAgent(cfg, agentEnv, defs)

named/euclo/agent.go
  → a.Environment = env                      ← full AgentEnvironment stored
  → (only extracts GraphDB from IndexManager for archaeo use)

named/euclo/execute.go:89-95
  → BuildContextRuntime(task, ContextRuntimeConfig{
        IndexManager: a.Environment.IndexManager,  ← passed here
    }, mode, work)

framework/contextmgr/context_policy.go:84
  → NewProgressiveLoader(..., cfg.IndexManager, ...)
```

All 7 steps must be intact for AST-aware context loading to function.

---

## Why It Keeps Breaking

The chain has no structural enforcement:

1. **No type-level requirement** — `IndexManager *ast.IndexManager` is an optional pointer. `nil` is valid everywhere.
2. **Silent degradation** — every function that uses `IndexManager` does a nil check and returns empty/nil quietly. No error reaches the caller.
3. **Long refactor surface** — any of the following trigger a re-wiring risk:
   - Deleting or restructuring an agent entry point (this caused the first break — `coding_agent.go` deletion)
   - Adding a new agent construction path
   - Extracting individual fields from `AgentEnvironment` instead of storing the full struct
   - Splitting bootstrap into phases
4. **No test coverage** — there is no integration test that boots a real runtime and asserts that AST queries return results

---

## What a Test Would Look Like

```go
// bootstrap_integration_test.go
func TestBootstrapIndexManagerWired(t *testing.T) {
    // 1. Bootstrap a real runtime against a temp workspace with a few .go files
    // 2. Wait for IndexManager.Ready()
    // 3. Assert ProgressiveLoader.executeASTQuery returns nodes for a known symbol
    // 4. Assert LoadRelatedFiles returns non-empty results for a file with known imports
}
```

Without this, each refactor of the init path is a silent correctness risk.

---

## Pre-Task Augmentation Gap (Related)

The indexer wiring is the structural prerequisite for a larger gap: euclo does not currently enrich the agent's starting context before the LLM sees the task.

From the architecture review (2026-04-03), what the agent does and does not receive before task execution:

| Signal | Status |
|---|---|
| Retrieved workflow knowledge (vector) | Partial — only for planning/debug/review modes; code mode disabled by default |
| AST-derived codebase map / signatures | Works when IndexManager is wired; not driven pre-task |
| Dependency graph expansion | Available via `LoadRelatedFiles`; never called pre-task |
| Relevant files from retrieval anchors | `ExpandContext` exists but code mode skips it |
| Failing tests / stack traces | Not pre-loaded; agent discovers via tool calls |
| Prior run logs | Not injected |
| Static analysis output | Not injected |
| Commit history | Not injected |
| User-pinned files | No UX exists yet (file picker exists for @-mentions, not session context) |

### The query building problem

`buildWorkflowRetrievalQuery` (runtime/retrieval.go:185) concatenates `task.Instruction` + `task.Context["verification"]` with deduplication. No query expansion, no HyDE, no synonym generation. Retrieval quality is entirely a function of how well the user phrases the task — a known weak point for small models.

### Code mode retrieval policy

`ResolveRetrievalPolicy` (runtime/retrieval.go:34) sets `WidenToWorkflow = false` for `code` mode. Retrieval only fires if `WidenWhenNoLocal && len(localPaths) == 0`. A coding task with explicit file paths never gets workflow retrieval. This is the most common euclo usage pattern.

---

## Recommended Actions

### Short-term (prevent third occurrence)
- [ ] Add bootstrap integration test asserting IndexManager is non-nil and functional after `BootstrapAgentRuntime`
- [ ] Update stale memory notes — the wiring gap described in project notes (2026-03-09) is now resolved

### Medium-term (activate pre-task enrichment)
- [ ] Enable retrieval expansion for `code` mode — at minimum set `WidenWhenNoLocal = true` unconditionally
- [ ] Drive pre-task AST context: on task receipt, extract anchor files/symbols from task instruction, run `LoadRelatedFiles` and `executeASTQuery`, inject results before first LLM call
- [ ] Improve query building: expand beyond raw task instruction (consider HyDE or task decomposition hints)

### Long-term (user-controlled context)
- [ ] Session context pane in TUI: pinned files persist for session, visible with token cost
- [ ] Extend `@file` picker to add files to session context (not just @-mention in message text)
- [ ] Show context expansion state in the TUI feed (which files are loaded, at what detail level)
