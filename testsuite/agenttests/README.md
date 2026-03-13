`dev-agent agenttest` suites are YAML files that define prompt-based checks for a specific agent + manifest + model(s).

Runs now execute inside derived temporary workspaces under `relurpify_cfg/test_runs/.../tmp/`, with a testsuite template profile materializing the temp `relurpify_cfg/` for each case.

Suites in this directory are the canonical source. Run them directly:

```
go build ./cmd/dev-agent
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
GOCACHE=$PWD/.gocache GOMODCACHE=$PWD/.gomodcache go build ./cmd/dev-agent
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
```

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
