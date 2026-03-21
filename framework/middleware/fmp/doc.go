// Package fmp defines the federated mesh protocol surface for resumable work.
//
// The package owns protocol mechanics rather than tenant administration:
// lineage and attempt records, leases and fencing, resumable context
// packaging, runtime endpoint contracts, export descriptors, and orchestration
// helpers for secure handoff flows. Nexus remains the tenant-aware control
// plane and supplies identity, session, and policy data through adapter
// interfaces defined here.
//
// Phase 1 of the spec-gap closure plan freezes the current FMP interface
// boundary so later phases can harden implementations without reopening the
// package split. The current migration boundary is:
//
//   - ContextPackager
//   - RuntimeEndpoint
//   - OwnershipStore
//   - DiscoveryStore
//   - TrustBundleStore
//   - BoundaryPolicyStore
//   - GatewayForwarder
//   - CapabilityProjector
//   - NexusAdapter
//
// These interfaces are intentionally narrow and should remain stable unless a
// later phase explicitly requires expansion.
package fmp
