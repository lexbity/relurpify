# Relurpish

End-user Bubble Tea shell for Relurpify.

## Quick start (zero external deps)

```bash
relurpish doctor --offline --fix   # materialize workspace config, check readiness
relurpish chat --offline           # boot euclo with no external process
```

The `--offline` flag selects the built-in offline inference backend, which
requires no network, no Ollama, and no API key. Turn submissions use a
deterministic scripted model — enough to verify plumbing (tool dispatch,
streaming, context compilation) without a real LLM.

## Panes

- `chat`
- `planner`
- `debug`
- `config`
- `session`

## Features

- interactive agent chat
- euclo planner/debug integration
- guidance and HITL handling
- queued task and session management
- runtime-backed config, capability, and prompt inspection
- workspace bootstrap via `relurpish doctor`
