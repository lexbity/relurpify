package core

type ContextReferenceKind string

const (
	ContextReferenceRetrievalEvidence ContextReferenceKind = "retrieval_evidence"
	ContextReferenceRuntimeMemory     ContextReferenceKind = "runtime_memory"
	ContextReferenceFile              ContextReferenceKind = "file"
)

type ContextReference struct {
	Kind     ContextReferenceKind `json:"kind,omitempty" yaml:"kind,omitempty"`
	ID       string               `json:"id,omitempty" yaml:"id,omitempty"`
	URI      string               `json:"uri,omitempty" yaml:"uri,omitempty"`
	Version  string               `json:"version,omitempty" yaml:"version,omitempty"`
	Detail   string               `json:"detail,omitempty" yaml:"detail,omitempty"`
	Metadata map[string]string    `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}
