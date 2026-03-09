package mapping

import (
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/mcp/protocol"
	"github.com/stretchr/testify/require"
)

func TestExportedResourceUsesAnnotationURI(t *testing.T) {
	resource := ExportedResource(core.CapabilityDescriptor{
		ID:   "resource:docs",
		Name: "docs",
		Annotations: map[string]any{
			"mcp_uri":   "file:///docs/guide.md",
			"mime_type": "text/markdown",
		},
	})
	require.Equal(t, "file:///docs/guide.md", resource.URI)
	require.Equal(t, "text/markdown", resource.MIMEType)
}

func TestPromptResultFromAnnotationInterpolatesArguments(t *testing.T) {
	result, ok := PromptResultFromAnnotation(core.CapabilityDescriptor{
		Annotations: map[string]any{
			"prompt_text": "Hello {name}",
		},
	}, map[string]any{"name": "Lex"})
	require.True(t, ok)
	require.Equal(t, "Hello Lex", result.Messages[0].Text)
}

func TestPromptResultFromCorePreservesTextBlocks(t *testing.T) {
	result := PromptResultFromCore(&core.PromptRenderResult{
		Description: "summary",
		Messages: []core.PromptMessage{{
			Content: []core.ContentBlock{core.TextContentBlock{Text: "hello"}},
		}},
	})
	require.Equal(t, "summary", result.Description)
	require.Len(t, result.Messages, 1)
	require.Equal(t, "hello", result.Messages[0].Text)
}

func TestResourceResultFromCorePreservesTextBlocks(t *testing.T) {
	result := ResourceResultFromCore(&core.ResourceReadResult{
		Contents: []core.ContentBlock{core.TextContentBlock{Text: "guide"}},
	})
	require.Len(t, result.Contents, 1)
	require.Equal(t, "guide", result.Contents[0].Text)
}

func TestCoreContentBlocksFromProtocolConvertsResourceAndBlob(t *testing.T) {
	blocks := CoreContentBlocksFromProtocol([]protocol.ContentBlock{
		{Type: "resource", URI: "file:///docs/guide.md", MIMEType: "text/markdown"},
		{Type: "blob", Blob: "blob-1", MIMEType: "application/octet-stream"},
	})
	require.Len(t, blocks, 2)
	require.IsType(t, core.ResourceLinkContentBlock{}, blocks[0])
	require.IsType(t, core.BinaryReferenceContentBlock{}, blocks[1])
}
