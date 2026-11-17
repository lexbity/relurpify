package euclotui

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/telemetry"
)

func (p *ChatPane) ObserveTelemetryEvent(ev telemetry.Event) {
	if p == nil || p.ux == nil {
		return
	}
	switch ev.Type {
	case telemetry.EventToolCall, telemetry.EventCapabilityCall:
		p.observeToolStart(ev)
	case telemetry.EventToolResult, telemetry.EventCapabilityResult:
		p.observeToolResult(ev)
	}
}

func (p *ChatPane) observeToolStart(ev telemetry.Event) {
	meta := ev.Metadata
	name := firstMetaString(meta, "tool", "capability", "capability_name", "capability_id", "name")
	if name == "" {
		name = string(ev.Type)
	}
	args := formatToolArgs(meta)
	id := toolEventCorrelationID(ev, meta)
	card := toolCallCard{
		ID:      id,
		Name:    name,
		Args:    args,
		State:   toolRunning,
		Started: ev.Timestamp,
		Anim:    0,
		Target:  1,
	}
	if card.Started.IsZero() {
		card.Started = time.Now()
	}
	p.ux.upsertToolCard(card)
	p.ux.thinkingActive = p.hasStreamingState()
	p.syncUXViewport(false)
}

func (p *ChatPane) observeToolResult(ev telemetry.Event) {
	meta := ev.Metadata
	id := toolEventCorrelationID(ev, meta)
	name := firstMetaString(meta, "tool", "capability", "capability_name", "capability_id", "name")
	args := formatToolArgs(meta)
	errText := firstMetaString(meta, "error", "tool_error", "capability_error")
	output := formatToolOutput(meta)
	success, hasSuccess := metaBool(meta, "success")
	state := toolDone
	if hasSuccess && !success {
		state = toolError
	}
	if errText != "" {
		state = toolError
	}
	started := time.Time{}
	// Try to preserve existing start time.
	if idx := p.findToolCardIndex(id); idx >= 0 {
		existing := p.ux.toolCards[idx]
		started = existing.Started
		if name == "" {
			name = existing.Name
		}
		if args == "" {
			args = existing.Args
		}
	}
	if started.IsZero() {
		if d := metaInt(meta, "duration_ms"); d > 0 {
			started = ev.Timestamp.Add(-time.Duration(d) * time.Millisecond)
		} else {
			started = ev.Timestamp
		}
	}
	ended := ev.Timestamp
	if ended.IsZero() {
		ended = time.Now()
	}
	card := toolCallCard{
		ID:      id,
		Name:    name,
		Args:    args,
		Output:  output,
		Err:     errText,
		State:   state,
		Started: started,
		Ended:   ended,
		Target:  1,
		Anim:    1,
	}
	p.ux.upsertToolCard(card)
	p.syncUXViewport(false)
}

func (p *ChatPane) findToolCardIndex(id string) int {
	if p == nil || p.ux == nil {
		return -1
	}
	for i := range p.ux.toolCards {
		if p.ux.toolCards[i].ID == id {
			return i
		}
	}
	return -1
}

func toolEventCorrelationID(ev telemetry.Event, meta map[string]any) string {
	if id := firstMetaString(meta, "call_id", "tool_call_id", "invocation_id"); id != "" {
		return id
	}
	if ev.SpanID != "" {
		return ev.SpanID
	}
	if ev.NodeID != "" {
		return ev.NodeID + ":" + string(ev.Type)
	}
	if ev.TraceID != "" {
		return fmt.Sprintf("%s:%d", ev.TraceID, ev.Seq)
	}
	return fmt.Sprintf("%s:%d", ev.TaskID, ev.Seq)
}

func formatToolArgs(meta map[string]any) string {
	for _, key := range []string{"args", "raw_args", "input"} {
		if v, ok := meta[key]; ok && v != nil {
			return stringifyToolValue(v)
		}
	}
	// Fallback: show interesting metadata keys
	if cmd := firstMetaString(meta, "command", "cmd"); cmd != "" {
		return cmd
	}
	return ""
}

func formatToolOutput(meta map[string]any) string {
	if out, ok := meta["output"]; ok && out != nil {
		return stringifyToolValue(out)
	}
	parts := []string{}
	if ref := firstMetaString(meta, "artifact_ref", "stdout_ref", "output_ref"); ref != "" {
		parts = append(parts, "ref: "+ref)
	}
	if v := metaInt(meta, "exit_code"); v != 0 || metaHas(meta, "exit_code") {
		parts = append(parts, fmt.Sprintf("exit_code: %d", v))
	}
	if v := metaInt(meta, "stdout_bytes"); v > 0 {
		parts = append(parts, fmt.Sprintf("stdout_bytes: %d", v))
	}
	return strings.Join(parts, "\n")
}

func stringifyToolValue(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case []byte:
		return string(t)
	default:
		b, err := json.MarshalIndent(v, "", "  ")
		if err == nil {
			return string(b)
		}
		return fmt.Sprint(v)
	}
}

func firstMetaString(meta map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := meta[k]; ok {
			switch s := v.(type) {
			case string:
				if strings.TrimSpace(s) != "" {
					return strings.TrimSpace(s)
				}
			}
		}
	}
	return ""
}

func metaBool(meta map[string]any, key string) (bool, bool) {
	v, ok := meta[key]
	if !ok {
		return false, false
	}
	b, ok := v.(bool)
	return b, ok
}

func metaInt(meta map[string]any, key string) int {
	v, ok := meta[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	}
	return 0
}

func metaHas(meta map[string]any, key string) bool {
	_, ok := meta[key]
	return ok
}
