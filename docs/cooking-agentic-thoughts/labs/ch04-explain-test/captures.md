# Chapter 4 captures

## Pin acknowledgment (real boot, 220×50 pty)

```text
Pinned recipe for this session: euclo.thoughtrecipe.explain_test (dispatch runs exactly this recipe; /recipe clear to unpin)
```

## Route selection, pinned vs unpinned (real telemetry)

| Turn | `decided_by` | `candidate_count` | family |
|---|---|---|---|
| Unpinned explanation ask (before pin) | `lattice:score` | 14 | `investigation` |
| Pinned explanation ask | `pinned` | 1 | `investigation` |

The 14 (not 12 as in Chapter 2) includes the two new candidate files
added after that chapter — registration changed the lattice's candidate
set immediately.

## Completion record, pinned turn (real telemetry)

```json
{"outcome": "success", "llm_calls": "1"}
```

Feed result line: `node=euclo.done | status=ok`, ~4s. No approval banner
appeared — the recipe is read-only, so governed mode had nothing to pause.

## The pin validator's refusal (real capture)

```text
Cannot pin /recipe: not registered in this workspace.
Recipes load once at boot — restart relurpish to pick up new recipe files.
Registered:
  euclo.thoughtrecipe.code_review
  ...
```

(Triggered by a mangled keystroke in the capture harness; the registered
list attachment is the validator's normal behavior for unknown IDs.)

## Session-restore note

The pinned turn restored a prior session (5 messages) and answered with a
single LLM call — context reuse is why the counters are small. A pinned
run on a fresh session performs the recipe's `file_read`/`file_list`
calls and shows larger counters.
