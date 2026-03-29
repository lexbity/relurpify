// Package capability implements the central capability registry for the Relurpify
// agent framework.
//
// # Overview
//
// The registry maps capability descriptors to their runtime implementations and
// enforces provider policies at the point of dispatch. All code that agents can
// invoke — local tools, prompts, resources, and provider-backed capabilities —
// flows through this package.
//
// # Registry
//
// CapabilityRegistry is the authoritative source for what an agent may call.
// It distinguishes three capability kinds:
//
//   - KindTool (local-native): sandboxed execution via the gVisor runner.
//   - KindPrompt: LLM prompt templates injected into context.
//   - KindResource: structured data resources attached to context.
//
// Capability kind is distinct from capability runtime family. The framework
// currently recognizes runtime families such as:
//
//   - local-tool: local callable tools and tool-like capability execution.
//   - provider: provider-backed capability execution.
//   - relurpic: opinionated higher-order execution behavior composed from
//     capabilities, skills, sub-agents, or multiple execution paradigms.
//
// Relurpic capabilities are therefore part of the canonical capability model,
// not a separate side system layered around tools. They are selected,
// authorized, and admitted through the same registry and policy machinery as
// other capabilities, while carrying a distinct runtime-family classification.
//
// # Policy evaluation
//
// Dispatch is gated by CapabilityPolicy and ProviderPolicy (defined in
// framework/core). The registry evaluates trust class, effect class, and
// provider-level rules before invoking a capability, and wraps every result
// in a CapabilityResultEnvelope carrying provenance and an InsertionDecision.
//
// # Tool formatting
//
// tool_formatting.go converts CapabilityDescriptor schemas to the JSON schema
// format expected by Ollama's tool-calling API.
//
// # Node support
//
// node_support.go wires node-device providers into the registry, enabling
// capabilities served by remote Nexus nodes.
package capability
