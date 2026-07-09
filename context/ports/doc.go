// Package ports holds consumer-defined interfaces that context declares for
// the execution domain to satisfy. Context owns the trigger/lifecycle contract;
// execution implements it and the composition root wires the concrete.
//
// Defined in later phases:
//
//	CompilerTrigger  — context/persistence triggers compilation without importing execution (P11)
//
package ports
