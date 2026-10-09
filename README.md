# <div align="center">Relurpify</div>

<div align="center">
  <img src="./logo.png" alt="Relurpify logo" width="240" />
</div>

<div align="center">
  To the day it rewrites itself
</div>

## What Is Relurpify?

Relurpify is a fullstack Agent framework 
- generic execution Agent library 
- LLM oriented memory/context/graph/sandbox management framework 
- archaeology memory system 
- extensive testsuite
- (Euclo) coding agent 
- TUI interfaces 

## Currently Available

### Relurpish Agent TUI

- Default Agent TUI 
- Euclo coding agent access 

## Requirements

- Go `1.25+`
- Docker or another supported container runtime
- gVisor `runsc`
- Ollama (default inference provider)

## Docs

Developer documentation lives in the repository's `devdocs` folder;
`developer-documentation.md` is the canonical internal reference. Contribution
gates, coverage requirements, and the release process are described in
[CONTRIBUTING.md](CONTRIBUTING.md).

The `go build` examples in this README were verified in this workspace. The
runtime examples are documented invocation shapes, not all re-run here.

> **Running without Ollama (CI/plumbing only):**
> ```bash
> go build ./app/relurpish
> ./relurpish doctor --offline --fix
> ```
> The built-in `offline` backend is a deterministic scripted model that
> exercises the agent plumbing (tool dispatch, streaming, compilation)
> without requiring Ollama, Docker, or network access. It is intended
> for testing and CI — not for end-user demo or production use.

In sandboxed environments you may also want repo-local Go caches:

```bash
export GOMODCACHE=$PWD/.gomodcache
export GOCACHE=$PWD/.gocache
```

## Install

### Build from source

```bash
go build ./app/relurpish
```

### Release binaries

Pre-built artifacts are attached to every
[GitHub release](https://github.com/lexbit/relurpify/releases): per-platform
archives, `checksums.txt`, SBOMs (`*.sbom.json`), `.deb`/`.rpm` packages, and
an AUR package. Releases are created as drafts; the maintainer reviews and
publishes them.

`relurpish`, `dev-agent`, `relurplint`, and `generate-config` are built for
Linux and macOS (amd64 + arm64). The internal tooling binaries `archcheck`,
`domaincheck`, and `driftcheck` additionally build for Windows (zip archives).
Every binary is statically linked (CGO disabled).

#### Linux (deb)

```bash
wget https://github.com/lexbit/relurpify/releases/download/v0.1.0/relurpify_0.1.0_amd64.deb
sudo dpkg -i relurpify_0.1.0_amd64.deb
```

#### Linux (rpm)

```bash
wget https://github.com/lexbit/relurpify/releases/download/v0.1.0/relurpify-0.1.0-1.x86_64.rpm
sudo rpm -i relurpify-0.1.0-1.x86_64.rpm
```

#### Linux / macOS (archive)

```bash
wget https://github.com/lexbit/relurpify/releases/download/v0.1.0/relurpify_0.1.0_linux_amd64.tar.gz
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

Release binaries report their build metadata (`--version` on `relurpish` and
`dev-agent`):

```bash
relurpish --version
# relurpish 0.1.0 (commit 594da458..., built 2026-10-08T...)
```

Source builds report `dev` / `none` / `unknown`.

### Optional: build all project binaries

```bash
go build ./...
```

## First-run (Ollama)

Before starting a chat session, ensure Ollama is running and has a model pulled:

```bash
# Start the Ollama daemon (in a separate terminal)
ollama serve

# Pull a model that the default catalog references
ollama pull gemma4:e4b
```

Then run doctor to verify the workspace is ready:

```bash
go build ./app/relurpish
./relurpish doctor
```

## Run Euclo in Relurpish

Start the terminal app with:

```bash
go run ./app/relurpish chat
```

This launches `relurpish` and starts the default Euclo coding workflow in the current workspace.

For a typical first-use flow:

```bash
go build ./app/relurpish
go run ./app/relurpish doctor
go run ./app/relurpish chat
```

## Testing

The main local checks are:

```bash
make test-unit
make test-dev-agent
make test-tape-fidelity
```

See `developer-documentation.md` in the repository's `devdocs` folder for the
full matrix and agent-test workflow.

## Additional Tools

Relurpify also includes developer tooling for internal workflows and testing:

```bash
# List discovered agents
go run ./app/dev-agent-cli agents list

# Run agent tests
go run ./app/dev-agent-cli agenttest run

# Scaffold a skill
go run ./app/dev-agent-cli skill init my-skill --description "My focused workflow" --with-tests

# Validate a skill
go run ./app/dev-agent-cli skill validate my-skill
```
