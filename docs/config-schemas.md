# Config Schemas

All committed config files declare a schema in the first non-comment line. The loader rejects missing or unknown schemas.

## `relurpify/workspace/v1`

File: `relurpify_cfg/workspace.yaml`

- `paths.state_dir` - workspace-relative state root. Default: `.relurpify_state`.
- `model.provider` - default LLM provider name.
- `model.name` - default model name.
- `sandbox.backend` - default sandbox backend. Allowed values: `gvisor`, `docker`, `local`.
- `logging.level` - default log level. Allowed values: `debug`, `info`, `warn`, `error`.
- `logging.format` - log format. Allowed values: `json`, `text`.
- `audit.retention_days` - audit retention window in days. Default: `7`.
- `telemetry.enabled` - enable workspace telemetry. Default: `true`.

## `relurpify/policy/sandbox/v1`

File: `relurpify_cfg/security/sandbox.policy.yaml`

- `read_only_root` - root filesystem policy.
- `protected_paths` - additional protected paths.
- `no_new_privileges` - hardening toggle.
- `seccomp_profile` - seccomp profile name.
- `allowed_env_keys` - env keys exposed to sandboxed processes.
- `denied_env_keys` - env keys blocked from sandboxed processes.
- `network_rules` - network allow rules.

## `relurpify/policy/shell/v1`

File: `relurpify_cfg/security/shell.policy.yaml`

- `rules[]` - shell policy rules with `id`, `pattern`, `reason`, and `action`.

## `relurpify/policy/localtool/v1`

File: `relurpify_cfg/security/localtool.policy.yaml`

- `tools` - per-tool action policy map.

## `relurpify/policy/ingestion/v1`

File: `relurpify_cfg/security/workspaceingestion.policy.yaml`

- `rules[]` - ingestion filter rules.

## `relurpify/model/provider/v1`

File: `relurpify_cfg/model/provider/*.provider.yaml`

- `name` - provider name.
- `endpoint` - provider endpoint.
- `kind` - provider implementation kind.
- `available_models` - models advertised by the provider.

## `relurpify/model/profile/v1`

File: `relurpify_cfg/model/profiles/*.llm.yaml`

- `pattern` - model match glob.
- `tool_calling` - tool-calling constraints.
- `context` - context window settings.
- `generation` - generation defaults.

## `relurpify/tool/v1`

File: `relurpify_cfg/tools/**/*.tool.yaml`

- `name` - canonical tool name.
- `family` - tool family.
- `intent` - supported use cases.
- `description` - tool summary.
- `parameters[]` - typed parameter list.
- `execution` - command/runtime execution spec.
- `capability` - trust/risk/effect classification.

## `relurpify/agent/v1`

File: `relurpify_cfg/agents/*.agent.yaml`

- `kind` - `base` or `agent`.
- `sandbox` - sandbox runtime and security defaults.
- `model` - agent model reference.
- `filesystem` - filesystem permissions.
- `capabilities` - tool, prompt, and relurpic capability lists.
- `execution` - iteration and HITL settings.
- `audit` - audit defaults.
- `network` - network allow rules.

## Schema Migration

- Schema versions are explicit.
- New required fields or semantic changes require a new version.
- During migration, the loader should reject unsupported versions with a clear error.
- A version is removed only after the migration window ends and the docs are updated.
