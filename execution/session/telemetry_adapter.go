package session

import (
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

func (a modelTelemetryAdapter) Emit(event any) {
	if a.inner == nil {
		return
	}
	switch ev := event.(type) {
	case telemetry.Event:
		a.inner.Emit(ev)
	}
}

var _ model.Telemetry = modelTelemetryAdapter{}
