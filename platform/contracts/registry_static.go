package contracts

import "sort"

// StaticToolRegistry is a deterministic in-memory registry backed by loaded
// tool manifests.
type StaticToolRegistry struct {
	entries map[string]ToolManifest
	ordered []string
}

// NewStaticToolRegistry builds a registry from loaded manifests.
func NewStaticToolRegistry(manifests []*ToolManifest) *StaticToolRegistry {
	if len(manifests) == 0 {
		return &StaticToolRegistry{entries: map[string]ToolManifest{}}
	}
	entries := make(map[string]ToolManifest, len(manifests))
	for _, manifest := range manifests {
		if manifest == nil {
			continue
		}
		name := NormalizeToolName(manifest.Name)
		if name == "" {
			continue
		}
		entries[name] = *manifest
	}
	ordered := make([]string, 0, len(entries))
	for name := range entries {
		ordered = append(ordered, name)
	}
	sort.Strings(ordered)
	return &StaticToolRegistry{
		entries: entries,
		ordered: ordered,
	}
}

// LookupTool resolves a tool by canonical name.
func (r *StaticToolRegistry) LookupTool(name string) (ToolManifest, bool) {
	if r == nil {
		return ToolManifest{}, false
	}
	manifest, ok := r.entries[NormalizeToolName(name)]
	return manifest, ok
}

// ListTools returns the registry contents in deterministic order.
func (r *StaticToolRegistry) ListTools() []ToolManifest {
	if r == nil {
		return nil
	}
	if len(r.ordered) == 0 {
		return nil
	}
	out := make([]ToolManifest, 0, len(r.ordered))
	for _, name := range r.ordered {
		out = append(out, r.entries[name])
	}
	return out
}
