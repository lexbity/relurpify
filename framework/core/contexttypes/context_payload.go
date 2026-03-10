package core

// ContextFileContent captures explicit file context supplied by callers.
type ContextFileContent struct {
	Path      string `json:"path"`
	Content   string `json:"content"`
	Truncated bool   `json:"truncated"`
}
