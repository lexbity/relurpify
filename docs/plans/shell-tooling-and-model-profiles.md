# Shell Tooling, Shell Filter, and Model Profiles

## Purpose

This plan covers three related improvements to the Relurpify platform layer:

1. **Shell Filter** (`framework/sandbox`) — workspace-scoped regex blacklist that blocks or
   escalates dangerous shell command combinations before they reach a `CommandRunner`.
2. **Shell Command Constructor** (`platform/shell`) — validated command routing layer with
   user-defined skill bindings, so end users can define named shell capabilities as YAML.
3. **Model Profiles** (`platform/llm`) — model-aware configuration profiles that replace
   scattered per-model fixups (currently hardcoded for `qwen2.5-coder`) with a declarative
   YAML system that survives model switches.

Each phase is independently mergeable. Phases 1 and 3 have no dependencies on each other.
Phase 2 depends on Phase 1 (`ShellGuard`) being available.

---

## Background

### Current state

- Shell commands flow: `CommandTool.Execute` → `AuthorizeCommand` (glob patterns from manifest
  `bash_permissions`) → `CommandRunner.Run`. There is no workspace-level content filter.
- Tool calling quirks for `qwen2.5-coder` are applied unconditionally in four places:
  `ollama.go:parseArguments` (double-decode), `tool_formatting.go:normalizeMultilineJSONStringLiterals`,
  `react_think_node.go:textSuggestsPendingToolCall`, `react_think_node.go:repairDecision`.
  None are gated on model identity.
- `OllamaToolCalling` in `core.Config` / `AgentSpec` is a single bool with no model awareness.

### Key constraints

- `framework/config` (`Paths` struct) is the canonical path resolver for `relurpify_cfg/`.
  It only imports `path/filepath` — safe to import from `framework/sandbox` without cycles.
- `platform/shell` already imports `framework/sandbox` and `framework/capability` — correct
  layer for the command constructor.
- Model profile loading takes a `configDir string` at construction so tests can use a temp dir.
- The `LanguageModel` interface in `framework/core` must not change — model profile metadata
  is exposed via an optional `ProfiledModel` interface (type assertion by callers).

---

## Phase 1 — Shell Filter

**Goal:** Intercept dangerous command patterns at the platform layer before execution,
independent of per-agent manifest policy.

### New files

| File | Purpose |
|---|---|
| `framework/config/paths.go` | Add `ShellBlacklistFile()` and `ModelProfilesDir()` methods |
| `framework/sandbox/shell_blacklist.go` | `BlacklistRule`, `ShellBlacklist`, YAML loader |
| `framework/sandbox/shell_guard.go` | `ShellGuard` — `CommandRunner` decorator |
| `relurpify_cfg/shell_blacklist.yaml` | Workspace template (community rule set) |

### Types

```go
// framework/sandbox/shell_blacklist.go

type BlacklistAction string

const (
    BlacklistActionBlock BlacklistAction = "block"
    BlacklistActionHITL  BlacklistAction = "hitl"
)

type BlacklistRule struct {
    ID      string
    Pattern *regexp.Regexp
    Reason  string
    Action  BlacklistAction
}

// ShellBlacklist holds compiled rules loaded from shell_blacklist.yaml.
// A nil ShellBlacklist is valid and passes all commands through.
type ShellBlacklist struct {
    rules []BlacklistRule
}

// Load reads and compiles rules from path. Missing file returns empty blacklist, no error.
func Load(path string) (*ShellBlacklist, error)

// Check returns the first matching rule for the command string, or nil.
func (b *ShellBlacklist) Check(commandString string) *BlacklistRule
```

```go
// framework/sandbox/shell_guard.go

// ShellGuard wraps a CommandRunner and applies the blacklist before forwarding.
// It reconstructs the full command string as "args[0] args[1] ..." (same format
// used by AuthorizeCommand) so blacklist pattern authors have a consistent model.
type ShellGuard struct {
    inner     CommandRunner
    blacklist *ShellBlacklist
    manager   *authorization.PermissionManager
    agentID   string
}

func NewShellGuard(inner CommandRunner, blacklist *ShellBlacklist,
    manager *authorization.PermissionManager, agentID string) *ShellGuard

// Run implements CommandRunner. On block: returns error
// "shell filter blocked [<rule-id>]: <reason>".
// On hitl: escalates through manager.RequireApproval with the rule metadata.
func (g *ShellGuard) Run(ctx context.Context, req CommandRequest) (string, string, error)
```

### YAML format (`relurpify_cfg/shell_blacklist.yaml`)

```yaml
# Relurpify shell blacklist — workspace-level danger pattern filter.
# Applied before CommandRunner.Run() for all commands passing through a ShellGuard.
# Patterns are Go regular expressions matched against the full command string
# (args joined with spaces: "binary arg1 arg2 ...").
#
# Actions:
#   block — hard error, command is never executed
#   hitl  — requires human-in-the-loop approval via the permission manager
#
# Add workspace-specific rules below the community set.

version: "1"
rules:
  - id: recursive-delete-root
    pattern: 'rm\s+-[a-zA-Z]*r[a-zA-Z]*f\s+/'
    reason: recursive force delete from filesystem root
    action: block

  - id: recursive-delete-home
    pattern: 'rm\s+-[a-zA-Z]*r[a-zA-Z]*f\s+~'
    reason: recursive force delete from home directory
    action: block

  - id: pipe-to-shell
    pattern: '(curl|wget|fetch)\b.*\|\s*(ba)?sh'
    reason: remote code execution via pipe-to-shell download
    action: block

  - id: fork-bomb
    pattern: ':\(\)\s*\{.*\}\s*;?\s*:&'
    reason: fork bomb pattern
    action: block

  - id: etc-write
    pattern: '(>>?|tee\s+)/etc/'
    reason: writing to system configuration directory
    action: block

  - id: history-clear
    pattern: 'history\s+-[a-zA-Z]*c'
    reason: clearing shell history (audit log)
    action: block

  - id: privilege-escalation
    pattern: '\bsudo\s+su\b|\bsudo\s+-[a-zA-Z]*i\b'
    reason: privilege escalation to root shell
    action: hitl

  - id: package-manager-remove
    pattern: '\b(apt|apt-get|yum|dnf|pacman)\b.*\s(remove|purge|erase)\b.*--yes\b'
    reason: unattended system package removal
    action: hitl
```

### `framework/config/paths.go` additions

```go
func (p Paths) ShellBlacklistFile() string {
    return filepath.Join(p.ConfigRoot(), "shell_blacklist.yaml")
}

func (p Paths) ModelProfilesDir() string {
    return filepath.Join(p.ConfigRoot(), "model_profiles")
}
```

### Wiring

`ShellGuard` is wired at the same site where `LocalCommandRunner` or `SandboxCommandRunner` is
constructed (typically in app wiring / agent builder). The blacklist is loaded once at startup
via `sandbox.Load(config.New(workspace).ShellBlacklistFile())`.

---

## Phase 2 — Shell Command Constructor

**Goal:** Provide a validated command routing layer at `platform/shell/` with user-defined
named shell capability bindings. Bindings are auto-registered as tool capabilities when
`CommandLineTools()` is called.

**Depends on:** Phase 1 (`ShellGuard` available as `CommandRunner` decorator).

### New files

| File | Purpose |
|---|---|
| `platform/shell/command_query.go` | `ShellBinding`, `CommandQuery`, `CommandQueryBuilder` |
| `relurpify_cfg/skills/shell_bindings.yaml` | User-defined named shell capabilities template |

### Types

```go
// platform/shell/command_query.go

// ShellBinding is a user-defined named shell capability loaded from
// relurpify_cfg/skills/shell_bindings.yaml.
type ShellBinding struct {
    ID              string   // unique identifier
    Name            string   // capability name exposed to agents/skills
    Description     string
    Command         []string // argv — never shell-interpolated unless Shell: true
    Shell           bool     // if true, requires "sh"/"bash" in the allowed binary set
    ArgsPassthrough bool     // allow callers to append extra args
    Tags            []string
}

// CommandQuery resolves named bindings or validates raw args against the allowed
// binary set, then produces a sandbox.CommandRequest.
// Shell bindings with Shell:true are rejected when shell is not in the allowed set.
type CommandQuery struct {
    allowed  map[string]struct{} // binaries permitted per capability registry
    shellOK  bool                // true if sh or bash is in the allowed set
    bindings map[string]*ShellBinding
}

// Resolve returns a CommandRequest for a named binding with optional extra args.
func (q *CommandQuery) Resolve(name string, extraArgs []string) (sandbox.CommandRequest, error)

// ValidateRaw returns a CommandRequest for a raw argv slice, checking each
// binary against the allowed set. Never produces a bash -c command.
func (q *CommandQuery) ValidateRaw(args []string) (sandbox.CommandRequest, error)
```

```go
// CommandQueryBuilder constructs a CommandQuery from the capability registry and
// a bindings file path.
type CommandQueryBuilder struct {
    registry capabilityRegistry // interface: ListCapabilities() []core.CapabilityDescriptor
    bindingsPath string
}

func NewCommandQueryBuilder(registry capabilityRegistry, bindingsPath string) *CommandQueryBuilder
func (b *CommandQueryBuilder) Build() (*CommandQuery, error)
```

### YAML format (`relurpify_cfg/skills/shell_bindings.yaml`)

```yaml
# Relurpify shell bindings — user-defined named shell capabilities.
# Each binding becomes a named tool capability auto-registered when
# CommandLineTools() initializes the platform shell registry.
#
# Fields:
#   id              — unique identifier (snake_case)
#   name            — tool name exposed to agents
#   description     — shown in tool listings and prompts
#   command         — argv array, no shell interpolation (unless shell: true)
#   shell           — if true, wraps in bash -c; requires shell in allowed binary set
#   args_passthrough — if true, callers may append extra args to command
#   tags            — optional tags for capability filtering

version: "1"
bindings: []

# Examples (uncomment and adapt):
#
# - id: run-migrations
#   name: run_migrations
#   description: Runs pending database migrations
#   command: ["python", "manage.py", "migrate"]
#   shell: false
#   tags: [database, migration]
#
# - id: format-go
#   name: format_go
#   description: Formats all Go source files in the workspace
#   command: ["gofmt", "-w", "."]
#   shell: false
#   tags: [formatting]
#
# - id: custom-lint
#   name: run_custom_lint
#   description: Runs the project-specific lint script
#   command: ["./scripts/lint.sh"]
#   shell: true
#   tags: [lint, verification]
```

### Auto-registration in `CommandLineTools()`

`platform/shell/cli_registry.go` is updated so `CommandLineTools(basePath, runner)` also:

1. Loads bindings from `config.New(basePath).SkillsDir() + "/shell_bindings.yaml"`
   (missing file → no bindings, no error).
2. For each binding, constructs a `CommandTool` with `CommandToolConfig` derived from
   the binding fields.
3. Deduplicates by name against the existing tool set (binding names take lower priority
   than built-in tools to prevent shadowing).

Bindings with `Shell: true` are only registered when `bash` or `sh` appears in the runner's
allowed binary set. If shell is not available the binding is silently skipped (log at debug
level).

---

## Phase 3 — Model Profiles

**Goal:** Replace scattered per-model fixups with a declarative YAML profile system in
`platform/llm/`. Switching models no longer silently inherits qwen-specific workarounds.

**Depends on:** Nothing. Can be built before or after Phases 1 and 2.

### New files

| File | Purpose |
|---|---|
| `framework/core/llm_types.go` | Add `ProfiledModel` optional interface |
| `platform/llm/model_profile.go` | `ModelProfile` struct + name matching |
| `platform/llm/profile_registry.go` | `ProfileRegistry` — loads from `configDir` |
| `relurpify_cfg/model_profiles/default.yaml` | Baseline profile (all quirks off) |
| `relurpify_cfg/model_profiles/qwen2.5-coder.yaml` | Codifies current qwen fixups |

### `ProfiledModel` optional interface (`framework/core/llm_types.go`)

```go
// ProfiledModel is an optional extension for LanguageModel implementations that
// expose active model profile metadata. Callers type-assert to check support:
//
//   if pm, ok := model.(core.ProfiledModel); ok { ... }
//
// The LanguageModel interface is not changed.
type ProfiledModel interface {
    ToolRepairStrategy() string // "llm" | "heuristic-only"
    MaxToolsPerCall() int       // 0 = no limit
}
```

### `ModelProfile` struct (`platform/llm/model_profile.go`)

```go
type ModelProfile struct {
    // Pattern is matched against the model name. Supports prefix matching
    // and glob wildcards (e.g. "qwen2.5-coder*", "llama3*", "*").
    Pattern string `yaml:"pattern"`

    ToolCalling struct {
        // NativeAPI maps to core.Config.OllamaToolCalling when not explicitly set.
        NativeAPI               bool `yaml:"native_api"`
        // DoubleEncodedArgs enables double-decode in parseArguments (qwen quirk:
        // arguments JSON is sometimes returned as a quoted string).
        DoubleEncodedArgs       bool `yaml:"double_encoded_args"`
        // MultilineStringLiterals enables normalizeMultilineJSONStringLiterals
        // (qwen quirk: literal newlines inside JSON string values).
        MultilineStringLiterals bool `yaml:"multiline_string_literals"`
        // MaxToolsPerCall limits how many tool calls are processed per response.
        // 0 = no limit.
        MaxToolsPerCall         int  `yaml:"max_tools_per_call"`
    } `yaml:"tool_calling"`

    Repair struct {
        // Strategy controls repair behaviour when tool call parsing fails.
        // "llm"            — issue a second Generate call with a repair prompt.
        // "heuristic-only" — use text heuristics only, no second LLM call.
        Strategy    string `yaml:"strategy"`
        MaxAttempts int    `yaml:"max_attempts"`
    } `yaml:"repair"`

    Schema struct {
        // FlattenNested collapses nested object schemas to top-level properties
        // for models that cannot handle nested parameter schemas.
        FlattenNested     bool `yaml:"flatten_nested"`
        // MaxDescriptionLen truncates tool/parameter descriptions to this length.
        // 0 = no truncation.
        MaxDescriptionLen int  `yaml:"max_description_len"`
    } `yaml:"schema"`
}
```

### `ProfileRegistry` (`platform/llm/profile_registry.go`)

```go
// ProfileRegistry loads ModelProfile files from a directory and matches them
// by model name. The registry falls back to the built-in default profile
// when no file matches and no default.yaml is present.
type ProfileRegistry struct { ... }

// NewProfileRegistry loads all *.yaml files from configDir.
// Missing directory returns an empty registry using built-in defaults.
func NewProfileRegistry(configDir string) (*ProfileRegistry, error)

// Match returns the best-matching profile for modelName.
// Matching priority: longest specific prefix > "*" wildcard > built-in default.
func (r *ProfileRegistry) Match(modelName string) *ModelProfile
```

### YAML templates

**`relurpify_cfg/model_profiles/default.yaml`**
```yaml
# Default model profile — safe baseline applied when no specific profile matches.
# All quirk flags off; repair via heuristics only (no extra LLM call).
pattern: "*"
tool_calling:
  native_api: false
  double_encoded_args: false
  multiline_string_literals: false
  max_tools_per_call: 0
repair:
  strategy: heuristic-only
  max_attempts: 0
schema:
  flatten_nested: false
  max_description_len: 0
```

**`relurpify_cfg/model_profiles/qwen2.5-coder.yaml`**
```yaml
# Profile for qwen2.5-coder models.
# Codifies workarounds that were previously unconditional in ollama.go and
# react_think_node.go.
pattern: "qwen2.5-coder*"
tool_calling:
  native_api: true
  double_encoded_args: true
  multiline_string_literals: true
  max_tools_per_call: 1
repair:
  strategy: llm
  max_attempts: 1
schema:
  flatten_nested: false
  max_description_len: 0
```

### Wiring changes

**`platform/llm/ollama.go`**

`Client` gains a `profile *ModelProfile` field set via `SetProfile(p *ModelProfile)` or
`NewClientWithProfile(endpoint, model string, p *ModelProfile)`.

Three conditional sites replace unconditional behaviour:

| Current unconditional call | Becomes |
|---|---|
| `parseArguments` double-decode branch | `if c.profile.ToolCalling.DoubleEncodedArgs { ... }` |
| `normalizeMultilineJSONStringLiterals` call | `if c.profile.ToolCalling.MultilineStringLiterals { ... }` |
| `convertLLMToolSpecs` schema conversion | respect `MaxDescriptionLen` and `FlattenNested` |

`Client` implements `core.ProfiledModel`:

```go
func (c *Client) ToolRepairStrategy() string {
    if c.profile == nil { return "heuristic-only" }
    return c.profile.Repair.Strategy
}
func (c *Client) MaxToolsPerCall() int {
    if c.profile == nil { return 0 }
    return c.profile.ToolCalling.MaxToolsPerCall
}
```

**`agents/react/react_think_node.go`**

`repairDecision` is called conditionally:

```go
repairStrategy := "heuristic-only"
if pm, ok := n.agent.Model.(core.ProfiledModel); ok {
    repairStrategy = pm.ToolRepairStrategy()
}
if repairStrategy == "llm" {
    repaired, repairErr = n.repairDecision(ctx, tools, resp.Text, useToolCalling)
}
```

`filterToolCalls` respects `MaxToolsPerCall` when > 0:

```go
maxTools := 0
if pm, ok := n.agent.Model.(core.ProfiledModel); ok {
    maxTools = pm.MaxToolsPerCall()
}
if maxTools > 0 && len(out) > maxTools {
    out = out[:maxTools]
}
```

**`OllamaToolCalling` backwards compatibility**

`NativeAPI` from the profile is used to derive `OllamaToolCalling` in `core.Config` only
when `OllamaToolCalling` is at its zero value (false, unset). An explicitly set `true` in
the manifest or config always wins.

---

## Summary of new workspace files

```
relurpify_cfg/
  shell_blacklist.yaml              # Phase 1 — community rule template
  skills/
    shell_bindings.yaml             # Phase 2 — user-defined shell capabilities
  model_profiles/
    default.yaml                    # Phase 3 — baseline profile
    qwen2.5-coder.yaml              # Phase 3 — current model profile
```

## Summary of changed source files

| File | Phase | Change |
|---|---|---|
| `framework/config/paths.go` | 1 | Add `ShellBlacklistFile()`, `ModelProfilesDir()` |
| `framework/sandbox/shell_blacklist.go` | 1 | New |
| `framework/sandbox/shell_guard.go` | 1 | New |
| `framework/core/llm_types.go` | 3 | Add `ProfiledModel` interface |
| `platform/llm/model_profile.go` | 3 | New |
| `platform/llm/profile_registry.go` | 3 | New |
| `platform/llm/ollama.go` | 3 | Wire profile, gate three fixup sites |
| `platform/shell/command_query.go` | 2 | New |
| `platform/shell/cli_registry.go` | 2 | Auto-register bindings in `CommandLineTools()` |
| `agents/react/react_think_node.go` | 3 | Consult `ProfiledModel` for repair + max tools |
