package contextdata

import (
	"time"
)

// ChunkID is a stable artifact identifier for knowledge chunks.
// Defined locally to avoid import cycle with framework/knowledge.
type ChunkID string

// ReferenceType identifies the tier a reference belongs to.
type ReferenceType string

const (
	// RefTypeStreamedContext references compiled chunks in the streamed context.
	// These are read-only to graph nodes.
	RefTypeStreamedContext ReferenceType = "streamed"

	// RefTypeWorkingMemory references entries in mutable working memory.
	// Graph nodes may read and write these.
	RefTypeWorkingMemory ReferenceType = "working"

	// RefTypeRetrieval references results from scatter-gather retrieval operations.
	// These are controlled external read paths.
	RefTypeRetrieval ReferenceType = "retrieval"

	// RefTypeCheckpoint references materialized checkpoint state.
	// These represent recovery points requested by nodes.
	RefTypeCheckpoint ReferenceType = "checkpoint"
)

// ChunkReference identifies a knowledge chunk in the streamed context.
// This is a read-only reference used by the compiler assembly path.
type ChunkReference struct {
	ChunkID       ChunkID
	Source        string  // e.g., ranker name that contributed this
	Rank          int     // position in ranked list
	IsSummary     bool    // true if this is a summary substitution
	OriginalChunk ChunkID // if IsSummary, the chunk this summarizes
	TokenCount    int
	RetrievedAt   time.Time
}

// WorkingMemoryReference identifies a working memory entry.
// These references are mutable and scoped by task ID.
type WorkingMemoryReference struct {
	TaskID    string
	Key       string
	Class     MemoryClass
	CreatedAt time.Time
	UpdatedAt time.Time
	// ValueHash is a content hash for detecting changes without holding the value
	ValueHash string
}

// MemoryClass categorizes working memory entries.
type MemoryClass string

const (
	// MemoryClassEphemeral expires at the next checkpoint boundary.
	MemoryClassEphemeral MemoryClass = "ephemeral"

	// MemoryClassSession expires when the session ends.
	MemoryClassSession MemoryClass = "session"

	// MemoryClassTask expires when the task completes.
	MemoryClassTask MemoryClass = "task"
)

// RetrievalReference identifies a scatter-gather retrieval result.
// These are created when graph nodes trigger retrieval operations.
type RetrievalReference struct {
	QueryID     string
	QueryText   string
	Scope       string
	ChunkIDs    []ChunkID
	TotalFound  int
	FilteredOut int
	RetrievedAt time.Time
	Duration    time.Duration
}

// CheckpointReference identifies a materialized checkpoint.
// Checkpoints are requested by nodes but owned by the compiler.
type CheckpointReference struct {
	CheckpointID string
	SequenceNum  uint64
	RequestedBy  string // node ID that requested the checkpoint
	CreatedAt    time.Time
	// WorkingMemoryKeys are the working memory keys captured at this checkpoint
	WorkingMemoryKeys []string
}

// ReferenceBundle groups all references in an envelope for clone/merge operations.
type ReferenceBundle struct {
	StreamedContext []ChunkReference
	WorkingMemory   []WorkingMemoryReference
	Retrieval       []RetrievalReference
	Checkpoints     []CheckpointReference
}

// IsEmpty returns true if the bundle contains no references.
func (b *ReferenceBundle) IsEmpty() bool {
	if b == nil {
		return true
	}
	return len(b.StreamedContext) == 0 &&
		len(b.WorkingMemory) == 0 &&
		len(b.Retrieval) == 0 &&
		len(b.Checkpoints) == 0
}

// Clone creates a deep copy of the reference bundle.
func (b *ReferenceBundle) Clone() ReferenceBundle {
	if b == nil {
		return ReferenceBundle{}
	}

	out := ReferenceBundle{
		StreamedContext: make([]ChunkReference, len(b.StreamedContext)),
		WorkingMemory:   make([]WorkingMemoryReference, len(b.WorkingMemory)),
		Retrieval:       make([]RetrievalReference, len(b.Retrieval)),
		Checkpoints:     make([]CheckpointReference, len(b.Checkpoints)),
	}

	copy(out.StreamedContext, b.StreamedContext)
	copy(out.WorkingMemory, b.WorkingMemory)
	copy(out.Checkpoints, b.Checkpoints)

	// Deep copy retrieval references which contain slice fields
	for i, ref := range b.Retrieval {
		out.Retrieval[i] = RetrievalReference{
			QueryID:     ref.QueryID,
			QueryText:   ref.QueryText,
			Scope:       ref.Scope,
			ChunkIDs:    append([]ChunkID(nil), ref.ChunkIDs...),
			TotalFound:  ref.TotalFound,
			FilteredOut: ref.FilteredOut,
			RetrievedAt: ref.RetrievedAt,
			Duration:    ref.Duration,
		}
	}

	return out
}

// HasWorkingMemoryKey returns true if the bundle contains a working memory reference
// for the given task ID and key.
func (b *ReferenceBundle) HasWorkingMemoryKey(taskID, key string) bool {
	if b == nil {
		return false
	}
	for _, ref := range b.WorkingMemory {
		if ref.TaskID == taskID && ref.Key == key {
			return true
		}
	}
	return false
}

// GetWorkingMemoryRef returns the working memory reference for the given task ID and key.
func (b *ReferenceBundle) GetWorkingMemoryRef(taskID, key string) (WorkingMemoryReference, bool) {
	if b == nil {
		return WorkingMemoryReference{}, false
	}
	for _, ref := range b.WorkingMemory {
		if ref.TaskID == taskID && ref.Key == key {
			return ref, true
		}
	}
	return WorkingMemoryReference{}, false
}
