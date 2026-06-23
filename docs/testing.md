# Testing

This guide covers the repo test matrix and the main user-facing test commands.

Build verification in this workspace:

- `go build ./app/relurpish ./app/dev-agent-cli ./app/relurplint`

That confirms the three user-facing binaries compile. The test commands below are the intended invocations; they were not all executed in this workspace.

## Go test matrix

| Target | What it runs | Notes |
|---|---|---|
| `make test-unit` | `lint-config` then `go test ./... -count=1 -timeout 60s` | Default unit-test pass. |
| `make test-conformance` | `lint-config` then `go test ./testsuite/conformance -count=1 -timeout 60s` | Conformance-only slice. |
| `make test-integ` | `go test ./... -tags integration -count=1 -timeout 120s` | Integration-tagged tests. |
| `make test-scenario` | `go test ./... -tags scenario -count=1 -timeout 180s` | Scenario-tagged tests. |
| `make test-all` | `test-unit`, `test-integ`, `test-scenario` | Full standard matrix. |
| `make test-contract-migration` | Contract dissolution gate plus focused package subset | Used for the migration baseline. |
| `make test-dev-agent` | Builds and tests `app/dev-agent-cli/...` | Uses repo-local Go caches under `/tmp` when needed. |
| `make test-tape-fidelity` | Tape replay and fidelity checks | Covers `platform/llm`, `testsuite/agenttest/tapes`, and `app/relurpish/runtime`. |
| `make test-euclo-golden` | Euclo thoughtrecipe golden characterization | Re-baseline with `UPDATE_GOLDEN=1` when intentional changes are made. |

## Agent-test workflow

`dev-agent agenttest` runs YAML-driven suites from `testsuite/agenttests/`.

Basic invocations:

```bash
go build -o dev-agent ./app/dev-agent-cli
./dev-agent agenttest run
./dev-agent agenttest run --agent euclo
./dev-agent agenttest run --suite testsuite/agenttests/euclo.code.testsuite.yaml
```

Useful flags:

```text
--suite <path>              Repeatable suite path
--agent <name>              Discover suites for one agent
--case <name>               Run one case from a suite
--tag <tag>                 Filter cases by tag
--lane pr-smoke|merge-stable|quarantined-live
--profile ci-live|ci-replay|live|replay|developer-live
--strict                    Fail on non-skipped case failures
--skip-ast-index            Default true
--backend-reset none|model|server
--backend-reset-between
--backend-reset-on <regex>
```

The run artifacts for a case land under `relurpify_cfg/test_runs/{agent}/{run_id}/`.

The suite README is the reference for per-suite layout and examples:

- [testsuite/agenttests/README.md](../testsuite/agenttests/README.md)

## Golden baselines

When a golden test changes intentionally, re-baseline by rerunning the target with `UPDATE_GOLDEN=1`.

Example:

```bash
UPDATE_GOLDEN=1 go test ./named/euclo/thoughtrecipes/ -run TestGolden -count=1 -v
```

## Local cache note

Some make targets create local caches under `/tmp/relurpify-go-cache`, `/tmp/relurpify-go-tmp`, or `/tmp/relurpify-go-modcache` to avoid host cache permission problems. That is a convenience for sandboxed environments, not a repo requirement.
