# agenttests

`dev-agent agenttest` suites are YAML files that define prompt-based checks for a specific agent, manifest, and model. Every CLI flag listed here maps to a real consumer — the dead flags (`--sandbox`, `--lane`, `--bootstrap-timeout`, `--debug-agent`, `--backend-reset-between`) were removed together with their plumbing.

Runs execute inside derived temporary workspaces under the workspace's runtime state tree
(`.relurpify_state/test_run/<run_id>/tmp/...`), with a testsuite template profile
materializing the temporary `relurpify_cfg/` for each case.

The run artifacts for a case land at `.relurpify_state/test_run/<run_id>/` (override with
`--out`); every case's artifacts sit under `<run_id>/artifacts/<case>__<model>/`.

Run suites from the repo root:

```bash
go build -o dev-agent ./tooling/dev-agent-cli
./dev-agent agenttest run
./dev-agent agenttest run --agent euclo
./dev-agent agenttest run --suite testsuite/agenttests/euclo.code.testsuite.yaml
```

Common flags:

```text
--suite <path>                      repeatable; empty discovers suites in testsuite/agenttests/
--agent euclo                       run suites matching <agent>
--case <name>                       repeatable; empty runs every case
--tag <tag>                         repeatable; cases carrying at least one matching tag
--tier smoke|stable|live-flaky|quarantined
--profile live|record|replay|developer-live|ci-live|ci-replay
--strict                            fail the process if any non-skipped case fails
--include-quarantined
--out <dir>                         run-artifact output directory
--timeout 15m                       per-case execution timeout; 0 disables (case and suite
                                    timeouts in the YAML override the CLI value)
--skip-ast-index                    default true; live agenttests run without AST bootstrap
--max-retries 3                     per-case retry budget; 0 disables retries
--model / --endpoint                override the model / inference endpoint for all cases
--max-iterations 8                  agent-loop iteration cap
--debug-llm                         verbose LLM telemetry logging
--backend-reset none|model|server   (default none)
--backend-reset-on <regex>          repeatable; trigger reset+retry on matching errors
--backend-bin ollama                path/name of backend binary
--backend-service ollama            systemd service name for server restarts
```

Per-case timeouts resolve case `timeout` > suite `execution.timeout` > `--timeout` > the 15-minute default; a literal `0` in the effective choice disables the bound.

Interaction scripts: a case may carry an `interaction_script` whose entries answer DSL `ask` frames in order (`action` is the answer, `text` optional freetext). A run that pauses with no script entry left fails the case as `unanswered_frame`.

By default, live `agenttest` runs skip AST/bootstrap indexing so end-to-end validation measures agent behavior instead of paying workspace AST warmup cost on every case.

The canonical Euclo live-model family is represented by the checked-in `euclo.*.testsuite.yaml` files in this directory. For short debugging passes, start with:

```bash
./dev-agent agenttest run --suite testsuite/agenttests/euclo.debug.testsuite.yaml
./dev-agent agenttest run --suite testsuite/agenttests/euclo.code.testsuite.yaml
./dev-agent agenttest run --suite testsuite/agenttests/euclo.tdd.testsuite.yaml
```

## Performance baselines

Benchmark mode compares a run against **baselines** (owner ruling R-5). A baseline is a
deliberate snapshot of one executed run, stored as schema-v1 JSON in the workspace's
runtime state tree — never in the repository:

```text
<workspace>/.relurpify_state/test_run/baselines/<suite>/<case>__<model>.baseline.json
<workspace>/.relurpify_state/test_run/baselines/<suite>/suite.baseline.json
```

`<case>` and `<model>` are sanitized the same way golden tapes are (`:` and `/` become `_`),
so tooling stays uniform across the golden and workspace-state tiers.

```bash
# Run a suite exactly as `run` does and write the baselines from that run.
./dev-agent agenttest baseline --suite testsuite/agenttests/euclo.code.testsuite.yaml

# Baseline one case, or preview without writing anything.
./dev-agent agenttest baseline --suite testsuite/agenttests/euclo.code.testsuite.yaml --case basic_edit_task
./dev-agent agenttest baseline --suite testsuite/agenttests/euclo.code.testsuite.yaml --dry-run
```

`baseline` executes the suite through the same executor and reports as `run`; only on
completion does it promote each executed case's metrics (and, for benchmark-classified
suites, the per-case and suite-level score snapshots) into the baseline files. An existing
baseline is **overwritten** — a baseline is a snapshot, not an accumulator.

Every later `run` compares each case against its baseline and records the compare block
in the case report and `report.json`: per-metric deltas for total tokens, tool calls, phase
durations, and the framework counters, plus a `PerformanceWarnings` entry for any metric off
by more than 50%. The comparison is **report-only** (R-5c): a warning never fails a case,
never changes an exit code, and never feeds OSB evaluation. A missing baseline is recorded
as absence — it never reads as "within variance".

Re-baseline deliberately: after an intentional model, prompt, or framework change, run
`baseline` again. Each baseline records `captured_at` and `source_run` so a stale anchor is
visible rather than silent.

For tape workflow and coverage reporting:

```bash
./dev-agent agenttest promote --suite testsuite/agenttests/euclo.code.testsuite.yaml --run relurpify_cfg/test_runs/euclo/<run_id> --case basic_edit_task
./dev-agent agenttest report --agent euclo
./dev-agent agenttest rerecord --agent euclo
```

Performance-baseline cases use the `performance-baseline` tag so package-level benchmark work can be paired with end-to-end agent regression checks:

```bash
./dev-agent agenttest run --tag performance-baseline
```
