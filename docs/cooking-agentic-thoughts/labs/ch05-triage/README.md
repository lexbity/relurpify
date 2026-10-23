# Chapter 5 lab — triage and gate_demo

The chapter's two recipes and the captured evidence.

## Files

- `triage.erpe` — the full-toolkit recipe (types, typed captures,
  provenance annotation, ask user gate, route with a run-in-branch
  repairer). Lints green; verified registered, pinned, and dispatched.
- `gate_demo.erpe` — the zero-model-call route+gate recipe used to
  verify `ask user` machinery in isolation. Lints green; `route` as the
  sole execution construct is valid.
- `captures.md` — linter errors, dispatch receipts, the frame finding.

## Reproduce

```bash
relurplint --check recipes --workspace .   # both recipes: 0 diagnostics
# in the TUI, after a restart so both register:
/recipe euclo.thoughtrecipe.triage         # pin
# dispatch a triage task; read the receipt:
```

```bash
jq -c 'select(.resourceLogs) | .resourceLogs[]?.scopeLogs[]?.logRecords[]?
       | select(.eventName=="euclo.route.selected")' \
    .relurpify_state/telemetry/otlp/*.jsonl | tail -1
```
