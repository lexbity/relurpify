# Chapter 9 lab — the metronome

The stringkit regression suite and the captured run that proves the
harness works — in the negative.

## Files

- `euclo.stringkit.testsuite.yaml` — the Chapter 2 slugify turn as an
  agenttest case (final form, with the earned excludes and the fixed
  assertion).

## Reproduce

```bash
# from the relurpify repository root (dev-agent built from ./tooling/dev-agent-cli)
dev-agent agenttest run \
  --suite /path/to/euclo.stringkit.testsuite.yaml \
  --workspace /path/to/relurpify \
  --out /path/to/output-dir
```

Requires Ollama serving `gemma4:e4b` (record profile). Replay requires a
recorded tape.

## The captured run (verification machine, gemma4:e4b on CPU)

`0/1 passed (0 infra, 1 assertion)` — the case failed because the agent
produced no file change:

```json
{"FailureKind": "assertion", "DurationMS": 87936,
 "TokenUsage": {"PromptTokens": 132345, "CompletionTokens": 1150,
                "TotalTokens": 133495, "LLMCalls": 3},
 "route_selection": {"chosen_route": "euclo.thoughtrecipe.debug_tdd_repair",
                     "decided_by": "lattice:score"},
 "phase_metrics": [{"phase": "react", "duration_ms": 87281,
                    "llm_calls": 3, "tokens_used": 133495}],
 "knowledge": {"grounded_chunks": 3}}
```

Post-run workspace check: `stringkit.py` untouched, `pytest` still
`1 failed, 1 passed`. The suite caught a silent non-fix with a full
receipt chain — setup/execution reports, logs, OTLP telemetry.

## Three infra gotchas, captured on the way

1. **The derived workspace copies greedily.** An output directory inside
   the tree recursed into itself until paths exploded — exclude
   `.book-scratch/**` (and any output dir) in `workspace.exclude`.
2. **A documentation symlink breaks the copy.** `devdocs/Symblix/…` →
   `copy_file_range: is a directory` — exclude `devdocs/**`.
3. **The testsuite README's build path is stale** (`./app/dev-agent-cli`);
   the binary builds from `./tooling/dev-agent-cli`.

Also captured: governance inside the harness —
`sandbox.denied{action="fs:list" … reason="protected path: …/relurpify_cfg"}`
during the run.

## Flagged, not explained

- `RecordingMode: "off"`, `TapePath: ""` after a record-profile suite
  run: the tape did not record from the suite-level profile on this
  build. The committed reference tape with lineage lives at
  `testsuite/agenttests/tapes/euclo.gemma4.smoke/`.
  `[NEEDS CONFIRMATION: record-profile tape capture at your pin]`
- The README's `expect:` block shows a flatter shape than real suites
  use; the authoritative form is `expect.outcome:` (see the smoke suite
  and this lab's suite file).
