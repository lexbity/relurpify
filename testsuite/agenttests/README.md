`dev-agent agenttest` suites are YAML files that define prompt-based checks for a specific agent + manifest + model(s).

Runs now execute inside derived temporary workspaces under `relurpify_cfg/test_runs/.../tmp/`, with a testsuite template profile materializing the temp `relurpify_cfg/` for each case.

Suites in this directory are the canonical source. Run them directly:

```
go build -o dev-agent ./app/dev-agent-cli
./dev-agent agenttest run
./dev-agent agenttest run --agent coding
./dev-agent agenttest run --suite testsuite/agenttests/coding.go.testsuite.yaml
```

Phase 1 CI metadata is now supported in suite YAML:

```yaml
metadata:
  name: coding
  owner: agent-platform
  tier: stable
  quarantined: false
spec:
  execution:
    profile: ci-live
    strict: true
```

Optional (keep deps local to avoid host cache permission issues):
```
GOCACHE=$PWD/.gocache GOMODCACHE=$PWD/.gomodcache go build -o dev-agent ./app/dev-agent-cli
GOCACHE=$PWD/.gocache GOMODCACHE=$PWD/.gomodcache ./dev-agent agenttest run --agent coding
```

Flags:
```
--lane pr-smoke|merge-stable|quarantined-live
--timeout 2m
--profile ci-live
--strict
--tier smoke
--include-quarantined
--skip-ast-index                 default true; live agenttests run without AST bootstrap
--ollama-reset none|model|server   (default none)
--ollama-reset-between             reset before each case
--ollama-reset-on <regex>          repeatable; trigger reset+retry on matching errors
--ollama-bin ollama                path/name of ollama binary
--ollama-service ollama            systemd service name for server restarts
```

Examples:
```
# Unload model between cases
./dev-agent agenttest run --agent coding --ollama-reset model --ollama-reset-between

# Run the default PR smoke lane
./dev-agent agenttest run --lane pr-smoke

# Run the stable merge lane
./dev-agent agenttest run --lane merge-stable

# Run quarantined suites explicitly
./dev-agent agenttest run --lane quarantined-live

# Restart server and auto-retry on timeouts
./dev-agent agenttest run --agent coding --ollama-reset server --ollama-reset-between --timeout 2m

# Dedicated AST-enabled end-to-end coverage
./dev-agent agenttest run --agent coding --skip-ast-index=false
```

By default, live `agenttest` runs skip AST/bootstrap indexing so end-to-end validation measures agent behavior instead of paying workspace AST warmup cost on every case. AST-enabled end-to-end coverage should be run separately and explicitly with `--skip-ast-index=false`.

Committed suites in `testsuite/agenttests/` must declare CI metadata explicitly rather than relying on loader defaults. Every checked-in suite should include:

```yaml
metadata:
  owner: agent-platform
  tier: smoke
  quarantined: false
spec:
  execution:
    profile: ci-live
    strict: true
```

Coverage-matrix cases use the `coverage-matrix` tag so they can be run as a focused lane:

```
./dev-agent agenttest run --tag coverage-matrix
```

For short Euclo debugging passes before the broader live catalog, start with
the smallest stable slices:

```
./dev-agent agenttest run --suite testsuite/agenttests/euclo.debug.testsuite.yaml --tag level:1 --timeout 75s
./dev-agent agenttest run --suite testsuite/agenttests/euclo.review.testsuite.yaml --tag level:1 --timeout 75s
./dev-agent agenttest run --suite testsuite/agenttests/euclo.tdd.testsuite.yaml --timeout 75s
```

If the local model is not loaded, fall back to package-level scenario and
behavior tests in `named/euclo/...` first, then retry the live subset.

For a quick live-model Euclo bug-hunting pass, the canonical rapid suite family
now runs as three focused passes on a single local Ollama instance:

```
curl -s http://localhost:11434/api/generate -d '{"model":"qwen2.5-coder:14b","prompt":"ping","stream":false,"keep_alive":"10m"}'
./dev-agent agenttest run --suite testsuite/agenttests/euclo.rapid.chat.testsuite.yaml --timeout 75s
./dev-agent agenttest run --suite testsuite/agenttests/euclo.rapid.debug.testsuite.yaml --timeout 75s
./dev-agent agenttest run --suite testsuite/agenttests/euclo.rapid.archaeology.testsuite.yaml --timeout 90s
```

Rerun one family or one case when debugging:

```
./dev-agent agenttest run --suite testsuite/agenttests/euclo.rapid.debug.testsuite.yaml --case rapid_debug_single_bug --timeout 75s
./dev-agent agenttest run --suite testsuite/agenttests/euclo.rapid.chat.testsuite.yaml --case rapid_chat_implement_single_edit --timeout 75s
```

The legacy aggregate entrypoint remains available for compatibility:

```
./dev-agent agenttest run --suite testsuite/agenttests/euclo.rapid.testsuite.yaml --tier live-flaky
```

Or filter the same lightweight cases by tag:

```
./dev-agent agenttest run --agent euclo --tag rapid-iteration --tier live-flaky
```

Performance-baseline cases use the `performance-baseline` tag so package-level benchmark work can be paired with end-to-end agent regression checks:

```
./dev-agent agenttest run --tag performance-baseline
```

These runs now persist framework hot-path counters alongside the existing token and duration artifacts:
- `framework_perf.json` per case in the run artifacts directory
- `framework` counters embedded in committed performance baseline files
- regression warnings when branch cloning, context rescans, retrieval fixed-cost checks, runtime-memory query cost, or capability-registry rebuild counts materially exceed baseline

The current Euclo coverage matrix is represented by tagged cases for:
- `code` + `edit_verify_repair`
- `code` + `reproduce_localize_patch`
- `debug` + `reproduce_localize_patch`
- `tdd` + `test_driven_generation`
- `review` + `review_suggest_implement`
- `planning` + `plan_stage_execute`
- `code -> debug`, `planning -> code`, and `code -> planning` transitions
