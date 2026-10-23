# Chapter 8 lab — prompts

The edited react system prompt and the chapter's captured evidence.

## Files

- `agent.react.default.prompt` — the workspace copy exactly as it reads at
  chapter end: shipped content plus one added guidance line
  (`Before editing any file, read it in full.`), lint green.

## Reproduce

```bash
relurplint --check prompts --workspace .   # 0 diagnostics
# restart relurpish so the registry reloads, then run any react turn and read:
```

```bash
jq -c 'select(.resourceLogs) | .resourceLogs[]?.scopeLogs[]?.logRecords[]?
       | select(.eventName=="prompt.resolved")' \
    .relurpify_state/telemetry/otlp/*.jsonl | tail -1
```

The resolution receipt's shape (captured): the prompt ID rides in the
`task.id` attribute, with `cache_hit` (false on first resolution after a
fresh boot), `blocks_included`, `duration_ms`, and `paradigm`.

## Captured deltas between docs and build (verification machine, this pin)

1. **Unknown-reference resolution.** Documented: a `{name}` with no
   override and no default is a resolve error. Captured: a body line with
   an undeclared `{nonexistent_demo_var}` passed the lint *and* resolved
   successfully (`prompt.resolved`, turn `success`).
   `[NEEDS CONFIRMATION: unknown-reference handling at your pin]`
2. **`prompt.resolve_failed`** never fired across the full day's
   telemetry, including the broken-reference run.
   `[NEEDS CONFIRMATION: resolve_failed trigger conditions at your pin]`

## Operational warning (earned during capture)

After repeated force-killed relurpish instances, the workspace reached a
state where boots unlocked but dispatches stalled with zero telemetry.
Recovery that worked: kill every relurpish instance, clear
`.relurpify_state/sessions/`, fresh boot. Prefer graceful `ctrl+c`; hunt
zombies first when a dispatch goes silent.

## Restore points

The shipped file lives in the embedded bundle
(`userconfig/templates/embedfs/prompts/agents/agent.react.default.prompt`);
restoring the shipped text over your workspace copy is a file copy. The
chapter's edit is one line — revert it any time.
