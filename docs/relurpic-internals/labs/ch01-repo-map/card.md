# Lab — ch01-repo-map

**Chapter:** *Relurpic Internals — Inside Look*, Chapter 1 ("The Repo Is
the Map"). **Pin:** tag `BETA-RC1-FALL2026`; captured at commit
`1bc57c62029641dcc919c04a8076593794246409` while the tag is pending.

**What this lab is:** the chapter's verification transcript, not a code
artifact. Run the command card below in a clean clone at the pin and diff
your deterministic outputs against `transcript.txt`. Nothing in this lab
needs a model backend.

## Command card

| # | Command | Deterministic? | Expected |
|---|---|---|---|
| 1 | `git rev-parse HEAD` | yes (at the pin) | `1bc57c62…` |
| 2 | `go version` | no (your toolchain) | must satisfy the `go1.27.2` toolchain pin (`go.mod:5`) |
| 3 | `go build ./... ; echo $?` | yes | no output, exit 0 |
| 4 | `go list ./... \| wc -l` | yes | `165` |
| 5 | `make test-unit` | verdicts yes; timings no | `relurplint: all checks passed (0 diagnostics)`, 152 `ok` lines, 0 failures, exit 0 |
| 6 | `make domain-check` | yes | `[PASS] domain-direction: no violations` |
| 7 | `make domain-cycles` | yes | `[FAIL] domain-cycles: 1 violation(s)` naming `capability ↔ platform`, then `[PASS] All domain checks passed` (warn mode exits clean — see Chapter 1, §1.3) |
| 8 | `make check-config-tree-drift` | yes | `[PASS] relurpify_cfg/ matches embedded templates` |

## Honesty notes

- Timings in `transcript.txt` §3 (e.g. `app/relurpish/e2e 25.698s`) are
  from the capture machine and **will** differ. Compare verdict lines, not
  durations.
- `make domain-cycles` prints `[FAIL]` for the known mutual pair and still
  exits 0: it is a warn-mode reporter (`Makefile:90-94`). The direction
  check (card #6) is the one that gates.
- The runner note in Chapter 1 §1.2 is maintainer-confirmed: no
  `relurpify-runner` binary exists or has ever shipped; the supervisor's
  lookup degrades gracefully. This lab does not verify release packaging.

## Verification

`verify.sh ch01` (from the labs root) replays card items 3–8 in a scratch
clone at the pin and diffs the deterministic outputs. Card items 1–2 are
environment checks and are reported, not diffed.
