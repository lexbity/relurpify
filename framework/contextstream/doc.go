// Package contextstream orchestrates compiler-triggered context streaming.
//
// It does not assemble context itself. Instead, it provides request, result,
// trigger, and job primitives that let agent execution ask the compiler for a
// streamed update and then apply the compiler output back onto an envelope.
package contextstream
