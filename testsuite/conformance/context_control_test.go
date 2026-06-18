package conformance

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/knowledge"
	"codeburg.org/lexbit/relurpify/context/knowledge/graphdb"
	"codeburg.org/lexbit/relurpify/context/knowledge/retrieval"
	"codeburg.org/lexbit/relurpify/execution/compiler"
	"codeburg.org/lexbit/relurpify/execution/prompt"
	"codeburg.org/lexbit/relurpify/named/euclo/promptprovider"
)

func TestContextControl_UserFocusAnchorChangesCompilation(t *testing.T) {
	store := newContextControlStore(t)
	comp := compiler.NewCompiler(nil, nil, store)

	base := compileContextControlResult(t, comp, nil)
	pinned := compileContextControlResult(t, comp, []retrieval.AnchorRef{{
		AnchorID: "pin:" + filepath.Join("workspace", "alpha.txt"),
		Term:     filepath.Join("workspace", "alpha.txt"),
		Class:    "session_pin",
		Active:   true,
	}})

	require.Len(t, base.PinReferences, 0)
	require.Len(t, pinned.PinReferences, 1)
	require.Equal(t, filepath.ToSlash(filepath.Join("workspace", "alpha.txt")), filepath.ToSlash(pinned.PinReferences[0].Path))
	require.NotEqual(t, base.PinReferences, pinned.PinReferences)
}

func TestContextControl_RecipeStepContextChangesRenderedPrompt(t *testing.T) {
	env := contextdata.NewEnvelope("task-context", "session-context")
	provider := promptprovider.NewThoughtRecipeStepContextProvider()

	base := provider.Provide(prompt.NewRuntimeContext(env, "react", "thoughtrecipe"))
	riched := provider.Provide(prompt.NewRuntimeContext(env, "react", "thoughtrecipe").
		WithStateValue("execution_step_context_stream_query", "find relevant symbols").
		WithStateValue("execution_step_context_stream_max_tokens", 128).
		WithStateValue("execution_step_context_stream_mode", "summary"))

	require.NotEmpty(t, base.Content)
	require.Contains(t, riched.Content, "Step Context Stream Query: find relevant symbols")
	require.Contains(t, riched.Content, "Step Context Stream Max Tokens: 128")
	require.Contains(t, riched.Content, "Step Context Stream Mode: summary")
	require.NotEqual(t, base.Content, riched.Content)
}

func compileContextControlResult(t *testing.T, comp *compiler.Compiler, anchors []retrieval.AnchorRef) *compiler.CompilationResult {
	t.Helper()
	result, _, err := comp.Compile(context.Background(), compiler.CompilationRequest{
		Query: retrieval.RetrievalQuery{
			Text:    "context control",
			Anchors: anchors,
		},
		MaxTokens: 4000,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	return result
}

func newContextControlStore(t *testing.T) *knowledge.ChunkStore {
	t.Helper()
	engine, err := graphdb.Open(context.Background(), graphdb.DefaultOptions(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = engine.Close(context.Background())
	})
	store := &knowledge.ChunkStore{Graph: engine}
	chunk := knowledge.KnowledgeChunk{
		ID:          knowledge.ChunkID("chunk:alpha"),
		WorkspaceID: "workspace",
		TrustClass:  agentspec.TrustClassBuiltinTrusted,
		Body: knowledge.ChunkBody{
			Raw: "alpha content",
			Fields: map[string]any{
				"file_path": filepath.ToSlash(filepath.Join("workspace", "alpha.txt")),
				"content":   "alpha content",
			},
		},
	}
	_, err = store.Save(context.Background(), chunk)
	require.NoError(t, err)
	return store
}
