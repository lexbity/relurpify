package toolcapabilities

import (
	"encoding/json"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/descriptor"
	"codeburg.org/lexbit/relurpify/capability/ports"
	capresult "codeburg.org/lexbit/relurpify/capability/result"
	"github.com/stretchr/testify/require"
)

func TestChunkToolResultNoChunkingReturnsWhole(t *testing.T) {
	result := &ports.ToolResult{
		Success: true,
		Data:    map[string]interface{}{"stdout": `{"key":"value"}`},
	}
	returns := ports.ToolManifestReturns{}
	blocks := ChunkToolResult(result, returns, capresult.ContentProvenance{})
	require.Len(t, blocks, 1)
	_, ok := blocks[0].(capresult.StructuredContentBlock)
	require.True(t, ok)
}

func TestChunkToolResultNonJSONFallback(t *testing.T) {
	result := &ports.ToolResult{
		Success: true,
		Data:    map[string]interface{}{"stdout": "plain text"},
	}
	returns := ports.ToolManifestReturns{
		Type: "json",
		Chunking: &ports.ToolManifestReturnsChunking{
			Mode: ports.ChunkingModePerItem,
		},
	}
	blocks := ChunkToolResult(result, returns, capresult.ContentProvenance{})
	require.Len(t, blocks, 1)
}

func TestChunkPerItemFromArray(t *testing.T) {
	result := &ports.ToolResult{
		Success: true,
		Data:    map[string]interface{}{"stdout": `[{"path":"a.go","line":1},{"path":"b.go","line":2}]`},
	}
	returns := ports.ToolManifestReturns{
		Type: "json",
		Chunking: &ports.ToolManifestReturnsChunking{
			Mode:      ports.ChunkingModePerItem,
			RefFields: []string{"path", "line"},
		},
	}
	blocks := ChunkToolResult(result, returns, capresult.ContentProvenance{})
	require.Len(t, blocks, 2)
	for i, block := range blocks {
		sb, ok := block.(capresult.StructuredContentBlock)
		require.True(t, ok, "block %d should be StructuredContentBlock", i)
		data, ok := sb.Data.(map[string]interface{})
		require.True(t, ok)
		require.Contains(t, data, "stdout")
		require.Contains(t, data, "ref_path")
		require.Contains(t, data, "ref_line")
	}
}

func TestChunkPerItemFromObjectWithMatches(t *testing.T) {
	result := &ports.ToolResult{
		Success: true,
		Data:    map[string]interface{}{"stdout": `{"matches":[{"path":"a.go","line":1},{"path":"b.go","line":2}]}`},
	}
	returns := ports.ToolManifestReturns{
		Type: "json",
		Chunking: &ports.ToolManifestReturnsChunking{
			Mode:      ports.ChunkingModePerItem,
			RefFields: []string{"path"},
		},
	}
	blocks := ChunkToolResult(result, returns, capresult.ContentProvenance{})
	require.Len(t, blocks, 2)
}

func TestChunkPerItemWithItemPath(t *testing.T) {
	result := &ports.ToolResult{
		Success: true,
		Data:    map[string]interface{}{"stdout": `{"data":{"items":[{"id":1},{"id":2},{"id":3}]}}`},
	}
	returns := ports.ToolManifestReturns{
		Type: "json",
		Chunking: &ports.ToolManifestReturnsChunking{
			Mode:     ports.ChunkingModePerItem,
			ItemPath: "data.items[]",
		},
	}
	blocks := ChunkToolResult(result, returns, capresult.ContentProvenance{})
	require.Len(t, blocks, 3)
}

func TestChunkPerField(t *testing.T) {
	result := &ports.ToolResult{
		Success: true,
		Data:    map[string]interface{}{"stdout": `{"name":"test","version":1,"active":true}`},
	}
	returns := ports.ToolManifestReturns{
		Type: "json",
		Chunking: &ports.ToolManifestReturnsChunking{
			Mode: ports.ChunkingModePerField,
		},
	}
	blocks := ChunkToolResult(result, returns, capresult.ContentProvenance{})
	require.Len(t, blocks, 3)
	for _, block := range blocks {
		sb, ok := block.(capresult.StructuredContentBlock)
		require.True(t, ok)
		data, ok := sb.Data.(map[string]interface{})
		require.True(t, ok)
		require.Len(t, data, 1)
	}
}

func TestChunkWholeReturnsSingleBlock(t *testing.T) {
	result := &ports.ToolResult{
		Success: true,
		Data:    map[string]interface{}{"stdout": `[1,2,3]`},
	}
	returns := ports.ToolManifestReturns{
		Type: "json",
		Chunking: &ports.ToolManifestReturnsChunking{
			Mode: ports.ChunkingModeWhole,
		},
	}
	blocks := ChunkToolResult(result, returns, capresult.ContentProvenance{})
	require.Len(t, blocks, 1)
}

func TestChunkMalformedJSONFallback(t *testing.T) {
	result := &ports.ToolResult{
		Success: true,
		Data:    map[string]interface{}{"stdout": `{invalid json`},
	}
	returns := ports.ToolManifestReturns{
		Type: "json",
		Chunking: &ports.ToolManifestReturnsChunking{
			Mode: ports.ChunkingModePerItem,
		},
	}
	blocks := ChunkToolResult(result, returns, capresult.ContentProvenance{})
	require.Len(t, blocks, 1)
}

func TestChunkEmptyStdoutFallback(t *testing.T) {
	result := &ports.ToolResult{
		Success: true,
		Data:    map[string]interface{}{"stdout": ""},
	}
	returns := ports.ToolManifestReturns{
		Type: "json",
		Chunking: &ports.ToolManifestReturnsChunking{
			Mode: ports.ChunkingModePerItem,
		},
	}
	blocks := ChunkToolResult(result, returns, capresult.ContentProvenance{})
	require.Len(t, blocks, 1)
}

func TestChunkResultPreservesErrorBlock(t *testing.T) {
	result := &ports.ToolResult{
		Success: false,
		Error:   "something failed",
		Data:    map[string]interface{}{"stdout": `{"key":"value"}`},
	}
	returns := ports.ToolManifestReturns{
		Type: "json",
		Chunking: &ports.ToolManifestReturnsChunking{
			Mode: ports.ChunkingModePerItem,
		},
	}
	blocks := ChunkToolResult(result, returns, capresult.ContentProvenance{})
	hasError := false
	for _, b := range blocks {
		if _, ok := b.(capresult.ErrorContentBlock); ok {
			hasError = true
			break
		}
	}
	require.True(t, hasError)
}

func TestNavigatePath(t *testing.T) {
	var data any
	err := json.Unmarshal([]byte(`{"results":{"items":[{"x":1},{"x":2}]}}`), &data)
	require.NoError(t, err)
	items := navigatePath(data, "results.items[]")
	require.Len(t, items, 2)
}

func TestNavigatePathMissingReturnsNil(t *testing.T) {
	var data any
	json.Unmarshal([]byte(`{"a":1}`), &data)
	items := navigatePath(data, "missing.path")
	require.Nil(t, items)
}

func TestNewCapabilityResultEnvelopeWithBlocks(t *testing.T) {
	desc := descriptor.CapabilityDescriptor{
		ID:   "test:rg",
		Name: "cli_rg",
	}
	result := &ports.ToolResult{
		Success: true,
		Data:    map[string]interface{}{"stdout": "data"},
	}
	precomputed := []capresult.ContentBlock{
		capresult.StructuredContentBlock{Data: map[string]interface{}{"chunk": 1}},
		capresult.StructuredContentBlock{Data: map[string]interface{}{"chunk": 2}},
	}
	envelope := capresult.NewCapabilityResultEnvelopeWithBlocks(desc, result, capresult.ContentDispositionRaw, nil, nil, precomputed)
	require.NotNil(t, envelope)
	require.Len(t, envelope.ContentBlocks, 2)
}

func TestEnvelopeWithoutBlocksDefaults(t *testing.T) {
	desc := descriptor.CapabilityDescriptor{
		ID:   "test:echo",
		Name: "cli_echo",
	}
	result := &ports.ToolResult{
		Success: true,
		Data:    map[string]interface{}{"stdout": "hello"},
	}
	envelope := capresult.NewCapabilityResultEnvelope(desc, result, capresult.ContentDispositionRaw, nil, nil)
	require.NotNil(t, envelope)
	require.Len(t, envelope.ContentBlocks, 1)
}
