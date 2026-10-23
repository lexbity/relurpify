# Lab — ch08-bkc

**Chapter:** *Relurpic Internals — Inside Look*, Chapter 8
("Compilation and Grounding: the BKC"). **Pin:** tag
`BETA-RC1-FALL2026`; captured at commit
`1bc57c62029641dcc919c04a8076593794246409` while the tag is pending.

**What this lab is:** compile a context, verify the digest contract by
replay, exercise the epoch-scoped cache, ground captures with trust
floors, and confirm the stream-trigger delivery — via the compiler's
own tests, which run real stores and real digests. No model backend, no
sandbox.

## Command card

| # | Command | Deterministic? | Expected |
|---|---|---|---|
| 1 | `go test ./execution/compiler -run 'TestStrictReplay\|TestRecordLookupsNeverScan' -count=1 -v` | yes | `TestStrictReplayGoldenRecordPasses`, `TestStrictReplayDetectsNondeterminism`, `TestRecordLookupsNeverScan` all PASS |
| 2 | `go test ./execution/compiler -run 'TestCacheKeyIncludesEpoch\|TestCacheRunScopePreventsCrossRunHits' -count=1 -v` | yes | both PASS |
| 3 | `go test ./context/knowledge -run 'TestGroundingService' -count=1 -v` | yes | HappyPath, TrustFloors, Edges all PASS |
| 4 | `go test ./context/contextstream ./execution/agentgraph -count=1` | verdicts yes; timings no | both packages `ok` |
| 5 | `sed -n '819,832p' execution/compiler/compiler.go` | yes | `compilationDigest` — three inputs, no separators |

## Honesty notes

- The replay tests prove the digest contract on *records the tests
  build*; the collision demonstration in the chapter (§8.3) is
  byte-level inspection of the concatenation, not a separate harness —
  `"a"+11+"b"`, `"a1"+1+"b"`, and `"a"+1+"1b"` are the same bytes.
- `TestRecordLookupsNeverScan` corrects the architecture doc's list of
  full scans (Chapter 7's note, compiler-side proof here).
- The stage numbers in the code are partial (2, 5, 6, 6b carry inline
  comments); the canonical seven-stage list is the whitepaper's
  (`devdocs/relurpify-technical-whitepaper.md:209`).
- `go test` timings are machine-specific; verdicts are the comparison
  target.

## Verification

`verify.sh ch08` (from the labs root) replays card items 1–5 and diffs
the deterministic outputs.
