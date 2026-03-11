// Package pattern implements the core reasoning patterns used by Relurpify
// agents.
//
// # ReAct
//
// The ReAct (Reason-Act-Observe) pattern is the primary reasoning loop. It is
// split across eight files for clarity:
//
//   - react.go: core ReActAgent struct and entry point.
//   - react_think_node.go: LLM thought generation.
//   - react_act_node.go: tool invocation and dispatch.
//   - react_observe_node.go: observation capture and context update.
//   - react_completion.go: detects when the agent has finished.
//   - react_recovery.go: handles tool errors and malformed responses.
//   - react_decision.go: routing logic between think, act, and terminal nodes.
//   - react_messages.go: prompt message construction for each loop iteration.
//
// # Planner
//
// PlannerAgent (planner.go) produces a structured multi-step Plan from an
// instruction. It generates multiple candidate plans, scores them, and returns
// the highest-quality option for execution by ArchitectAgent or the graph
// runtime.
//
// # Reflection
//
// The reflection pattern (reflection.go) wraps any agent with a
// self-critique loop: after producing a result, the agent reviews its own
// output and refines it until it meets a quality threshold or exhausts its
// retry budget.
//
// # Capability insertion
//
// capability_insertion.go injects capability metadata into LLM prompts,
// describing available tools in the format the model expects. prompt_context.go
// assembles the full prompt context for each reasoning step.
package pattern
