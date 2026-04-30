package core

import "context"

type telemetryContextKey struct{}

// WithTelemetry annotates a context with a telemetry sink.
func WithTelemetry(ctx context.Context, t Telemetry) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, telemetryContextKey{}, t)
}

// TelemetryFromContext extracts a telemetry sink from a context.
func TelemetryFromContext(ctx context.Context) Telemetry {
	if ctx == nil {
		return nil
	}
	value := ctx.Value(telemetryContextKey{})
	if value == nil {
		return nil
	}
	telemetry, _ := value.(Telemetry)
	return telemetry
}
