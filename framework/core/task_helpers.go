package core

import "fmt"

// CloneTask returns a shallow copy of task with cloned Context and Metadata maps.
func CloneTask(task *Task) *Task {
	if task == nil {
		return nil
	}
	clone := *task
	if task.Context != nil {
		clone.Context = make(map[string]any, len(task.Context))
		for k, v := range task.Context {
			clone.Context[k] = v
		}
	}
	if task.Metadata != nil {
		clone.Metadata = make(map[string]string, len(task.Metadata))
		for k, v := range task.Metadata {
			clone.Metadata[k] = v
		}
	}
	return &clone
}

// StringSliceFromContext decodes a state entry into a copied string slice.
func StringSliceFromContext(state *Context, key string) []string {
	if state == nil {
		return nil
	}
	raw, ok := state.Get(key)
	if !ok || raw == nil {
		return nil
	}
	switch values := raw.(type) {
	case []string:
		return append([]string{}, values...)
	case []any:
		out := make([]string, 0, len(values))
		for _, value := range values {
			if value == nil {
				continue
			}
			out = append(out, fmt.Sprint(value))
		}
		return out
	default:
		return nil
	}
}
