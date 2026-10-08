package session

import (
	"context"

	"codeburg.org/lexbit/relurpify/model"
	"codeburg.org/lexbit/relurpify/telemetry"
)

type modelTelemetryAdapter struct {
	inner telemetry.Telemetry
}

func newModelTelemetryAdapter(inner telemetry.Telemetry) model.Telemetry {
	if inner == nil {
		return nil
	}
	return modelTelemetryAdapter{inner: inner}
}

func (a modelTelemetryAdapter) Emit(ctx context.Context, event any) {
	if a.inner == nil {
		return
	}
	if ev, ok := event.(telemetry.Event); ok {
		telemetry.StampCorrelation(ctx, &ev)
		a.inner.Emit(ev)
	}
}

var _ model.Telemetry = modelTelemetryAdapter{}
