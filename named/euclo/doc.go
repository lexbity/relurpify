// Package euclo implements the coding-specialized top-level agent in Relurpify.
//
// Euclo receives user requests (core.Task plus contextdata.Envelope) and resolves
// them to deterministic execution routes before any execution begins. It uses a
// two-tier hybrid classification model:
//   1. Tier-1: Rule-based keyword family scoring (deterministic)
//   2. Tier-2: LLM-backed capability selection within the winning family
//
// Architecture:
//   - Euclo is an agentgraph.WorkflowExecutor
//   - All execution phases are graph nodes with explicit NodeContract
//   - Context enrichment via contextstream.Request from graph nodes
//   - User-controlled ingestion as first-class feature
//   - Thought recipes compile to agentgraph.Graph subgraphs
//   - Policy decisions produced by Euclo's policy layer, enforced by authorization.PermissionManager
//   - Interaction frames durable in envelope and evented via event.Log
//
// Package Structure:
//   - intake/     : Task normalization and two-tier classification
//   - families/   : Keyword family registry
//   - ingestion/  : User-controlled context pipeline
//   - recipes/    : Thought recipe registry and graph compilation
//   - capabilities/: Relurpic capability registry
//   - interaction/: UX-agnostic frame protocol
//   - orchestrate/: Dispatcher and route fork
//   - policy/     : Route policy decisions
//   - reporting/  : Telemetry and outcome classification
//   - state/      : Envelope working-memory keys and accessors
//
// Usage:
//
//	env := agentenv.Open(...)
//	agent := euclo.New(env, euclo.WithConfig(config))
//	if err := agent.Initialize(cfg); err != nil {
//	    return err
//	}
//	result, err := agent.Execute(ctx, task, envelope)
//
// Note: Euclo does not import REFERENCE_ONLY/euclo_broken_legacy or platform/ packages.
// Platform access goes through capability invocations via framework/capability.Registry.
package euclo
