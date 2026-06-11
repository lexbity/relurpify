package knowledge

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

func deterministicChunkID(kind, ref string) ChunkID {
	return ChunkID(fmt.Sprintf("chunk:%s:%s", kind, hashStrings(kind, ref)))
}

func hashStrings(values ...string) string {
	h := sha256.New()
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

func chunkIDsToStrings(ids []ChunkID) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if strings.TrimSpace(string(id)) != "" {
			out = append(out, string(id))
		}
	}
	return out
}
