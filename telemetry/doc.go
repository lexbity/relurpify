// Package telemetry provides the runtime's internal event vocabulary and its
// in-process bus: structured records of node executions, tool calls, and LLM
// interactions flow through Telemetry.Emit into the sinks wired at
// composition. The only output besides the BroadcastSink (TUI feed) is the
// OpenTelemetry export leg — telemetry/otelleg — which turns the event stream
// into OTel spans, log records, and metrics.
package telemetry
