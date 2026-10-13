package otelleg

import (
	"strings"

	"codeburg.org/lexbit/relurpify/telemetry"
)

// spanStatus reports whether a finish event denotes success ("ok") or failure
// ("error"). It is the logic migrated from the deleted telemetry/otel_exporter.go
// (spec §8.1), extended with the two keys the current emitters use but the old
// exporter never saw: the capability result's "capability_error" and the
// graph/node/agent finish "status" field.
func spanStatus(ev telemetry.Event) string {
	if success, ok := ev.Metadata[metaSuccess]; ok {
		if b, ok := success.(bool); ok && b {
			return statusOK
		}
		return statusError
	}
	if _, ok := ev.Metadata[metaError]; ok {
		return statusError
	}
	if _, ok := ev.Metadata[metaToolError]; ok {
		return statusError
	}
	if _, ok := ev.Metadata[metaCapabilityError]; ok {
		return statusError
	}
	if status, ok := ev.Metadata[metaStatus]; ok {
		if s, ok := status.(string); ok && s != "" && !strings.EqualFold(s, statusOK) {
			return statusError
		}
	}
	return statusOK
}
