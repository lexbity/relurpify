# Chapter 10 lab — house style

The live prompts with the chapter's edits, the drafting recipe, and the
wiring finding.

## Files

- `agent.react.default.prompt` — live system prompt: shipped + two reader
  lines (Ch 8's verification line, and the read-in-full line from Ch 8's
  hands-on).
- `agent.react.default.user.prompt` — live per-turn frame: shipped + the
  Ch 10 house line (`When a task names a test, run it and report the real
  result.`).
- `draft_prompt.erpe` — the drafting recipe (level 6.5: machine-drafted,
  human-installed). Lints green.

## Captured: the edited user prompt resolves (real telemetry)

After restart + dispatch of "Run the tests in this workspace…":

```text
euclo.route.selected     (decided_by: "lattice:score")
prompt.resolved          agent.react.default        cache_hit: false
prompt.resolved          agent.react.default.user   cache_hit: false
prompt.resolved          agent.react.default        cache_hit: false
prompt.resolved          agent.react.default.user   cache_hit: false
euclo.execution.complete   outcome: "unverified"
```

`unverified` = the turn completed without producing or verifying
artifacts. New outcome vocabulary beyond Chapter 2's success/failure.

## Captured: the wiring finding

Resolution counts across every run of the book's verification sessions:

| Prompt ID | Resolutions |
|---|---|
| `agent.react.default` | 21 |
| `agent.react.default.user` | 21 |
| everything else (`framework/*` 8, paradigm defaults, rewoo pair, named) | 0 |

No Go code references the `framework/*` IDs. The blocks are shipped,
materialized, lintable — and unwired as of the pin.
`[NEEDS CONFIRMATION: framework block wiring at your pin]`

## The levels ladder (one-page cheat)

| Rung | Mechanism you own | Built |
|---|---|---|
| 1 Static | edited prompt file | Ch 8 |
| 2 Workflow | pipeline stages | Ch 6 |
| 3 Control flow | `route:` | Ch 5 |
| 4 Delegation | `delegate` / `run` in branches | Ch 5 |
| 5 Higher-order | plan document consumed by a build step | Ch 7 (pair in Ch 11) |
| 6 Template meta | `draft_prompt` recipe → `drafts/` | Ch 10 |
| 7 Self-improving | **human-gated by design** — machine drafts, human installs | Ch 10 |
