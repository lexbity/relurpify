package knowledge

import "sync"

// ViewRenderer lazily produces a typed view for a chunk.
type ViewRenderer func(KnowledgeChunk) (ChunkView, bool)

// ViewRendererRegistry holds registered lazy view renderers.
type ViewRendererRegistry struct {
	mu        sync.RWMutex
	renderers map[ViewKind]ViewRenderer
}

func (r *ViewRendererRegistry) Register(kind ViewKind, renderer ViewRenderer) {
	if kind == "" || renderer == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.renderers == nil {
		r.renderers = make(map[ViewKind]ViewRenderer)
	}
	r.renderers[kind] = renderer
}

func (r *ViewRendererRegistry) RenderViews(chunk KnowledgeChunk, kinds ...ViewKind) []ChunkView {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.renderers) == 0 || len(kinds) == 0 {
		return nil
	}
	out := make([]ChunkView, 0, len(kinds))
	for _, kind := range kinds {
		renderer, ok := r.renderers[kind]
		if !ok {
			continue
		}
		view, ok := renderer(chunk)
		if ok {
			out = append(out, view)
		}
	}
	return out
}
