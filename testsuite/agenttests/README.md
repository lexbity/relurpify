`dev-agent agenttest` suites are YAML files that define prompt-based checks for a specific agent, manifest, and model.

The command examples below are documented invocation shapes. They were not all
re-run in this workspace.

Runs execute inside derived temporary workspaces under `relurpify_cfg/test_runs/.../tmp/`, with a testsuite template profile materializing the temporary `relurpify_cfg/` for each case.

The run artifacts for a case land alongside that workspace at `relurpify_cfg/test_runs/{agent}/{run_id}/`.

Run suites from the repo root:

```bash
go build -o dev-agent ./app/dev-agent-cli
./dev-agent agenttest run
./dev-agent agenttest run --agent euclo
./dev-agent agenttest run --suite testsuite/agenttests/euclo.code.testsuite.yaml
```

Optional local-cache setup:

```bash
GOCACHE=$PWD/.gocache GOMODCACHE=$PWD/.gomodcache go build -o dev-agent ./app/dev-agent-cli
GOCACHE=$PWD/.gocache GOMODCACHE=$PWD/.gomodcache ./dev-agent agenttest run --agent euclo
```

Common flags:

```text
--lane pr-smoke|merge-stable|quarantined-live
--timeout 2m
--profile ci-live
--strict
--tier smoke
--include-quarantined
--skip-ast-index                 default true; live agenttests run without AST bootstrap
--backend-reset none|model|server   (default none)
--backend-reset-between             reset before each case
--backend-reset-on <regex>          repeatable; trigger reset+retry on matching errors
--backend-bin ollama                path/name of backend binary
--backend-service ollama            systemd service name for server restarts
```

By default, live `agenttest` runs skip AST/bootstrap indexing so end-to-end validation measures agent behavior instead of paying workspace AST warmup cost on every case.

The canonical Euclo live-model family is represented by the checked-in `euclo.*.testsuite.yaml` files in this directory. For short debugging passes, start with:

```bash
./dev-agent agenttest run --suite testsuite/agenttests/euclo.debug.testsuite.yaml --tag level:1 --timeout 75s
./dev-agent agenttest run --suite testsuite/agenttests/euclo.review.testsuite.yaml --tag level:1 --timeout 75s
./dev-agent agenttest run --suite testsuite/agenttests/euclo.tdd.testsuite.yaml --timeout 75s
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
