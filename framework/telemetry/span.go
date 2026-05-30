// Package telemetry provides telemetry sink implementations and span-export
// adapters. The OTel SDK is never imported by platform/contracts (I2);
// adapters live here at the framework level where SDK imports are permitted.
package telemetry

import (
	"fmt"
	"strconv"
	"time"
)

// SpanContext identifies a span within a distributed trace.
type SpanContext struct {
	TraceID string
	SpanID  string
}

// SpanAttributes is a set of key-value pairs describing a span.
type SpanAttributes map[string]string

// SpanEvent captures a completed span for structured telemetry export.
type SpanEvent struct {
	Name       string
	Attributes SpanAttributes
	SpanCtx    SpanContext
	ParentCtx  SpanContext
	Duration   time.Duration
	Status     string
}

// SpanExporter exports completed spans to a backend (OTel, logging, etc.).
type SpanExporter interface {
	ExportSpan(name string, attrs SpanAttributes, spanCtx SpanContext, parentCtx SpanContext, duration time.Duration, status string)
}

// NopExporter is a SpanExporter that discards all spans. Useful as a default
// when no OTel backend is configured — disabling the exporter is a no-op.
type NopExporter struct{}

func (NopExporter) ExportSpan(name string, attrs SpanAttributes, spanCtx SpanContext, parentCtx SpanContext, duration time.Duration, status string) {}

// ToolSpanAttrs builds a SpanAttributes map from tool call metadata.
// Attributes included: tool.name, tool.family, capability.trust_class,
// capability.effect_class, exit_code, stdout_bytes, artifact_ref, elapsed.
// Param values are included only if listed in extraAttrs.
func ToolSpanAttrs(name, family, trustClass string, effectClasses []string, exitCode int, stdoutBytes int64, artifactRef, elapsed string, args map[string]interface{}, extraAttrs []string) SpanAttributes {
	attrs := SpanAttributes{
		"tool.name":   name,
		"tool.family": family,
	}
	if trustClass != "" {
		attrs["capability.trust_class"] = trustClass
	}
	for i, ec := range effectClasses {
		var key string
		if i == 0 {
			key = "capability.effect_class"
		} else {
			key = "capability.effect_class_" + strconv.Itoa(i)
		}
		attrs[key] = ec
	}
	if exitCode != 0 {
		attrs["exit_code"] = strconv.Itoa(exitCode)
	}
	if stdoutBytes > 0 {
		attrs["stdout_bytes"] = strconv.FormatInt(stdoutBytes, 10)
	}
	if artifactRef != "" {
		attrs["artifact_ref"] = artifactRef
	}
	if elapsed != "" {
		attrs["elapsed"] = elapsed
	}
	if len(extraAttrs) > 0 && args != nil {
		allow := make(map[string]struct{}, len(extraAttrs))
		for _, a := range extraAttrs {
			allow[a] = struct{}{}
		}
		for k, v := range args {
			if _, ok := allow[k]; ok {
				attrs["param."+k] = fmt.Sprint(v)
			}
		}
	}
	return attrs
}
