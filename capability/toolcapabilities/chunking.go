package toolcapabilities

import (
	"encoding/json"
	"strings"

	capability "codeburg.org/lexbit/relurpify/capability"
	"codeburg.org/lexbit/relurpify/capability/ports"
)

// ChunkToolResult decomposes a tool result into content blocks according to
// the manifest's returns.chunking configuration. When chunking is not
// configured or parsing fails, a single whole-blob block is returned.
//
// The stdout value in result.Data is parsed as the declared returns.type
// (e.g. "json"), then decomposed according to chunking.Mode:
//
//	per_item  → one block per array element at item_path
//	per_field → one block per top-level field
//	whole     → single block (fallback)
func ChunkToolResult(result *ports.ToolResult, returns toolcapabilities.ToolManifestReturns, provenance capability.ContentProvenance) []capability.ContentBlock {
	if result == nil {
		return nil
	}

	chunking := returns.Chunking
	if chunking == nil || chunking.Mode == "" || chunking.Mode == toolcapabilities.ChunkingModeWhole {
		return capability.CapabilityResultBlocks(result, provenance)
	}

	stdout, ok := result.Data["stdout"].(string)
	if !ok || strings.TrimSpace(stdout) == "" {
		return capability.CapabilityResultBlocks(result, provenance)
	}

	switch returns.Type {
	case "json":
		return chunkJSON(stdout, *chunking, result, provenance)
	default:
		return capability.CapabilityResultBlocks(result, provenance)
	}
}

func chunkJSON(stdout string, chunking toolcapabilities.ToolManifestReturnsChunking, result *ports.ToolResult, provenance capability.ContentProvenance) []capability.ContentBlock {
	var parsed any
	if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
		return capability.CapabilityResultBlocks(result, provenance)
	}

	switch chunking.Mode {
	case toolcapabilities.ChunkingModePerItem:
		return chunkJSONPerItem(parsed, chunking, result, provenance)
	case toolcapabilities.ChunkingModePerField:
		return chunkJSONPerField(parsed, chunking, result, provenance)
	default:
		return capability.CapabilityResultBlocks(result, provenance)
	}
}

func chunkJSONPerItem(parsed any, chunking toolcapabilities.ToolManifestReturnsChunking, result *ports.ToolResult, provenance capability.ContentProvenance) []capability.ContentBlock {
	var items []any

	if chunking.ItemPath != "" {
		items = navigatePath(parsed, chunking.ItemPath)
	} else if arr, ok := parsed.([]any); ok {
		items = arr
	} else if m, ok := parsed.(map[string]any); ok {
		for _, key := range []string{"matches", "items", "results", "data"} {
			if val, exists := m[key]; exists {
				if arr, ok := val.([]any); ok {
					items = arr
					break
				}
			}
		}
	}

	if len(items) == 0 {
		return capability.CapabilityResultBlocks(result, provenance)
	}

	blocks := make([]capability.ContentBlock, 0, len(items))
	for _, item := range items {
		itemData := map[string]any{
			"stdout": item,
		}
		if m, ok := item.(map[string]any); ok && len(chunking.RefFields) > 0 {
			for _, field := range chunking.RefFields {
				if val, exists := m[field]; exists {
					itemData["ref_"+field] = val
				}
			}
		}
		blocks = append(blocks, capability.StructuredContentBlock{
			Data:       itemData,
			Provenance: provenance,
		})
	}
	return blocks
}

func chunkJSONPerField(parsed any, chunking toolcapabilities.ToolManifestReturnsChunking, result *ports.ToolResult, provenance capability.ContentProvenance) []capability.ContentBlock {
	m, ok := parsed.(map[string]any)
	if !ok || len(m) == 0 {
		return capability.CapabilityResultBlocks(result, provenance)
	}

	blocks := make([]capability.ContentBlock, 0, len(m))
	for key, val := range m {
		fieldData := map[string]any{
			key: val,
		}
		if len(chunking.RefFields) > 0 {
			for _, field := range chunking.RefFields {
				if v, exists := m[field]; exists {
					fieldData["ref_"+field] = v
				}
			}
		}
		blocks = append(blocks, capability.StructuredContentBlock{
			Data:       fieldData,
			Provenance: provenance,
		})
	}
	return blocks
}

// navigatePath traverses a parsed JSON value along a simple dot-path.
func navigatePath(root any, path string) []any {
	parts := strings.Split(path, ".")
	current := root
	for _, part := range parts {
		part = strings.TrimSuffix(part, "[]")
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		m, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = m[part]
		if current == nil {
			return nil
		}
	}
	if arr, ok := current.([]any); ok {
		return arr
	}
	return nil
}
