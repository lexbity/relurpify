`coding-agent agenttest` suites are YAML files that define prompt-based checks for a specific agent + manifest + model(s).

- Copy these into a workspace’s `relurpify_cfg/testsuites/` directory, or generate them via `coding-agent agenttest init`.
- Run them via `coding-agent agenttest run`.

---

go build ./cmd/coding-agent
Run:

./coding-agent agenttest init --force
./coding-agent agenttest run --agent coding
Optional (avoid host cache perms / keep deps local):

GOCACHE=$PWD/.gocache GOMODCACHE=$PWD/.gomodcache go build ./cmd/coding-agent
GOCACHE=$PWD/.gocache GOMODCACHE=$PWD/.gomodcache ./coding-agent agenttest run --agent coding

./coding-agent agenttest run --agent coding --timeout 2m

coding-agent agenttest run:
--ollama-reset none|model|server (default none)
--ollama-reset-between (reset before each case)
--ollama-reset-on <regex> (repeatable; defaults include timeout/EOF/connection reset)
--ollama-bin ollama (path/name)
--ollama-service ollama (for systemctl restart)
Example usage:

Unload model between cases: ./coding-agent agenttest run --agent coding --ollama-reset model --ollama-reset-between
Restart server and auto-retry on timeouts: ./coding-agent agenttest run --agent coding --ollama-reset server --ollama-reset-between --timeout 2m

