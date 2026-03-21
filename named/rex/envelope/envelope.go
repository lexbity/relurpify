package envelope

import (
	"fmt"
	"strings"

	"github.com/lexcodex/relurpify/framework/core"
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

// Normalize builds an Envelope from task and state.
func Normalize(task *core.Task, state *core.Context) Envelope {
	env := Envelope{
		Source:   "task",
		Metadata: map[string]string{},
	}
	if task == nil {
		return env
	}
	env.TaskID = strings.TrimSpace(task.ID)
	env.Instruction = strings.TrimSpace(task.Instruction)
	env.Metadata = cloneStringMap(task.Metadata)
	if task.Context != nil {
		env.Workspace = stringValue(task.Context["workspace"])
		env.ModeHint = stringValue(task.Context["mode_hint"])
		env.ResumedRoute = stringValue(task.Context["rex.route"])
		env.WorkflowID = stringValue(task.Context["workflow_id"])
		env.RunID = stringValue(task.Context["run_id"])
		env.Source = firstNonEmpty(stringValue(task.Context["source"]), env.Source)
		env.EditPermitted = boolValue(task.Context["edit_permitted"]) || boolValue(task.Context["mutation_allowed"])
		env.CapabilitySnapshot = stringSlice(task.Context["capability_snapshot"])
		if env.ModeHint == "" {
			env.ModeHint = stringValue(task.Context["euclo.mode"])
		}
	}
	if state != nil {
		if env.WorkflowID == "" {
			env.WorkflowID = strings.TrimSpace(state.GetString("rex.workflow_id"))
		}
		if env.WorkflowID == "" {
			env.WorkflowID = strings.TrimSpace(state.GetString("euclo.workflow_id"))
		}
		if env.RunID == "" {
			env.RunID = strings.TrimSpace(state.GetString("rex.run_id"))
		}
		if env.ResumedRoute == "" {
			env.ResumedRoute = strings.TrimSpace(state.GetString("rex.route"))
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
