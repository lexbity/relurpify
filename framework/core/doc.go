// Package core defines the foundational types shared across every layer of the
// Relurpify framework: agents, tools, capabilities, providers, and the runtime
// state that carries information through a task execution.
//
// # Domain overview
//
// Agent & task — Agent, AgentRuntimeSpec, Task, Plan, and the spec merge/overlay
// system that composes manifest-declared skill configurations at runtime.
//
// Execution state — Context is the mutable state bag threaded through every
// graph node and tool invocation. It holds interactions, tool observations,
// budget signals, and per-scope key/value pairs.
//
// Capabilities — CapabilityDescriptor, CapabilityKind (Tool/Prompt/Resource),
// TrustClass, EffectClass, RiskClass, and InsertionAction model where a
// capability came from and how its output may be used. CapabilityResultEnvelope
// wraps every tool result with provenance, an insertion decision, and a policy
// snapshot so the agent loop can enforce content-security rules.
//
// # Trust model — execution authorization
//
// TrustClass in this package answers: "how much does the runtime trust this
// capability to execute and access resources?" It is an execution-authorization
// concept, not a network or transport concept.
//
//   - BuiltinTrusted: framework-owned capability, implicitly safe.
//   - WorkspaceTrusted: declared in the workspace manifest and approved.
//   - ProviderLocalUntrusted: sourced from a local provider but not workspace-approved.
//   - RemoteDeclared: arrived from a remote node, declared but not yet approved.
//   - RemoteApproved: remote capability that has passed the approval handshake.
//
// Do not conflate TrustClass with event ingress origin (see named/rex/events,
// which uses IngressOrigin to classify where an inbound event came from) or
// with node enrollment trust (see app/nexus, which uses TrustClass on
// NodeEnrollment to record the result of the node pairing handshake — a
// different application of the same type).
//
// Providers — Provider is the common interface for all capability sources
// (builtin, plugin, MCP client/server, agent-runtime, LSP, node-device).
// ProviderPolicy, CapabilityPolicy, and GlobalPolicy form the declarative
// authorization layer evaluated by the capability registry.
//
// LLM — LanguageModel, LLMOptions, LLMResponse, Message, ToolCall, and Tool
// define the model-calling interface implemented by platform/llm.
//
// Permissions & HITL — ToolPermissions, PermissionSet, and ApprovalBinding
// express what file-system, network, and execution rights a tool requires.
// HITLRequest carries human-in-the-loop approval flows.
//
// Sessions & nodes — SessionInfo, NodeInfo, and NodeDescriptor support the
// Nexus gateway model where agents run as addressable nodes in a mesh.
//
// Telemetry & events — Event, EventType, and related types feed the telemetry
// materializer and the shared event log.
package core
