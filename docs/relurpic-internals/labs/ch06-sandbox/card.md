# Lab — ch06-sandbox

**Chapter:** *Relurpic Internals — Inside Look*, Chapter 6 ("Sandbox and
Governed Execution"). **Pin:** tag `BETA-RC1-FALL2026`; captured at
commit `1bc57c62029641dcc919c04a8076593794246409` while the tag is
pending.

**What this lab is:** the tool-call journey, verified at both ends: the
doctor's readiness rows on a real machine (sandbox toolchain
interrogated, degraded semantics live), the governed turn through the
product path, and the enforcement machinery green. No model backend
needed; Docker and `runsc` are welcome but their absence is itself a
valid capture (degraded rows, never unsandboxed fallback).

## Command card

| # | Command | Deterministic? | Expected |
|---|---|---|---|
| 1 | `go build -o /tmp/relurpify-ch06/relurpish ./app/relurpish` | yes | builds clean |
| 2 | `/tmp/relurpify-ch06/relurpish doctor --workspace /tmp/ch06-ws` | shapes yes; versions/paths no | `sandbox_backend: ok [required]` quoting the runsc version; `docker: degraded [warning]` or `ok` depending on pairing; `runtime_image: degraded [required]` (unpinned) unless you set `security.sandbox.image_digest`; `Result: ready` |
| 3 | `go test ./app/relurpish/e2e -run TestBootTurnToolCall -count=1` | yes | `ok … app/relurpish/e2e` |
| 4 | `go test ./capability/... -count=1` | verdicts yes; timings no | 12 packages `ok` (incl. `capability/sandbox`) |
| 5 | `sed -n '166,186p' capability/result/result.go` and `sed -n '376,390p' capability/result/result.go` | yes | `DefaultInsertionDecision` switch; `InsertionRestrictiveness` ladder with `default: return 4` |

## Honesty notes

- Capture machine: Docker 29.9.2, `runsc` release-20260406.0. Your
  versions and the Docker pairing verdict will differ; the *row shapes*
  (required vs warning, ok vs degraded) are the comparison target.
- The e2e turn harness isolates the container (recording command
  runner) and auto-approves HITL — the decision path is the product's
  own (Chapter 5 §5.6 note applies).
- `runtime_image: degraded` is the *expected* state on a fresh
  workspace: the image is tag-based until the operator pins a digest.
  Pinning it is Chapter 6 §6.3's precedence, not a defect.
- Doctor may emit several `config loaded` lines during materialization
  (before/after the starter config lands); the final one is the
  workspace's fingerprint.

## Verification

`verify.sh ch06` (from the labs root) replays card items 1, 3–5 and
diffs the deterministic outputs; item 2 is reported structurally (row
names and severities, not versions).
