# <div align="center">Relurpify</div>

<div align="center">
  <img src="./logo.png" alt="Relurpify logo" width="240" />
</div>

<div align="center">To the day it rewrites itself</div>

[![CI](https://github.com/lexbity/relurpify/actions/workflows/ci.yml/badge.svg)](https://github.com/lexbity/relurpify/actions/workflows/ci.yml)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/go-%3E%3D1.26-blue)](go.mod)

---

## Why Relurpify?

Own the tools that you creatively build with.

Not trust a black box, not rent them from a platform, not hand your
workspace to something you cannot inspect, cannot stop, and cannot
understand. 

Originally built as an Generative AI experiment - I wanted to build a 
platform so that others on their own workstation
 build smart, deterministic, automated tools that is **theirs** ;
safe, governed, and fully under their control.

No strings attached - it's Apache 2 licensed. 
No telemetry phoning home - OpenTelementry supported. 
No opaque model calls you cannot see, cannot replay, and cannot pin. 

Just you, your machine, and
an LLM that works for you — not the other way around.

And maybe, someday, the project will rewrite itself.

## How Relurpify does it

**Expose the LLM. Never treat it as a black box.**

Every layer of Relurpify is designed so the model is a *participant* in a
governed system — not the system itself:

- **The model proposes, the framework disposes.** Every tool call is a
  capability invocation that passes through admission, policy evaluation,
  human approval, sandboxed execution, and trust-classified result
  insertion. The model never touches your machine directly.
- **Context is compiled, not stuffed.** What the model sees is a
  deterministic artifact with a digest, a token budget, and provenance —
  not a raw transcript dump.
- **Behavior is characterizable.** Deterministic routing, record/replay
  tapes, golden files, and conformance suites treat agent behavior as
  something you can pin and regression-test.
- **Express desired meta behavour** Through Euclo thoughtrecipes - agent process
  intended tools and requirements treated as a scriptable tool
- **Architecture is machine-checked.** Import direction, layering, and
  design rules are enforced by AST gates in CI — not by convention.

## What

Relurpify is a local-first agent framework written in Go. It ships a full stack:

### Agent Backend framework

- **Governed execution** — capability trust/effect classes, policy engine,
  HITL approval gate in the default `governed` mode.
- **Sandboxed commands** — gVisor (`runsc`) or Docker backends, with the
  workspace mounted and network isolated by default.
- **Compiled and Streamed Context** — retrieval, trust filtering run deterministically over
  a knowledge-chunk memory graph; every compilation is recorded and replayable.
- **LLM model provider agnostic** — project supports OpenAI/v1, Ollama, lmstudio 
  for tooling and thinking models with native or fallback tool calling, bounded
  decision repair for malformed output, and per-model profiles.
- **Agent execution paradigm collection** - generic implementations 
  of common standard agent structures - react, pipeline, planner and 
  uncommon agent execution paradigmns - HTN, blackboard, goalcon etc. 
- **Graph Node** - construct custom agent execution from node topologies
- **Durable jobs queue** — at-least-once, atomic claim, executed by
  `relurpify-runner`.
- **Enforced architecture** — machine-checked layering and hygiene gates;
  the charter forbids shims, compatibility layers, aliases, and stubs.

### Memory & context

- **Knowledge memory** — a provenance-tracked chunk graph stored in an
  embedded Badger-backed graph database, with retrieval rankers, AST
  indexing, and staleness invalidation.
- **Context streaming** — deterministic, budgeted context assembly with
  replayable compilation and a streaming renderer for the TUI.

  
### End products

- **Euclo** — Execute agent behavior as a deterministic tool through a python-like 'thoughtrecipe' DSL.

- **relurpish** — the terminal UI. A Bubble Tea TUI for chatting with Euclo,
  running `doctor`/`status`, and watching governed execution unfold.
  
- **relurplint** — the workspace linter. Config, tools, recipes, prompts,
  and agents — all machine-checked.
  
- **dev-agent** — a CLI only Euclo agent test-suite runner. YAML-driven agent tests with
  outcome/security/benchmark assertions, tape replay, and conformance
  suites.

### Telemetry

- **OpenTelemetry** — every run exports OTel traces (lifecycle spans), log
  records, and metrics. By default everything stays local and offline:
  OTLP-JSON segments under `.relurpify_state/telemetry/otlp/` with 7-day
  retention, readable with `jq`; set `telemetry.endpoint` in
  `workspace.yaml` to forward to any OTLP/HTTP collector.

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
| [Contributing](CONTRIBUTING.md) | DCO, gates, coverage floor, release process |
| [Testing](docs/TESTING.md) | Unit, integration, and agent test layers; tapes |
| [Changelog](CHANGELOG.md) | Release history |
| [Code of conduct](docs/CODE_OF_CONDUCT.md) | Community rules |

## Status

Relurpify is under active development; checkout
[contributing guide](CONTRIBUTING.md) if you want to ride along.
