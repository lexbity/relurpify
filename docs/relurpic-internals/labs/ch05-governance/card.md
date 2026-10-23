# Lab — ch05-governance

**Chapter:** *Relurpic Internals — Inside Look*, Chapter 5 ("The
Governance Engine"). **Pin:** tag `BETA-RC1-FALL2026`; captured at
commit `1bc57c62029641dcc919c04a8076593794246409` while the tag is
pending.

**What this lab is:** the governed-turn + decision/audit transcript. A
real governed turn runs through the product runtime (offline backend —
no model backend or sandbox needed), followed by the full governance
package suite. No code artifacts; the decision and audit shapes are
recorded as the serialized JSON they use. The test harness doubles two
enforcement seams (sandbox command runner, HITL approval) so the
*decisions* are deterministic — Chapter 4 §4.4 and Chapter 5 §5.6
explain the cut.

## Command card

| # | Command | Deterministic? | Expected |
|---|---|---|---|
| 1 | `go test ./app/relurpish/e2e -run TestBootTurnToolCall -count=1 -v` | verdicts yes; timestamps/temp paths no | `--- PASS: TestBootTurnToolCall`; log shows `strict_mode=true`, `config_fingerprint=sha256:…`, `tools_loaded=99`, and `tool admission warning: skipping unlisted tool "query_ast"` |
| 2 | `go test ./governance/... -count=1` | verdicts yes; timings no | 11 packages `ok`, 0 failures |
| 3 | `sed -n '22,47p' governance/authorization/hitl.go` | yes | `PermissionRequest` + `PermissionDecision` shapes |
| 4 | `sed -n '52,61p' governance/policy/audit.go` | yes | `AuditChainEntry` with `Sequence`, `PreviousHash`, `RecordHash` |

## Honesty notes

- The turn runs on the offline scenario model and a recording sandbox
  command runner (`app/relurpish/e2e/harness_test.go:20-31`); HITL is
  auto-approved by the harness. The decision path — admission, policy
  evaluation, risk derivation, audit wiring — is the product's own.
- The `config_fingerprint` and temp workspace paths differ per run;
  compare the log line *kinds* (strict mode, fingerprint present,
  admission warning present), not the values.
- The admission warning names `query_ast` because the test workspace
  ships a tool manifest that its own allowlist does not list — the
  allowlist excludes-and-names rather than failing the boot.
- Timings (0.10s / 0.265s / 0.371s in the transcript) are
  machine-specific.

## Verification

`verify.sh ch05` (from the labs root) replays card items 1–4 and diffs
the deterministic outputs (verdict lines, log-line kinds, declaration
sites).
