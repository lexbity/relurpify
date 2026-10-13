# <div align="center">Relurpify</div>

<div align="center">
  <img src="./logo.png" alt="Relurpify logo" width="240" />
</div>

<div align="center">To the day it rewrites itself</div>

> **Audience**: developers evaluating or installing Relurpify.
> **Purpose**: what Relurpify is, what it ships, and how to get it running.
> **Prerequisites**: none; setup details live in [docs/GETTING_STARTED.md](docs/GETTING_STARTED.md).

[TODO: badges — CI, latest release, license]

[TODO: screenshot — relurpish TUI, chat tab with a completed task and the diff tab open]

## What is Relurpify?

Relurpify is a local-first agent framework written in Go.
It runs LLM-driven coding agents against a real workspace, and it makes every
rung of that ladder explicit and governed:

- Context is compiled, not stuffed: what the model sees is a deterministic
  artifact with a digest, a token budget, and provenance.
- Every action is mediated: model-driven work reaches your machine only through
  capabilities that pass a policy engine, a human-approval gate, and a
  sandboxed execution chain.
- Memory is structured: results and observations persist as provenance-tracked
  knowledge chunks, so stale knowledge is skipped rather than repeated.
- Behavior is characterizable: deterministic routing, LLM record/replay tapes,
  golden files, and conformance suites treat agent behavior as something you
  can pin and regression-test.

The built-in coding agent is **Euclo**.
The terminal UI is **relurpish**.

## Features

- **Governed execution** — every model action is a capability with declared
  trust and effect classes; writes and risky steps pause for your approval
  (HITL) in the default `governed` mode.
- **Sandboxed commands** — agent commands run in a container through gVisor
  (`runsc`) or the Docker backend, with the workspace mounted and network
  isolated by default.
- **Deterministic routing** — a keyword classifier scores your instruction into
  intent families, and a thoughtrecipe or capability is picked
  deterministically; the same input takes the same route.
- **Thoughtrecipes** — declarative `.erpe` workflows (trigger, tool scope,
  steps, captures) that you can author and pin; see
  [docs/RECIPE_DSL.md](docs/RECIPE_DSL.md).
- **Compiled context** — retrieval, trust filtering, and token budgeting run
  deterministically over a knowledge-chunk memory graph; every compilation is
  recorded and replayable.
- **Thinking-model friendly** — native or fallback tool calling, bounded
  decision repair for malformed output, and per-model profiles.
- **Characterizable** — YAML-driven agent tests with outcome/security/benchmark
  assertions, LLM tape replay, conformance suites, and a 70% per-package
  coverage floor.
- **Enforced architecture** — machine-checked layering and hygiene gates; the
  charter forbids shims, compatibility layers, aliases, and stubs.

## Prerequisites

| Requirement | Version / notes |
|---|---|
| Go | 1.25+ (builds offline-friendly) |
| Container runtime | Docker, plus gVisor `runsc` for the default `gvisor` sandbox backend (the `docker` backend needs Docker alone) |
| Model backend | Ollama (default), LM Studio, any OpenAI-compatible endpoint, or the built-in `offline` scripted model |
| OS | Linux or macOS (amd64 / arm64) |

## Quick start

```bash
# 1. Start a model backend and pull the default model
ollama serve
ollama pull gemma4:e4b

# 2. Build the TUI from the repository root
go build ./app/relurpish

# 3. Check your project directory; the first run materializes
#    the starter configuration into relurpify_cfg/
cd ~/code/your-project
/path/to/relurpish doctor

# 4. Start the coding agent
/path/to/relurpish chat
```

No Ollama or Docker handy? The `offline` backend exercises the full plumbing
with a deterministic scripted model:

```bash
go build -o relurpish ./app/relurpish
./relurpish doctor --offline --fix
./relurpish chat --offline
```

The offline backend is for testing and CI, not for real work.

## Install

### Build from source

```bash
go build ./app/relurpish
```

All project binaries:

| Binary | Build | Purpose |
|---|---|---|
| `relurpish` | `go build ./app/relurpish` | Terminal UI, `doctor`, `status` |
| `relurpify-runner` | `go build ./ayenitd/cmd/relurpify-runner` | Background job runner (spawned by the app) |
| `relurplint` | `go build ./app/relurplint` | Workspace linter: config, tools, recipes, prompts, agents |
| `dev-agent` | `go build ./tooling/dev-agent-cli` | Agent test-suite runner |
| `generate-config` | `go build ./userconfig/templates/cmd/generate-config` | Materializes the default config tree |

### Release binaries

Pre-built artifacts are attached to every
[GitHub release](https://github.com/lexbity/relurpify/releases): per-platform
archives, `checksums.txt`, SBOMs (`*.sbom.json`), `.deb`/`.rpm` packages, and
an AUR package.
Releases are created as drafts; the maintainer reviews and publishes them.
Every binary is statically linked (CGO disabled).

#### Linux (deb)

```bash
wget https://github.com/lexbity/relurpify/releases/download/v0.1.0/relurpify_0.1.0_amd64.deb
sudo dpkg -i relurpify_0.1.0_amd64.deb
```

#### Linux (rpm)

```bash
wget https://github.com/lexbity/relurpify/releases/download/v0.1.0/relurpify-0.1.0-1.x86_64.rpm
sudo rpm -i relurpify-0.1.0-1.x86_64.rpm
```

#### Linux / macOS (archive)

```bash
wget https://github.com/lexbity/relurpify/releases/download/v0.1.0/relurpify_0.1.0_linux_amd64.tar.gz
tar -xzf relurpify_0.1.0_linux_amd64.tar.gz
```

Download `checksums.txt` from the same release and verify before installing:

```bash
sha256sum -c checksums.txt
```

#### Arch Linux (AUR)

```bash
yay -S relurpify-bin
```

#### Version

```bash
relurpish --version
# relurpish 0.1.0 (commit 594da458..., built 2026-10-08T...)
```

Release binaries report build metadata; source builds report `dev` / `none` / `unknown`.

## Usage examples

Start a chat session in your project:

```bash
relurpish chat --workspace .
```

Check workspace readiness without the TUI:

```bash
relurpish doctor
relurpish status --format json
```

Lint the workspace configuration:

```bash
relurplint --check all --workspace .
```

Run the agent test suites:

```bash
go run ./tooling/dev-agent-cli agenttest run
```

## Documentation

| Doc | What it covers |
|---|---|
| [Docs index and glossary](docs/README.md) | Start here |
| [Architecture](docs/ARCHITECTURE.md) | System shape, enforced layering, a turn end to end |
| [Getting started](docs/GETTING_STARTED.md) | Setup, configuration, environment variables, troubleshooting |
| [Relurpish guide](docs/RELURPISH.md) | Daily use of the TUI: commands, approvals, postures |
| [Recipe DSL](docs/RECIPE_DSL.md) | Authoring thoughtrecipes (`.erpe`) |
| [Prompt DSL](docs/PROMPT_DSL.md) | Prompt file format and the prompt registry |
| [Configuration](docs/CONFIGURATION.md) | The `relurpify_cfg/` tree reference |
| [Contributing](docs/CONTRIBUTING.md) | DCO, gates, coverage floor, release process |
| [Testing](docs/TESTING.md) | Unit, integration, and agent test layers; tapes |
| [Changelog](CHANGELOG.md) | Release history |
| [Code of conduct](docs/CODE_OF_CONDUCT.md) | Community rules |

## Status

Relurpify is under active development by a solo maintainer and is not yet
version 1.0.
Expect rapid change; see the [changelog](CHANGELOG.md) and
[contributing guide](docs/CONTRIBUTING.md) if you want to ride along.
