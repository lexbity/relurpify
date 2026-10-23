# Chapter 4 lab — explain_test

The first reader-authored recipe, exactly as captured.

## Files

- `explain_test.erpe` — the recipe as written in the hands-on (byte-identical
  to the workspace copy used for the captures).
- `captures.md` — the captured receipts.

## Reproduce

```bash
# grammar, types, symbols, lowering — no registry
relurplint --check recipes --workspace .

# in the TUI
/recipe                                      # find euclo.thoughtrecipe.explain_test
/recipe euclo.thoughtrecipe.explain_test     # pin for this session
# …give an explanation task…
/recipe clear                                # return selection to the lattice

# the proof
jq -c 'select(.resourceLogs) | .resourceLogs[]?.scopeLogs[]?.logRecords[]?
       | select(.eventName=="euclo.route.selected")' \
    .relurpify_state/telemetry/otlp/*.jsonl | tail -1
```

Captured rule: restart relurpish after any recipe edit — registration and
pin validation both read the boot-time registry.
