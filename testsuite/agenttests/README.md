`dev-agent agenttest` suites are YAML files that define prompt-based checks for a specific agent + manifest + model(s).

Runs now execute inside derived temporary workspaces under `relurpify_cfg/test_runs/.../tmp/`, with a testsuite template profile materializing the temp `relurpify_cfg/` for each case.

Suites in this directory are the canonical source. Run them directly:

```
go build ./cmd/dev-agent
./dev-agent agenttest run
./dev-agent agenttest run --agent coding
./dev-agent agenttest run --suite testsuite/agenttests/coding.go.testsuite.yaml
```

Optional (keep deps local to avoid host cache permission issues):
```
GOCACHE=$PWD/.gocache GOMODCACHE=$PWD/.gomodcache go build ./cmd/dev-agent
GOCACHE=$PWD/.gocache GOMODCACHE=$PWD/.gomodcache ./dev-agent agenttest run --agent coding
```

Flags:
```
--timeout 2m
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

# Restart server and auto-retry on timeouts
./dev-agent agenttest run --agent coding --ollama-reset server --ollama-reset-between --timeout 2m
```
