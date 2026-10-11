# agenttests

`dev-agent agenttest` suites are YAML files that define prompt-based checks for a specific agent, manifest, and model. Every CLI flag listed here maps to a real consumer — the dead flags (`--sandbox`, `--lane`, `--bootstrap-timeout`, `--debug-agent`, `--backend-reset-between`) were removed together with their plumbing.

Runs execute inside derived temporary workspaces under `relurpify_cfg/test_runs/.../tmp/`, with a testsuite template profile materializing the temporary `relurpify_cfg/` for each case.

The run artifacts for a case land alongside that workspace at `relurpify_cfg/test_runs/{agent}/{run_id}/`.

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
--case <name>                       run a single case by name
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
