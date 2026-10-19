# Configuration

> **Audience**: users and framework developers who configure a workspace.
> **Purpose**: the complete `relurpify_cfg/` reference: every file, its keys, and the validation rules.
> **Prerequisites**: [Getting started](GETTING_STARTED.md); environment variables live there, not here.

## Where configuration lives

All workspace configuration lives in `relurpify_cfg/` at the workspace root.
The tree is **generated** from an embedded template bundle; doctor
materializes it once on an empty workspace and never overwrites your edits
(`--fix` is the explicit overwrite).
Treat the tree like source code: it is what the runtime loads and
`relurplint` validates.

For the Relurpify repository itself, the checked-in `relurpify_cfg/` is a
materialization of the same bundle, and a CI gate
(`make check-config-tree-drift`) fails if the two diverge.
Framework config changes go into the templates first, then regenerate.

## Validation rules

Every config file is validated strictly at load:

| Rule | Detail |
|---|---|
| Schema declaration | The first non-comment line must be `schema: <kind>/<version>`, for example `schema: relurpify/workspace/v1` |
| Unknown fields | Rejected; a typo is an error, not a silent default |
| YAML anchors and aliases | Rejected outright |
| Secret-like fields | Field names such as `apikey`, `token`, `password`, `secret`, `privatekey`, `credential` are banned in any config file |
| Registered schemas | The declared schema kind/version must be registered in the loader |

Secrets never live in config files.
`RELURPIFY_LLM_API_KEY` is the only secret, and it is read from the
environment (see the [env var table](GETTING_STARTED.md#environment-variables)).

## The tree

```text
relurpify_cfg/
├── workspace.yaml                  # provider, model, sandbox, logging, postures, runner
├── model/
│   ├── provider/                   # one manifest per inference provider (4 ship)
│   └── profiles/                   # per-model behavior profiles (3 ship)
├── security/                       # sandbox, shell blacklist, tool policies, ingestion (4 files)
├── tools/                          # 99 tool manifests
├── prompts/                        # the prompt corpus; see PROMPT_DSL
└── euclo/                          # thoughtrecipes + families.yaml; see RECIPE_DSL
```

## workspace.yaml

Shipped defaults:

| Key | Default | Description |
|---|---|---|
| `schema` | `relurpify/workspace/v1` | Schema declaration (required first line) |
| `paths.state_dir` | `.relurpify_state` | Runtime state directory, relative to the workspace |
| `model.provider` | `ollama` | Active provider name; must match a provider manifest |
| `model.name` | `gemma4:e4b` | Active model; must be served by the provider |
| `sandbox.backend` | `gvisor` | `gvisor` or `docker` |
| `selection.posture` | `deterministic` | `deterministic` (ambiguous intents ask you) or `model_assisted` (tier-2 model may break ties) |
| `logging.level` | `info` | Log level |
| `logging.format` | `json` | Log format |
| `audit.retention_days` | `7` | Audit chain retention |
| `audit.enforcement` | `strict` | Audit enforcement posture |
| `telemetry.enabled` | `false` | Telemetry writing |
| `runner.enabled` | `true` | Background service runner |
| `runner.workers` | `2` | Runner worker count |
| `runner.queues` | `[knowledge]` | Queues the runner drains |
| `runner.poll_interval` | `500ms` | Spool poll interval |
| `runner.heartbeat` | `5s` | `status.json` heartbeat interval |
| `runner.refresh_interval` | `0s` | Cadence for scheduled `knowledge.refresh` submission; `0s` disables it |

Strict-load interaction: with `RELURPIFY_STRICT` set, the five keys with
built-in defaults (`paths.state_dir`, `logging.level`, `logging.format`,
`audit.retention_days`, `telemetry.enabled`) must be present or loading fails;
see [Getting started](GETTING_STARTED.md#environment-variables).

## Provider manifests

One file per inference provider under `model/provider/`, schema
`relurpify/model/provider/v1`.

| Shipped file | Kind | Endpoint | Models | Native tool calling |
|---|---|---|---|---|
| `ollama.provider.yaml` | `ollama` | `http://localhost:11434` | `gemma4:e4b`, `gemma4:12b`, `codellama:13b` | yes |
| `lmstudio.provider.yaml` | `lmstudio` | `http://localhost:1234` | discovered | no |
| `openai_compatible.provider.yaml` | `openai_compatible` | `http://localhost:8080/v1` | yours | yes |
| `offline.provider.yaml` | `offline` | none | `gemma4:e4b`, `offline-synthetic` | scripted |

Implemented kinds: `ollama`, `lmstudio`, `openai_compatible`, `offline`, and
`tape` (record/replay for tests; no manifest ships).
Manifest fields include the request timeout, the available model list, the
native-tool-calling flag, and a concurrency cap (`max_concurrent`).

## Model profiles

Glob-matched behavior per model under `model/profiles/`, schema
`relurpify/model/profile/v1`.
A `default` profile is mandatory; profiles tune tool-calling intent, context
size, generation parameters, and quirks.

| Shipped file | Pattern | Notable settings |
|---|---|---|
| `default.llm.yaml` | `*` | intent `auto`, 4 concurrent tools, 8192-token context, temperature 0.2 |
| `gemma4.llm.yaml` | `gemma4*` | native tool calling, 2 concurrent tools, `repair.strategy: llm` (bounded repair of malformed decisions for thinking models) |
| `qwen2.5-coder.llm.yaml` | `qwen2.5-coder*` | native tool calling, 1 concurrent tool, `double_encode_args: true` (tool-args quirk), temperature 0.1 |

## Security policies

Four files under `security/`; missing pieces fail closed, they never open up.

| File | Schema | What it controls |
|---|---|---|
| `sandbox.policy.yaml` | `relurpify/policy/sandbox/v1` | Container constraints: `protected_paths` (shipped: `relurpify_cfg`), `no_new_privileges`, orphan reaping, and `image_digest` (pin it for a reproducible sandbox) |
| `shell.policy.yaml` | `relurpify/policy/shell/v1` | Shell command blacklist; ships `deny-git-reset-hard` blocking `git reset --hard` |
| `localtool.policy.yaml` | `relurpify/policy/localtool/v1` | Per-tool execute posture; ships `bash: ask` and `cli_git: ask` |
| `workspaceingestion.policy.yaml` | `relurpify/policy/ingestion/v1` | Which files the knowledge layer may ingest; ships an allow rule for the workspace |

Design rules the policies sit on top of:

- The default tool posture is `ask`; permissive defaults are rejected by a
  dedicated lint gate.
- Egress to private or loopback hosts is unconditionally denied.
- `.git` and configuration roots are protected paths.
- Effect/risk/trust class strings in capability blocks are normalized at the
  load boundary; `make lint-class-normalization` fails any code path that
  skips the normalization.

## Tool manifests

The 98 files under `tools/` declare every model-callable tool: parameters,
execution backend, flag expansion, sandbox limits, result chunking, exit-code
error maps, rate limits, and a mandatory capability block declaring trust,
risk, and effect classes.

| Category | Count | Examples |
|---|---|---|
| `file/` | 6 | `file_read`, `file_write`, `file_list` |
| `git/` | 5 | `cli_git` (blame, branch, commit, diff, history) |
| `lsp/` | 7 | `lsp_get_diagnostics`, `lsp_get_definition` |
| `search/` | 3 | `search_grep`, `search_semantic`, `search_find_similar` |
| `shell/` | 73 | build, text, network, system, archive, fileops tools plus `bash` |
| `test/` | 5 | `python_pytest`, `python_unittest`, `go_test`, `node_npm_test`, `rust_cargo_test` |

Tool names in recipes and policies resolve through the registry, never by
literal string match; see [RECIPE_DSL](RECIPE_DSL.md#tool-vocabulary) for the
resolution order.

## Safe and dangerous changes

Safe:

- Scalar values in `workspace.yaml` (provider, model, posture, logging).
- New provider manifests and model profiles.
- Security policy values within the fail-closed rules above.
- New tool manifests, thoughtrecipes, and prompt files.
- Anything under `euclo/`: your edits win over the templates.

Dangerous:

- Hand-editing the generated tree inside the Relurpify repository itself
  (the drift gate fails; edit the templates and regenerate).
- Renaming config keys (unknown fields are errors downstream).
- Trying to set a permissive default tool posture (rejected by lint).
- Declaring network egress to private or loopback hosts (denied regardless of
  policy).
- Wrong-case class strings in capability blocks (normalized at load, but the
  lint gate flags unnormalized sites in code).

## Load semantics

- Precedence: CLI flag > environment > workspace config > built-in default.
- A missing `workspace.yaml` is fine: defaults apply.
- A present but partial `workspace.yaml` under strict mode is a load error
  that names the defaulted keys.
- Doctor reports provider health, profile match, and API-key presence as
  blocking diagnostics; the UI stays locked until they resolve.
- After every load the runtime logs a fingerprint of the resolved,
  secret-free configuration.

## Where to go next

- [Recipe DSL](RECIPE_DSL.md) — the `euclo/` directory in depth.
- [Prompt DSL](PROMPT_DSL.md) — the `prompts/` directory in depth.
- [Getting started](GETTING_STARTED.md#troubleshooting) — when loading fails.
