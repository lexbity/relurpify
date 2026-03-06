# Installation

## Synopsis

Relurpify expects an explicitly configured local environment: Go to build the binaries, Docker plus gVisor for sandboxed command execution, and Ollama for local model inference. This page documents the manual setup path.

---

## Dependencies

| Dependency | Why it is required |
|------------|--------------------|
| **Go 1.21+** | Builds `relurpish` and `coding-agent` |
| **Docker** or **containerd** | Container runtime for sandboxed execution |
| **gVisor (`runsc`)** | Mandatory sandbox runtime |
| **Ollama** | Local LLM inference backend |

Relurpify intentionally refuses to degrade into an unsandboxed "just run shell commands on the host" mode for normal runtime operation.

---

## Install The Stack

### 1. Go

Install Go 1.21 or later from [go.dev/dl](https://go.dev/dl), then verify:

```bash
go version
```

Make sure `$GOPATH/bin` is on your `$PATH`.

### 2. Docker

Install Docker Engine from [docs.docker.com/engine/install](https://docs.docker.com/engine/install), then verify:

```bash
docker info
```

### 3. gVisor

Install `runsc` and register it with Docker:

```bash
# Install runsc using your platform's package method
# See https://gvisor.dev/docs/user_guide/install/

sudo runsc install
sudo systemctl restart docker
```

Verify Docker sees the runtime:

```bash
docker info --format '{{json .Runtimes}}' | grep runsc
```

### 4. Ollama

Install Ollama from [ollama.com](https://ollama.com) and pull a code-capable model:

```bash
ollama pull qwen2.5-coder:14b
curl http://localhost:11434/api/tags
```

---

## Install The Binaries

Install the primary TUI/runtime:

```bash
go install github.com/lexcodex/relurpify/app/relurpish@latest
```

Optional: install the CLI used for testsuites and utility commands:

```bash
go install github.com/lexcodex/relurpify/cmd/coding-agent@latest
```

---

## Workspace Layout

Relurpify is workspace-aware. Run it from a project that contains `relurpify_cfg/`.

Create the basic directory structure:

```bash
cd /your/project
mkdir -p relurpify_cfg/agents
```

Useful files:

- `relurpify_cfg/agent.manifest.yaml` — active manifest used by `relurpish`
- `relurpify_cfg/config.yaml` — optional workspace defaults
- `relurpify_cfg/agents/` — optional additional agent definitions or presets

The fastest manual setup path is to copy a shipped manifest into the default location and then edit it for your project:

```bash
cp /path/to/relurpify/relurpify_cfg/agents/coding-go.yaml relurpify_cfg/agent.manifest.yaml
```

Update at least:

- filesystem paths under `spec.defaults.permissions.filesystem`
- executable allowlists under `spec.defaults.permissions.executables`
- the model name under `spec.agent.model.name`

An optional `config.yaml` looks like:

```yaml
model: qwen2.5-coder:14b
agents:
    - coding-go
allowed_tools: []
permission_profile: workspace_write
last_updated: 1709500000
```

If `config.yaml` is missing, runtime defaults come from the manifest and CLI flags.

---

## First Run

Once the manifest points at the correct workspace and model:

```bash
cd /your/project
relurpish chat
```

The TUI opens with the Chat pane active.

---

## Useful Flags

| Flag | Purpose |
|------|---------|
| `--workspace <path>` | Override the workspace root |
| `--manifest <path>` | Override the active manifest path |
| `--agent <name>` | Select an agent preset or definition |
| `--ollama-endpoint <url>` | Override the Ollama endpoint |
| `--ollama-model <name>` | Override the model name |
| `--runsc <path>` | Override the `runsc` binary path |
| `--container-runtime <name>` | Select `docker` or `containerd` |
| `--sandbox-platform <name>` | Select `kvm` or `ptrace` |
| `--serve` | Start the HTTP API alongside the TUI |
| `--addr <addr>` | Override the HTTP listen address |

Example:

```bash
relurpish chat \
  --workspace /my/project \
  --manifest /my/project/relurpify_cfg/agent.manifest.yaml \
  --ollama-model qwen2.5-coder:14b
```

---

## Framework Usage

If you are embedding Relurpify as a Go dependency instead of running the binaries:

```bash
go get github.com/lexcodex/relurpify@latest
```

Then import the packages you need from `framework/`, `agents/`, or `tools/`.

---

## See Also

- [Architecture](architecture.md) — understand what you are setting up
- [Configuration](configuration.md) — workspace config and manifest schema
- [Permission Model](permission-model.md) — how the manifest controls agent behaviour
