# Relurpify

 Relurpify is an extensible Go framework that orchestrates planning agents,
  reasoning graphs, and IDE-facing tools to accelerate code modifications. 
  It exposes an HTTP API, an editor-friendly LSP wrapper, and a CLI so you can embed 
  the same automation stack in multiple environments.

Experimental project for learning purposes and for local agenic automation ; whose sole goal 
is to one day re-write itself.

---

## Repository Layout

```
framework/   Graph runtime, shared context, memory, telemetry, tool registry
agents/      Planner, coder, reflection, and ReAct-inspired orchestrators
agents/templates  Reference agent manifest templates for relurpify_cfg init
tools/       File, git, search, execution, and LSP proxy implementations
cmd/         CLI entry points (server, relurpify toolbox, coder helper)
server/      HTTP + LSP servers and dependency wiring
persistence/ Workflow + message stores for pause/resume and logging
llm/         Ollama HTTP client that satisfies core.LanguageModel
scripts/     Helper scripts (documentation generation, etc.)
relurpify_cfg/ Workspace configuration, manifests, memory, and telemetry outputs
```

Use `ARCHITECTURE.md` for a high-level diagram and data-flow outline. The generated docs website (via `scripts/gen-docs.sh`) bundles that outline next to the Golds API pages.

## Prerequisites

- **Go 1.21+**
- **Local Ollama instance** (or an HTTP-compatible endpoint) with a code-capable model such as `codellama`
- **golds** documentation tool (optional, only required for static docs): `go install go101.org/golds@latest`

In sandboxed environments you can keep module/cache directories inside the repo:

```bash
export GOMODCACHE=$PWD/.gomodcache
export GOCACHE=$PWD/.gocache
```

---

## Build, Run, and Test

### Install dependencies

```bash
go mod tidy
```

### Build everything

```bash
go build ./...
```

### Build CLI apps

```bash
go build ./cmd/coding-agent
go build ./app/relurpish
```

### Run the full test suite

```bash
go test ./...
```

### Coverage report

```bash
go test ./... -coverprofile=coverage.out
go tool cover -func=coverage.out
```

### Run integration agent tests

```bash
go run ./cmd/coding-agent agenttest run --suite testsuite/agenttests/coding.testsuite.yaml
```

### CI helper

```bash
RELURPIFY_AGENTTEST_SUITE=testsuite/agenttests/coding.testsuite.yaml ./scripts/ci.sh
```

### Launch the HTTP server

```bash
export OLLAMA_ENDPOINT=http://localhost:11434
export OLLAMA_MODEL=deepseek-r1:7b

go run ./cmd/coding-agent start --instruction "Summarize README.md"
```

The server exposes `POST /api/task`. Example request:

```bash
curl -s http://localhost:8080/api/task \
  -H 'Content-Type: application/json' \
  -d '{
    "instruction": "Summarize README.md and list missing tests",
    "type": "analysis",
    "context": {"path": "README.md"}
  }' | jq
```

### Use the CLI toolbox instead of the raw server

```bash
# Start a session with the coding agent
go run ./cmd/coding-agent start --agent coding --instruction "Add logging to framework/context.go"

# Validate manifests
go run ./cmd/coding-agent agents validate --name coding

# Initialize testsuites + manifests
go run ./cmd/coding-agent agenttest init
```

### Skill workflows

```bash
# Scaffold a new skill
go run ./cmd/coding-agent skill init my-skill --description "My focused workflow" --with-tests

# Validate skill manifest + resources
go run ./cmd/coding-agent skill validate my-skill

# Diagnose tool/permission compatibility (optional agent manifest)
go run ./cmd/coding-agent skill doctor my-skill --manifest relurpify_cfg/agent.manifest.yaml

# Run the skill testsuite.yaml
go run ./cmd/coding-agent skill test my-skill
```

### Generate documentation (HTML site + architecture outline)

```bash
./scripts/gen-docs.sh
open docs/index.html   # or serve docs/ via any static file server
```

---

## Running Agents inside Editors

The [`server/lsp_server.go`](server/lsp_server.go) package adapts the framework to language-server events:

1. Editors trigger custom commands (e.g., “AI fix”, “AI refactor”).
2. The LSP server collects document context and forwards it as a `core.Task`.
3. The configured agent (default: coding agent with reflection) builds a graph, invokes tools/LLMs, and streams edits back.
