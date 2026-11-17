package euclotui

import (
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/named/euclo/reporting"
	"codeburg.org/lexbit/relurpify/telemetry"
)

// ExecutionEventFromTelemetry maps a telemetry.Event to an euclotui.ExecutionEvent.
// Returns ok=false for non-euclo and non-edit event types. The mapping handles
// JSON-round-tripped metadata where ints arrive as float64 and string slices
// arrive as []any.
func ExecutionEventFromTelemetry(ev telemetry.Event) (ExecutionEvent, bool) {
	typ := reporting.EventType(ev.Type)
	meta := ev.Metadata

	// Copy the metadata into a fresh map: the broadcast sink delivers the same
	// Event (and its Metadata map) to every subscriber, so mutating it in place
	// would corrupt the payload seen by other subscribers.
	var payload map[string]any
	if len(meta) > 0 {
		payload = make(map[string]any, len(meta)+1)
		for k, v := range meta {
			payload[k] = v
		}
	}

	out := ExecutionEvent{
		Type:   typ,
		TaskID: ev.TaskID,
		Header: reporting.EventHeader{
			TaskID: ev.TaskID,
		},
		Payload: payload,
	}

	if v, ok := meta["step_id"].(string); ok {
		out.StepID = v
	}
	if v := asInt(meta["index"]); v > 0 {
		out.Index = v
	}
	if v := asInt(meta["total"]); v > 0 {
		out.Total = v
	}
	if v, ok := meta["paradigm"].(string); ok {
		out.Paradigm = v
	}
	if v, ok := meta["success"].(bool); ok {
		out.Success = v
	}
	if v := asInt(meta["duration_ms"]); v > 0 {
		out.DurationMs = int64(v)
	}
	if v, ok := meta["recipe_id"].(string); ok {
		out.RecipeID = v
	}
	if v := asStringSlice(meta["skipped_step_ids"]); len(v) > 0 {
		if out.Payload == nil {
			out.Payload = make(map[string]any)
		}
		out.Payload["skipped_step_ids"] = v
	}

	// Check if this is a known euclo lifecycle event.
	switch typ {
	case
		reporting.EventTypeRecipeSelected,
		reporting.EventTypeStepStartedEuclo,
		reporting.EventTypeStepCompletedEuclo,
		reporting.EventTypeBranchResolved,
		reporting.EventTypeIntakeComplete,
		reporting.EventTypeRouteSelected,
		reporting.EventTypeProjectionCompleted,
		reporting.EventTypeFamilySelected,
		reporting.EventTypeIngestionComplete,
		reporting.EventTypeGateResult,
		reporting.EventTypeParallelFanout,
		reporting.EventTypeVerifyStarted,
		reporting.EventTypeVerifyComplete,
		reporting.EventTypeExecutionComplete:
		return out, true
	}
	// NOTE: frame.emitted / frame.resolved are intentionally NOT bridged here.
	// Interaction frames reach the router through the dedicated HandleFrame →
	// ApplyInteractionFrame path (which carries the typed *InteractionFrame).
	// Accepting them here too would double-append chat milestones.

	// Map tool.edited events to PatchHunks.
	if ev.Type == telemetry.EventToolEdited {
		out.PatchHunks = patchHunksFromEditMeta(meta)
		return out, len(out.PatchHunks) > 0
	}

	// Map tool/capability lifecycle events for live tool-call cards.
	switch ev.Type {
	case telemetry.EventToolCall:
		out.Type = reporting.EventType("tool.call")
		out.Payload = toolCallPayload(ev, meta, "tool")
		return out, true
	case telemetry.EventToolResult:
		out.Type = reporting.EventType("tool.result")
		out.Payload = toolResultPayload(ev, meta, "tool")
		if v, ok := meta["success"].(bool); ok {
			out.Success = v
		}
		if v := asInt(meta["duration_ms"]); v > 0 {
			out.DurationMs = int64(v)
		}
		return out, true
	case telemetry.EventCapabilityCall:
		out.Type = reporting.EventType("capability.call")
		out.Payload = toolCallPayload(ev, meta, "capability")
		return out, true
	case telemetry.EventCapabilityResult:
		out.Type = reporting.EventType("capability.result")
		out.Payload = toolResultPayload(ev, meta, "capability")
		if v, ok := meta["success"].(bool); ok {
			out.Success = v
		}
		if v := asInt(meta["duration_ms"]); v > 0 {
			out.DurationMs = int64(v)
		}
		return out, true
	}

	return ExecutionEvent{}, false
}

func toolCallPayload(ev telemetry.Event, meta map[string]any, kind string) map[string]any {
	p := map[string]any{
		"tool_kind": kind,
		"call_id":   toolCallCorrelationID(ev, meta),
	}
	if name := firstNonEmptyString(meta, "tool", "capability", "capability_id", "capability_name", "name"); name != "" {
		p["name"] = name
	}
	if args, ok := meta["args"]; ok {
		p["args"] = args
	}
	if v, ok := meta["span_attrs"].(map[string]any); ok {
		p["span_attrs"] = v
	}
	if raw, ok := meta["raw_args"]; ok {
		p["raw_args"] = raw
	}
	return p
}

func toolResultPayload(ev telemetry.Event, meta map[string]any, kind string) map[string]any {
	p := map[string]any{
		"tool_kind": kind,
		"call_id":   toolCallCorrelationID(ev, meta),
	}
	if name := firstNonEmptyString(meta, "tool", "capability", "capability_id", "capability_name", "name"); name != "" {
		p["name"] = name
	}
	if args, ok := meta["args"]; ok {
		p["args"] = args
	}
	if v, ok := meta["success"].(bool); ok {
		p["success"] = v
	}
	if errText := firstNonEmptyString(meta, "error", "tool_error", "capability_error"); errText != "" {
		p["error"] = errText
	}
	if out, ok := meta["output"]; ok {
		p["output"] = out
	}
	if ref := firstNonEmptyString(meta, "artifact_ref", "stdout_ref"); ref != "" {
		p["output_ref"] = ref
	}
	if v := asInt(meta["exit_code"]); v != 0 {
		p["exit_code"] = v
	}
	if v := asInt(meta["duration_ms"]); v > 0 {
		p["duration_ms"] = v
	}
	if v := asInt(meta["stdout_bytes"]); v > 0 {
		p["stdout_bytes"] = v
	}
	return p
}

func toolCallCorrelationID(ev telemetry.Event, meta map[string]any) string {
	if id := firstNonEmptyString(meta, "call_id", "tool_call_id", "invocation_id"); id != "" {
		return id
	}
	if ev.SpanID != "" {
		return ev.SpanID
	}
	if ev.NodeID != "" {
		return ev.NodeID
	}
	if ev.TraceID != "" && ev.Seq > 0 {
		return fmt.Sprintf("%s:%d", ev.TraceID, ev.Seq)
	}
	return fmt.Sprintf("%s:%d", ev.TaskID, ev.Seq)
}

func firstNonEmptyString(meta map[string]any, keys ...string) string {
	for _, key := range keys {
		if v := asString(meta[key]); v != "" {
			return v
		}
	}
	return ""
}

func patchHunksFromEditMeta(meta map[string]any) []PatchHunk {
	path, _ := meta["path"].(string)
	if path == "" {
		return nil
	}
	hunk := PatchHunk{
		File:   path,
		StepID: asString(meta["step_id"]),
		Origin: asString(meta["origin"]),
	}
	if v := asInt(meta["lines_added"]); v > 0 {
		hunk.LinesAdded = v
	}
	if v := asInt(meta["lines_removed"]); v > 0 {
		hunk.LinesRemoved = v
	}
	if v, ok := meta["preview"].(string); ok && v != "" {
		hunk.Body = v
	}
	return []PatchHunk{hunk}
}

// ExecEventApplier wraps EucloEventRouter with monotonic-seq deduplication.
// Events with Seq ≤ the last applied Seq for the same (Type, StepID) key are
// dropped, preventing duplicate or out-of-order application.
type ExecEventApplier struct {
	router    *EucloEventRouter
	highWater map[execEventKey]uint64
}

type execEventKey struct {
	taskID string
	typeID reporting.EventType
	stepID string
}

// NewExecEventApplier creates an applier backed by the given router.
func NewExecEventApplier(router *EucloEventRouter) *ExecEventApplier {
	return &ExecEventApplier{
		router:    router,
		highWater: make(map[execEventKey]uint64),
	}
}

// Apply translates a telemetry.Event into an ExecutionEvent and applies it
// to the underlying router. Returns the resulting snapshot and whether the
// event was accepted (non-duplicate).
func (a *ExecEventApplier) Apply(ev telemetry.Event) (EucloProjectionSnapshot, bool) {
	if a == nil || a.router == nil {
		return EucloProjectionSnapshot{}, false
	}

	execEv, ok := ExecutionEventFromTelemetry(ev)
	if !ok {
		return EucloProjectionSnapshot{}, false
	}

	// Task identity scopes sequence numbers. A later run may reuse step IDs
	// and restart its event sequence without being mistaken for a stale event.
	if ev.Seq > 0 {
		key := execEventKey{taskID: ev.TaskID, typeID: execEv.Type, stepID: execEv.StepID}
		if last, exists := a.highWater[key]; exists && ev.Seq <= last {
			return a.router.Snapshot(), false
		}
		a.highWater[key] = ev.Seq
	}

	return a.router.ApplyExecutionEvent(execEv), true
}

// Snapshot delegates to the underlying router.
func (a *ExecEventApplier) Snapshot() EucloProjectionSnapshot {
	if a == nil || a.router == nil {
		return EucloProjectionSnapshot{}
	}
	return a.router.Snapshot()
}

// ── coercion helpers ─────────────────────────────────────────────────────────

// asInt coerces a value from JSON-decoded metadata to int.
// Handles float64 (JSON number), int, and int64.
func asInt(v any) int {
	if v == nil {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	default:
		return 0
	}
}

// asStringSlice coerces a value to []string, handling both []string and
// []any (JSON-decoded arrays). Returns nil when v is nil or the wrong type.
func asStringSlice(v any) []string {
	if v == nil {
		return nil
	}
	switch s := v.(type) {
	case []string:
		return s
	case []any:
		out := make([]string, 0, len(s))
		for _, elem := range s {
			if str, ok := elem.(string); ok {
				out = append(out, str)
			} else {
				out = append(out, fmt.Sprint(elem))
			}
		}
		return out
	default:
		return nil
	}
}

// asString safely extracts a string from a metadata value.
func asString(v any) string {
	if v == nil {
		return ""
	}
	s, _ := v.(string)
	return strings.TrimSpace(s)
}
