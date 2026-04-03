# Relurpic Capabilities

## Synopsis

Relurpic capability is a framework-level capability classification used for
opinionated execution behavior that is more structured than a single tool call
and more reusable than a named top-level agent.

In the current implementation, relurpic is represented as the capability
runtime family `relurpic`.

This means relurpic capabilities are already part of the framework's canonical
capability model. They are not a separate side system layered on top of tools.

---

## Current Definition

The current framework definition is centered on
`core.CapabilityRuntimeFamilyRelurpic`.

Primary code references:

- `framework/core/capability_types.go`
- `framework/core/agent_spec.go`
- `framework/core/capability_policy_eval.go`
- `framework/capability/capability_registry.go`
- `framework/manifest/skill_manifest.go`
- `framework/skills/skill_policy_resolver.go`
- `framework/authorization/policy_compile.go`

The framework currently treats relurpic as:

- a valid capability runtime family
- a selector target in manifests and skill policy
- an authorization and admission category
- a registry-visible capability class

So the ownership today is:

- `framework/` owns the primitive and policy surface
- `agents/` may provide generic relurpic-backed implementations
- `named/` may define domain-specific relurpic capability sets on top of the
  primitive

---

## What A Relurpic Capability Is

A relurpic capability is a capability whose execution behavior may compose:

- one or more sub-agents
- one or more execution paradigms
- framework skills
- ordinary capabilities, including tools
- workflow-like reasoning or staged fulfillment

This is the reason it belongs in the capability model instead of being treated
as a separate agent-only abstraction.

Compared with other common constructs:

| Construct | Meaning |
|----------|---------|
| Tool capability | Concrete callable unit, usually local or provider-backed |
| Prompt capability | Prompt-shaped capability admitted through the registry |
| Resource capability | Structured data/resource exposure through the registry |
| Skill | Reusable instruction/policy contribution that changes runtime behavior |
| Relurpic capability | Opinionated execution behavior composed from capabilities, skills, and execution routines |

Relurpic capability is therefore not a synonym for:

- tool
- skill
- top-level agent
- workflow graph

It may use any of those internally.

---

## Why Relurpic Is A Runtime Family

The framework distinguishes capability kind from runtime family.

- `CapabilityKind` answers: what sort of callable thing is this?
- `CapabilityRuntimeFamily` answers: what execution/runtime class does it
  belong to?

Relurpic fits the runtime-family axis because its main distinction is not the
shape of the input/output object. Its distinction is the execution model and
policy profile around that capability.

That matters for:

- capability selection
- policy evaluation
- skill policy targeting
- authorization and delegation
- runtime inspection
- security review

The current framework already uses runtime family this way for:

- local tools
- provider-backed capabilities
- relurpic capabilities

---

## Ownership Boundaries

### Framework

`framework/` owns:

- the `relurpic` runtime-family constant and canonical type definition
- selector matching for relurpic capabilities
- manifest and skill-policy targeting
- capability admission and registry treatment
- authorization/delegation policy treatment
- generic runtime plumbing for invoking admitted relurpic capabilities

`framework/` should also own the canonical concept documentation for relurpic
capabilities.

### Agents

`agents/` may own:

- generic relurpic-backed implementations
- generic relurpic helpers and adapters
- generic execution paradigms used by relurpic capabilities

`agents/` should not own the framework primitive itself.

### Named Agents

Packages under `named/` may own specialized relurpic capability families for a
specific top-level agent.

For example:

- Euclo may own coding-specific relurpic capabilities
- another named agent may own domain-specific relurpic capabilities for its own
  runtime

That specialized ownership sits on top of the framework primitive rather than
redefining it.

---

## Relationship To Skills

Skills and relurpic capabilities are related, but they are not the same thing.

Skills usually contribute:

- instructions
- policy overlays
- capability selection constraints
- reusable runtime guidance

Relurpic capabilities usually contribute:

- execution behavior
- orchestration logic
- structured reasoning flow
- composition of paradigms, tools, and sub-agents

A relurpic capability may consume skill policy or require specific skills, but
it is still a capability-level runtime behavior.

---

## Relationship To Tools

Tools are a special case of capability, not the whole capability model.

A relurpic capability may:

- call tools
- call other capabilities
- invoke sub-agents
- run staged reasoning
- coordinate verification and recovery

So relurpic capabilities should be viewed as higher-order capability behavior,
not as a tool alias.

---

## Relationship To Top-Level Agents

A top-level agent and a relurpic capability are different layers.

A top-level agent owns:

- session/runtime identity
- modal behavior
- orchestration state
- continuity and restore guarantees
- final reporting and artifact policy

A relurpic capability owns:

- a reusable opinionated behavior
- the internal execution recipe for that behavior

A top-level agent may select and run relurpic capabilities as part of its
runtime.

For Euclo specifically, this means:

- the framework owns relurpic capability as a primitive
- Euclo should own Euclo-specific relurpic capabilities
- Euclo may compose `/agents` paradigms through those relurpic capabilities

That does not mean Euclo-specific proof contracts belong in `framework/`.
For example:

- Euclo-owned semantic review gating
- Euclo-owned assurance and waiver semantics
- Euclo-owned TDD lifecycle enforcement
- Euclo-owned bounded failed-verification repair

Those are runtime contracts of a named agent built on top of the relurpic
primitive, not framework-level definitions of what relurpic means.

---

## Policy And Manifest Surface

Because relurpic is a runtime family, manifests and skill policy may already
target it through capability selectors.

This allows runtime policy such as:

- allow only relurpic capabilities matching certain IDs
- deny relurpic capabilities by selector
- include relurpic runtime-family capabilities in skill-driven policy
- compile authorization/delegation rules against relurpic capabilities

This is one of the main reasons the concept belongs in `framework/`.

---

## Current Limitations

The framework already defines relurpic as a runtime family, but the concept is
still light on explicit contract documentation compared with tools, manifests,
or authorization.

Current gaps include:

- no dedicated framework package just for relurpic concepts
- no canonical framework-level descriptor fields beyond general capability
  metadata
- no single document previously explaining how relurpic differs from tool or
  skill
- some relurpic behavior ownership still living in `agents/` or named-agent
  code by convention rather than by documented rule

This document establishes the concept, but the runtime-specific contracts for a
particular relurpic capability family should still be documented by the owning
layer.

---

## Euclo Implication

Given the current framework model, Euclo should not redefine what a relurpic
capability is.

Instead:

- the framework continues to own relurpic as a capability runtime family
- Euclo owns the coding-specific relurpic capability families built on that
  primitive
- `/agents` remains the library of reusable execution paradigms and generic
  relurpic helpers where applicable

This is consistent with the current implementation and with the intended
layering rules.

In practice, the current Euclo layering is:

- `framework/` defines relurpic as a capability runtime family and exposes
  policy/planning interfaces
- `/platform` provides backend-specific verification and compatibility helpers
- `named/euclo` defines the coding-specific relurpic capabilities and enforces
  assurance, review, verification, TDD, repair, and reporting contracts

That separation keeps relurpic general-purpose at the framework level while
still allowing named agents to provide strong domain-specific behavior
guarantees.
