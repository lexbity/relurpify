# Chapter 2 lab — captured receipts and the command card

This chapter's turns were executed for real (Linux, Ollama 0.40.8 serving
`gemma4:e4b`, pytest 9.1.1, Go-built relurpish from the tree that became
`BETA-RC1-FALL2026`). The session was driven through a correctly sized
pseudo-terminal by an expect-style script (`drive_demo.py` in the author's
capture kit), keystroke by keystroke: boot → task → approval banner → `s`
→ completion. Everything the chapter quotes exists on disk in the demo
workspace.

## The two turns, as captured

| Fact | Turn 1 (the order) | Turn 2 (the repair) |
|---|---|---|
| Task | "Fix slugify and run the tests to verify" | "SyntaxError from a broken module docstring; repair; run python_pytest on test_stringkit.py (at repo root)" |
| Approval grant | `s` → `Approved hitl-1792775907209689657` | `s` → `Approved hitl-1792776806567828155` |
| Route selection | `decided_by: "lattice:score"`, `family: "debug"`, `candidate_count: 12` | same pipeline |
| LLM calls | 5 | 5 |
| Telemetry outcome | `success` | `success` |
| Model's claim | "tests passed" (via `python_pytest` on nonexistent `tests/`) | accurate |
| `pytest` reality | `SyntaxError: invalid syntax` — 1 error | `2 passed in 0.02s` |

## Reading your own receipts

Run from the stringkit workspace after a turn:

```bash
# why did that route run? (decided_by, family, candidate_count)
jq -c 'select(.resourceLogs) | .resourceLogs[]?.scopeLogs[]?.logRecords[]?
       | select(.eventName=="euclo.route.selected")' \
    .relurpify_state/telemetry/otlp/*.jsonl | tail -1

# how did the turn end — really? (outcome, llm_calls)
jq -c 'select(.resourceLogs) | .resourceLogs[]?.scopeLogs[]?.logRecords[]?
       | select(.eventName=="euclo.execution.complete")' \
    .relurpify_state/telemetry/otlp/*.jsonl | tail -1

# the approval record lives in the chained audit log
ls .relurpify_state/audit/

# the fact-check
python3 -m pytest test_stringkit.py
git diff
```

> **Known documentation bug, reproduced:** the shorter filter
> `jq '.. | .logRecords[]?'` (seen in some project docs) fails on jq 1.8
> with `Cannot index array with string`. Use the `select(.resourceLogs)`
> form above.

## What this lab proves about the product

- The documented demo replicates end to end: gate opens, turn classifies,
  selects, pauses for approval, writes governed, bounds, completes — all
  inside a minute on the captured hardware.
- `outcome: "success"` records a completed execution; a false completion
  claim passed through it while pytest showed `SyntaxError`. The runtime's
  receipt and the repository's health are different documents. Chapter 2
  teaches the difference; Chapter 9 pins it with deterministic tests.
