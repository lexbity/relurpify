// Package errors defines error types for chainer stage execution.
//
// Error types wrap pipeline execution failures with context about which link
// failed, the cause, and optional diagnostic information (expected schema,
// actual output, retry count).
//
// # Error Types
//
// LinkDecodeError: LLM response could not be parsed (Parse function failed)
// LinkValidationError: Parsed output doesn't match declared schema
// LinkApplyError: Failure writing result to shared context
//
// Each error type implements error interface and optionally wraps the underlying cause.
package errors
