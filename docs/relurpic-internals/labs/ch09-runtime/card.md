# Lab — ch09-runtime

**Chapter:** *Relurpic Internals — Inside Look*, Chapter 9 ("Runtime,
Sessions, and the Sidecar"). **Pin:** tag `BETA-RC1-FALL2026`; captured
at commit `1bc57c62029641dcc919c04a8076593794246409` while the tag is
pending.

**What this lab is:** queue a job, watch the runner drain it — with the
sidecar built from source, since no release has ever carried it
(decision D10). The runner self-submits a scheduled `knowledge.refresh`
job via the spool, claims it, executes it, and completes it; the e2e
suite then puts the whole jobs contract on record. No model backend, no
sandbox.

## Command card

| # | Command | Deterministic? | Expected |
|---|---|---|---|
| 1 | `go build -o /tmp/relurpify-ch09/relurpify-runner ./ayenitd/cmd/relurpify-runner` | yes | clean build |
| 2 | `timeout 5 …/relurpify-runner --workspace W --state-dir S --workers 1 --refresh-interval 500ms` | shapes yes; IDs/timestamps no | creates `ast/ jobs/{spool,store} logs/`; `status.json` with `started_at`/`heartbeat_at` ~5 s apart; exits 143 (SIGTERM) |
| 3 | `cat S/logs/runner.jsonl` | shapes yes; IDs/timestamps no | `runner.started` → `job.submitted` (producer `relurpify-runner`, kind `knowledge.refresh`) → `job.claimed` → `knowledge.refresh complete` → `job.completed` → `runner.stopped` |
| 4 | `go test ./ayenitd -run TestRunnerE2E -count=1 -v` | yes | `SpoolToCompleted`, `RecoverAfterHardStop`, `LockSingleExecutor`, `TelemetryJSONL` all PASS |

## Honesty notes

- D10 applies: the runner has never shipped in any release; the lab
  builds it from source. The seam working is not the release path
  working — Chapter 1 §1.2 and Chapter 9 §9.4 keep the two separate.
- The `job.claimed` line appears twice in the transcript (the second is
  the handler's own completion log reusing the claim log type with
  `message: "knowledge.refresh complete"`); the `job.completed` record
  is the store's terminal state.
- `marked_stale: 0` is the empty-substrate fixture's value; the *claim*
  is that the refresh handler reports what it marked, not the number.
- Correlate IDs and timestamps differ per run; compare event kinds,
  producers, and the claimed→completed ordering.
- On a workspace with an existing `relurpify_cfg/`, the runner reads it
  through userconfig like everything else; the lab's scratch workspace
  runs on defaults.

## Verification

`verify.sh ch09` (from the labs root) replays card items 1–4 and diffs
the deterministic outputs (event-kind sequences, test verdicts, exit
codes).
