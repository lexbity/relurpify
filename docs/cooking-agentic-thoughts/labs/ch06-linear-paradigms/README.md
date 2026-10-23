# Chapter 6 lab — the linear paradigms

Three recipes, one per linear paradigm, plus the captured validation story.

## Files

- `add_truncate.erpe` — pipeline: plan stage (read-only) → implement stage
  (write + pytest). Second stage reads the first stage's capture.
- `summarize_funcs.erpe` — chainer: three links, each `prompt` + `from` +
  `capture`. Links take `prompt`, never `goal`.
- `polish_docstring.erpe` — reflection: `review` writes
  `scratch.review`/`scratch.review_issues`; `revise when scratch.review
  contains issues:` runs a declared fixer agent.

## Reproduce

```bash
relurplint --check recipes --workspace .   # 0 diagnostics
# restart relurpish so all three register, then pin and dispatch each:
#   /recipe euclo.thoughtrecipe.add_truncate
#   /recipe euclo.thoughtrecipe.summarize_funcs
#   /recipe euclo.thoughtrecipe.polish_docstring
```

Read the receipts after each dispatch:

```bash
jq -c 'select(.resourceLogs) | .resourceLogs[]?.scopeLogs[]?.logRecords[]?
       | select(.eventName=="euclo.execution.complete")' \
    .relurpify_state/telemetry/otlp/*.jsonl | tail -1
```

## Captured validation story (verification machine, gemma4:e4b on CPU)

| Layer | What it catches | Captured example |
|---|---|---|
| lint | grammar, types, declarations | `[error recipes.lower] ...:21:9: unknown agent "fixer"` |
| boot | vocabulary + prompt-import resolution | (Ch 4–5 recipes) |
| runtime | execution-only requirements | `chainer link "link" requires a prompt clause` (goal-shaped links, v1 draft) |

Also captured:

- **Infrastructure rejection:** a second relurpish instance held the
  Badger lock → `boot.degraded` + `task.rejected` with
  `Cannot acquire directory lock on ".../memory/ast_index"`. One
  workspace, one relurpish.
- **Anomalous receipt, flagged not explained:** the corrected chainer
  completed with `outcome: "success"`, `llm_calls: "0"` — flagged
  `[NEEDS CONFIRMATION: chainer link execution and counters at your pin]`.
- **Paradigm contract pointers:** react (react_think_node.go — decision
  parse/repair), reflection (contract.go — scratch.review guarantee),
  pipeline (contract.go — stage order), chainer
  (named/euclo/thoughtrecipes/chain_builder.go — link requirements).
- **Cost expectation:** 1–5 minutes per model call on the verification
  CPU; two-phase recipes multiply. Run on your hardware; read receipts.
