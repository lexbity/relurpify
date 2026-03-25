package retrieval

import (
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

func TestMemoryEnvelopesBuildSharedDeclarativeMemoryShape(t *testing.T) {
	envelopes := MemoryEnvelopes([]core.ContentBlock{
		core.StructuredContentBlock{Data: map[string]any{
			"type":    "retrieval_evidence",
			"text":    "retrieval backed declarative memory",
			"summary": "retrieval backed declarative memory",
			"citations": []PackedCitation{{
				DocID: "doc:1",
			}},
		}},
	}, core.MemoryClassDeclarative, "project")
	require.Len(t, envelopes, 1)
	require.Equal(t, "doc:1", envelopes[0].Key)
	require.Equal(t, "retrieval backed declarative memory", envelopes[0].Summary)
	require.Equal(t, core.MemoryClassDeclarative, envelopes[0].MemoryClass)
	require.Equal(t, "project", envelopes[0].Scope)
}

func TestMemoryEnvelopesPreferStructuredReferenceIdentity(t *testing.T) {
	envelopes := MemoryEnvelopes([]core.ContentBlock{
		core.StructuredContentBlock{Data: map[string]any{
			"type":    "retrieval_evidence",
			"text":    "reference-backed evidence",
			"summary": "reference-backed evidence",
			"reference": map[string]any{
				"id":  "record:1",
				"uri": "memory://workflow/1",
			},
		}},
	}, core.MemoryClassDeclarative, "workflow:wf-1")
	require.Len(t, envelopes, 1)
	require.Equal(t, "record:1", envelopes[0].Key)
}
