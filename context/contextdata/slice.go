package contextdata

import "time"

// StreamedChunk is one body-carrying entry of the current compiled slice.
type StreamedChunk struct {
	ChunkID       ChunkID
	ContentHash   string
	Body          string // verbatim chunk content, as ranked by the compiler
	TokenEstimate int
	TrustClass    string // agentspec trust class label; "" when unset
	IsSummary     bool   // summary-substituted chunk
}

// StreamedSlice is the current backward-pass delivery: one per envelope. It
// replaces the previous slice only under the epoch guard in
// contextstream.ApplyResult (latest-compilation-wins, D-4).
type StreamedSlice struct {
	RequestID    string
	Epoch        uint64
	CompiledAt   time.Time
	BudgetTokens int
	FinalTokens  int // the compiler's own accounting (authoritative)
	Chunks       []StreamedChunk
	// GapMessages carries the skipped-stale gap messages accompanying the
	// slice (existing stale-gap semantics, rendered as trailing lines).
	GapMessages []string
	CacheHit    bool
}

// SliceStamp is the redacted history entry for one applied slice: bodies are
// transient and never enter snapshots or artifacts (D-11); stamps carry IDs,
// digests, token counts, and epochs.
type SliceStamp struct {
	RequestID   string
	Epoch       uint64
	ChunkIDs    []ChunkID
	ChunkHashes []string
	FinalTokens int
	CacheHit    bool
}

// SetStreamedSlice replaces the envelope's current slice under the envelope
// lock and appends the redacted stamp to the slice history.
func (e *Envelope) SetStreamedSlice(slice *StreamedSlice) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.streamed = slice
	if slice != nil {
		e.streamHistory = append(e.streamHistory, slice.stamp())
	}
}

// StreamedSliceSnapshot returns a copy of the current slice (including
// bodies — the renderer's debugging exception), or nil when none is stored.
func (e *Envelope) StreamedSliceSnapshot() *StreamedSlice {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return copyStreamedSlice(e.streamed)
}

// StreamedSliceStamps returns the redacted history of applied slices: IDs,
// epochs, token counts — never bodies (D-11).
func (e *Envelope) StreamedSliceStamps() []SliceStamp {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]SliceStamp, len(e.streamHistory))
	copy(out, e.streamHistory)
	return out
}

func (s *StreamedSlice) stamp() SliceStamp {
	if s == nil {
		return SliceStamp{}
	}
	stamp := SliceStamp{
		RequestID:   s.RequestID,
		Epoch:       s.Epoch,
		ChunkIDs:    make([]ChunkID, len(s.Chunks)),
		ChunkHashes: make([]string, len(s.Chunks)),
		FinalTokens: s.FinalTokens,
		CacheHit:    s.CacheHit,
	}
	for i, chunk := range s.Chunks {
		stamp.ChunkIDs[i] = chunk.ChunkID
		stamp.ChunkHashes[i] = chunk.ContentHash
	}
	return stamp
}

func copyStreamedSlice(slice *StreamedSlice) *StreamedSlice {
	if slice == nil {
		return nil
	}
	out := *slice
	out.Chunks = make([]StreamedChunk, len(slice.Chunks))
	copy(out.Chunks, slice.Chunks)
	out.GapMessages = make([]string, len(slice.GapMessages))
	copy(out.GapMessages, slice.GapMessages)
	return &out
}
