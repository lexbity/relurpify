# Chapter 2 lab — the receipts command card

The commands that read one governed turn's records. Run from the stringkit
workspace after a completed turn.

```bash
# why did that route run? (decided_by)
jq -r '.. | .logRecords[]? | select(.eventName=="euclo.route.selected")' \
    .relurpify_state/telemetry/otlp/*.jsonl | tail -3

# how did the turn end — really? (outcome, llm_calls, tokens)
jq -r '.. | .logRecords[]? | select(.eventName=="euclo.execution.complete")' \
    .relurpify_state/telemetry/otlp/*.jsonl | tail -1

# the approval record
ls .relurpify_state/audit/

# the work itself, against your Chapter 1 baseline
git diff --stat
python3 -m pytest test_stringkit.py   # expect: 2 passed
```

Event names and fields per the project's TUI guide and how-to (verified
October 2026 by the maintainer): `euclo.route.selected` carries
`decided_by` (`pinned` | `explicit` | `lattice:score` | `default_recipe` |
`clarification`); `euclo.execution.complete` carries the real `llm_calls`
and token usage, and the true cause on failure.

## Status of this lab

Unlike `ch01-stringkit`, this lab holds **commands, not captured
transcripts**: on the verification machine the TUI startup gate held chat
locked (Chapter 1–2 Warning boxes; `[NEEDS CONFIRMATION: exact TUI unlock
behavior at BETA-RC1-FALL2026]`), so no live turn was captured yet. The
first successful turn on your machine produces the real artifacts these
commands read. When the gate question resolves, captured receipts replace
this note.
