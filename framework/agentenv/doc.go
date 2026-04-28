/*
Package agentenv provides the WorkspaceEnvironment struct and related types for
agent runtime configuration.

WorkspaceEnvironment is the canonical runtime environment shared by all agents
in a workspace session. It is produced by ayenitd.Open() and passed directly to
agent constructors. It carries references to framework subsystems including the
capability registry, permission manager, AST index manager, search engine, memory
stores, retriever, compiler, and event infrastructure.

Layering Contract:
- WorkspaceEnvironment is assembled exclusively by the composition root (ayenitd.Open())
- Platform packages MUST NOT import framework/agentenv to construct WorkspaceEnvironment
- Platform packages may receive WorkspaceEnvironment as a dependency via dependency injection
- The only platform imports from framework/agentenv are in verification resolvers, which
  use VerificationPlanRequest (a framework-level type not moved to platform/contracts)

Type Aliases:
The following types are re-exported from platform/contracts for backward compatibility:
- CompatibilitySurface = contracts.CompatibilitySurface
- CompatibilitySurfaceRequest = contracts.CompatibilitySurfaceRequest

The canonical definitions live in platform/contracts. These aliases allow existing
code to continue using the agentenv package without breaking changes.
*/
package agentenv
