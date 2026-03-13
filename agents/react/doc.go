// Package react implements the ReAct reasoning runtime used by Relurpify
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
// # Capability insertion
//
// capability_insertion.go injects capability metadata into LLM prompts,
// describing available tools in the format the model expects. prompt_context.go
// assembles the full prompt context for each reasoning step.
package react
