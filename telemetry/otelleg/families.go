package otelleg

import "codeburg.org/lexbit/relurpify/telemetry"

// spanFamily describes one lifecycle family: the point events that open and
// close a span, and the metadata keys that identify a span within the family.
// It is the single definition shared by the log/span routing (slice 2) and the
// span assembler (slice 3), so the two can never drift (spec D2).
type spanFamily struct {
	// name is the family label used in span keys and diagnostics.
	name string
	// startTypes open a span.
	startTypes []telemetry.EventType
	// endTypes close a span (including error and cancel endings).
	endTypes []telemetry.EventType
	// identityKeys are tried in order; the first present metadata value
	// identifies the span within the family.
	identityKeys []string
}

// spanFamilies is the curated lifecycle table of spec D2. It is immutable; the
// //nolint records that it is intentional package state, not accidental.
var spanFamilies = []spanFamily{ //nolint:gochecknoglobals // immutable lifecycle-family table
	{
		name:         familyGraph,
		startTypes:   []telemetry.EventType{telemetry.EventGraphStart},
		endTypes:     []telemetry.EventType{telemetry.EventGraphFinish},
		identityKeys: []string{keyRunID},
	},
	{
		name:         familyNode,
		startTypes:   []telemetry.EventType{telemetry.EventNodeStart},
		endTypes:     []telemetry.EventType{telemetry.EventNodeFinish, telemetry.EventNodeError},
		identityKeys: []string{keyNodeID, keySpanID},
	},
	{
		name:         familyAgent,
		startTypes:   []telemetry.EventType{telemetry.EventAgentStart},
		endTypes:     []telemetry.EventType{telemetry.EventAgentFinish},
		identityKeys: []string{keyRunID, keyAgentID},
	},
	{
		name:         familyDelegation,
		startTypes:   []telemetry.EventType{telemetry.EventDelegationStart},
		endTypes:     []telemetry.EventType{telemetry.EventDelegationFinish, telemetry.EventDelegationCancel},
		identityKeys: []string{keyDelegationID, keyTaskID},
	},
	{
		name:         familyLLM,
		startTypes:   []telemetry.EventType{telemetry.EventLLMPrompt},
		endTypes:     []telemetry.EventType{telemetry.EventLLMResponse},
		identityKeys: []string{keySpanID, keyRequestID},
	},
	{
		name:         familyTool,
		startTypes:   []telemetry.EventType{telemetry.EventToolCall},
		endTypes:     []telemetry.EventType{telemetry.EventToolResult},
		identityKeys: []string{keySpanID, keyCallID},
	},
	{
		name:         familyCapability,
		startTypes:   []telemetry.EventType{telemetry.EventCapabilityCall},
		endTypes:     []telemetry.EventType{telemetry.EventCapabilityResult},
		identityKeys: []string{keySpanID, keyCallID},
	},
}

// lifecycleFamilyFor returns the family that owns a lifecycle event type. The
// boolean is false for a non-lifecycle (point) event, which is a log record.
func lifecycleFamilyFor(et telemetry.EventType) (spanFamily, bool) {
	for _, fam := range spanFamilies {
		if containsEventType(fam.startTypes, et) || containsEventType(fam.endTypes, et) {
			return fam, true
		}
	}
	return spanFamily{}, false
}

// isLifecycleType reports whether et opens or closes a span.
func isLifecycleType(et telemetry.EventType) bool {
	_, ok := lifecycleFamilyFor(et)
	return ok
}

// isLifecycleStart reports whether et opens a span.
func isLifecycleStart(et telemetry.EventType) bool {
	for _, fam := range spanFamilies {
		if containsEventType(fam.startTypes, et) {
			return true
		}
	}
	return false
}

func containsEventType(types []telemetry.EventType, et telemetry.EventType) bool {
	for _, t := range types {
		if t == et {
			return true
		}
	}
	return false
}
