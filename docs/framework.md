# Framework Overview

## Scope

This document summarizes the shared runtime layer that underpins agents,
applications, and platform integrations.

The full package-by-package reference remains under [docs/framework/](framework/README.md).

## Shared Contracts

- `framework/core` owns the shared data model for tasks, manifests, tools,
  backend capabilities, and runtime configuration.
- `framework/capability` owns capability admission and the native vs fallback
  tool-calling decision.
- `framework/manifest` parses and validates agent manifests.
- `framework/pipeline` coordinates model calls with the selected calling mode.
- `framework/retrieval` selects embeddings from the active managed backend or
  provider-configured fallback.

## Backend Integration

The provider-neutral backend contract is split across:

- `framework/core.BackendCapabilities`
- `platform/llm.ManagedBackend`
- `platform/llm.ProviderConfig`

The runtime and TUI consume these types to inspect backend health, available
models, and native tool-calling support without importing a concrete provider
package.

## Where It Is Used

- `app/relurpish/runtime` owns runtime bootstrap, probe, and doctor flows.
- `app/relurpish/tui` renders provider, model, and backend-state metadata.
- `ayenitd` constructs the effective runtime environment and embedder.
- `testsuite/agenttest` uses provider-backed clients for live model testing.
