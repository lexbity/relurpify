# Platform

## Synopsis

The `platform/` layer provides concrete implementations of agent-callable tools
and the inference facade. Platform packages are the only place where external
binaries, language runtimes, and provider APIs are invoked. Everything above
(agents, framework) depends only on abstractions from `framework/core`.

---

## Package Map

```
platform/
├── llm/          Managed backend facade, telemetry wrapper, tape recorder
├── shell/        Bash command execution
├── git/          Git operations
├── fs/           File read/write/search
├── search/       Code search and workspace indexing
├── lsp/          Language Server Protocol (IDE features)
├── browser/      Multi-backend browser automation and extraction
├── ast/          AST-based symbol lookup tools
├── db/
│   └── sqlite/   SQLite query execution
└── lang/
    ├── go/       Go build/test/vet/fmt tools
    ├── python/   pytest/pip/mypy tools
    ├── rust/     Cargo build/test/clippy tools
    └── js/       npm/node/eslint tools
```

---

## llm

`platform/llm` exposes a provider-neutral managed backend factory plus
transport-specific provider packages.

The root package owns:

- `ManagedBackend` and related capability/health types
- `ProviderConfig` and `New(cfg)` factory selection
- `InstrumentedModel` wrappers
- `TapeModel` capture/replay

Provider implementations live in subpackages:

- `platform/llm/ollama` for Ollama-native transport
- `platform/llm/lmstudio` for OpenAI-compatible LM Studio transport
- `platform/llm/openaicompat` for shared OpenAI-compatible HTTP behavior

The default provider remains Ollama when no explicit provider is configured.
Each provider package returns a `core.LanguageModel` implementation through the
managed backend facade.

**InstrumentedModel** (`instrumented_model.go`) — wraps any `LanguageModel` to
emit telemetry events per call: token counts, latency, model name, and a
truncated prompt digest.

**TapeModel** (`tape_model.go`) — records request/response pairs to a tape file
(capture mode) and plays them back deterministically (replay mode), enabling
agent integration tests to run without a live backend.

---

## shell

`platform/shell` provides bash command execution as an agent capability.

The shell tool runs a command string via `bash -c` inside the active sandbox
runner and returns stdout, stderr, and exit code as a structured observation.
`cli_registry.go` maintains a registry of discovered CLI tools so agents can
check availability before invoking a binary. `process_metadata.go` records
spawned-process metadata for audit and telemetry.

---

## git

`platform/git` provides version-control operations as registered capability tools.

Available tools: `git_status`, `git_diff`, `git_log`, `git_show`, `git_clone`,
`git_add`, `git_commit`, `git_push`, `git_pull`, `git_branch`, `git_checkout`,
`git_stash`. Each runs via the sandbox runner and returns structured output.

---

## fs

`platform/fs` provides filesystem tools for reading, writing, and finding files
within the agent's permitted workspace paths.

Core tools: `read_file`, `write_file`, `list_dir`, `find_files`, `file_exists`.
All operations are checked against the `PermissionManager` before execution.

`permission_cache.go` caches per-path permission lookups to avoid redundant
policy evaluations when the same files are accessed repeatedly within a session.

---

## search

`platform/search` provides code search and content indexing tools for workspace
exploration.

The search tool combines glob-based file discovery with ripgrep-style content
filtering and AST index queries. Results are ranked by relevance and returned as
structured observations so agents can locate relevant files without loading
entire directories into context.

---

## lsp

`platform/lsp` integrates LSP (Language Server Protocol) capabilities into the
agent tool surface.

`lsp.go` implements an LSP client over JSON-RPC (sourcegraph/jsonrpc2).
`lsp_process_client.go` manages the language server subprocess for a given
workspace and language.

Available agent tools: go-to-definition, hover documentation, find-references,
document symbols, workspace diagnostics.

---

## browser

`platform/browser` provides Relurpify's browser automation subsystem for
interactive web workflows, structured extraction, localhost app testing, and
browser-driven research.

Technical package details are documented in
[`platform/browser.md`](../platform/browser.md).

The browser layer is framework-first rather than transport-first. It exposes a
browser session and tool model over multiple protocol backends instead of
making CDP or WebDriver semantics the public contract. Workspace ownership and
service lifecycle now live in `ayenitd/service/browser`.

### Architecture

The browser subsystem is organized into five layers:

1. manifest and skill policy
2. model-facing browser tool
3. browser session and supervisor
4. protocol backend
5. sandbox and runtime enforcement

The model-facing surface is a single `browser` tool in v1 with action dispatch.
Core actions include:

- `open`
- `navigate`
- `click`
- `type`
- `wait`
- `extract`
- `get_text`
- `get_accessibility_tree`
- `get_html`
- `current_url`
- `screenshot`
- `execute_js`
- `close`

This keeps the tool surface compact for smaller models while allowing multiple
backend implementations underneath.

### Session and supervisor model

The browser session wrapper is responsible for:

- permission checks before navigation and other gated actions
- HITL escalation where policy requires it
- structured extraction and truncation
- token-budgeted page context
- tab tracking
- normalized page snapshots and result packaging
- mapping backend-specific failures into Relurpify browser errors

A workspace browser service owns:

- backend construction
- browser process launch
- temporary profile creation
- download directory setup
- reconnect and relaunch behavior
- cleanup on cancellation and shutdown

This keeps long-lived browser state under workspace ownership rather than
leaving process lifecycle to ad hoc tool calls or the relurpish runtime.

### Backend model

The browser subsystem supports a transport-agnostic backend interface with
multiple implementations:

- CDP for Chromium-family browsers
- WebDriver Classic for standards-based remote control
- WebDriver BiDi for event-capable standards-based automation

CDP is the strongest backend for strict security and request interception.
WebDriver Classic is compatibility-oriented and cannot provide the same level of
page-initiated subresource enforcement. BiDi provides an event-capable standard
path without collapsing into Classic-specific assumptions.

Backend implementations are responsible for:

- protocol command dispatch
- event handling
- download, tab, and navigation hook points
- error normalization
- capability reporting for unsupported operations

### Security model

Browser automation follows the same security model as other platform
capabilities:

- manifest network permissions remain the host allowlist for browser egress
- browser actions are gated by runtime policy and HITL rules
- browser subprocesses stay inside the normal sandboxed runtime model
- arbitrary page-context JavaScript execution is treated as a privileged action
- raw browser output is not inserted into model context without budgeting and
  provenance

Enforcement happens at three layers:

1. tool-action gating
2. protocol-command and event gating
3. sandbox and browser network enforcement

This matters because pages can trigger redirects, popups, downloads, and other
side effects below the top-level tool call.

### Browser configuration and extraction

Browser behavior is expected to be controlled from agent policy, including:

- which browser actions are allowed, denied, or require approval
- which hosts may be reached
- whether downloads are allowed
- where downloads may be written
- whether credential entry requires approval
- extraction defaults and token budgets

Extraction is token-budget-aware by default:

- `get_html` and accessibility output must support truncation
- `extract` should prefer structured output
- large results should carry truncation metadata

### Error model

Backends normalize into Relurpify-level browser errors such as:

- no such element
- stale element reference
- element not interactable
- timeout
- navigation blocked by permission policy
- script evaluation failure
- backend disconnected
- unsupported operation

Errors should carry backend and operation metadata so audit, telemetry, and
agent recovery logic can reason about them consistently.

---

## ast

`platform/ast` exposes AST-based code intelligence as agent-callable tools.

`ast_symbol_provider.go` queries the `framework/ast` index to resolve symbol
definitions — name, kind, file, line — without re-parsing source files.
`ast_tool.go` registers the provider as a capability tool.

---

## db/sqlite

`platform/db/sqlite` provides SQL execution tools against SQLite databases
within the agent's permitted workspace.

Agents may run read-only SELECT queries or, where the manifest grants write
permission, DML and DDL statements. Results are returned as structured tables.

---

## lang/go

`platform/lang/go` (package `golang`) provides Go language tools:
`go_build`, `go_test` (with `-run` filter), `go_vet`, `go_fmt`,
`go_mod_tidy`, `go_generate`. Each returns structured compiler and test output.

---

## lang/python

`platform/lang/python` provides Python language tools:
`pytest` (with filter and verbosity options), `pip_install`,
`python_run`, `mypy`.

---

## lang/rust

`platform/lang/rust` provides Rust language tools:
`cargo_build`, `cargo_test` (with filter), `cargo_clippy`,
`cargo_fmt`, `cargo_check`.

---

## lang/js

`platform/lang/js` provides JavaScript/TypeScript tools:
`npm_install`, `npm_test`, `npm_run` (arbitrary scripts),
`node_run`, `eslint`.

---
