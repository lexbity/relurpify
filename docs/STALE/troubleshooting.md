# Troubleshooting

This file collects the verified sharp edges that are useful during local
development.

## Ollama or model readiness

Symptoms:

- `doctor` reports the model backend as unhealthy
- agent tests fail because the selected model is not present

Checks:

- start Ollama
- confirm the model named in the suite or workspace config is actually pulled
- rerun `relurpish doctor`

The repo currently contains mixed examples for `gemma4:e4b` and `gemma4:12b`.
The suite YAML is authoritative for a given run.

## Host cache permission errors

Some commands may fail when the Go build cache points at a read-only host path.
Use repo-local or `/tmp` caches instead.

Examples:

- `GOCACHE=$PWD/.gocache`
- `GOMODCACHE=$PWD/.gomodcache`
- `GOCACHE=/tmp/relurpify-go-cache`

## `doctor --fix`

`relurpish doctor --fix` can materialize or overwrite starter workspace config.
Use it when `relurpify_cfg/` is missing or when the starter files need to be
regenerated from the embedded templates.

## `ayenitd`

`ayenitd` is not a standalone user-facing binary in the current tree. It is an
internal service lifecycle package that the runtime uses to register workspace
services and bootstrap indexing. If you are looking for a launch command, use
`relurpish` instead.

## Nexus

See [docs/known-limitations.md](known-limitations.md) for the current Nexus
status. Treat `nexus` names in older comments and docs as historical unless a
verified implementation reappears.

## Workspace indexing

If the workspace index is not ready:

- check `ayenitd/bkc_bootstrap.go`
- check `context/knowledge/ast/index_manager.go`
- check the workspace permissions passed into the runtime

## Background jobs

If a background Euclo job appears to hang:

- check the job submitter implementation
- check `named/euclo/orchestrate/background.go`
- check the telemetry events for submission/completion

## What not to assume

- Do not assume a single persistence engine for all state.
- Do not assume `nexus` is live just because old comments mention it.
- Do not assume every command example has been re-run in this workspace.
- See [docs/known-limitations.md](known-limitations.md) for the repo-level
  limitations that should stay visible in the top-level docs.
