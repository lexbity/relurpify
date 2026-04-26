package memory

import "time"

type ArtifactStorageKind string

const (
	ArtifactStorageInline ArtifactStorageKind = "inline"
)

type WorkflowArtifactRecord struct {
	ArtifactID        string              `json:"artifact_id,omitempty" yaml:"artifact_id,omitempty"`
	WorkflowID        string              `json:"workflow_id,omitempty" yaml:"workflow_id,omitempty"`
	RunID             string              `json:"run_id,omitempty" yaml:"run_id,omitempty"`
	Kind              string              `json:"kind,omitempty" yaml:"kind,omitempty"`
	ContentType       string              `json:"content_type,omitempty" yaml:"content_type,omitempty"`
	StorageKind       ArtifactStorageKind `json:"storage_kind,omitempty" yaml:"storage_kind,omitempty"`
	SummaryText       string              `json:"summary_text,omitempty" yaml:"summary_text,omitempty"`
	SummaryMetadata   map[string]any      `json:"summary_metadata,omitempty" yaml:"summary_metadata,omitempty"`
	InlineRawText     string              `json:"inline_raw_text,omitempty" yaml:"inline_raw_text,omitempty"`
	RawSizeBytes      int64               `json:"raw_size_bytes,omitempty" yaml:"raw_size_bytes,omitempty"`
	CompressionMethod string              `json:"compression_method,omitempty" yaml:"compression_method,omitempty"`
	CreatedAt         time.Time           `json:"created_at,omitempty" yaml:"created_at,omitempty"`
}
