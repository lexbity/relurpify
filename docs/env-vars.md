# Environment Variables

`cfgload` is the only package that reads environment variables. The application captures the environment once at startup and passes it through the load boundary.

## Recognized Variables

- `RELURPIFY_WORKSPACE`
- `RELURPIFY_MODEL_PROVIDER`
- `RELURPIFY_MODEL_NAME`
- `RELURPIFY_SANDBOX_BACKEND`
- `RELURPIFY_OLLAMA_HOST`
- `RELURPIFY_LOG_LEVEL`
- `RELURPIFY_STRICT`
- `EDITOR`
- `XDG_DATA_HOME`
- `RELURPIFY_LLM_API_KEY`
- `RELURPIFY_NEXUS_TOKEN`
- `RELURPIFY_NEXUS_ADMIN_TOKEN`

## Notes

- `RELURPIFY_WORKSPACE` selects the workspace root when no CLI override is provided.
- `RELURPIFY_MODEL_PROVIDER` and `RELURPIFY_MODEL_NAME` override the workspace model selection.
- `RELURPIFY_SANDBOX_BACKEND` overrides the workspace sandbox backend.
- `RELURPIFY_OLLAMA_HOST` overrides the Ollama endpoint.
- `RELURPIFY_LOG_LEVEL` overrides workspace logging level.
- `RELURPIFY_STRICT` enables strict config loading.
- `EDITOR` sets the editor used by UI edit actions.
- `XDG_DATA_HOME` controls the machine-local shared template root.
- `RELURPIFY_LLM_API_KEY`, `RELURPIFY_NEXUS_TOKEN`, and `RELURPIFY_NEXUS_ADMIN_TOKEN` are secrets and are never written to disk.
