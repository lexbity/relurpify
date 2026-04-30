package euclo

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
)

// CollectPerformancePhases extracts Euclo phase names from the working snapshot.
func CollectPerformancePhases(snapshot *contextdata.Envelope) []string {
	if snapshot == nil {
		return nil
	}
	seen := map[string]struct{}{}
	var phases []string
	appendPhase := func(phase string) {
		phase = strings.TrimSpace(phase)
		if phase == "" {
			return
		}
		if _, ok := seen[phase]; ok {
			return
		}
		seen[phase] = struct{}{}
		phases = append(phases, phase)
	}

	interactionState := toStringAnyMap(workingValue(snapshot, "euclo.interaction_state"))
	for _, phase := range toStringSlice(interactionState["phases_executed"]) {
		appendPhase(phase)
	}
	for _, raw := range toAnySlice(workingValue(snapshot, "euclo.profile_phase_records")) {
		record := toStringAnyMap(raw)
		appendPhase(toString(record["phase"]))
	}
	for _, raw := range toAnySlice(workingValue(snapshot, "euclo.interaction_records")) {
		record := toStringAnyMap(raw)
		appendPhase(toString(record["phase"]))
	}
	return phases
}

// PhaseDurationMS returns the duration for a named Euclo phase.
func PhaseDurationMS(snapshot *contextdata.Envelope, phase string) int64 {
	if snapshot == nil || strings.TrimSpace(phase) == "" {
		return 0
	}
	for _, raw := range toAnySlice(workingValue(snapshot, "euclo.interaction_records")) {
		record := toStringAnyMap(raw)
		if !strings.EqualFold(strings.TrimSpace(toString(record["phase"])), strings.TrimSpace(phase)) {
			continue
		}
		duration := strings.TrimSpace(toString(record["duration"]))
		if duration == "" {
			return 0
		}
		if parsed, err := time.ParseDuration(duration); err == nil {
			return parsed.Milliseconds()
		}
	}
	return 0
}

// WriteInteractionTape serializes Euclo interaction records into a JSONL tape.
func WriteInteractionTape(path string, snapshot map[string]any) error {
	if snapshot == nil {
		return nil
	}
	raw, ok := snapshot["euclo.interaction_records"]
	if !ok || raw == nil {
		return nil
	}
	lines, err := marshalInteractionRecords(raw)
	if err != nil {
		return err
	}
	if len(lines) == 0 {
		return nil
	}
	return os.WriteFile(path, lines, 0o644)
}

func workingValue(snapshot *contextdata.Envelope, key string) any {
	if snapshot == nil {
		return nil
	}
	value, _ := snapshot.GetWorkingValue(key)
	return value
}

func marshalInteractionRecords(raw any) ([]byte, error) {
	records, ok := raw.([]any)
	if !ok {
		if typed, ok := raw.([]map[string]any); ok {
			records = make([]any, 0, len(typed))
			for _, item := range typed {
				records = append(records, item)
			}
		} else {
			return nil, nil
		}
	}
	var out []byte
	for _, record := range records {
		line, err := json.Marshal(record)
		if err != nil {
			return nil, err
		}
		out = append(out, line...)
		out = append(out, '\n')
	}
	return out, nil
}

func toAnySlice(raw any) []any {
	if raw == nil {
		return nil
	}
	switch typed := raw.(type) {
	case []any:
		return typed
	case []map[string]any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, item)
		}
		return out
	case []string:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, item)
		}
		return out
	default:
		data, err := json.Marshal(raw)
		if err != nil {
			return nil
		}
		var out []any
		if err := json.Unmarshal(data, &out); err != nil {
			return nil
		}
		return out
	}
}

func toStringSlice(raw any) []string {
	values := toAnySlice(raw)
	if len(values) == 0 {
		if typed, ok := raw.([]string); ok {
			return append([]string(nil), typed...)
		}
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if s, ok := value.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func toStringAnyMap(raw any) map[string]any {
	if raw == nil {
		return nil
	}
	if typed, ok := raw.(map[string]any); ok {
		return typed
	}
	if typed, ok := raw.(map[string]interface{}); ok {
		return typed
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	return out
}

func toString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}
