package compiler

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strings"
)

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func cloneMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func stringValue(v any) string {
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

func toChunkViews(in []llmChunkView) []ChunkView {
	if len(in) == 0 {
		return nil
	}
	out := make([]ChunkView, 0, len(in))
	for _, view := range in {
		if view.Kind == "" {
			continue
		}
		out = append(out, ChunkView{Kind: view.Kind, Data: view.Data})
	}
	return out
}

func mergeChunkViews(existing, refined []ChunkView) []ChunkView {
	if len(refined) == 0 {
		return append([]ChunkView(nil), existing...)
	}
	if len(existing) == 0 {
		return append([]ChunkView(nil), refined...)
	}
	byKind := make(map[ViewKind]int, len(existing))
	out := append([]ChunkView(nil), existing...)
	for i, view := range out {
		byKind[view.Kind] = i
	}
	for _, view := range refined {
		if idx, ok := byKind[view.Kind]; ok {
			out[idx] = view
			continue
		}
		byKind[view.Kind] = len(out)
		out = append(out, view)
	}
	return out
}

func deterministicChunkID(kind, ref string) ChunkID {
	return ChunkID(fmt.Sprintf("chunk:%s:%s", kind, hashStrings(kind, ref)))
}

func hashStrings(values ...string) string {
	h := sha1.New()
	for _, value := range values {
		_, _ = h.Write([]byte(value))
		_, _ = h.Write([]byte{'\n'})
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

func estimateTokens(raw string) int {
	if raw == "" {
		return 0
	}
	return max(1, len(raw)/4)
}

func sameSourceSet(a, b []ProvenanceSource) bool {
	if len(a) != len(b) {
		return false
	}
	counts := make(map[string]int, len(a))
	for _, source := range a {
		counts[source.Kind+"\x00"+source.Ref]++
	}
	for _, source := range b {
		key := source.Kind + "\x00" + source.Ref
		counts[key]--
		if counts[key] < 0 {
			return false
		}
	}
	for _, value := range counts {
		if value != 0 {
			return false
		}
	}
	return true
}
