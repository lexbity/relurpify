// Package capabilities provides tool scoping, policy evaluation, and provenance tracking for ChainerAgent.
//
// # Tool Scoping
//
// ScopedLinkStage implements pipeline.ToolScopedStage, enabling per-link tool restrictions:
//   - Link.AllowedTools: whitelist of tools this link can access
//   - Link.RequiredTools: tools this link must have
//   - Empty AllowedTools: access all tools (backward compatible)
//
// # Policy Evaluation
//
// PolicyEvaluator checks whether a link can invoke a specific tool:
//   - Consults framework capability registry for policies
//   - Considers tool trust class and link trust domain
//   - Enforces whitelist/required tool constraints
//   - Returns decision (Allow/Deny/RequireApproval)
//
// # Provenance Tracking
//
// ProvenanceTracker wraps tool results in CapabilityResultEnvelope:
//   - Insertion decision: how to present result (direct/summarized/metadata-only)
//   - Trust metadata: tool trust class, approval binding
//   - Policy snapshot: which policies governed this invocation
//   - Approved by: user/system approval trail
//
// # Integration Flow
//
//  1. LinkStage declares AllowedTools/RequiredTools
//  2. pipeline.Runner queries AllowedToolNames()
//  3. Policy evaluator checks each tool access
//  4. Tool result wrapped in CapabilityResultEnvelope
//  5. Provenance tracked in context and telemetry
package capabilities
