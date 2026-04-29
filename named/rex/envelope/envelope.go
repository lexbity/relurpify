package envelope

import (
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/rex/rexkeys"
)

// Envelope is rex's normalized intake shape.
type Envelope struct {
	TaskID             string
	Instruction        string
	Workspace          string
	ModeHint           string
	ResumedRoute       string
	EditPermitted      bool
	WorkflowID         string
	RunID              string
	Source             string
	CapabilitySnapshot []string
	Metadata           map[string]string
}

// Normalize builds an Envelope from task and framework envelope.
func Normalize(task *core.Task, envelope *contextdata.Envelope) Envelope {
	env := Envelope{
		Source:   "task",
		Metadata: map[string]string{},
	}
	if task == nil {
		return env
	}
	env.TaskID = strings.TrimSpace(task.ID)
	env.Instruction = strings.TrimSpace(task.Instruction)
	env.Metadata = mapStringString(task.Metadata)
	if task.Context != nil {
		env.Workspace = stringValue(task.Context["workspace"])
		env.ModeHint = stringValue(task.Context["mode_hint"])
		env.ResumedRoute = stringValue(task.Context["rex.route"])
		env.WorkflowID = stringValue(task.Context[rexkeys.WorkflowID])
		env.RunID = stringValue(task.Context[rexkeys.RunID])
		env.Source = firstNonEmpty(stringValue(task.Context["source"]), env.Source)
		env.EditPermitted = boolValue(task.Context["edit_permitted"]) || boolValue(task.Context["mutation_allowed"])
		env.CapabilitySnapshot = stringSlice(task.Context["capability_snapshot"])
	}
	if envelope != nil {
		if env.WorkflowID == "" {
			if val, ok := envelope.GetWorkingValue(rexkeys.RexWorkflowID); ok {
				env.WorkflowID = strings.TrimSpace(fmt.Sprint(val))
			}
		}
		if env.RunID == "" {
			if val, ok := envelope.GetWorkingValue(rexkeys.RexRunID); ok {
				env.RunID = strings.TrimSpace(fmt.Sprint(val))
			}
		}
		if env.ResumedRoute == "" {
			if val, ok := envelope.GetWorkingValue("rex.route"); ok {
				env.ResumedRoute = strings.TrimSpace(fmt.Sprint(val))
			}
		}
	}
	if env.TaskID == "" {
		env.TaskID = fmt.Sprintf("rex-task-%d", len(env.Instruction))
	}
	return env
}

func stringValue(raw any) string {
	if raw == nil {
		return ""
	}
	value := strings.TrimSpace(fmt.Sprint(raw))
	if value == "<nil>" {
		return ""
	}
	return value
}

func boolValue(raw any) bool {
	if value, ok := raw.(bool); ok {
		return value
	}
	return false
}

func stringSlice(raw any) []string {
	switch typed := raw.(type) {
	case []string:
		return append([]string{}, typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if value := strings.TrimSpace(fmt.Sprint(item)); value != "" {
				out = append(out, value)
			}
		}
		return out
	default:
		return nil
	}
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func mapStringString(in map[string]any) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		if s := fmt.Sprint(v); v != nil {
			out[k] = strings.TrimSpace(s)
		}
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
