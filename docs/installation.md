# Installation

## Synopsis

Relurpify has a deliberately opinionated dependency stack. Every component exists for a specific reason, and the system will refuse to start if any mandatory dependency is missing. This document explains what you need, why you need it, and how to get it running.

---

## Why This Stack

| Dependency | Why it's required |
|------------|------------------|
| **Go 1.21+** | Relurpify is written in Go; binaries are compiled and installed via `go install` |
| **Docker** (or containerd) | Container runtime used to launch gVisor-sandboxed tool execution |
| **gVisor (`runsc`)** | Kernel-level sandbox for all agent-executed commands; non-negotiable |
| **Ollama** | Local LLM inference; all reasoning happens on your machine |

gVisor is a hard requirement. An AI agent that can run arbitrary shell commands without kernel-level isolation is an unacceptable security risk on a real codebase. If you want an agent without this constraint, there are simpler tools available.

---

## Dependency Installation

### 1. Go

Install Go 1.21 or later from [go.dev/dl](https://go.dev/dl). Verify:

```bash
go version
```

Make sure `$GOPATH/bin` (typically `~/go/bin`) is in your `$PATH`. This is where `go install` places binaries.

### 2. Docker

Install Docker Engine from [docs.docker.com/engine/install](https://docs.docker.com/engine/install).

> containerd is also supported as an alternative to Docker. The instructions below use Docker.

Verify Docker is running:

```bash
docker info
```

### 3. gVisor

gVisor provides the `runsc` runtime. Install it and register it with Docker:

```bash
# Install runsc (replace with your platform's package method)
# See https://gvisor.dev/docs/user_guide/install/

# Register runsc with Docker
sudo runsc install
sudo systemctl restart docker
```

Verify Docker can see the `runsc` runtime:

```bash
docker info --format '{{json .Runtimes}}' | grep runsc
```

### 4. Ollama

Install Ollama from [ollama.com](https://ollama.com) and pull a code-capable model:

```bash
ollama pull qwen2.5-coder:14b
```

Any model available in Ollama works. `qwen2.5-coder:14b` is the default. Verify Ollama is running:

```bash
curl http://localhost:11434/api/tags
```

---

## Installing Relurpify

Relurpify is installed as a global Go binary. Run from anywhere:

```bash
go install github.com/lexcodex/relurpify/app/relurpish@latest
```

This compiles and places the `relurpish` binary in `$GOPATH/bin`. You can then run `relurpish` from any project directory.

---

## Workspace Setup

Relurpify is workspace-aware. When you run `relurpish` it looks for a `relurpify_cfg/` directory in the current working directory. This directory is the configuration root for that project.

**Create a workspace in your project:**

```bash
cd /your/project
mkdir -p relurpify_cfg/agents
```

At minimum, you need:

- `relurpify_cfg/config.yaml` — sets the default model
- `relurpify_cfg/agent.manifest.yaml` — the default agent security contract
- `relurpify_cfg/agents/` — per-agent manifests

A minimal `config.yaml`:

```yaml
default_model:
    name: qwen2.5-coder:14b
```

Copy a manifest from the Relurpify repository's `relurpify_cfg/agents/` directory as a starting point, then edit the `filesystem` paths to match your project.

---

## Environment Check

Before launching the TUI, run the environment probe to confirm every dependency is available:

```bash
relurpish wizard
```

This checks:
- `runsc` binary presence and version
- Docker / containerd presence and gVisor registration
- Ollama endpoint health and available models
- Workspace configuration

If anything is missing, the output will describe what needs to be fixed before the runtime can start.

---

## First Run

Once the environment check passes:

```bash
cd /your/project
relurpish chat
```

The TUI opens with the Chat pane active. Type your first instruction and press `enter`.

---

## Overriding Defaults

All paths and settings can be overridden via flags:

| Flag | Default | Purpose |
|------|---------|---------|
| `--workspace <path>` | Current directory | Workspace root |
| `--manifest <path>` | `relurpify_cfg/agent.manifest.yaml` | Agent manifest |
| `--agent <name>` | `coding` | Agent preset |
| `--ollama-endpoint <url>` | `http://localhost:11434` | Ollama URL |
| `--ollama-model <name>` | From manifest | Model override |
| `--runsc <path>` | `runsc` | Path to runsc binary |
| `--container-runtime <name>` | `docker` | `docker` or `containerd` |
| `--sandbox-platform <name>` | auto | `kvm` or `ptrace` |

Example — running against a specific workspace and manifest:

```bash
relurpish chat \
  --workspace /my/project \
  --manifest /my/project/relurpify_cfg/agents/coding-go.yaml
```

---

## For Developers / Framework Users

If you want to build on top of the Relurpify framework (custom agents, custom tools, embedded usage), add it as a Go module dependency:

```bash
go get github.com/lexcodex/relurpify@latest
```

Then import the packages you need:

```go
import (
    "github.com/lexcodex/relurpify/framework/core"
    "github.com/lexcodex/relurpify/framework/graph"
)
```

See [Developer: Custom Agents](dev/custom-agents.md) and [Developer: Custom Tools](dev/tools.md) for guides.

---

## See Also

- [Architecture](architecture.md) — understand what you're setting up
- [Configuration](configuration.md) — workspace config and manifest schema
- [Permission Model](permission-model.md) — how the manifest controls agent behaviour
