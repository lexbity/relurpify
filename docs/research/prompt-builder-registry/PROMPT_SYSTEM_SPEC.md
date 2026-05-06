# Prompt System — Engineering Specification

**Status**: Draft v2
**Audience**: Engineers implementing the system
**Scope**: File format, parser semantics, registry, provider model, runtime integration
**Schema version**: `framework.prompt/v1`

---

## 1. Document Purpose

This document specifies the design and implementation contract for the framework's prompt system. It covers:

1. The `.prompt` file format — syntax, semantics, validation rules
2. The `framework/prompt` package — registry, parser, resolver, types, provider interface
3. The provider model — how `agents/promptprovider` and `named/<agent>/promptprovider` packages contribute runtime context to prompts
4. Runtime integration — how the registry interacts with the envelope, captures, telemetry, and the rest of the framework

The document is implementation-oriented. Rationale appears only where a decision is non-obvious or where a tempting alternative is being explicitly rejected.

## 2. System Overview

### 2.1 Conceptual Model

A **prompt** is a file-resident, composable, statically-defined description of model instructions. At runtime, the prompt system resolves a prompt config against runtime context and produces a single string passed to the LLM.

Prompts are workspace configuration. They live as text files in `<workspace>/relurpify_cfg/prompts/`, are cloned from a default template tree at workspace initialization, and are owned by the user thereafter.

Three Go packages participate in the system:

- **`framework/prompt`** — owns the prompt resource type. Defines `PromptConfig`, the registry, the `.prompt` parser, the resolver pipeline, the `ContextProvider` interface, and the runtime context type. Imports nothing paradigm-specific or agent-specific.
- **`agents/promptprovider`** — provides context provider implementations for generic execution paradigms (react, pipeline, htn, goalcon, blackboard, rewoo). Exposes `RegisterAll(r prompt.Registry) error`; called by named agents during their own initialization.
- **`named/<agent>/promptprovider`** — provides agent-specific context provider implementations (e.g., Euclo's recipe-step-aware providers). Exposes `RegisterAll(r prompt.Registry) error`; called by the named agent during its own initialization.

Prompt files live in none of these packages. They live in the workspace configuration tree, scanned at `ayenitd.Open()` time.

### 2.2 Architectural Principles

Five principles govern the design. Where the spec is ambiguous, fall back to these.

**(P1) Resolves to a string.** The LLM receives a single string. Composition, conditions, providers, and inheritance are pipeline machinery — invisible to the model.

**(P2) Logic in metadata, not content.** Conditional logic, ordering, inheritance, variable declarations live in metadata fields. Block content is clean prose. No template control-flow syntax inside content.

**(P3) Blocks are self-contained.** Any block may be excluded at resolution time via `when`. Blocks must therefore be coherent in isolation. Inter-block sentence-level continuity is forbidden.

**(P4) Visibility is the contract.** All prompts the system uses live in the workspace tree. Framework-internal prompts are visible and editable, with metadata signaling criticality. The system does not hide prompts.

**(P5) Frozen workspaces.** The system assumes the pure-template config model: workspace clones template at init, never auto-syncs. The prompt system does not implement migration. Restore-from-template is the recovery mechanism for corrupted prompts.

### 2.3 Lifecycle Overview

```
Workspace init      → template cloned to relurpify_cfg/prompts/
ayenitd.Open()      → prompt.BuildRegistry(workspacePath) constructs registry,
                      calls LoadDir(relurpify_cfg/prompts/), attaches to WorkspaceEnvironment
Named-agent init    → Agent.Initialize() calls agents/promptprovider.RegisterAll(env.PromptRegistry)
                      then named/<agent>/promptprovider.RegisterAll(env.PromptRegistry)
Runtime resolve     → env.PromptRegistry.Resolve(id, ctx) → assembled string
LLM invocation      → string passed as system prompt
```

Provider registration happens during named-agent initialization, not at `ayenitd.Open()`. This mirrors the pattern used by capability handlers: `ayenitd` builds the registry and places it on `WorkspaceEnvironment`; each named agent registers its own providers (and the paradigm providers it depends on) during `Agent.Initialize()`. `ayenitd` never imports `agents/` or `named/` for this purpose.

## 3. The `.prompt` File Format

### 3.1 File Structure

A `.prompt` file is UTF-8 text with three sections:

1. **Front matter** — fenced by `---` lines at the very top of the file. Contains config-level metadata: id, name, inheritance, tags, variable declarations, lifecycle flags.
2. **Blocks** — Markdown-style sections delimited by `# Heading` lines. Each block contains optional metadata (tilde lines) and prose content.
3. **End-of-file** — no trailer required.

A canonical example:

```
---
apiVersion: framework.prompt/v1
id: agent.euclo.debug.locate_failure
name: Locate Test Failure
extends: agent.euclo.debug.base
description: Identify the source and likely cause of a test failure.

tags:
  paradigm: [react]
  agent: [euclo]
  domain: [debug, code-analysis]
  kind: task
  stability: stable

variables:
  failing_test:
    default: "(unknown)"
  language:
    default: "the project language"

requires_providers:
  - react.tools
  - euclo.recipe_step_context
---

# Task

Identify the source of the failing test and articulate the likely cause.
You are working on {failing_test} in {language}.

# Capabilities
~ kind: capability

Use `test_run` to execute tests and observe failure output.
Use `ast_query` to inspect code structure around the failure.

# Available Tools
~ from: provider
~ provider: react.tools

# Output
~ kind: format
~ order: late

Your response must produce two fields:

- `failure_location`: the file path and line where the failure originates
- `likely_cause`: a short description of the probable root cause
```

### 3.2 Front Matter

#### 3.2.1 Format

Front matter is a YAML-like block fenced by `---` lines. The opening `---` must be the first line of the file. The closing `---` ends front matter and begins the block section.

Although the syntax mirrors YAML, the parser implementation is **not** required to accept arbitrary YAML. It must accept the schema defined in §3.2.3 and may reject syntax outside that schema. This intentionally limits parser surface area and makes the format self-contained.

#### 3.2.2 Required and Optional Fields

| Field | Type | Required | Description |
|---|---|---|---|
| `apiVersion` | string | yes | Schema version. Must be `framework.prompt/v1` |
| `id` | string | yes | Unique identifier. Namespaced (see §3.3) |
| `name` | string | yes | Human-readable display name |
| `extends` | string | no | Parent prompt id |
| `description` | string | no | One-line description |
| `tags` | map | no | Tag schema (§3.4) |
| `variables` | map | no | Variable declarations (§3.5) |
| `framework_critical` | bool | no | Defaults to `false`. See §3.6 |
| `requires_providers` | string list | no | Provider names this prompt depends on (§3.7) |

Unknown front matter fields produce a load-time validation **warning**, not an error. This allows future extensions without breaking older configs.

#### 3.2.3 Front Matter Grammar

```ebnf
front_matter   = "---" NEWLINE entry* "---" NEWLINE
entry          = key ":" value NEWLINE
                | key ":" NEWLINE indented_value
key            = identifier
value          = scalar | list | map_inline
scalar         = string_unquoted | string_quoted | bool | int
list           = "[" (scalar ("," scalar)*)? "]"
map_inline     = "{" (key ":" scalar ("," key ":" scalar)*)? "}"
indented_value = (INDENT entry)+
identifier     = [a-zA-Z_][a-zA-Z0-9_]*
```

The parser is line-oriented. Indentation is two spaces. Strings without colons, leading whitespace, or special characters may be unquoted. Lists and inline maps follow the bracketed forms shown above.

### 3.3 Identifier Namespacing

Prompt IDs follow a strict namespacing convention. The first segment determines ownership and access rules.

| Prefix | Owner | Editable? | Cloneable? |
|---|---|---|---|
| `framework.*` | framework | yes (visible) | yes |
| `agent.<paradigm>.*` | agents package | yes (visible) | yes |
| `agent.<named>.*` | named agent | yes (visible) | yes |
| `capability.<id>.*` | capability author | yes (visible) | yes |
| `user.*` | end user | yes | no (user-created) |

All IDs are dot-segmented, lowercase, with segment characters `[a-z0-9_]`. Empty segments are forbidden. The maximum total length is 256 characters.

The `framework.*` prefix has no enforcement at the registry level — the system follows P4 (visibility is the contract). Tooling (linters, restore commands) uses the prefix to identify framework-critical prompts and warn on edits.

### 3.4 Tags

Tags categorize prompts along five orthogonal axes. Each axis serves different consumers (registry queries, validation, tooling, policy).

#### 3.4.1 Tag Schema

```yaml
tags:
  paradigm: [react, pipeline]      # list[enum]
  agent:    [euclo, framework]      # list[string]
  domain:   [debug, code-analysis]  # list[string], free-form
  kind:     task                    # enum, single value
  stability: stable                 # enum, single value
```

#### 3.4.2 Axis Definitions

**`paradigm`** — a list of execution paradigms this prompt is compatible with. Enforced at resolve time: a paradigm not listed cannot consume this prompt. Empty or omitted means *paradigm-agnostic* (any paradigm may consume).

Allowed values (extensible by convention, validated against the registered paradigm set at load time):
`react`, `pipeline`, `htn`, `goalcon`, `blackboard`, `rewoo`.

**`agent`** — a list of named agents (or `framework`) this prompt is intended for. **Hint, not constraint.** A different agent may consume the prompt; the tag exists for filtering, organization, and tooling. Empty or omitted means *generic*.

**`domain`** — free-form subject-matter tags. The team maintains a recommended vocabulary but the registry does not enforce it. Used by tooling for search and grouping.

**`kind`** — semantic role. Single value from a fixed enum:
- `persona` — identity, who-the-model-is content
- `task` — what to do for this invocation
- `capability` — instructions about available tools/capabilities
- `format` — output structure and style
- `constraint` — rules and prohibitions
- `fragment` — reusable piece intended for inheritance, not direct invocation

**`stability`** — lifecycle status. Single value from a fixed enum:
- `experimental` — under development, may change without notice
- `beta` — usable but not guaranteed stable
- `stable` — production-suitable
- `deprecated` — slated for removal; tooling should warn on use

Default if omitted: `stability: stable`.

#### 3.4.3 Inheritance and Tags

When a prompt `extends` another, tag rules are:

- **`paradigm`**: child's set must be a subset of parent's. A child cannot broaden compatibility. Empty parent (paradigm-agnostic) allows any child set.
- **`agent`**: no inheritance constraint. Child may declare any agent set.
- **`domain`**: child's set is unioned with parent's at registry indexing time.
- **`kind`**: child overrides parent. If absent, inherits parent's value.
- **`stability`**: child overrides parent. If absent, defaults to `stable`.

Validation rejects child paradigm sets that broaden parent's.

### 3.5 Variables

Variables are declared in front matter and referenced in block content with single-brace interpolation: `{variable_name}`.

#### 3.5.1 Declaration

```yaml
variables:
  failing_test:
    default: "(unknown)"
  language:
    default: "the project language"
```

Each variable is a map. Currently the only supported key is `default`. The schema allows additive extensions without breaking parsers (e.g., future `description`, `type`, `required` keys).

#### 3.5.2 Interpolation Syntax

Variables are referenced in block content as `{variable_name}`. Dot notation is supported for nested values supplied at runtime: `{supabase.url}`.

Escape literal braces with `\{` and `\}`. The parser must preserve `\{` as a literal `{` in output.

#### 3.5.3 Resolution Order

For each `{name}` reference, the resolver attempts in order:

1. Runtime-supplied value from `RuntimeContext.Variables`
2. Default declared in front matter
3. Empty string (warn but do not error)

Resolution is single-pass. Variables in resolved values are not recursively interpolated (no template-language behavior).

#### 3.5.4 Inheritance

Child variables are unioned with parent variables. If a child declares a variable with the same name as a parent, the child's declaration wins entirely (full override).

### 3.6 Framework-Critical Flag

Front-matter `framework_critical: true` marks a prompt whose correctness governs framework operations. The registry does not enforce protection — P4 dictates visibility. The flag drives tooling behavior:

- UI/CLI displays a warning banner when editing
- Restore-from-template lists framework-critical prompts first
- Linters check stricter rules (warns on incomplete inheritance, missing required providers)
- Diagnostic commands report framework-critical health

The flag is **independent of the `agent: [framework]` tag**. The tag is categorization; the flag is operational disposition. A prompt may have either, both, or neither. Convention: framework-internal prompts (e.g., `framework.classification.tier2`) carry both.

### 3.7 Required Providers

`requires_providers` declares context providers this prompt depends on. The registry validates at load time that every name in this list resolves to a registered provider; failure produces a load-time error for this prompt.

```yaml
requires_providers:
  - react.tools
  - euclo.recipe_step_context
```

Without this declaration, missing-provider failures occur at resolve time and produce empty content for that block (with telemetry warnings). The declaration moves detection earlier and makes the dependency explicit.

**Timing note**: `requires_providers` validation runs at load time. Because prompt files are loaded by `ayenitd` before named agents call `Initialize()`, providers registered during agent initialization are not yet present when `LoadDir` runs. The registry therefore defers `requires_providers` validation to a second pass triggered after all agents have initialized. See §5.1 for the two-phase init sequence.

If a prompt has `~ from: provider` blocks but does not list the referenced provider in `requires_providers`, that is a validation **warning**, not an error. Some prompts intentionally accept missing providers (e.g., for graceful degradation).

### 3.8 Blocks

#### 3.8.1 Block Delimiters

A block begins with a Markdown-style heading line: `# Block Name`. The heading must start at column 0. Heading text becomes the block's `name` field. The block's `id` is derived from the name by lowercasing and replacing whitespace and non-`[a-z0-9_]` characters with `_`.

Blocks end at the next `# ` heading or end of file.

The parser does **not** support `##`, `###`, or other heading levels. The format reserves them for future extension.

#### 3.8.2 Block Metadata (Tilde Lines)

Lines beginning with `~ key: value` immediately after a heading specify block metadata. Tilde lines must be contiguous with the heading — no blank lines between heading and tildes. The first non-tilde, non-blank line begins the block content.

```
# Output
~ kind: format
~ order: late

Your response must produce two fields:
...
```

#### 3.8.3 Block Metadata Schema

| Key | Type | Description |
|---|---|---|
| `kind` | enum | Same values as tag `kind` (§3.4.2). Defaults to inheriting from front-matter `tags.kind` |
| `order` | enum or int | Assembly position. See §3.8.4 |
| `when` | expression | Conditional inclusion. See §3.9 |
| `from` | enum | `static` (default) or `provider`. See §3.8.5 |
| `provider` | string | Name of context provider. Required if `from: provider` |
| `locked` | bool | Defaults to `false`. See §3.8.6 |

Unknown block metadata keys produce validation warnings.

#### 3.8.4 Order

Blocks are sorted ascending by `order` at assembly time. Order accepts:

- **Named positions**: `early`, `middle` (default), `late`, `last`
- **Numeric values**: integers 1–999

Named positions map internally to integers:

```
early  = 10
middle = 50
late   = 80
last   = 99
```

Numeric overrides allow fine-grained control: `order: 85` places after `late` but before `last`. Within the same numeric value, ties break by file order (the order blocks appear in the file).

For inherited blocks, file order is determined by traversal order: parent blocks before child-declared blocks, except where child overrides parent by id.

Recommended convention (not enforced):

| Range | Purpose |
|---|---|
| 1–20 | Persona, identity |
| 21–50 | Capability, tool instructions |
| 51–70 | Output format, style |
| 71–90 | Constraints, warnings |
| 91–99 | Last-mile context, dynamic injection |

#### 3.8.5 Source: `from`

Blocks have one of two sources:

- **`from: static`** (default) — content comes from the block's prose body in the file.
- **`from: provider`** — content is supplied at resolve time by a registered context provider. The block must have a `provider:` field. Block prose body is ignored if present (parser warns).

`from: provider` blocks have no static content. They are placeholders that the resolver fills from runtime context via the named provider.

#### 3.8.6 Locked Blocks

`~ locked: true` marks a block that cannot be overridden in child prompts. If a child prompt declares a block with the same id as a locked parent block, validation fails at registry load time.

This is distinct from `framework_critical` (file-level disposition). Locked is *structural protection across inheritance*; critical is *operational signaling to tooling*.

### 3.9 Conditional Inclusion

Blocks with `~ when: <expression>` are conditionally included based on runtime state.

#### 3.9.1 Expression Grammar

```ebnf
expr        = or_expr
or_expr     = and_expr ("||" and_expr)*
and_expr    = unary_expr ("&&" unary_expr)*
unary_expr  = primary | primary "exists" | "(" expr ")"
primary     = comparison | path | literal
comparison  = path op (path | literal)
op          = "==" | "!=" | ">" | "<" | ">=" | "<="
path        = identifier ("." identifier)*
literal     = quoted_string | int | float | bool
identifier  = [a-zA-Z_][a-zA-Z0-9_]*
```

#### 3.9.2 Semantics

- **Truthiness**: a bare path `~ when: supabase.connected` evaluates the path's value as boolean. Booleans evaluate directly; numbers are true if non-zero; strings are true if non-empty; null/undefined is false.
- **Equality**: `==` and `!=` compare values. Type coercion rules: numbers compare numerically; strings compare lexicographically; booleans only with booleans; comparing across types is false (with warning).
- **Ordering**: `>`, `<`, `>=`, `<=` are numeric only. String or boolean operands produce evaluation error → block excluded with warning.
- **`exists`**: `path exists` is true if the path resolves to any defined value (including false, 0, empty string). False if undefined.
- **Logical**: `&&` and `||` short-circuit, standard precedence. Parentheses for grouping.

#### 3.9.3 Forbidden Constructs

The expression language explicitly excludes:

- Function calls
- String concatenation, arithmetic, any value-producing operation
- Loops, ternary, conditional expressions other than the boolean operators above
- Side effects of any kind

The evaluator must be a hand-written parser. Use of `eval()`, `unsafe`, or any reflection-based code path is forbidden — the expression language is a security boundary (see §6).

#### 3.9.4 Evaluation Errors

If an expression cannot be evaluated (malformed reference, type error, etc.):

1. The block is **excluded** from assembly
2. A warning event is emitted (§7)
3. Resolution continues

This is consistent with P5's "frozen workspace" stance: a malformed expression in one block must not prevent the rest of the prompt from resolving.

### 3.10 Inheritance

#### 3.10.1 Resolution

When a prompt declares `extends: <parent-id>`:

1. Parent is loaded recursively (parents-of-parents resolved first)
2. Parent's blocks form the base set
3. Child's blocks override parent's by block `id` (full override, not merge)
4. Child's new blocks (id not in parent set) append to the set
5. Locked parent blocks cannot be overridden — validation error if child attempts
6. Inheritance chain depth is limited to 8 levels — exceeded → validation error
7. Circular inheritance is detected at load time → validation error

#### 3.10.2 Locked-Block Semantics

A locked parent block is included verbatim (parent's content, parent's metadata) in the child's effective block set. The child cannot:

- Override the block by declaring a same-id block (validation error)
- Drop the block via `when` if the parent's `when` doesn't apply (locked blocks may still have `when`; that condition still applies)

The child *can* still freely interleave new blocks around a locked block.

#### 3.10.3 Variable and Tag Inheritance

See §3.5.4 (variables) and §3.4.3 (tags).

### 3.11 Validation Rules

The registry validates at load time and rejects prompts that fail any of:

**Schema errors (rejection):**
- Missing required fields (`apiVersion`, `id`, `name`)
- Unknown `apiVersion`
- ID does not match namespace pattern
- Duplicate block ids within a single prompt
- `extends` references unknown parent (after all prompts are loaded)
- Circular inheritance
- Inheritance depth exceeds 8
- Child overrides locked parent block
- Block has `from: provider` without `provider:`
- `paradigm` tag declares unknown paradigm
- Child paradigm tag broadens parent's

**Provider validation (deferred to post-agent-init pass):**
- `requires_providers` references unregistered provider (see §5.1)

**Semantic warnings (not rejection):**
- Unknown front matter or block metadata fields
- Variable declared but unused in any block
- Variable referenced but not declared (resolves to empty string)
- `order` collisions
- Block content empty after trimming (for `from: static`)
- Block body present for `from: provider`
- Block uses provider not in `requires_providers`

A rejected prompt is logged and excluded from the registry; other prompts continue to load. The registry remains functional with a partial set; consumers receive errors when resolving rejected ids.

## 4. The `framework/prompt` Package

### 4.1 Package Structure

```
framework/prompt/
  doc.go              — package documentation
  types.go            — exported types (PromptConfig, PromptBlock, Tags, etc.)
  registry.go         — Registry interface + default implementation
  parser/
    parser.go         — .prompt file parser
    front_matter.go   — front matter sub-parser
    blocks.go         — block parser
    expression.go     — when-expression parser/evaluator
  resolver/
    resolver.go       — assembly pipeline
    inherit.go        — inheritance merge
    interpolate.go    — variable interpolation
  context.go          — RuntimeContext, ContextChunk, ContextProvider
  validate.go         — validation logic
  events.go           — telemetry event types
  errors.go           — error types
  prompttest/         — test helpers (MockRegistry, test fixtures)
```

The package imports nothing from `agents/`, `named/`, or any paradigm-specific code.

### 4.2 Core Types

#### 4.2.1 PromptConfig

```go
package prompt

// PromptConfig is the parsed representation of a .prompt file.
// It is immutable after registry indexing.
type PromptConfig struct {
    APIVersion         string
    ID                 string
    Name               string
    Description        string
    Extends            string  // parent prompt id, empty if none
    FrameworkCritical  bool
    RequiresProviders  []string
    Tags               Tags
    Variables          map[string]VariableDecl
    Blocks             []PromptBlock

    // populated by registry indexing, not parser
    sourcePath         string
    parentResolved     *PromptConfig
}

type Tags struct {
    Paradigm  []string
    Agent     []string
    Domain    []string
    Kind      string
    Stability string
}

type VariableDecl struct {
    Default string
}

type PromptBlock struct {
    ID        string
    Name      string
    Kind      string
    Order     int
    When      Expression  // nil if no condition
    From      BlockSource // SourceStatic or SourceProvider
    Provider  string      // empty for SourceStatic
    Locked    bool
    Content   string      // raw prose, pre-interpolation
}

type BlockSource int

const (
    SourceStatic   BlockSource = iota
    SourceProvider             // from: provider
)
```

#### 4.2.2 RuntimeContext

```go
// RuntimeContext is the input to Resolve. It carries everything the resolver
// needs to assemble a final string: variable values, state for when-expressions,
// and a handle to the envelope for providers that read it.
type RuntimeContext struct {
    Variables   map[string]string
    State       map[string]any   // evaluated against when-expressions
    Envelope    contextdata.Envelope
    Paradigm    string           // execution paradigm consuming the prompt
    ConsumerID  string           // id of agent or capability invoking resolve
}

// ContextChunk is the runtime-supplied content for a provider block.
type ContextChunk struct {
    Content string
}
```

#### 4.2.3 ContextProvider

The provider interface is intentionally minimal. The base interface has one method; richer behaviors layer through optional interfaces.

```go
// ContextProvider supplies content for a provider block at resolve time.
// Providers are registered with the registry by name and referenced from
// .prompt files via the `provider:` block metadata field.
//
// Providers must be safe to call concurrently. A single provider instance
// may serve many concurrent Resolve calls.
type ContextProvider interface {
    // Provide returns the content for a provider block.
    // The implementation reads only from ctx; it must not retain ctx after return.
    Provide(ctx RuntimeContext) ContextChunk
}

// DescribingProvider is an optional interface. Providers that implement it
// supply metadata for tooling (registry introspection, validation, UI).
type DescribingProvider interface {
    ContextProvider
    Describe() ProviderMetadata
}

type ProviderMetadata struct {
    Name        string
    Description string
    Paradigms   []string  // paradigms this provider applies to (empty = any)
    ReadsKeys   []string  // envelope keys this provider reads (for static analysis)
}

// FailableProvider is an optional interface. Providers that implement it
// can return an error from Provide, which the resolver handles per §5.5.
// Providers that do not implement this interface are assumed to never fail.
type FailableProvider interface {
    ContextProvider
    ProvideOrFail(ctx RuntimeContext) (ContextChunk, error)
}
```

Three things to note:

- **Concurrency**: providers serve many Resolve calls in parallel. Implementations must be re-entrant. The registry does not serialize.
- **Lifetime**: providers are registered once during agent initialization and live for the process lifetime. There is no Close, no per-resolve setup/teardown.
- **Failure model**: by default, providers cannot fail. A provider that needs to signal failure must implement `FailableProvider`. The resolver checks for this interface at registration time and dispatches accordingly.

### 4.3 Registry Interface

```go
// Registry is the central index of prompts and providers. It is created
// once at ayenitd.Open() time and lives for the process lifetime.
type Registry interface {
    // Loading. Called during ayenitd.Open().
    LoadDir(dir string) error
    LoadFS(fs fs.FS, prefix string) error

    // Provider registration. Called by named agents during Initialize().
    RegisterProvider(name string, provider ContextProvider) error

    // Deferred provider validation. Called once after all agents have initialized.
    // Validates requires_providers declarations against the full registered set.
    ValidateProviders() []ValidationIssue

    // Lookup.
    Get(id string) (*PromptConfig, bool)
    All() []*PromptConfig
    Filter(opts FilterOptions) []*PromptConfig
    Count() int

    // Resolution.
    Resolve(id string, ctx RuntimeContext) (string, error)
    ResolveDryRun(id string, ctx RuntimeContext) (DryRunResult, error)

    // Introspection (for tooling, debug UIs, validators).
    DependsOn(id string) ([]string, error)
    DependentsOf(id string) ([]string, error)
    Variables(id string) ([]VariableDecl, error)
    Validate(id string) []ValidationIssue
    ValidateAll() map[string][]ValidationIssue
}

type FilterOptions struct {
    Paradigm  string  // exact match against tags.paradigm; empty = any
    Agent     string  // exact match against tags.agent; empty = any
    Domain    string  // contains-match; empty = any
    Kind      string  // exact match; empty = any
    Stability string  // exact match; empty = any
}

type DryRunResult struct {
    Final          string
    BlocksIncluded []BlockTrace
    BlocksExcluded []BlockTrace
    Variables      map[string]string  // resolved values
    Warnings       []ValidationIssue
}

type BlockTrace struct {
    BlockID    string
    Source     BlockSource
    Order      int
    Reason     string  // why included or excluded
}

type ValidationIssue struct {
    Severity  IssueSeverity  // Error | Warning
    Code      string         // stable code, e.g. "circular_inheritance"
    Message   string
    Location  string         // file path + line if applicable
}
```

### 4.4 Default Implementation

A standard implementation lives in `framework/prompt/registry.go`. It is the sole expected implementation; the interface exists primarily for testability and mocking, not for multiple production implementations.

```go
type registry struct {
    mu        sync.RWMutex
    prompts   map[string]*PromptConfig          // id -> config
    providers map[string]ContextProvider        // name -> provider
    deps      map[string][]string               // id -> ids it extends or uses
    rdeps     map[string][]string               // id -> ids that extend or use it
    paradigms map[string]struct{}               // known paradigm names (for validation)
    issues    map[string][]ValidationIssue      // id -> issues from load
}

func NewRegistry() Registry {
    return &registry{
        prompts:   make(map[string]*PromptConfig),
        providers: make(map[string]ContextProvider),
        deps:      make(map[string][]string),
        rdeps:     make(map[string][]string),
        paradigms: defaultParadigms(),
        issues:    make(map[string][]ValidationIssue),
    }
}
```

Concurrency: the registry uses RWMutex. Loads (write) and provider registration (write) are exclusive; lookups and resolves (read) are concurrent. The expected pattern is *load-once-at-open, register-providers-at-agent-init, resolve-many-at-runtime*; write contention is not an optimization concern.

#### 4.4.1 LoadDir

```go
func (r *registry) LoadDir(dir string) error {
    files, err := walkPromptFiles(dir)
    if err != nil {
        return fmt.Errorf("scan prompts dir: %w", err)
    }

    // Two-pass load: parse all, then validate inheritance.
    // Provider validation is deferred — see ValidateProviders().
    parsed := make(map[string]*PromptConfig, len(files))
    for _, path := range files {
        cfg, err := parser.ParseFile(path)
        if err != nil {
            r.recordIssue("", IssueError, "parse_failed", err.Error(), path)
            continue
        }
        if existing, dup := parsed[cfg.ID]; dup {
            r.recordIssue(cfg.ID, IssueError, "duplicate_id",
                fmt.Sprintf("duplicate id, also defined at %s", existing.sourcePath),
                path)
            continue
        }
        parsed[cfg.ID] = cfg
    }

    r.mu.Lock()
    defer r.mu.Unlock()

    // Pass 2: validate cross-prompt references (inheritance, paradigm tags).
    // requires_providers is validated later via ValidateProviders().
    for id, cfg := range parsed {
        issues := r.validateStructure(cfg, parsed)
        if hasErrors(issues) {
            r.issues[id] = issues
            continue
        }
        r.prompts[id] = cfg
        r.indexDeps(cfg)
        if len(issues) > 0 {
            r.issues[id] = issues  // warnings only
        }
    }
    return nil
}
```

#### 4.4.2 RegisterProvider

```go
func (r *registry) RegisterProvider(name string, provider ContextProvider) error {
    if name == "" {
        return errors.New("provider name must not be empty")
    }
    if provider == nil {
        return errors.New("provider must not be nil")
    }
    r.mu.Lock()
    defer r.mu.Unlock()
    if _, exists := r.providers[name]; exists {
        return fmt.Errorf("provider %q already registered", name)
    }
    r.providers[name] = provider
    return nil
}
```

Collisions are errors, not silent overrides. Multiple agent packages registering the same provider name is a programming bug; the registry refuses to mask it.

#### 4.4.3 ValidateProviders

Called once after all agents have completed `Initialize()`. Validates `requires_providers` declarations against the full registered provider set, recording errors against the relevant prompt IDs.

```go
func (r *registry) ValidateProviders() []ValidationIssue {
    r.mu.Lock()
    defer r.mu.Unlock()

    var all []ValidationIssue
    for id, cfg := range r.prompts {
        for _, name := range cfg.RequiresProviders {
            if _, ok := r.providers[name]; !ok {
                issue := ValidationIssue{
                    Severity: IssueError,
                    Code:     "missing_required_provider",
                    Message:  fmt.Sprintf("prompt %s requires provider %q which is not registered", id, name),
                }
                r.issues[id] = append(r.issues[id], issue)
                all = append(all, issue)
            }
        }
    }
    return all
}
```

#### 4.4.4 Resolve

```go
func (r *registry) Resolve(id string, ctx RuntimeContext) (string, error) {
    r.mu.RLock()
    defer r.mu.RUnlock()

    cfg, ok := r.prompts[id]
    if !ok {
        return "", &NotFoundError{ID: id}
    }
    if !paradigmCompatible(cfg, ctx.Paradigm) {
        return "", &ParadigmMismatchError{
            ID:        id,
            Required:  cfg.Tags.Paradigm,
            Actual:    ctx.Paradigm,
        }
    }

    return r.assemble(cfg, ctx)
}

func (r *registry) assemble(cfg *PromptConfig, ctx RuntimeContext) (string, error) {
    // 1. Merge inheritance
    blocks, err := r.mergeInheritance(cfg)
    if err != nil {
        return "", err
    }

    // 2. Inject provider content
    blocks = r.injectProviderContent(blocks, ctx)

    // 3. Evaluate when-expressions; drop blocks that fail
    blocks = r.filterByCondition(blocks, ctx.State)

    // 4. Sort by order, then file order for ties
    sort.SliceStable(blocks, func(i, j int) bool {
        return blocks[i].effectiveOrder < blocks[j].effectiveOrder
    })

    // 5. Interpolate variables in static content
    vars := mergeVariables(cfg, ctx.Variables)
    parts := make([]string, 0, len(blocks))
    for _, b := range blocks {
        content := interpolate(b.Content, vars)
        if strings.TrimSpace(content) == "" {
            continue
        }
        parts = append(parts, content)
    }

    return strings.Join(parts, "\n\n"), nil
}
```

Resolution is *stateless and deterministic*. Same `(cfg, ctx)` always produces the same output. This is required for snapshotting (§5.4) and for golden-file testing.

### 4.5 Error Types

```go
type NotFoundError struct {
    ID string
}
func (e *NotFoundError) Error() string {
    return fmt.Sprintf("prompt not found: %s", e.ID)
}

type ParadigmMismatchError struct {
    ID       string
    Required []string
    Actual   string
}
func (e *ParadigmMismatchError) Error() string {
    return fmt.Sprintf("prompt %s requires paradigm %v, got %s",
        e.ID, e.Required, e.Actual)
}

type ValidationError struct {
    Issues []ValidationIssue
}
func (e *ValidationError) Error() string { /* ... */ }
```

Caller-distinguishable error types let consumers respond appropriately:

- `NotFoundError` → bug or misconfigured workspace; surface as fatal
- `ParadigmMismatchError` → bug; surface with details about which paradigm was expected
- `ValidationError` → registry corruption; restore-from-template

## 5. Runtime Integration

### 5.1 Initialization Sequence

The prompt registry follows the same construction pattern as the capability registry. `ayenitd` constructs the instance and places it on `WorkspaceEnvironment`; named agents register their own providers during `Initialize()`.

**Phase 1 — `ayenitd.Open()`**: build registry, load prompt files.

```go
// In ayenitd — analogous to BuildBuiltinCapabilityBundle.
// Constructs the registry and loads prompt files. Does not register
// any providers — that happens during named-agent initialization.
func BuildPromptRegistry(workspacePath string) (prompt.Registry, error) {
    registry := prompt.NewRegistry()
    promptDir := filepath.Join(workspacePath, "relurpify_cfg", "prompts")
    if err := registry.LoadDir(promptDir); err != nil {
        return nil, fmt.Errorf("load prompts: %w", err)
    }
    // requires_providers validation is deferred until ValidateProviders()
    // is called after all agents have initialized.
    return registry, nil
}
```

`WorkspaceEnvironment` carries the registry:

```go
// In framework/agentenv — field added alongside the existing Registry field.
type WorkspaceEnvironment struct {
    // ...existing fields...
    PromptRegistry prompt.Registry
}

func (e WorkspaceEnvironment) WithPromptRegistry(r prompt.Registry) WorkspaceEnvironment {
    e.PromptRegistry = r
    return e
}
```

**Phase 2 — Named-agent `Initialize()`**: register providers.

Each named agent registers paradigm providers and its own agent-specific providers during `Initialize()`, using `env.PromptRegistry` directly. This mirrors how capability handlers are registered via `relurpicabilities.RegisterAll(env)`.

```go
// named/euclo/agent.go
func (a *Agent) Initialize(config *core.Config) error {
    if a.initialized {
        return nil
    }

    if err := relurpicabilities.RegisterAll(a.env); err != nil {
        return fmt.Errorf("failed to register relurpic capabilities: %w", err)
    }

    if a.env.PromptRegistry != nil {
        // Register providers for the paradigms euclo uses (react, pipeline).
        if err := promptprovider.RegisterAll(a.env.PromptRegistry); err != nil {
            return fmt.Errorf("register paradigm prompt providers: %w", err)
        }
        // Register euclo-specific providers.
        if err := eucloprovider.RegisterAll(a.env.PromptRegistry); err != nil {
            return fmt.Errorf("register euclo prompt providers: %w", err)
        }
    }

    // ...rest of initialization...
    a.initialized = true
    return nil
}
```

**Phase 3 — Post-init provider validation**: after all agents have initialized, `ayenitd` calls `ValidateProviders()` to surface any `requires_providers` mismatches as a single pass.

```go
// In ayenitd, after all agents are initialized.
if issues := workspace.PromptRegistry.ValidateProviders(); len(issues) > 0 {
    for _, issue := range issues {
        log.Printf("prompt registry: %s: %s", issue.Code, issue.Message)
    }
}
```

**`agents/promptprovider.RegisterAll`**:

```go
// agents/promptprovider/register.go
package promptprovider

import "codeburg.org/lexbit/relurpify/framework/prompt"

// RegisterAll registers context providers for all generic execution paradigms.
// Called by each named agent that uses these paradigms during Initialize().
// RegisterAll is idempotent: re-registering a provider that is already present
// is a no-op (the registry returns an error on collision; RegisterAll silently
// skips already-registered names).
func RegisterAll(r prompt.Registry) error {
    providers := []struct {
        name     string
        provider prompt.ContextProvider
    }{
        {"react.tools", &reactToolsProvider{}},
        {"react.scratchpad_format", &reactScratchpadProvider{}},
        {"pipeline.stage_outputs", &pipelineStagesProvider{}},
        // ... other paradigm providers
    }
    for _, p := range providers {
        if err := r.RegisterProvider(p.name, p.provider); err != nil {
            // Skip already-registered providers (expected when multiple agents
            // share paradigms and each calls RegisterAll).
            if isAlreadyRegistered(err) {
                continue
            }
            return err
        }
    }
    return nil
}
```

**`named/euclo/promptprovider.RegisterAll`**:

```go
// named/euclo/promptprovider/register.go
package eucloprovider

import "codeburg.org/lexbit/relurpify/framework/prompt"

func RegisterAll(r prompt.Registry) error {
    providers := []struct {
        name     string
        provider prompt.ContextProvider
    }{
        {"euclo.recipe_step_context", &recipeStepContextProvider{}},
        // ... other euclo-specific providers
    }
    for _, p := range providers {
        if err := r.RegisterProvider(p.name, p.provider); err != nil {
            return err
        }
    }
    return nil
}
```

### 5.2 Provider Implementation Example

A provider reads from the `contextdata.Envelope` via agent-specific state accessors.

```go
// named/euclo/promptprovider/recipe_step_context.go
package eucloprovider

import (
    "fmt"
    "strings"

    "codeburg.org/lexbit/relurpify/framework/prompt"
    "codeburg.org/lexbit/relurpify/named/euclo/state"
)

type recipeStepContextProvider struct{}

func (p *recipeStepContextProvider) Provide(ctx prompt.RuntimeContext) prompt.ContextChunk {
    captures := state.GetCaptures(ctx.Envelope)
    if len(captures) == 0 {
        return prompt.ContextChunk{}
    }
    var b strings.Builder
    b.WriteString("Outputs from prior recipe steps:\n\n")
    for k, v := range captures {
        fmt.Fprintf(&b, "- %s: %v\n", k, v)
    }
    return prompt.ContextChunk{Content: b.String()}
}

func (p *recipeStepContextProvider) Describe() prompt.ProviderMetadata {
    return prompt.ProviderMetadata{
        Name:        "euclo.recipe_step_context",
        Description: "Surfaces captures from prior recipe steps",
        Paradigms:   []string{"react", "pipeline"},
        ReadsKeys:   []string{state.KeyCaptures},
    }
}
```

A `.prompt` file consumes it:

```
# Prior Step Outputs
~ from: provider
~ provider: euclo.recipe_step_context
~ order: late
~ when: euclo.has_captures
```

The `when` clause references `RuntimeContext.State["euclo.has_captures"]`, set to true by the orchestrator when prior captures exist.

### 5.3 Telemetry Integration

The registry emits structured events. Event types:

| Event | When | Data |
|---|---|---|
| `framework.prompt.resolved` | After successful Resolve | id, paradigm, length, blocks_included, blocks_excluded, providers_called, duration_ms, cache_hit |
| `framework.prompt.resolve_failed` | Resolve returns error | id, paradigm, error_kind, error_message |
| `framework.prompt.context_missing` | Provider returns empty ContextChunk for required block | id, block_id, provider_name |
| `framework.prompt.validation_warning` | Load-time warning | id, code, message |
| `framework.prompt.validation_error` | Load-time error | id, code, message |
| `framework.prompt.provider_failed` | FailableProvider returns error | id, block_id, provider_name, error |

The existing `core.Telemetry` interface is scoped to graph-execution tracing. The prompt registry's events are named-event style rather than trace spans. Implement a `PromptTelemetry` sub-interface, analogous to `core.BudgetTelemetry` and `core.CheckpointTelemetry`, rather than overloading `core.Telemetry`:

```go
// In framework/prompt/events.go
type PromptTelemetry interface {
    EmitPromptResolved(e ResolvedEvent)
    EmitPromptResolveFailed(e ResolveFailedEvent)
    EmitPromptContextMissing(e ContextMissingEvent)
    EmitPromptValidationIssue(e ValidationIssueEvent)
    EmitPromptProviderFailed(e ProviderFailedEvent)
}
```

The registry receives a `PromptTelemetry` at construction:

```go
func NewRegistryWithTelemetry(t PromptTelemetry) Registry { ... }
```

Default `NewRegistry()` uses a no-op sink.

### 5.4 Snapshot vs Re-resolve

The system supports long-running sessions with resume semantics. A prompt assembled at step N may be needed again at step N+1 if the session pauses for HITL or other reasons.

**The spec mandates snapshotting at first assembly.** Re-resolve introduces silent drift bugs: prompt files edited mid-session, runtime state mutated, providers returning different content because envelope keys changed. Snapshotting gives consistency at the cost of staleness, and staleness is the more debuggable failure mode.

Implementation:

```go
// At step entry, before invoking the LLM:
func (s *Step) prepareSystemPrompt(ctx context.Context, env *contextdata.Envelope) (string, error) {
    cacheKey := s.promptCacheKey()  // includes step id + content hash of prompt config
    if snapshot, ok := env.GetWorkingValue(cacheKey); ok {
        return snapshot.(string), nil
    }
    rtCtx := buildRuntimeContext(env, s.paradigm, s.id)
    resolved, err := s.promptRegistry.Resolve(s.promptID, rtCtx)
    if err != nil {
        return "", err
    }
    env.SetWorkingValue(cacheKey, resolved, contextdata.MemoryClassTask)
    return resolved, nil
}
```

The cache key includes the step id and a content hash of the **resolved block list after inheritance merge** (so prompt-config edits *between* sessions are detected and re-resolved). Within a session, once a step has assembled, that string is stable until session end.

### 5.5 Failure Handling

Resolution failures are handled per the table:

| Failure | Behavior | Telemetry |
|---|---|---|
| Prompt id not found | Return `NotFoundError` | resolve_failed |
| Paradigm mismatch | Return `ParadigmMismatchError` | resolve_failed |
| Block `when` evaluation error | Exclude block, continue | validation_warning |
| Variable unresolved | Substitute empty string, continue | (debug log) |
| `from: provider` block, provider not registered | Empty content for block, continue | context_missing |
| `FailableProvider.ProvideOrFail` returns error | Empty content for block, continue | provider_failed |
| Inheritance cycle | Resolve fails (caught at load, not resolve) | validation_error |

The general posture: **resolution prefers degraded output to no output**. A missing provider block means the LLM gets a less-informed prompt; a failed resolve means the agent step cannot run. The former is recoverable; the latter cascades into orchestration failure.

The two hard-fail cases (`NotFoundError`, `ParadigmMismatchError`) are programming errors, not runtime conditions. They should never occur in a correctly configured system; hard-failing surfaces the bug fast.

## 6. Security

### 6.1 Threat Model

| Threat | Mitigation |
|---|---|
| User-edited prompt that breaks framework operations | Visible in workspace, restorable via template; framework_critical flag drives tooling |
| Malicious prompt content (prompt injection in prose) | Out of scope; prompt content is treated as trusted authoring |
| `when`-expression code execution | Hand-rolled parser/evaluator; no eval(), no reflection |
| Provider supplying unsanitized content from untrusted sources | Provider's responsibility; the prompt system does not sanitize |
| ID collision shadowing framework prompts | Registry rejects duplicates at load |
| Inheritance bomb (deep chain, exponential blowup) | Depth limit (8); cycle detection |

### 6.2 Expression Evaluator Security

The `when` evaluator is the only computation point in the prompt system. Its security properties:

- **No code paths invoke `reflect`, `unsafe`, or runtime evaluation.** All operations are hand-coded.
- **Bounded execution.** No loops, no recursion through user input. Worst-case complexity is linear in expression length.
- **No side effects.** The evaluator reads from `RuntimeContext.State` only. It does not write, does not call providers, does not invoke I/O.
- **Type-checked operators.** Comparison operators reject incompatible types rather than coercing.

Treat the expression language as a **security boundary**. Any extension to its grammar requires security review. Function calls are forbidden because they would create a path for user-authored prompts to influence runtime behavior beyond block inclusion.

### 6.3 Prompt Authoring Trust Model

Prompts in `relurpify_cfg/prompts/` are trusted by the framework — they are workspace configuration authored by the workspace owner. The system does not protect against malicious prompts (a malicious prompt is a malicious workspace, which is out of scope).

What the system does protect against:

- Accidental corruption of framework-critical prompts (visibility + restore-from-template + lint warnings)
- Cross-namespace shadowing (registry rejects duplicate ids)
- Inheritance corruption (cycle detection, depth limit, locked blocks)

What the system does **not** protect against:

- A workspace owner intentionally modifying `framework.classification.tier2` to bias classification
- Prompt content that attempts to manipulate downstream LLM behavior (prompt injection within prose is not in scope; this is a model-layer concern)

## 7. Validation and Testing

### 7.1 Validation Surfaces

Validation runs in three places:

1. **Load time** (`LoadDir`) — validates structure: schema, inheritance, paradigm tags. Errors → exclude from registry; warnings → include with issues recorded.
2. **Post-init** (`ValidateProviders`) — validates `requires_providers` against the full registered provider set. Called once after all agents have initialized.
3. **Resolve time** — validates paradigm compatibility, runtime variable presence (warns), expression evaluation (warns).

### 7.2 Validation API

```go
// Validate runs full validation on a single prompt id and returns issues.
// Used by linters, CI, and the workspace's validation command.
func (r *registry) Validate(id string) []ValidationIssue { ... }

// ValidateAll runs validation across the entire registry.
// Returns issues keyed by prompt id.
func (r *registry) ValidateAll() map[string][]ValidationIssue { ... }

// ValidateProviders checks requires_providers against the registered provider set.
// Returns all issues found; also records them against the relevant prompt ids.
func (r *registry) ValidateProviders() []ValidationIssue { ... }
```

### 7.3 Golden-File Testing

The registry supports deterministic resolution testing:

```go
result, err := registry.Resolve("agent.euclo.debug.locate_failure", testCtx)
require.NoError(t, err)
goldenfile.Assert(t, "testdata/locate_failure.golden", result)
```

A `goldenfile` package convention checks the resolved string against a checked-in golden file. This catches accidental prompt drift the way snapshot tests catch UI drift.

### 7.4 DryRun for Debug

`ResolveDryRun` returns assembly trace data for debugging:

```go
result, err := registry.ResolveDryRun(id, ctx)
fmt.Println(result.Final)
for _, b := range result.BlocksExcluded {
    fmt.Printf("excluded: %s (%s)\n", b.BlockID, b.Reason)
}
```

This is the data source for the platform-layer "show me the assembled prompt" debug pane.

### 7.5 Mock Registry for Tests

Components that depend on the registry should accept it via interface, not by importing the concrete type. A test helper lives in `framework/prompt/prompttest`:

```go
package prompttest

func New() *MockRegistry { ... }
func (m *MockRegistry) With(id, content string) *MockRegistry { ... }
func (m *MockRegistry) WithProvider(name string, fn func(prompt.RuntimeContext) prompt.ContextChunk) *MockRegistry { ... }
```

## 8. Performance

### 8.1 Expected Costs

Resolution is cheap relative to the LLM call that follows:

| Operation | Expected | Notes |
|---|---|---|
| YAML/front matter parse | < 1 ms per file | Linear in file size |
| Inheritance resolution | < 100 µs | Tree traversal, depth ≤ 8 |
| Condition evaluation | < 10 µs per expression | Hand parser, bounded |
| Variable interpolation | < 50 µs | Linear in content length |
| Provider invocation | provider-dependent | See §8.3 |
| String join | negligible | |

End-to-end resolve: typically < 1 ms for prompts without expensive providers.

### 8.2 Caching Strategy

Two cache layers:

**Static cache** (registry-internal): parsed `PromptConfig` and resolved-inheritance block lists. Cached at load time, never invalidated during process lifetime (the workspace is frozen).

**Snapshot cache** (per-session): the final resolved string. Stored in the envelope per §5.4. Lives for session duration.

The static cache lets the registry compute inheritance once. The snapshot cache lets resolve-many-times-per-session cost amortize to zero after the first call.

### 8.3 Provider Cost

Providers vary in cost. A provider that reads a small envelope key is microseconds. A provider that fetches and formats a large knowledge graph result is milliseconds. The registry does not introspect or limit provider cost; it is the provider author's responsibility to be reasonable.

For expensive providers, the recommended pattern is to compute and cache in the envelope at the responsible upstream point (e.g., recipe step entry), and have the provider read the cached envelope key. Providers should not perform heavy computation per resolve.

### 8.4 Concurrent Resolution

`Resolve` is safe for concurrent invocation. Multiple goroutines may resolve the same or different prompts simultaneously. The registry uses RWMutex for the prompt and provider maps; reads are concurrent.

## 9. Open Questions and Future Work

### 9.1 Deferred for v1

These are intentionally not in scope for v1 but warrant tracking:

- **`RecipeStep.Prompt` migration**: existing recipe steps carry an inline `prompt string` field. The logical completion of this system is recipe steps referencing a prompt id (`prompt_id: agent.euclo.debug.locate_failure`) rather than embedding prose. The transition path — whether to add `prompt_id` alongside `prompt`, repurpose the field, or leave both in parallel — is deferred but must be resolved before prompt file authoring begins, to avoid building two independent prompt systems for the same agent flows.
- **Migration tooling**: `relurpify config migrate` for evolving from one schema version to another. Currently out of scope per P5 (frozen workspaces).
- **Hot reload**: a file watcher that re-loads changed `.prompt` files without process restart. Architecturally compatible — the registry's load path is idempotent — but adds operational complexity.
- **Provider versioning**: providers may evolve their output format. Currently the registry tracks providers by name only. A `provider@version` form may be useful later.
- **Per-block telemetry**: emitting events at block-inclusion granularity. Useful for analytics but high-volume.
- **Cross-prompt lints**: e.g., "two prompts in the same agent both have `kind: persona`, that's almost always a bug."

### 9.2 Compatibility Forward

The schema is `framework.prompt/v1`. Forward compatibility commitments:

- Unknown front-matter fields → warning, not error. Adding fields is non-breaking.
- Unknown block metadata fields → warning, not error.
- Unknown tag values → warning except for `paradigm` (which is constraint-bearing).
- Unknown `apiVersion` → error. A `v2` requires explicit support; the registry will not load `v2` files until the implementation supports them.

When `v2` is introduced, both versions must be supportable concurrently for at least one minor framework release, to allow workspace-level transitions.

## 10. Appendix: Worked Examples

### 10.1 Minimal Standalone Prompt

```
---
apiVersion: framework.prompt/v1
id: agent.simple.example
name: Simple Example
tags:
  paradigm: [react]
  kind: task
  stability: experimental
---

# Task

Summarize the user's request in one sentence.
```

### 10.2 Inheritance + Variables

Parent (`agent.euclo.debug.base.prompt`):

```
---
apiVersion: framework.prompt/v1
id: agent.euclo.debug.base
name: Debug Agent Base
tags:
  paradigm: [react]
  agent: [euclo]
  kind: fragment
  stability: stable

variables:
  language:
    default: "the project language"
---

# Persona

You are a debugging assistant working inside an automated repair pipeline
for {language}. You will be given a specific task. Trust the pipeline,
do your part, hand off cleanly.

# Constraints
~ kind: constraint
~ order: late
~ locked: true

Make no assumptions about code you have not inspected.
Prefer evidence from the codebase over inference.
```

Child (`agent.euclo.debug.locate_failure.prompt`):

```
---
apiVersion: framework.prompt/v1
id: agent.euclo.debug.locate_failure
name: Locate Failure
extends: agent.euclo.debug.base
tags:
  paradigm: [react]
  agent: [euclo]
  domain: [debug]
  kind: task

variables:
  failing_test:
    default: "(unknown)"

requires_providers:
  - react.tools
  - euclo.recipe_step_context
---

# Task

Identify the source of the failing test ({failing_test}) and articulate
the likely cause.

# Available Tools
~ from: provider
~ provider: react.tools

# Prior Step Outputs
~ from: provider
~ provider: euclo.recipe_step_context
~ when: euclo.has_captures
~ order: late

# Output
~ kind: format
~ order: 85

Your response must produce two fields:

- `failure_location`: the file path and line where the failure originates
- `likely_cause`: a short description of the probable root cause
```

Resolved (assuming `failing_test = "TestUserAuth"`, `language = "Go"`, `react.tools` returns `"Available tools: test_run, ast_query"`, `euclo.has_captures = false`):

```
You are a debugging assistant working inside an automated repair pipeline
for Go. You will be given a specific task. Trust the pipeline,
do your part, hand off cleanly.

Identify the source of the failing test (TestUserAuth) and articulate
the likely cause.

Available tools: test_run, ast_query

Make no assumptions about code you have not inspected.
Prefer evidence from the codebase over inference.

Your response must produce two fields:

- `failure_location`: the file path and line where the failure originates
- `likely_cause`: a short description of the probable root cause
```

Assembly order: Persona (10), Task (50, file order), Available Tools (50, file order after Task), Constraints (80, locked from parent), Output (85). Prior Step Outputs excluded because `euclo.has_captures = false`. Use `ResolveDryRun` to inspect actual order applied.

### 10.3 Conditional Block

```
---
apiVersion: framework.prompt/v1
id: agent.euclo.review.with_supabase
name: Review with Database Awareness
extends: agent.euclo.review.base
---

# Database Operations
~ when: supabase.connected && supabase.url exists
~ kind: capability
~ order: 40

You have access to a Supabase database at {supabase.url}.
Use it for querying live data when the review requires runtime context.
```

The block is included only when both `supabase.connected` is truthy and `supabase.url` is defined.

### 10.4 Framework-Critical Prompt

```
---
apiVersion: framework.prompt/v1
id: framework.classification.tier2
name: Tier-2 Classification
description: Selects a capability within the winning family for ambiguous classifications.
framework_critical: true
tags:
  agent: [framework]
  kind: task
  stability: stable

variables:
  family_id:
    default: "(unknown)"
  candidate_capabilities:
    default: "(none provided)"

requires_providers:
  - framework.classification_signals
---

# Task

Select the most appropriate capability from the candidates for this user request.
The classifier has identified family `{family_id}` as the winning domain.

# Candidates
~ kind: capability
~ order: 30

Available capabilities:

{candidate_capabilities}

# Classification Signals
~ from: provider
~ provider: framework.classification_signals
~ order: 50

# Output
~ kind: format
~ order: 80

Respond with the capability id you select, and a one-sentence justification.
Format: `<capability_id>: <justification>`
```

This file lives in the workspace alongside agent prompts. It is editable. The `framework_critical: true` flag triggers tooling protections; the `agent: [framework]` tag identifies it as a framework-internal prompt for filtering purposes.

---

**End of specification.**
