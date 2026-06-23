# Central Data-only Relurpify Configuration directory

## 1. Guiding Principles

These principles ground every decision in this spec. When implementation details conflict, refer back here.

### 1.1 One Load Boundary

All configuration is read at process startup by a single loader in a single package (`userconfig/config`). After that boundary, application and runtime packages consume the resolved `AppConfig`, `WorkspaceConfig`, `ModelConfig`, `Security` bundle, and `Secrets` values instead of reading process env directly.

### 1.2 Fail Fast, Fail Loudly

A missing required file, a schema violation, an unresolved variable, a type error — any of these must halt the process with a specific, actionable error message including the file path and the offending line or key. There are no silent defaults for security-relevant config.

Non-security defaults (e.g. `audit.retention_days`) may use documented defaults with a structured log warning noting which key used a default and what value was chosen.

**Violated today by:** `resolveWorkspaceConfigOverrides` returning a default struct on file read failure.

### 1.3 One Schema Per File Kind

Every config file declares its schema in the first non-comment line. The loader reads the schema declaration before parsing any content. Unknown schemas are hard errors. Schema versions allow explicit evolution without implicit drift.

### 1.4 Static Config Is Read-Only

`relurpify_cfg/` is a read-only configuration root. It contains no runtime-generated state. Databases, logs, telemetry, session data, and test artifacts live in `.relurpify_state/` (gitignored). The filesystem security model enforces this: agents cannot write to `relurpify_cfg/` regardless of their manifest permissions.


### 1.5 Secrets Never Touch Disk

API keys, tokens, and credentials are never fields in config structs that get serialized to disk. They come exclusively from environment variables, resolved at the load boundary into a separate `Secrets` struct. The `Secrets` struct is never written to disk, never logged, never included in config dumps.

### 1.6 Paths Are Workspace-Relative

No committed config file contains absolute paths. The variable `${workspace}` is the only path root, resolved to the absolute workspace path during loading. Hardcoded paths like `${workspace}/**` are a deployment error, not a config value.

### 1.7 `relurpify_cfg` Is Excluded From Agent Filescopes

Agents cannot read or write `relurpify_cfg/` regardless of their declared filesystem permissions. This is enforced at the `FileScopePolicy` level, not by convention. The security model depends on config files being immutable from the agent's perspective — an agent that can modify its own manifest can escalate its own privileges.

### 1.8 Configuration Is Data, Not Code

Config files declare *what* — tool parameters, agent capability lists, policy rules, model adapter quirks. They do not express *how* — workflow logic, planning phases, verification rules, conditional branching. Workflow logic belongs in ThoughtRecipes (the DSL). Behavioral instructions belong in `.prompt` v2 files. Any config that requires conditional evaluation is in the wrong place.




## Directory Layout

The checked-in configuration tree currently contains:

- `workspace.yaml` for workspace-level settings.
- `security/` for sandbox, shell, local tool, and workspace-ingestion policy files.
- `model/provider/` for provider manifests.
- `model/profiles/` for model profile manifests.
- `tools/` for capability/tool manifests.
- `tooltests/` for checked-in tooltest fixtures.

What is not here belongs in `.relurpify_state/` or is generated at build/test time:

- Runtime logs and telemetry.
- Session artifacts and test-run outputs.
- Temporary working directories.

### Notes

- The historical loader path name is stale and should not be used in new documentation.
- The checked-in tree uses the `userconfig/config` package as the loading boundary.
