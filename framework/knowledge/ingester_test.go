package knowledge

import (
	"context"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/platform/contracts"
	"github.com/stretchr/testify/require"
)

func TestOutputIngester_IngestLLMResponse(t *testing.T) {
	store := newTestStore(t)
	ing := NewOutputIngester(store, &EventBus{})

	source, err := store.Save(KnowledgeChunk{
		ID:          ChunkID("chunk:source"),
		WorkspaceID: "ws",
		Provenance:  ChunkProvenance{CompiledBy: CompilerDeterministic, Timestamp: time.Now().UTC()},
		Body:        ChunkBody{Raw: "source"},
	})
	require.NoError(t, err)

	env := contextdata.NewEnvelope("task-1", "session-1")
	env.NodeID = "node-1"
	env.SetWorkingValue("workflow.id", "workflow-1", contextdata.MemoryClassTask)
	env.AddStreamedContextReference(contextdata.ChunkReference{ChunkID: contextdata.ChunkID(source.ID), Rank: 1})
	ctx := contextdata.WithEnvelope(WithOutputIngester(context.Background(), ing), env)

	saved, err := ing.IngestLLMResponseFull(ctx, &contracts.LLMResponse{
		Text:         "hello world",
		FinishReason: "stop",
		Usage:        contracts.TokenUsageReport{PromptTokens: 12, CompletionTokens: 3, TotalTokens: 15},
	})
	require.NoError(t, err)
	require.NotNil(t, saved)
	require.Equal(t, SourceOriginLLM, saved.SourceOrigin)
	require.Equal(t, agentspec.TrustClassLLMGenerated, saved.TrustClass)
	require.Equal(t, "session-1", saved.WorkspaceID)
	require.NotEmpty(t, saved.ContentHash)

	byHash, err := store.FindByContentHash(saved.ContentHash)
	require.NoError(t, err)
	require.Len(t, byHash, 1)

	edges, err := store.LoadEdgesFrom(source.ID, EdgeKindDerivesFrom)
	require.NoError(t, err)
	require.Len(t, edges, 1)
	require.Equal(t, saved.ID, edges[0].ToChunk)
}

func TestOutputIngester_IngestToolResult_LinkedToLLMChunk(t *testing.T) {
	store := newTestStore(t)
	ing := NewOutputIngester(store, &EventBus{})

	response, err := ing.IngestObservation(context.Background(), "seed response")
	require.NoError(t, err)

	env := contextdata.NewEnvelope("task-1", "session-1")
	env.AddStreamedContextReference(contextdata.ChunkReference{ChunkID: contextdata.ChunkID(response.ID), Rank: 1})
	ctx := contextdata.WithEnvelope(WithOutputIngester(context.Background(), ing), env)

	saved, err := ing.IngestToolResult(ctx, "file_read", []byte("tool output"))
	require.NoError(t, err)
	require.NotNil(t, saved)

	edges, err := store.LoadEdgesFrom(saved.ID, EdgeKindGrounds)
	require.NoError(t, err)
	require.Len(t, edges, 1)
	require.Equal(t, response.ID, edges[0].ToChunk)
}

func TestOutputIngester_Idempotent(t *testing.T) {
	store := newTestStore(t)
	ing := NewOutputIngester(store, nil)
	ctx := context.Background()

	first, err := ing.IngestObservation(ctx, "same output")
	require.NoError(t, err)
	second, err := ing.IngestObservation(ctx, "same output")
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID)

	byHash, err := store.FindByContentHash(first.ContentHash)
	require.NoError(t, err)
	require.Len(t, byHash, 1)
}

func TestOutputIngester_EventFired(t *testing.T) {
	store := newTestStore(t)
	bus := &EventBus{}
	events, cancel := bus.Subscribe(4)
	defer cancel()
	ing := NewOutputIngester(store, bus)

	_, err := ing.IngestObservation(context.Background(), "event payload")
	require.NoError(t, err)

	select {
	case event := <-events:
		require.Equal(t, EventChunkIngested, event.Kind)
		payload, ok := event.Payload.(ChunkIngestedPayload)
		require.True(t, ok)
		require.NotEmpty(t, payload.ChunkID)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for chunk ingested event")
	}
}
