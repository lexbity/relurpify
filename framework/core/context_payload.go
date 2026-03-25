package core

// ContextFileContent captures explicit file context supplied by callers.
type ContextFileContent struct {
	Path      string            `json:"path"`
	Content   string            `json:"content"`
	Summary   string            `json:"summary,omitempty"`
	Reference *ContextReference `json:"reference,omitempty"`
	Truncated bool              `json:"truncated"`
}
