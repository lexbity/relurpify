package contextdata

// AddStreamedContextReference adds a streamed context chunk reference.
// This is primarily called by the compiler during context assembly.
func (e *Envelope) AddStreamedContextReference(ref ChunkReference) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.References.StreamedContext = append(e.References.StreamedContext, ref)
}

// StreamedChunkIDs returns the IDs of all chunks in the streamed context.
// This is read-only data assembled by the compiler.
func (e *Envelope) StreamedChunkIDs() []ChunkID {
	e.mu.RLock()
	defer e.mu.RUnlock()

	ids := make([]ChunkID, len(e.References.StreamedContext))
	for i, ref := range e.References.StreamedContext {
		ids[i] = ref.ChunkID
	}
	return ids
}
