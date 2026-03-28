// Package archaeographqlserver provides an app-level GraphQL transport over the
// archaeo runtime.
//
// The package keeps transport thin. It delegates reads, mutations, and
// subscriptions to archaeo services and bindings instead of reimplementing
// archaeology lifecycle rules in the server layer.
//
// The current server exposes two kinds of API surface:
//
//   - typed GraphQL inputs for mutations and query arguments
//   - map-shaped GraphQL outputs via the custom Map scalar
//
// The output shape is intentionally close to the underlying archaeology
// payloads. That means nested response keys follow the runtime JSON shape,
// which is currently snake_case rather than GraphQL-style camelCase.
//
// GraphQL is an additional transport, not a replacement for direct in-process
// access through archaeo bindings.
package archaeographqlserver
