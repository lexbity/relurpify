# Lab — ch11-test-estate

**Chapter:** *Relurpic Internals — Inside Look*, Chapter 11 ("The Test
Estate"). **Pin:** tag `BETA-RC1-FALL2026`; captured at commit
`1bc57c62029641dcc919c04a8076593794246409` while the tag is pending.

**What this lab is:** author one suite exercising all three OSB
assertion kinds against the stringkit fixture, run it live, follow the
tape lifecycle as far as the pin allows — and verify the replay half
against the committed corpus. **This is the book's first
model-dependent lab**: Ollama serving `gemma4:e4b` (the Book 1 setup)
is required for step 2.

## Files

- `euclo.stringkit.osb.testsuite.yaml` — the authored suite (lab copy;
  the discovery-visible copy lives at
  `testsuite/agenttests/euclo.stringkit.osb.testsuite.yaml`,
  untracked/book-owned)
- `transcript.txt` — captured outputs from the pin

## Command card

| # | Command | Deterministic? | Expected |
|---|---|---|---|
| 1 | discovery dry-run: `dev-agent agenttest run --suite <lab copy> --profile record` | yes | `no testsuites matched the requested filters` (suites are discovered from `testsuite/agenttests/`) |
| 2 | `dev-agent agenttest run --suite testsuite/agenttests/euclo.stringkit.osb.testsuite.yaml --profile record --out RUN` | verdict yes; content no | `[record]: 1/1 passed (0 infra, 0 assertion)`; admission warning + `sandbox.denied` protected-path line in the log |
| 3 | `jq -r '.osb_benchmark' RUN/report.json` | shape yes; match rate varies | `total_observations: 4`; `tools_expected` may record a miss (advisory) — the case still passes |
| 4 | `dev-agent agenttest promote --run RUN --all --suite …` | yes (at the pin) | `case "stringkit_osb_read" tape invalid: open …/tape.jsonl: no such file or directory` |
| 5 | `make test-tape-fidelity && make test-conformance && make test-euclo-golden` | yes | all three `ok` / `PASS` |

## Honesty notes

- Divergence flagged in the chapter: the docs grade Benchmark "soft";
  the code is two-grade — observations advisory, thresholds hard
  (`benchmark_evaluator.go:24-26`, `runner.go:698-705`). The lab
  demonstrates the advisory side live (§2b's recorded miss on a
  passing case).
- The promote failure (§3) is a *verified gap at the pin*, not a lab
  mistake: `llm.New` applies `TapePath` only for the `tape` kind and
  only for replay (`platform/llm/factory.go:57-62`,
  `providers_tape.go:29`). Do not "fix" it in the lab; Chapter 12's
  pathway map owns it.
- Live-run content (the model's paragraph, tool-call counts) varies per
  run; verdicts, event shapes, and the promotion error are the
  comparison targets.
- The lab suite excludes this checkout's bulky untracked directories
  (`devdocs/**`, `docs/**`, …) — a clean clone needs fewer excludes.

## Verification

`verify.sh ch11` (from the labs root) replays card items 1, 4–5 and
diffs the deterministic outputs; items 2–3 are reported structurally
(verdict line, OSB summary shape).
