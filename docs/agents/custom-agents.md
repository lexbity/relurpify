# Custom Agents

## Synopsis

There are two practical extension paths in the current codebase:

- create or customize manifests in `relurpify_cfg/agents/` when the built-in runtimes already cover the control scheme you want
- register a new named runtime when you need a new top-level orchestration model

If you only need different permissions, prompts, models, or workspace defaults, do not write a new Go agent. Start with a manifest.

That distinction is important because a manifest change is usually cheap to own
and easy to review, while a new runtime becomes a long-term maintenance burden.
Most teams overestimate how often they need a new execution paradigm. In this
repo, the generic runtimes already cover most of the useful orchestration
shapes, so the default assumption should be configuration first, code second.

---

## Start With Manifests

Most customization is manifest-level:

- change the implementation selected by `spec.agent.implementation`
- tune model, prompt, tool policy, and capability selectors
- add workspace-owned manifests under `relurpify_cfg/agents/`

The easiest entry point is:

```bash
go run ./app/dev-agent-cli agents create --name my-agent --description "Custom agent"
```

That scaffolds a manifest in `relurpify_cfg/agents/`. You can then validate and list it with:

```bash
go run ./app/dev-agent-cli agents test my-agent
go run ./app/dev-agent-cli agents list
```

For many teams, that is enough.

Manifest-level customization is the right path whenever you are changing
behaviour within an existing control model. That includes cases such as:

- selecting a different generic runtime
- narrowing or widening admitted capabilities
- changing prompts, model defaults, or verification expectations
- copying a shared manifest into the workspace and making it authoritative

---

## When You Need Code

Write a new Go runtime only when you need a control scheme that existing implementations do not provide. In the current architecture, top-level agent execution is centered on `graph.WorkflowExecutor` rather than the older `core.Agent` examples that may exist in older notes.

The runtime contract is:

```go
type WorkflowExecutor interface {
    Initialize(config *core.Config) error
    Execute(ctx context.Context, task *core.Task, state *core.Context) (*core.Result, error)
    Capabilities() []core.Capability
    BuildGraph(task *core.Task) (*graph.Graph, error)
}
```

Shared runtime dependencies are passed through `agentenv.AgentEnvironment`, which includes the model, capability registry, memory store, config, and indexing/search helpers.

That environment object is the main guardrail against ad hoc runtime wiring.
Using it keeps custom runtimes aligned with the rest of the system, including
policy enforcement, capability scoping, persistence, and retrieval surfaces.

---

## Built-In Routing

Manifest-driven construction ultimately flows through the named-agent factory in
`named/factory/factory.go`.

Current built-in implementation routing includes:

- `coding` -> Euclo
- `react`
- `planner`
- `architect`
- `pipeline`
- `reflection`
- `htn`
- `blackboard`
- `goalcon`
- `rewoo`
- `eternal`
- `rex`

If your use case fits one of those, prefer a manifest over a new runtime.

In other words, do not create a new agent because you want a new persona or a
slightly different prompt. Create a new runtime only when the actual execution
pattern is different enough that existing paradigms cannot express it cleanly.

---

## Registering A Named Runtime

If you do need a new top-level runtime, implement `graph.WorkflowExecutor` and register it with the named-agent factory:

```go
package myagent

import (
    "context"

    "github.com/lexcodex/relurpify/framework/agentenv"
    "github.com/lexcodex/relurpify/framework/core"
    "github.com/lexcodex/relurpify/framework/graph"
    namedfactory "github.com/lexcodex/relurpify/named/factory"
)

type Agent struct {
    env agentenv.AgentEnvironment
}

func New(env agentenv.AgentEnvironment) *Agent {
    return &Agent{env: env}
}

func (a *Agent) Initialize(cfg *core.Config) error { return nil }

func (a *Agent) Capabilities() []core.Capability {
    return []core.Capability{core.CapabilityPlan, core.CapabilityExecute}
}

func (a *Agent) BuildGraph(task *core.Task) (*graph.Graph, error) {
    return nil, nil
}

func (a *Agent) Execute(ctx context.Context, task *core.Task, state *core.Context) (*core.Result, error) {
    return &core.Result{Success: true}, nil
}

func init() {
    namedfactory.RegisterNamedAgent("myagent", func(workspace string, env agentenv.AgentEnvironment) graph.WorkflowExecutor {
        return New(env)
    })
}
```

Then select it from a manifest:

```yaml
spec:
  agent:
    implementation: myagent
```

If you want it to appear as a workspace-owned preset, create a manifest in `relurpify_cfg/agents/myagent.yaml`.

The code registration step and the manifest step solve different problems. The
Go registration makes the runtime constructible. The manifest makes it
selectable and configurable in a workspace. You usually need both if you want
other users or tools to consume the runtime in a standard way.

---

## Practical Guidance

- Put new top-level runtimes under `named/` when they own their own orchestration model.
- Reuse `agentenv.AgentEnvironment` rather than creating ad hoc dependency wiring.
- Use the capability registry you are given; do not bypass manifest and policy enforcement.
- Prefer narrowing or cloning the registry when you need read-only or phase-specific scopes.
- Keep manifest examples aligned with the runtime you actually register.

If your design is mostly a different composition of existing paradigms, check whether it belongs as a new named runtime, a manifest preset, or a skill before adding another agent package.

The practical bar should be fairly high. A new runtime is justified when it
introduces a new execution contract, a new state model, or a new coordination
strategy that cannot be expressed as a composition of existing ones without
becoming fragile or misleading.

---
