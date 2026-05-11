// Package prompt provides the workspace prompt registry, parser, and
// resolution primitives for relurpify.
//
// The v2 prompt contract is intentionally small: schema, id, tags, variables,
// and a markdown body. Higher-level orchestration, provider wiring, and other
// workflow concerns are owned outside this package.
//
// This package imports nothing from agents/ or named/.
package prompt
