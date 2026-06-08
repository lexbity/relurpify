// Package ports holds consumer-defined interfaces that governance/security
// declares for the capability, context, execution, and platform domains to
// satisfy. Governance evaluates policy over these interfaces — it never
// imports the producing domain.
//
// Governance owns the interface contract; the producer satisfies it and the
// composition root wires the concrete implementation.
//
// Defined in later phases:
//
//	DescriptorView  — policy engine reads capability descriptors without importing capability (P6)
//	SandboxRuntime  — sandbox selection without importing platform (P8)
//	StateView       — read-only context state without importing context (P13)
//	SearchScope     — context search without importing context (P13)
//	LifecycleView   — agent lifecycle without importing execution (P14)
//	DelegationSink  — delegation transitions without importing execution (P14)
//	Config          — governance receives config via injection, not import (P15)
package ports
