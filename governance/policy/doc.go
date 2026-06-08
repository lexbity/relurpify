// Package policy evaluates capability policy — what an agent is permitted to
// do, under what conditions, and with what procedural safeguards.
//
// The package owns the policy vocabulary consumed by governance, capability,
// execution, and named-agent code. Higher-level domains import these value
// types rather than duplicating policy enums locally.
package policy
