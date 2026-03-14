// Package agents implements the agent types and orchestration capabilities
// for the Relurpify framework.
//
// Framework ownership note:
//
// The framework layer owns manifests, workspace/global config schemas,
// effective contract resolution, skill resolution that affects the sandbox
// envelope, capability admission, and policy compilation.
//
// The agents package owns concrete agent implementations, the runtime-facing
// registry/builder surfaces for those agents, and coordination capabilities.
// Deprecated compatibility wrappers for framework-owned config/contract/skill
// APIs may still exist here temporarily, but new code should use the
// corresponding framework packages directly.
//
// # Agent types
//
//   - CodingAgent: general-purpose coding assistant with five operating modes:
//     Chat (conversational), Refactor, Document, Sandbox (isolated execution),
//     and Analyze (read-only inspection).
//   - ArchitectAgent: plan-then-execute — uses PlannerAgent to generate a
//     multi-step plan, then drives ReActAgent through each step, persisting
//     workflow state for recovery across interruptions.
//   - PipelineAgent: executes a deterministic sequence of typed pipeline
//     stages declared via framework/pipeline contracts.
//   - EternalAgent: long-lived stateful agent designed for persistent
//     background tasks and continuous monitoring loops.
//
// # Orchestration capabilities (relurpic: namespace)
//
// Five built-in capabilities enable agents to coordinate with each other:
//
//   - relurpic:planner.plan — invokes PlannerAgent, returns a structured plan.
//   - relurpic:architect.execute — invokes ArchitectAgent synchronously or
//     in the background.
//   - relurpic:reviewer.review — structured LLM review returning approve +
//     findings list.
//   - relurpic:verifier.verify — returns verified flag + evidence + missing items.
//   - relurpic:executor.invoke — narrows the active capability set to a single
//     non-coordination callable.
//
// # Skill system
//
// Skills are reusable capability bundles declared in SkillManifest YAML files.
// Framework-owned skill resolution and capability admission live under
// framework/skills. The agents package may expose temporary compatibility
// wrappers under agents/skills, but those are not the canonical APIs.
package agents
