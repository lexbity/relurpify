# Getting Started

> **Audience**: new users and developers who want a working relurpish workspace.
> **Purpose**: from zero to a first completed agent turn.
> **Prerequisites**: none; this is the starting doc.

## Prerequisites

| Requirement | Version / notes |
|---|---|
| Go | 1.25+ |
| Container runtime | Docker; plus gVisor `runsc` for the default `gvisor` sandbox backend (the `docker` backend needs Docker alone) |
| Model backend | Ollama (default, endpoint `http://localhost:11434`), LM Studio (`http://localhost:1234`), any OpenAI-compatible endpoint, or the built-in `offline` scripted model |
| OS | Linux or macOS (amd64 / arm64) |
| git | Recommended in the workspace; the diff and verify tooling works best on a clean repository |

## 1. Build

From the repository root:

```bash
go build -o relurpish ./app/relurpish
go build -o relurplint ./app/relurplint
```

In sandboxed or locked-down machines, repo-local Go caches help:

```bash
export GOMODCACHE=$PWD/.gomodcache
export GOCACHE=$PWD/.gocache
```

You can also run without installing: `go run ./app/relurpish ...`.

## 2. Prepare a model backend

The default provider is Ollama with the model `gemma4:e4b`:

```bash
ollama serve                # in its own terminal, if not installed as a service
ollama pull gemma4:e4b
```

Other backends:

- **LM Studio**: start the local server (default `http://localhost:1234`).
- **OpenAI-compatible endpoint**: set the endpoint in
  `relurpify_cfg/model/provider/openai_compatible.provider.yaml` and export
  `RELURPIFY_LLM_API_KEY`.
- **Offline**: no backend needed; see [Running offline](#running-offline).

Thinking models are supported; the shipped `gemma4` profile arms bounded
decision repair for malformed model output.

## 3. Initialize a workspace

A **workspace** is any project directory you want the agent to work in.

```bash
mkdir -p ~/code/my-demo && cd ~/code/my-demo
git init                    # recommended
/path/to/relurpish doctor
```

On an empty workspace, doctor **materializes the starter configuration** into
`relurpify_cfg/`:

- `workspace.yaml` — provider, model, sandbox, logging, postures
- `model/provider/` and `model/profiles/` — endpoints and per-model behavior
- `security/` — sandbox, shell blacklist, tool policies, ingestion rules
- `tools/` — 98 tool manifests
- `prompts/` — the prompt corpus
- `euclo/` — shipped thoughtrecipes plus `families.yaml`

Doctor never overwrites your edits; `--fix` explicitly (re)materializes, and
`--yes` applies fixes without prompting.
`--set-provider <name>` switches the provider and re-runs the diagnosis.

Two informational warnings are normal on a healthy machine: `runner: missing`
(the app spawns the runner on first chat) and `runtime_image: degraded` (set
`image_digest` in `relurpify_cfg/security/sandbox.policy.yaml` to pin the
container image).

## 4. Configure the model

Edit `relurpify_cfg/workspace.yaml`:

```yaml
model:
  provider: ollama      # ollama | lmstudio | openai_compatible | offline
  name: gemma4:e4b      # any model your backend serves
```

The full tree is documented in [Configuration](CONFIGURATION.md).
The `--inference-model` CLI flag only sets a backend default; the workspace
model configuration governs agent turns.

## 5. Run

```bash
/path/to/relurpish chat
```

Doctor is the readiness surface; run it any time:

```bash
/path/to/relurpish doctor          # human-readable report
/path/to/relurpish status          # script-friendly diagnostics (--format json)
```

`status` exits non-zero when the workspace is degraded, so it works as a CI probe.

## Verify the installation

1. `relurpish doctor` ends with `Result: ready`.
2. `relurpish chat` opens with a Postures banner, e.g.
   `Postures — approvals: governed; intent selection: deterministic`.
3. Type a read-only ask, for example
   "list the files in this repository and summarize what it is".
   The turn completes without approvals because it performs no writes.

## Environment variables

Parsed only by the configuration boundary; precedence is CLI flag >
environment > workspace config > built-in default.

| Variable | Required | Default | Description |
|---|---|---|---|
| `RELURPIFY_WORKSPACE` | no | current directory | Default workspace root |
| `RELURPIFY_MODEL_PROVIDER` | no | `ollama` | Override the provider |
| `RELURPIFY_MODEL_NAME` | no | workspace config (`gemma4:e4b`) | Override the model |
| `RELURPIFY_SANDBOX_BACKEND` | no | `gvisor` | `gvisor` or `docker` |
| `RELURPIFY_OLLAMA_HOST` | no | `http://localhost:11434` | Ollama endpoint |
| `RELURPIFY_LLM_API_KEY` | yes for `openai_compatible` | none | The only secret; never put it in config files |
| `RELURPIFY_STRICT` | no | unset | `1`/`true`/`yes`/`on` turns missing workspace defaults into load errors; see below |
| `RELURPIFY_LOG_LEVEL` | no | none | Sets the runtime recording mode; despite the name it does not change log verbosity |
| `RELURPIFY_REDUCE_MOTION` | no | unset | Any non-empty value enables reduced-motion rendering |
| `EDITOR` | no | system default | Editor used by UI edit actions |
| `XDG_DATA_HOME` | no | `~/.local/share` | Shared data root (`~/.local/share/relurpify`) |

`RELURPIFY_STRICT` details: five `workspace.yaml` keys have built-in defaults
(`paths.state_dir`, `logging.level`, `logging.format`, `audit.retention_days`,
`telemetry.enabled`).
Unset or `0`/`false`/`no`/`off`, missing keys are filled and logged.
`1`/`true`/`yes`/`on`, a `workspace.yaml` missing any of them fails loading and
names the keys.
Any other value is a boot error.

## Running offline

The `offline` provider is a deterministic scripted model that exercises tool
dispatch, streaming, and compilation without a backend, Docker, or network:

```bash
relurpish doctor --offline --fix
relurpish chat --offline
```

It is intended for plumbing checks and CI, not for real coding work.

## Troubleshooting

| Symptom | Cause and fix |
|---|---|
| UI opens locked on the Doctor screen | A required dependency is missing (sandbox toolchain, workspace). Fix what the Doctor tab lists, then run `/doctor` in the UI to re-check |
| `model backend not reachable` | Start `ollama serve` (or your provider); doctor shows per-provider health and hints |
| Turn fails with `react decision unparseable` | The model produced no parsable decision; `/retry` in the UI. If it happens often, switch models in `relurpify_cfg/workspace.yaml` |
| Turn failed with `capability ... blocked: input schema invalid` | A recipe step invoked a capability without required inputs; check the recipe's `do`/`on` clause ([RECIPE_DSL](RECIPE_DSL.md)) |
| New recipe not listed in `/recipe` | Recipes load once at boot; restart relurpish. `/recipe` lists skipped files with reasons |
| Config load error naming defaulted keys | You set `RELURPIFY_STRICT` but `workspace.yaml` omits defaultable keys; set them or unset the variable |
| Boot error about an unrecognized boolean | A `RELURPIFY_*` boolean variable has a typo; only `1/true/yes/on` and `0/false/no/off` are accepted |
| Doctor warns about low disk space | The workspace volume is below 256 MB free; the runtime needs headroom for state and artifacts |
| Sandbox warnings at boot (`unpinned image`) | Informational; pin `security.sandbox.image_digest` for a reproducible sandbox |

## Where to go next

- [Relurpish guide](RELURPISH.md) — daily use: commands, approvals, recipes.
- [Configuration](CONFIGURATION.md) — every file in `relurpify_cfg/`.
- [Recipe DSL](RECIPE_DSL.md) — author your own workflows.
- [Architecture](ARCHITECTURE.md) — how the system works under the hood.
