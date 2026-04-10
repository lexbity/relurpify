# Platform Shell

## Scope

`platform/shell` is the local tooling layer for command-oriented agent actions.
It provides the registry, catalog, query, and execution plumbing used to expose
Unix-style utilities, workspace-aware command presets, and a small set of
legacy multi-step execution helpers.

This package is not the sandbox or authorization authority. It describes tools,
normalizes requests, and delegates execution through the framework's sandbox
runner and policy layers.

## Package Layout

The package is organized into:

- `platform/shell/catalog` for canonical tool metadata and schema handling
- `platform/shell/query` for bounded discovery and instantiation queries
- `platform/shell/execute` for reusable command preset execution
- `platform/shell/command` for the generic CLI wrapper used by most shell tools
- `platform/shell/text`, `fileops`, `network`, `system`, `build`, `archive`, and `scheduler` for tool families
- `platform/shell/telemetry` for optional local telemetry events
- `platform/shell/execution.go` for older multi-step execution helpers

The root package assembles the default tool surface and the canonical catalog.

## Root Package Responsibilities

`platform/shell` provides three top-level entry points:

- `CommandLineTools(basePath, runner)` returns the default tool set
- `CommandLineToolsWithTelemetry(basePath, runner, telemetry)` returns the same set with optional query telemetry
- `ToolCatalog()` returns a deterministic canonical catalog for the shell families

Assembly order matters.

- `text`, `fileops`, `system`, `build`, `archive`, `network`, and `scheduler` are loaded in that order
- duplicate tool names are dropped after normalization
- `platform/shell/system` therefore wins over `platform/shell/build` for the duplicated `strace` name
- language and database helpers from `platform/lang/*` and `platform/db/sqlite` are appended after the base shell families
- query tools from `platform/shell/query` are appended last and are backed by the shell catalog

Any tool that implements `SetCommandRunner` receives the shared sandbox runner during assembly.

## Canonical Catalog Model

The catalog layer is the source of truth for shell discovery metadata.

Important traits:

- names and aliases are normalized through `catalog.NormalizeName`
- entries are stored in deterministic canonical order
- alias lookups resolve to the canonical entry
- duplicate canonical names and conflicting aliases are rejected at registration time
- the catalog supports a declarative `CommandToolSpec` form for authoring entries

Catalog entries carry:

- canonical name and aliases
- family and intent labels
- short and long descriptions
- structured parameter and output schemas
- command preset metadata
- tags
- deprecation state and replacement information
- examples

Command presets capture the executable form of an entry:

- command template
- default arguments
- stdin support
- working-directory support
- result style

The current command-family catalog is built from family registries rather than
from the full platform tool surface.

## Query Surface

`platform/shell/query` exposes two read-only tools:

- `shell_tool_discover`
- `shell_tool_instantiate`

Discovery is for ranked search. Instantiation is for resolving a tool and
materializing a framework-facing request envelope.

Discovery query signals include:

- tool name
- aliases
- family
- intent
- keywords
- required parameters
- preferred output style
- workspace hints
- maximum results
- deprecated-tool opt-in

Workspace hints currently include:

- Cargo workspace presence
- Go module presence
- package.json presence
- Python source presence
- notebook presence
- git repository presence
- language
- project type

Instantiation accepts:

- tool name
- aliases
- family
- structured arguments
- workspace hints
- deprecated-tool opt-in

Validation rules are intentionally strict:

- discovery must include at least one search signal
- discovery max results is capped at 25
- instantiation must identify a tool, alias, or family
- unknown input fields are rejected during parsing

## Discovery Behavior

The discovery engine scores catalog entries against the normalized query.

Scoring signals currently include:

- exact tool name matches
- alias matches
- family matches
- intent matches
- keyword matches
- required-parameter matches
- preferred-output matches
- workspace-context bias
- a small deprecation penalty

Results are sorted by score descending and name ascending for deterministic
output. The result payload includes:

- the normalized query string
- the matched tool list
- a family summary

Telemetry is optional. When configured, the engine emits lightweight events for
query start, validation failures, alias resolution, and completion.

## Instantiation Behavior

Instantiation resolves a tool in this order:

- direct tool name
- aliases
- family, if the family is unambiguous

After resolution, the engine:

- rejects deprecated tools unless explicitly allowed
- validates the structured arguments against the entry schema
- builds CLI arguments from the validated arguments
- extracts `working_directory` and `stdin` when the preset allows them
- returns both a reusable command preset and a sandbox command request

The result is a structured materialization step, not execution. Execution still
goes through the sandbox runner.

## Command Wrapper Layer

`platform/shell/command` is the generic wrapper used by most local CLI tools.

The wrapper exposes a common parameter shape:

- `args`
- `stdin`
- `working_directory`

Its execution path:

- builds a reusable preset
- resolves relative work directories against the workspace root
- delegates process execution to `platform/shell/execute`
- returns stdout, stderr, and execution metadata in a stable envelope

The wrapper also exposes permission metadata through the framework permission
set helpers. `HITLRequired` is supported on a per-command basis.

## Execution Adapter

`platform/shell/execute` is the lower-level reusable executor.

It is responsible for:

- normalizing command presets
- applying a default timeout and category when omitted
- resolving workspace-relative paths
- forwarding stdin when the preset allows it
- building the final sandbox command request
- returning a stable result envelope with stdout, stderr, elapsed time, and metadata

Cargo handling is a special case:

- if the preset command is `cargo`, the executor may inject `--manifest-path`
- when the selected workspace is nested inside another Cargo workspace, the executor isolates the run in a temporary copy
- the isolated copy excludes `.git`, `target`, and `*.bak` files
- the temporary tree is removed after execution

This logic is shared by the generic command wrapper and the query-instantiated
presets.

## Legacy Execution Helpers

`platform/shell/execution.go` contains older multi-step tools that do not fit the
simple wrapper model.

Current helpers include:

- `exec_run_tests`
- `exec_run_code`
- `exec_run_linter`
- `exec_run_build`

These helpers:

- build compound command lines directly
- return stdout and stderr in a plain result envelope
- use the same sandbox command runner abstraction
- still expose permission and agent-spec setter hooks

Important caveat:

- the current `authorizeCommand` path is disabled as an import-cycle workaround
- these helpers therefore depend on the surrounding framework policy path rather
  than enforcing their own authorization logic
- `exec_run_code` is the highest-risk surface because it explicitly represents
  arbitrary code execution

## Security and Policy Boundary

The shell package is intentionally not the policy authority.

The surrounding framework is responsible for:

- authorization decisions
- sandbox execution constraints
- command allowlists or blacklists
- process isolation

Within `platform/shell`, the main security-relevant behaviors are:

- normalizing tool names and aliases before lookup
- keeping discovery and instantiation read-only
- exposing explicit permission metadata for command wrappers
- isolating Cargo runs when workspace nesting would otherwise leak across manifests
- preserving a narrow parameter surface for query tools

## Tool Families

### Text

Text tools are mostly single-binary wrappers for stream and text processing.

Representative tools include:

- `awk`
- `echo`
- `sed`
- `perl`
- `jq`
- `yq`
- `tr`
- `cut`
- `paste`
- `column`
- `sort`
- `uniq`
- `comm`
- `rev`
- `wc`
- `patch`
- `ed`
- `ex`
- `xxd`
- `hexdump`
- `diff`
- `colordiff`

Behavioral profile:

- mostly pure wrappers
- standard input is supported by the generic command adapter
- output is captured as stdout and stderr

### File Operations

File operation tools cover search, inspection, and basic mutation utilities.

Representative tools include:

- `git` passthrough wrapper
- `find`
- `fd`
- `rg`
- `ag`
- `locate`
- `tree`
- `stat`
- `file`
- `touch`
- `mkdir`

Behavioral profile:

- mostly read-oriented inspection tools
- `git` here is a compatibility shim for generic command passthrough
- `mkdir` is preconfigured with recursive creation behavior

### Network

Network tools are wrappers around local inspection and connectivity utilities.

Representative tools include:

- `curl`
- `wget`
- `nc`
- `dig`
- `nslookup`
- `ip`
- `ss`
- `ping`

Behavioral profile:

- mostly pure wrappers
- used for probing, inspection, and download-style requests

### System

System tools cover host introspection and administrative utilities.

Representative tools include:

- `lsblk`
- `df`
- `du`
- `ps`
- `top`
- `htop`
- `lsof`
- `strace`
- `time`
- `uptime`
- `systemctl`

Behavioral profile:

- mostly host-inspection wrappers
- `systemctl` is the clearest destructive/system-management surface
- `strace` overlaps with the build family and is deduped by name in the root assembly

### Build

Build tools wrap common build, language, and debugging binaries.

Representative tools include:

- `make`
- `cmake`
- `cargo`
- `go`
- `python`
- `node`
- `npm`
- `sqlite3`
- `rustfmt`
- `pkg-config`
- `gdb`
- `valgrind`
- `ldd`
- `objdump`
- `perf`
- `strace`

Behavioral profile:

- mostly build and verification wrappers
- `cargo` gets special handling for manifest-path injection and nested-workspace isolation
- `gdb`, `perf`, and `strace` are the highest-risk presets in this family

### Archive

Archive tools expose compression and archive utilities.

Representative tools include:

- `tar`
- `gzip`
- `bzip2`
- `xz`

Behavioral profile:

- pure archive and compression wrappers

### Scheduler

Scheduler tools expose time-based job utilities.

Representative tools include:

- `at`
- `crontab`

Behavioral profile:

- pure wrappers for local scheduling commands

### Supplemental Platform Tools

The root shell assembly also appends domain-specific platform tools that are not
part of the shell-family catalog:

- `platform/lang/rust`
- `platform/lang/python`
- `platform/lang/js`
- `platform/lang/go`
- `platform/db/sqlite`

These tools provide workspace-aware detection, metadata, and project-specific
checks. They are included in the assembled tool list, but they are not part of
`ToolCatalog()`'s shell-family registry.

## Operational Notes

- all catalog and query names are normalized, so spelling variants and punctuation
  differences collapse to the same lookup key
- command wrapper metadata is deterministic because family registries are ordered
  and deduped before exposure
- query tools are read-only and do not expose framework permissions
- the default command wrapper always exposes args, stdin, and working-directory
  handling
- query instantiation returns a request envelope but does not execute it
- telemetry is optional and stays local to the shell query engine

## Source Map

- [`/home/lex/Public/Relurpify/platform/shell/doc.go`](../../platform/shell/doc.go)
- [`/home/lex/Public/Relurpify/platform/shell/catalog/catalog.go`](../../platform/shell/catalog/catalog.go)
- [`/home/lex/Public/Relurpify/platform/shell/query/query.go`](../../platform/shell/query/query.go)
- [`/home/lex/Public/Relurpify/platform/shell/query/tool.go`](../../platform/shell/query/tool.go)
- [`/home/lex/Public/Relurpify/platform/shell/execute/executor.go`](../../platform/shell/execute/executor.go)
- [`/home/lex/Public/Relurpify/platform/shell/command/cli_command.go`](../../platform/shell/command/cli_command.go)
- [`/home/lex/Public/Relurpify/platform/shell/execution.go`](../../platform/shell/execution.go)
- [`/home/lex/Public/Relurpify/platform/shell/telemetry/telemetry.go`](../../platform/shell/telemetry/telemetry.go)
- [`/home/lex/Public/Relurpify/platform/shell/text`](../../platform/shell/text)
- [`/home/lex/Public/Relurpify/platform/shell/fileops`](../../platform/shell/fileops)
- [`/home/lex/Public/Relurpify/platform/shell/network`](../../platform/shell/network)
- [`/home/lex/Public/Relurpify/platform/shell/system`](../../platform/shell/system)
- [`/home/lex/Public/Relurpify/platform/shell/build`](../../platform/shell/build)
- [`/home/lex/Public/Relurpify/platform/shell/archive`](../../platform/shell/archive)
- [`/home/lex/Public/Relurpify/platform/shell/scheduler`](../../platform/shell/scheduler)
