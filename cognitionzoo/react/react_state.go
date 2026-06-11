package react

import (
	"fmt"
	"time"

	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/context/contextdata"
)

// ToolObservation records a single tool execution result within the ReAct loop.
type ToolObservation struct {
	Tool      string         `json:"tool"`
	Phase     string         `json:"phase"`
	Summary   string         `json:"summary"`
	Args      map[string]any `json:"args,omitempty"`
	Data      map[string]any `json:"data,omitempty"`
	Success   bool           `json:"success"`
	Timestamp time.Time      `json:"timestamp"`
}

func activeToolSet(env *contextdata.Envelope) map[string]struct{} {
	out := map[string]struct{}{}
	if env == nil {
		return out
	}
	raw, ok := contextdata.GetTyped[any](env, "react.active_tools")
	if !ok {
		return out
	}
	switch values := raw.(type) {
	case []string:
		for _, value := range values {
			out[value] = struct{}{}
		}
	case []any:
		for _, value := range values {
			out[fmt.Sprint(value)] = struct{}{}
		}
	}
	return out
}

func recordActiveToolNames(env *contextdata.Envelope, tools []ports.Tool) {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name())
	}
	env.SetWorkingValueWithClass("react.active_tools", names, contextdata.MemoryClassTask)
}

func (a *ReActAgent) enforceBudget(env *contextdata.Envelope) {
}

func (a *ReActAgent) recordLatestInteraction(env *contextdata.Envelope) {
}

func (a *ReActAgent) manageContextSignals(env *contextdata.Envelope) {
}
