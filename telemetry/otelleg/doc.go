// Package otelleg is the single OpenTelemetry export leg for relurpify.
//
// It consumes the internal telemetry.Event vocabulary (via telemetry.Telemetry)
// and exports OTel traces, log records, and metrics through exactly two sinks:
// OTLP/HTTP when an endpoint is configured, and an always-available local
// OTLP-JSON segment store. It is the only output besides the in-process
// BroadcastSink.
//
// The package deliberately does not import userconfig: the telemetry domain must
// not depend on configuration decoding. Wiring sites convert the resolved
// userconfig telemetry section into Settings and pass it to New.
package otelleg
