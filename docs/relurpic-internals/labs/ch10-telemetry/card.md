# Lab — ch10-telemetry

**Chapter:** *Relurpic Internals — Inside Look*, Chapter 10 ("Telemetry
as the Nervous System"). **Pin:** tag `BETA-RC1-FALL2026`; captured at
commit `1bc57c62029641dcc919c04a8076593794246409` while the tag is
pending.

**What this lab is:** reconstruct a run from telemetry alone. Boot a
runtime in a scratch workspace with one script-safe command, then read
the run back out of its OTLP-JSON segment with `jq` — event names,
attribute keys, the projection boundary working. Plus the turn leg
(AC-3) and the forwarding story (boxed). No model backend, no sandbox.

## Command card

| # | Command | Deterministic? | Expected |
|---|---|---|---|
| 1 | `go build -o /tmp/relurpify-ch06/relurpish ./app/relurpish && …/relurpish status --workspace /tmp/ch06-ws --format json` | yes | boots, renders JSON, exits 0 |
| 2 | `ls W/.relurpify_state/telemetry/otlp/` | shape yes; date no | `relurpify-YYYY-MM-DD.jsonl` |
| 3 | `jq -r '.resourceLogs[0].scopeLogs[0].logRecords[]?.eventName' S \| sort \| uniq -c` | names yes; counts no | `sandbox.image_unpinned` (1), `state_change` (hundreds), `grounding.ports_unwired`, `context_policy_reloaded`, `runtime.shutdown_drain` |
| 4 | attribute-key census over the segment | yes | keys are decision metadata (`agent_id`, `capability_id`, `provider`, `fingerprint`, `security_event`, …); **none** of `args · prompt · response · text · stdout · stderr` appears |
| 5 | `go test ./app/relurpish/e2e -run TestBootTurnToolCall -count=1` | yes | `ok` (AC-3 inside: ≥1 tool span, ≥10 named log records, line-by-line OTLP-JSON) |
| 6 | `sed -n '20p' telemetry/otelleg/settings.go` | yes | `defaultFileRetention = 168 * time.Hour` |

## Honesty notes

- The captured boot is a *status* boot (no turn): its event mix is a
  boot's mix. The turn leg is verified by the e2e test's AC-3 check,
  whose segments live in the test's temp workspace — the test is the
  verification, not a kept artifact.
- Counts differ per machine and run (211 `state_change` on the capture
  machine); the *event-name set* and the *absence of blocklisted keys*
  are the portable claims.
- Forwarding is boxed by the plan: no OTLP/HTTP collector is assumed.
  The testable half without a collector is the fail-closed endpoint
  validation (`telemetry/otelleg/settings.go:101-135`).
- The segment line count differs per run; each line is a complete
  OTLP/HTTP JSON body (that is what "parses line-by-line" means in
  AC-3).

## Verification

`verify.sh ch10` (from the labs root) replays card items 1–3, 5–6 and
diffs the deterministic outputs (event-name sets, blocklist absence,
retention constant, test verdicts).
