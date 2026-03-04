# Custom Tools

## Synopsis

Tools are the actions an agent can take on the world. Every file read, git call, test run, and shell command goes through a tool. This document explains how the tool system works and how to add new tools.

---

## Why Tools Are Separate From Agents

Separating tools from agents means the same tool can be used by multiple agent types, and each agent's permission scope determines which tools it can see. The tool registry is the single source of truth for what actions exist; the manifest determines which subset any given agent can use.

---

## How It Works

### The Tool Interface

Every tool implements `core.Tool`:

```go
type Tool interface {
    Name() string
    Description() string
    Category() string
    Parameters() []ToolParameter
    Execute(ctx context.Context, state *Context, args map[string]interface{}) (*ToolResult, error)
    IsAvailable(ctx context.Context, state *Context) bool
    Permissions() ToolPermissions
    Tags() []string
}
```

`Parameters()` returns the JSON schema that Ollama uses to understand how to call the tool. The schema is converted to Ollama's tool-calling format at the point of each LLM call.

`Permissions()` declares what the tool needs — the permission manager compares this against the manifest's declared permissions before `Execute` is called.

`Tags()` groups tools for policy management (e.g. `["read-only"]`, `["execute"]`, `["destructive"]`).

### Permission-Aware Tools

Tools that interact with the permission manager implement `PermissionAware`:

```go
type PermissionAware interface {
    SetPermissionManager(*PermissionManager)
}
```

The tool registry injects the permission manager at startup via `registry.UsePermissionManager(agentID, manager)`. Inside `Execute`, the tool calls `manager.CheckFileAccess(...)` or `manager.CheckExecutable(...)` before acting.

### Manifest-Aware Tools

Tools that need to read the agent's manifest spec (for bash permission patterns, for example) implement `AgentSpecAware`:

```go
type AgentSpecAware interface {
    SetAgentSpec(*AgentRuntimeSpec)
}
```

The registry injects the spec alongside the permission manager.

### The Tool Registry

`framework/toolsys.ToolRegistry` is the central catalog:

- **Registration** — `registry.Register(tool)` adds a tool at startup
- **Retrieval** — `registry.Get(name)` returns a tool by name
- **Filtering** — `registry.FilterForAgent(spec)` returns only tools permitted by the manifest
- **Policy queries** — `GetToolPolicies()`, `GetTagPolicies()` expose current per-tool and per-tag policies
- **Live updates** — `UpdateToolPolicy()`, `UpdateTagPolicy()` apply policy changes without restart

---

## Built-in Tools Reference

### File Tools (`tools/files.go`)

| Name | Tags | Description |
|------|------|-------------|
| `file_read` | read-only | Read a UTF-8 file |
| `file_write` | destructive | Write or overwrite a file |
| `file_create` | destructive | Create a new file (fails if exists) |
| `file_delete` | destructive | Delete a file |
| `file_list` | read-only | List directory contents |
| `file_search` | read-only | Search for a pattern within files |

### Git Tools (`tools/git.go`)

| Name | Tags | Description |
|------|------|-------------|
| `git_command` | destructive | Run git operations (diff, log, commit, branch, blame) |

### Search Tools (`tools/search.go`)

| Name | Tags | Description |
|------|------|-------------|
| `grep` | read-only | Pattern search across files |
| `similarity_search` | network | Vector similarity search |
| `semantic_search` | network | Hybrid semantic + keyword search |

### Execution Tools (`tools/execution.go`)

| Name | Tags | Description |
|------|------|-------------|
| `run_tests` | execute | Run project tests |
| `run_build` | execute | Build the project |
| `run_linter` | execute | Run the configured linter |
| `execute_code` | execute | Execute a code snippet |
| `bash_execute` | execute | Run a shell command |

All execution tools route through `CommandRunner`. In production this is `SandboxCommandRunner` — commands run inside a gVisor container via `docker run --runtime=runsc`.

### LSP Tools

| Name | Tags | Description |
|------|------|-------------|
| `lsp_definition` | read-only | Jump to definition |
| `lsp_references` | read-only | Find all references |
| `lsp_hover` | read-only | Get hover documentation |
| `lsp_diagnostics` | read-only | Get language diagnostics |
| `lsp_search_symbols` | read-only | Search workspace symbols |
| `lsp_document_symbols` | read-only | Get symbols in a file |
| `lsp_format` | destructive | Format a file |

LSP tools proxy to an attached language server. If no LSP connection is active they return a descriptive error.

### Analysis Tools

| Name | Tags | Description |
|------|------|-------------|
| `ast_analyze` | read-only | AST analysis of a file; returns symbols and structure |

---

## Writing a Custom Tool

### 1. Implement the Interface

```go
package tools

import (
    "context"
    "github.com/lexcodex/relurpify/framework/core"
)

type EchoTool struct{}

func (t *EchoTool) Name() string        { return "echo" }
func (t *EchoTool) Description() string { return "Echo the input text back" }
func (t *EchoTool) Category() string    { return "utility" }
func (t *EchoTool) Tags() []string      { return []string{"read-only"} }

func (t *EchoTool) Parameters() []core.ToolParameter {
    return []core.ToolParameter{
        {
            Name:        "text",
            Type:        "string",
            Description: "Text to echo",
            Required:    true,
        },
    }
}

func (t *EchoTool) Permissions() core.ToolPermissions {
    // This tool needs no special permissions
    return core.ToolPermissions{}
}

func (t *EchoTool) IsAvailable(_ context.Context, _ *core.Context) bool {
    return true
}

func (t *EchoTool) Execute(
    ctx context.Context,
    state *core.Context,
    args map[string]interface{},
) (*core.ToolResult, error) {
    text, _ := args["text"].(string)
    return &core.ToolResult{
        Success: true,
        Output:  text,
    }, nil
}
```

### 2. Add Permission Checking (if needed)

For tools that access files or executables, implement `PermissionAware`:

```go
type MyFileTool struct {
    manager *runtime.PermissionManager
}

func (t *MyFileTool) SetPermissionManager(m *runtime.PermissionManager) {
    t.manager = m
}

func (t *MyFileTool) Execute(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.ToolResult, error) {
    path, _ := args["path"].(string)
    if t.manager != nil {
        if err := t.manager.CheckFileAccess(ctx, "agent", core.FileSystemRead, path); err != nil {
            return nil, err
        }
    }
    // ... read the file
}
```

### 3. Register in BuildToolRegistry

Add your tool to the registry wiring in `app/relurpish/runtime/runtime.go`:

```go
func BuildToolRegistry(workspace string, runner CommandRunner, opts ToolRegistryOptions) (*toolsys.ToolRegistry, *ast.IndexManager, error) {
    registry := toolsys.NewToolRegistry()
    // ... existing tools ...
    if err := registry.Register(&tools.EchoTool{}); err != nil {
        return nil, nil, err
    }
    return registry, indexManager, nil
}
```

### 4. Declare in Manifest

Add the tool to the agent's manifest if it needs explicit permissions, or rely on `default_tool_policy: ask` to prompt on first use.

---

## See Also

- [Permission Model](../permission-model.md) — how tool execution is authorised
- [Configuration](../configuration.md) — declaring tool permissions in manifests
- [Context Budget](context-budget.md) — how tool results affect the token budget
