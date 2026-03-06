# Relurpify

To the day(s) it re-writes itself.

Relurpify is an local Agentic automation framework - whose sole goal is to one day re-write itself. 

It features a ground up stack all in golang , including 
sandboxing with GVisor -> Ollama Integration -> Relurpify Apps/Agents


## Installation Prerequisites

- **Go 1.21+**
- **docker**
- **Ollama Endpoint (local or remote)** 
- **golds** documentation tool (optional, only required for static docs): `go install go101.org/golds@latest`


In sandboxed environments you can keep module/cache directories inside the repo:

```bash
export GOMODCACHE=$PWD/.gomodcache
export GOCACHE=$PWD/.gocache
```

---

## Build, Run, and Test

### Install dependencies

```bash
go mod tidy
```

### Build everything

```bash
go build ./...
```

### Build Relurpish Agent TUI

```bash
go build ./app/relurpish
```

### Skill workflows

```bash
# Scaffold a new skill
go run ./cmd/coding-agent skill init my-skill --description "My focused workflow" --with-tests

# Validate skill manifest + resources
go run ./cmd/coding-agent skill validate my-skill

# Diagnose tool/permission compatibility (optional agent manifest)
go run ./cmd/coding-agent skill doctor my-skill --manifest relurpify_cfg/agent.manifest.yaml

# Run the skill testsuite.yaml
go run ./cmd/coding-agent skill test my-skill
```

### Generate Code documentation

```bash
./scripts/gen-docs.sh
open docs/index.html   # or serve docs/ via any static file server
```

---

# Further Details

Checkout docs/


# Roadmap

- [x] core graph oriented library
- [x] gVisor Sandboxing + permissions integration 
- [x] HTIL
- [x] AST and context manager (context compression)
- [x] planner agent
- [x] react agent 
- [x] Coder (modal) agent
- [x] Ollama integration 

- [] Framework Tooling
    - [x] Essential (Linux assumed) local toolset
    - [x] Toolset: Git support 
    - [x] Toolset: python
    - [x] Toolset: golang 
    - [x] Toolset: rust 
    - [x] Toolset: Nodejs + Npm
    - [x] 'Skills' Support 
    
- [] Test Coverage
    - [x] core framework unit tests
    - [x] Automated Agent testsuite 
    - [] Agent Testsuite: Skills coverage (partially completed)
    - [] Extend Agent testsuite with standardized agent benchmarks (improvement over trust-me-bro^tm benchmarks)
    
- [] standard CLI-TUI interface for Agents
    - [x] Coder Agent support
    - [x] Chat
    - [x] Tasks
    - [x] Settings
    - [x] Setup Wizard
    - [x] Notifications with HTIL support 
    
- [x] Documentation (ongoing) 

- [] Node-based Workflow App (GUI) 
    - [] Terminal integration 
    - [] Relurpish TUI support 
    - [] Basic Nodeset 
    - [] Script Node 
    - [] Skill Node(s)
    
- [] (more experiments to come)
