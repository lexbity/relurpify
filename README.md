# Relurpify

Relurpify is a local-first agent framework and terminal runtime for code-oriented workflows. The main entry point is `relurpish`, a Bubble Tea TUI backed by local manifests, sandboxed tool execution, and Ollama-based models.

## What You Run

- `relurpish` in app/relurpish for interactive local use
- `dev-agent` in app/dev-agent-cli for developer workflows, agent tests, and manifest/skill utilities
- `nexus` in app/nexus for distributed coordination
- `nexusish` in app/nexusish for Nexus administration

## Requirements

- Go 1.25+
- Docker or another supported container runtime
- gVisor `runsc`
- Ollama

In sandboxed environments you may also want repo-local Go caches:

```bash
export GOMODCACHE=$PWD/.gomodcache
export GOCACHE=$PWD/.gocache
```

## First Run

```bash
go build ./app/relurpish
go run ./app/relurpish doctor
go run ./app/relurpish chat
```

`doctor` checks local dependencies and initializes `relurpify_cfg/` when needed. After that, `chat` starts the TUI in the current workspace.

## Common Commands

```bash
# Build everything
go build ./...

# List discovered agents
go run ./app/dev-agent-cli agents list

# Run agent tests
go run ./app/dev-agent-cli agenttest run

# Scaffold a skill
go run ./app/dev-agent-cli skill init my-skill --description "My focused workflow" --with-tests

# Validate a skill
go run ./app/dev-agent-cli skill validate my-skill
```

## Documentation

The repository documentation lives under `docs/`.
