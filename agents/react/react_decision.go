package react

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// decisionPayload models the JSON output of the think step.
type decisionPayload struct {
	Thought   string                 `json:"thought"`
	Action    string                 `json:"action"`
	Tool      string                 `json:"tool"`
	Arguments map[string]interface{} `json:"arguments"`
	Complete  bool                   `json:"complete"`
	Reason    string                 `json:"reason"`
	Summary   string                 `json:"summary"`
	Timestamp time.Time              `json:"timestamp"`
}

// parseDecision extracts the model's JSON payload (or falls back to the raw
// text) and normalizes it into the decisionPayload struct.
func parseDecision(raw string) (decisionPayload, error) {
	var payload decisionPayload
	snippet := ExtractJSON(raw)
	if snippet == "{}" {
		payload.Thought = strings.TrimSpace(raw)
		payload.Complete = true
		payload.Timestamp = time.Now().UTC()
		return payload, nil
	}
	var generic map[string]interface{}
	if err := json.Unmarshal([]byte(snippet), &generic); err != nil {
		return payload, err
	}
	if thought, ok := generic["thought"].(string); ok && thought != "" {
		payload.Thought = thought
	} else if payload.Thought == "" {
		payload.Thought = strings.TrimSpace(raw)
	}
	if tool, ok := generic["tool"].(string); ok {
		payload.Tool = tool
	} else if name, ok := generic["name"].(string); ok {
		payload.Tool = name
	}
	if action, ok := generic["action"].(string); ok {
		payload.Action = action
	}
	if args, ok := generic["arguments"]; ok {
		payload.Arguments = normalizeArguments(args)
	}
	if payload.Arguments == nil {
		payload.Arguments = map[string]interface{}{}
	}
	if complete, ok := generic["complete"].(bool); ok {
		payload.Complete = complete
	}
	if reason, ok := generic["reason"].(string); ok {
		payload.Reason = reason
	}
	if summary, ok := generic["summary"].(string); ok {
		payload.Summary = summary
	}
	action := strings.ToLower(strings.TrimSpace(payload.Action))
	switch {
	case action == "complete":
		payload.Complete = true
	case action == "tool" && payload.Tool != "":
		payload.Complete = false
	case strings.Contains(action, "tool") && payload.Tool != "":
		payload.Action = "tool"
		payload.Complete = false
	case strings.Contains(action, "complete") && payload.Tool == "":
		payload.Action = "complete"
		payload.Complete = true
	}
	payload.Timestamp = time.Now().UTC()
	return payload, nil
}

// normalizeArguments coerces stringified JSON arguments into maps so tools
// always receive structured input.
func normalizeArguments(value interface{}) map[string]interface{} {
	switch v := value.(type) {
	case map[string]interface{}:
		return v
	case string:
		var obj map[string]interface{}
		if err := json.Unmarshal([]byte(v), &obj); err == nil {
			return obj
		}
		return map[string]interface{}{"value": v}
	default:
		return map[string]interface{}{}
	}
}

// parseError converts an error message string into an error value.
func parseError(err string) error {
	if err == "" {
		return nil
	}
	return errors.New(err)
}
