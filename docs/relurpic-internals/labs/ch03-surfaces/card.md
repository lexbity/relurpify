# Lab — ch03-surfaces

**Chapter:** *Relurpic Internals — Inside Look*, Chapter 3 ("The
Surfaces, Graded"). **Pin:** tag `BETA-RC1-FALL2026`; captured at commit
`1bc57c62029641dcc919c04a8076593794246409` while the tag is pending.

**What this lab is:** the verify-five-claims worksheet. Five *high*
promises from the compatibility table
(`devdocs/HANDOFF-docs/API_SURFACES.md` §9.1), each traced to its
declaration site, with the captured evidence in `transcript.txt` — plus
the one grading correction the chapter found. No model backend needed.

## Command card

| # | Claim | Command / check | Expected (captured) |
|---|---|---|---|
| 1 | Config schemas are versioned | `grep -rl "^schema:" relurpify_cfg/ \| wc -l` and `\| sort -u` over the tags | `112` files; 9 distinct tags (`tool` ×99, `model/provider` ×4, `model/profile` ×3, one each of workspace / policies ×4 / `euclo/families`) |
| 2 | The streaming contract is normative and race-gated | `sed -n '13,28p' model/ports.go` then `make check-stream-contract` | six `R1`–`R6` rules in the interface comment; `ok … testsuite/conformance 1.021s` (timing machine-specific) |
| 3 | Compilation records carry a deterministic digest | `sed -n '84,96p' execution/compiler/types.go` | `DeterministicDigest string // Hash for determinism verification` |
| 4 | Checksums + SBOMs ship with every release | `sed -n '243,249p' .goreleaser.yaml` | `checksums.txt` + per-artifact `.sbom.json` |
| 5 | The library API is not stable | `cat AGENTS.md` + `sed -n '14,49p' tooling/arch/symcheck.go` | the four-word charter; the `RemovedSymbols` table |
| 6 | (Correction) the runner row | `sed -n '174,187p' app/relurpish/runtime/runner_supervisor.go` | the lookup seam exists; no `relurpify-runner` binary has ever shipped (plan D10) |

## Honesty notes

- Claim 3 is verified at the *declaration site* only; Chapter 8's lab
  replays a real compilation and diffs the digest.
- Claim 4 is verified as declared configuration; the artifacts themselves
  appear when the release tag is published.
- The `check-stream-contract` timing (1.021s) is machine-specific —
  compare the verdict, not the duration.
- Claim 6 is maintainer-confirmed (plan decision D10), not derivable
  from the tree alone: the tree shows the seam, the maintainer states
  the binary never shipped.

## Verification

`verify.sh ch03` (from the labs root) replays card items 1–5 and diffs
the deterministic outputs (tag counts, declaration-site lines, gate
verdicts). Item 6 is reported, not diffed.
