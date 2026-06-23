# Configuration

Relurpify keeps process-environment handling in `userconfig/config`. That package is the load boundary for workspace config, model/provider manifests, security policies, tool manifests, and env-only secrets.

## Load boundary

The configuration loader resolves:

- workspace config from `relurpify_cfg/workspace.yaml`
- provider manifests from `relurpify_cfg/model/provider/*.provider.yaml`
- profile manifests from `relurpify_cfg/model/profiles/*.llm.yaml`
- security policy files from `relurpify_cfg/security/*.yaml`
- tool manifests from `relurpify_cfg/tools/**/*.tool.yaml`
- env-only secrets and startup hints

The checked-in config tree is data-only. Runtime state belongs in `.relurpify_state/`.

## `RELURPIFY_*` environment variables

These variables are consumed by `userconfig/config`.

| Name | Kind | Purpose | Notes |
|---|---|---|---|
| `RELURPIFY_WORKSPACE` | override | Set the workspace root passed into config loading. | Used when the CLI does not set `--workspace`. |
| `RELURPIFY_MODEL_PROVIDER` | override | Override the workspace model provider. | Consumed during config load. |
| `RELURPIFY_MODEL_NAME` | override | Override the workspace model name. | Consumed during config load. |
| `RELURPIFY_SANDBOX_BACKEND` | override | Override the sandbox backend. | Consumed during config load. |
| `RELURPIFY_OLLAMA_HOST` | override | Override the Ollama endpoint host. | Consumed during config load. |
| `RELURPIFY_LOG_LEVEL` | override | Override the log level. | Consumed during config load. |
| `RELURPIFY_STRICT` | override | Enable strict config loading. | Invalid boolean values fail fast. |
| `RELURPIFY_LLM_API_KEY` | secret | API key used by auth-requiring provider kinds. | Required when the selected provider kind is `openai_compatible`. |
| `RELURPIFY_NEXUS_TOKEN` | secret | Secret-only token. | Purpose is not documented in code comments or docs yet. |
| `RELURPIFY_NEXUS_ADMIN_TOKEN` | secret | Secret-only admin token. | Purpose is not documented in code comments or docs yet. |
| `RELURPIFY_REDUCE_MOTION` | preference | Force reduced-motion mode in the TUI. | Any non-empty value enables the preference. |

## Other startup environment variables

These are also read at startup, but they are not part of the `RELURPIFY_*` config set.

| Name | Purpose | Notes |
|---|---|---|
| `EDITOR` | Default editor used by `relurpish`. | Falls back to `vi` when empty. |
| `XDG_DATA_HOME` | Base for the shared local state root. | Used to derive the shared `relurpify` directory. |
| `CI` | Reduced-motion hint. | Any non-empty value enables reduced motion. |
| `GITHUB_ACTIONS` | Reduced-motion hint. | Any non-empty value enables reduced motion. |
| `SSH_TTY` | Reduced-motion hint. | Any non-empty value enables reduced motion. |
| `SSH_CONNECTION` | Reduced-motion hint. | Any non-empty value enables reduced motion. |
| `TERM` | Terminal type hint. | Passed through for UI/runtime terminal behavior. |

## Provider kinds

Provider manifests currently validate these kinds:

- `ollama`
- `openai_compatible`
- `lmstudio`
- `offline`

Auth requirement:

- `openai_compatible` requires `RELURPIFY_LLM_API_KEY`.
- the other kinds do not require that key according to the current loader.

## Checked-in config layout

The current `relurpify_cfg/` tree contains:

- `workspace.yaml`
- `security/`
- `model/provider/`
- `model/profiles/`
- `tools/`
- `tooltests/`

Anything that is runtime output, generated state, or session-specific artifacts belongs in `.relurpify_state/`, not in `relurpify_cfg/`.

## Practical notes

- `relurpify_cfg/README.md` is the highest-level checked-in config overview.
- `testsuite/agenttests/README.md` is the best reference for agent-test fixture layout.
- `app/relurpish` reads the same config boundary, then adds TUI-specific flags and startup state.
