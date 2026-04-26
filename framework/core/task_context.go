package core

import "context"

type taskContextKey struct{}

// TaskContext carries the current task metadata through the execution context.
type TaskContext struct {
	ID          string   `json:"id,omitempty" yaml:"id,omitempty"`
	Type        TaskType `json:"type,omitempty" yaml:"type,omitempty"`
	Instruction string   `json:"instruction,omitempty" yaml:"instruction,omitempty"`
}

// WithTaskContext annotates a context with task metadata.
func WithTaskContext(ctx context.Context, task TaskContext) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, taskContextKey{}, task)
}

// TaskContextFrom extracts task metadata from a context.
func TaskContextFrom(ctx context.Context) (TaskContext, bool) {
	if ctx == nil {
		return TaskContext{}, false
	}
	value := ctx.Value(taskContextKey{})
	if value == nil {
		return TaskContext{}, false
	}
	task, ok := value.(TaskContext)
	return task, ok
}
