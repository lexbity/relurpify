// Package relurpic defines Euclo-owned relurpic capabilities.
//
// Relurpic capability is a framework primitive through
// core.CapabilityRuntimeFamilyRelurpic. This package does not redefine that
// primitive. Instead, it provides Euclo's coding-specific capability catalog on
// top of the framework abstraction.
//
// The package owns:
//   - stable Euclo capability IDs
//   - descriptor metadata for primary and supporting capability roles
//   - mode-family association
//   - mutability posture
//   - Archaeo association
//   - executor recipe and paradigm mix hints
//   - transition compatibility hints
//
// Euclo runtime selection uses these descriptors to bind a primary capability
// owner and supporting capabilities onto a UnitOfWork. Execution paradigms from
// /agents remain reusable substrate; this package describes the behavior
// assemblies that Euclo orchestrates.
package relurpic
