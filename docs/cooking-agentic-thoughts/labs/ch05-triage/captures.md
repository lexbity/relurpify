# Chapter 5 captures

## Two linter catches on the way to green (real errors)

```text
# annotation after the destination (wrong position):
relurpify_cfg/euclo/triage.erpe: [error recipes.parse] ...:42:50: expected "->"

# vocabulary declaration inside a delegate block:
relurpify_cfg/euclo/triage.erpe: [error recipes.parse] ...:40:11: may invoke is only allowed in trigger and run blocks
```

Fixes: `as claimed` sits after the source type, before the arrow; per-step
vocabulary requires `run` blocks (route branches included), not `delegate`.

## Dispatch receipts for the pinned triage run (real telemetry)

```json
{"decided_by": "pinned", "candidate_count": "1"}
```

Pin acknowledgment:

```text
Pinned recipe for this session: euclo.thoughtrecipe.triage (dispatch runs exactly this recipe; /recipe clear to unpin)
```

## The approval gate sat on reads (audit chain, real records)

```json
{"action": "tool:file_read", "type": "hitl", "result": "tool_allowed"}
```

The early approval banner in the pinned triage run approved
`file_read` use — the audit chain identified it.

## The gate experiment (real telemetry, gate_demo turn)

```text
euclo.interaction.frame.emitted   (frame-1792781054761170596)
euclo.step.started                (node.id: ask.10.1.0.execute)
```

Input bar state: `running — > blocked`.

Finding: the emitted ask-user frame did not render an answerable row in
the TUI chat feed, and `/hitl` reported `No pending approvals` while the
turn sat paused. `[NEEDS CONFIRMATION: ask-user frame rendering at your pin]`.
`/stop` exits a paused turn.

## Step failure under the default policy (real event, killed run)

```json
{"node.id": "run.15.5.repairer.0.execute", "action_taken": "abort",
 "kind": "cancelled", "message": "node react_think execution failed: context canceled",
 "on_error": "abort", "paradigm": "react"}
```

## The inspector spin (real feed content, pre-fix draft)

With `python_pytest` in the inspector's scope, the model repeated the same
thought ("run python_pytest on the tests to find the failing test case…")
across bounded retries. Removing the runner from that step's scope ended
the spin. Scope is a focus boundary as well as a security one.

## Honest limit

The full two-phase triage arc (inspect → confirm → repair → verify) did
not complete inside a reasonable capture window on the verification
machine's CPU with `gemma4:e4b`; decision-repair retries were visible as
thoughts growing fragment by fragment. Operator move: `/stop`, simplify,
retry. Chapter 9 adds deterministic tooling for recipe development.
