// Package tui implements the relurpish host shell.
//
// The package owns the terminal chrome, layout, input routing, overlay stack,
// HITL row presentation, session caching, and generic agent switching logic.
// It does not own agent-specific surface rendering.
//
// Reserved chords are consumed before any surface or region handles a key.
// Region opt-out hooks for surface-owned input/nav never extend to those
// chords.
//
// Guest surfaces are resolved through the stable surface contracts exposed by
// this package. The base-framework control center lives in
// app/relurpish/relurpifyenvtui, and the Euclo surface lives in
// app/relurpish/euclotui.
package tui
