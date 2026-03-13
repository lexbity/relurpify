// Package htn implements a Hierarchical Task Network (HTN) agent.
//
// HTN planning decomposes complex tasks into networks of primitive subtasks
// according to a method library (declared recipes). The language model never
// decides how to structure work — it only executes focused leaf tasks, making
// this pattern maximally small-model-friendly.
//
// Callers register Methods that map TaskType values to ordered SubtaskSpec
// sequences. When HTNAgent.Execute is called, the agent:
//
//  1. Classifies the incoming task type (rule-based first, optional LLM fallback)
//  2. Finds the best-matching Method in the library
//  3. Decomposes into a core.Plan of primitive subtasks
//  4. Executes the plan via graph.PlanExecutor with the configured primitive executor
//
// Default built-in methods cover code generation, modification, review, and
// analysis workflows. Additional methods can be registered at construction time
// or overridden per-agent.
package htn
