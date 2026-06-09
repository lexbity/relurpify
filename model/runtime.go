package model

import "codeburg.org/lexbit/relurpify/telemetry"

// ModelBackend is the narrow model backend surface consumed by execution
// and workspace session code. Each backend wraps a LanguageModel with
// lifecycle and debug controls.
type ModelBackend interface {
	Model() LanguageModel
	Close() error
	SetDebugLogging(bool)
}

// ModelFactory wraps a backend model with app-level instrumentation and
// profile handling after workspace telemetry has been assembled.
type ModelFactory func(telemetry.Telemetry, bool) LanguageModel

// ModelProduct bundles a model backend with its factory function.
// The app composition root constructs this product and passes it to
// workspace session construction.
type ModelProduct struct {
	Backend      ModelBackend
	ModelFactory ModelFactory
}
