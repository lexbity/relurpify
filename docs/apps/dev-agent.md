# dev-agent

`dev-agent` is the developer-facing CLI in `app/dev-agent-cli`. It is used for
manifest utilities, skill scaffolding, and YAML-driven agent tests.

## Common Commands

```bash
dev-agent agents list
dev-agent agents create --name my-agent
dev-agent skill init my-skill --description "My focused workflow" --with-tests
dev-agent skill validate my-skill
dev-agent agenttest run
dev-agent start --instruction "Summarize this repository"
```

## Main Command Groups

| Command | Purpose |
|---------|---------|
| `agents` | List, create, and validate manifests |
| `skill` | Scaffold, validate, test, and inspect skills |
| `agenttest` | Run, refresh, and promote agent tests |
| `start` | Start a development agent run directly |
| `session` | Inspect saved session snapshots |
| `config` | Read and update `config.yaml` |
