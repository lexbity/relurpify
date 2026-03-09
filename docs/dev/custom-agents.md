# Custom Agents

## Synopsis

The five built-in agent types cover the common reasoning patterns. When you need a different pattern — a domain-specific orchestration loop, a multi-stage pipeline, or a specialised evaluation process — you can implement the `core.Agent` interface and wire it into the registry.

---

## Why Custom Agents

Built-in agents are general purpose. A custom agent lets you encode domain knowledge directly into the reasoning structure: the graph nodes, branching conditions, context loading strategy, and tool subset can all be tailored to a specific task without relying on the LLM to figure out the workflow from a prompt alone.

This is the extension point for teams embedding Relurpify into a larger workflow.

---

## How It Works

### The Agent Interface

```go
// framework/core
type Agent interface {
    Name() string
    Capabilities() []Capability
    Initialize(ctx context.Context, cfg *Config, capabilities *capability.Registry) error
    Execute(ctx context.Context, task Task, shared *SharedContext) (Result, error)
    Reset() error
}
```

**`Name()`** — unique identifier used for routing and telemetry.

**`Capabilities()`** — declares what this agent can do (e.g. `CapabilityCode`, `CapabilityPlan`, `CapabilityExplain`). Used by the registry to match agent to task when no explicit agent is named.

**`Initialize()`** — called once after the agent is created. Build your graph, set up sub-agents, and configure the capability set the agent will use. Keep this idempotent — it may be called again on reset.

**`Execute()`** — called for each instruction. Receives the task and a shared context, returns a result. This is where your graph runs.

**`Reset()`** — clears session state (memory, context) while keeping the graph and tool configuration intact.

### Relationship to the Graph Runtime

Agents are not required to use the graph runtime — `Execute` can do anything that returns a `Result`. But the graph runtime is the right choice for any multi-step workflow because it gives you telemetry, cycle detection, and HITL support for free.

The idiomatic pattern is: build a `Graph` in `Initialize`, call `graph.Execute(ctx, shared)` in `Execute`.

---

## Minimal Example

```go
package myagent

import (
    "context"
    "github.com/lexcodex/relurpify/framework/core"
    "github.com/lexcodex/relurpify/framework/graph"
    "github.com/lexcodex/relurpify/framework/capability"
)

type SummaryAgent struct {
    graph *graph.Graph
    model core.LanguageModel
}

func (a *SummaryAgent) Name() string { return "summary" }

func (a *SummaryAgent) Capabilities() []core.Capability {
    return []core.Capability{core.CapabilityExplain}
}

func (a *SummaryAgent) Initialize(
    ctx context.Context,
    cfg *core.Config,
    capabilities *capability.Registry,
) error {
    a.model = cfg.Model // injected by the runtime

    g := graph.New("summary-workflow")

    // Single LLM node: read the task, produce a summary
    summarise := g.AddNode(graph.NodeLLM, "summarise", func(ctx context.Context, shared *core.SharedContext) error {
        // The graph runtime calls Ollama and appends the response
        return nil
    })
    done := g.AddNode(graph.NodeTerminal, "done", nil)

    g.AddEdge(summarise, done, nil) // unconditional
    g.SetStart(summarise)

    a.graph = g
    return nil
}

func (a *SummaryAgent) Execute(
    ctx context.Context,
    task core.Task,
    shared *core.SharedContext,
) (core.Result, error) {
    shared.AddMessage(core.MessageUser, task.Instruction)
    return a.graph.Execute(ctx, shared)
}

func (a *SummaryAgent) Reset() error {
    return nil
}
```

---

## Delegating to Sub-Agents

For multi-stage pipelines, build scoped sub-agents inside `Initialize`:

```go
func (a *MyAgent) Initialize(ctx context.Context, cfg *core.Config, capabilities *capability.Registry) error {
    // Planner with read-only tool scope
    plannerTools := capabilities.FilterByTags([]string{"read-only"})
    a.planner = planner.New(cfg, plannerTools)

    // Executor with full scope
    a.executor = react.New(cfg, capabilities)

    return nil
}

func (a *MyAgent) Execute(ctx context.Context, task core.Task, shared *core.SharedContext) (core.Result, error) {
    // Phase 1: plan
    plan, err := a.planner.Execute(ctx, task, shared)
    if err != nil {
        return core.Result{}, err
    }
    // Phase 2: execute each step
    return a.executor.ExecutePlan(ctx, plan, shared)
}
```

This is essentially how `CodingAgent` in `architect` mode works.

---

## Registering a Custom Agent

Add your agent to the registry in `agents/registry.go`:

```go
func (r *Registry) RegisterBuiltins(cfg *core.Config, capabilities *capability.Registry) {
    // ... existing agents ...
    r.Register("summary", func() core.Agent {
        return &myagent.SummaryAgent{}
    })
}
```

Then reference it in a manifest:

```yaml
spec:
    agent:
        implementation: summary
```

---

## Using the Framework as a Library

If you are embedding Relurpify in another Go application rather than extending the `relurpish` binary, import the packages directly:

```go
import (
    "github.com/lexcodex/relurpify/framework/core"
    "github.com/lexcodex/relurpify/framework/graph"
    "github.com/lexcodex/relurpify/framework/runtime"
    "github.com/lexcodex/relurpify/framework/capability"
    "github.com/lexcodex/relurpify/llm"
)
```

The typical setup sequence:

```go
// 1. Load manifest and register agent
registration, err := runtime.RegisterAgent(ctx, runtime.RuntimeConfig{
    ManifestPath: "relurpify_cfg/agents/coding-go.yaml",
    Sandbox:      runtime.DefaultSandboxConfig(),
    BaseFS:       workspaceDir,
})

// 2. Build capability registry
registry := capability.NewRegistry()
registry.UsePermissionManager(registration.ID, registration.Permissions)

// 3. Create LLM client
model := llm.NewClient("http://localhost:11434", "qwen2.5-coder:14b")

// 4. Build and run agent
cfg := &core.Config{Model: model, AgentSpec: registration.Manifest.Spec.Agent}
agent := myagent.New()
agent.Initialize(ctx, cfg, registry)

shared := core.NewSharedContext()
result, err := agent.Execute(ctx, core.Task{Instruction: "..."}, shared)
```

---

## See Also

- [Graph Runtime](graph.md) — the execution engine agents run on
- [Custom Tools](tools.md) — adding new tools for your agent to use
- [Context Budget](context-budget.md) — managing token usage in long-running agents
- [Permission Model](../permission-model.md) — scoping tool access per agent
