# Lab — ch02-constitution

**Chapter:** *Relurpic Internals — Inside Look*, Chapter 2 ("The
Constitution, Enforced"). **Pin:** tag `BETA-RC1-FALL2026`; captured at
commit `1bc57c62029641dcc919c04a8076593794246409` while the tag is pending.

**What this lab is:** the chapter's verification transcript. You break
ADR-001 (one environment boundary) with a one-line diff, watch two
independent fence layers catch you, and revert. Run in a clean checkout;
every step is reversible. No model backend needed.

## Files

- `card.md` — this card
- `violation.diff` — the exact one-line violation (apply with
  `git apply violation.diff`, revert with `git checkout -- model/runtime.go`)
- `transcript.txt` — captured outputs from the pin

## Command card

| # | Command | Deterministic? | Expected |
|---|---|---|---|
| 1 | `make lint-arch` | verdicts yes | ends `[PASS] All architecture gates passed` + three `[PASS] … All domain checks passed` (direction, governance-no-orchestration, context-ports) |
| 2 | `make check-gates-honest` | yes | `[PASS] check-gates-honest: all gate patterns and AST gates present` |
| 3 | `git apply violation.diff && make envcheck` | yes | `[FAIL] env: model/runtime.go:8 reads the process environment via os.Getenv`, exit 1 |
| 4 | `make grep-architecture-gates` | yes | `[FAIL] grep-architecture-gates: direct env access remains outside userconfig`, quoting the raw line `./model/runtime.go:8:var labViolation = os.Getenv("HOME")` |
| 5 | `git checkout -- model/runtime.go && make envcheck` | yes | `[PASS] envcheck: no env access outside userconfig (1087 files scanned)` |
| 6 | `git status --porcelain model/` | yes | empty output |

## Honesty notes

- Step 1 compiles several gate binaries on first run; wall-clock time is
  machine-specific. Compare verdict lines only.
- The violation position `model/runtime.go:8` is deterministic at the pin
  because the diff is applied to a known file state; on a different base
  the line number moves but the semantic verdict does not.
- The two failure outputs are *structurally different on purpose*: the
  AST gate names the resolved selector and position; the grep fence
  quotes the raw line. Chapter 2 §2.3 explains the division.
- If `make check-gates-honest` fails on your machine, your checkout is
  not at the pin — do not "fix" the Makefile; diff it.

## Verification

`verify.sh ch02` (from the labs root) replays card items 1–6 in a scratch
clone at the pin and diffs the deterministic outputs.
