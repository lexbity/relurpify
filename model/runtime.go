package model

import "context"

// ModelBackend is the narrow model backend surface consumed by execution
// and workspace session code. Each backend wraps a LanguageModel with
// lifecycle and debug controls.
type ModelBackend interface {
	Model() LanguageModel
	Close() error
	SetDebugLogging(bool)
}

// EventSink is the narrow, ctx-carrying event emit port consumed by model
// wrappers. Deliberately not telemetry.Telemetry: model must not gain a DAG
// edge to the telemetry domain — providers emit plain events and the
// composition root adapts.
type EventSink interface {
	Emit(ctx context.Context, event any)
}

// ModelFactory wraps a backend model with app-level instrumentation and
// profile handling after workspace telemetry has been assembled.
type ModelFactory func(EventSink, bool) LanguageModel

// ModelProduct bundles a model backend with its factory function.
// The app composition root constructs this product and passes it to
// workspace session construction.
type ModelProduct struct {
	Backend      ModelBackend
	ModelFactory ModelFactory
}
