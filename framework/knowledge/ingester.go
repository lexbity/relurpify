package knowledge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

type outputIngesterContextKey struct{}

// WithOutputIngester stores an output ingester in context under both the
// framework-local key (for typed retrieval) and the contracts.ResponseIngester
// key (so platform/llm.InstrumentedModel can reach it without importing framework packages).
func WithOutputIngester(ctx context.Context, ing *OutputIngester) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx = context.WithValue(ctx, outputIngesterContextKey{}, ing)
	return contracts.WithResponseIngester(ctx, ing)
}

// OutputIngesterFromContext extracts an output ingester from context.
func OutputIngesterFromContext(ctx context.Context) *OutputIngester {
	if ctx == nil {
		return nil
	}
	ing, _ := ctx.Value(outputIngesterContextKey{}).(*OutputIngester)
	return ing
}

// OutputIngester converts runtime outputs into durable knowledge chunks.
type OutputIngester struct {
	Store         *ChunkStore
	Events        *EventBus
	InlineLimit   int
	DefaultSource string
}

// NewOutputIngester constructs an output ingester.
func NewOutputIngester(store *ChunkStore, events *EventBus) *OutputIngester {
	return &OutputIngester{
		Store:       store,
		Events:      events,
		InlineLimit: 8192,
	}
}

// IngestLLMResponse implements contracts.ResponseIngester.
// The returned chunk is discarded; callers needing the chunk use IngestLLMResponseFull.
func (ing *OutputIngester) IngestLLMResponse(ctx context.Context, resp *contracts.LLMResponse) error {
	_, err := ing.IngestLLMResponseFull(ctx, resp)
	return err
}

// IngestLLMResponseFull stores an LLM response as a knowledge chunk and returns it.
func (ing *OutputIngester) IngestLLMResponseFull(ctx context.Context, resp *contracts.LLMResponse) (*KnowledgeChunk, error) {
	if resp == nil || strings.TrimSpace(resp.Text) == "" {
		return nil, nil
	}
	env, _ := contextdata.EnvelopeFrom(ctx)
	return ing.ingestText(ctx, ingestTextInput{
		kind:           "llm_response",
		text:           resp.Text,
		sourceOrigin:   SourceOriginLLM,
		trustClass:     agentspec.TrustClassLLMGenerated,
		memoryClass:    MemoryClassStreamed,
		storageMode:    StorageModeSummarized,
		sessionID:      sessionIDFromEnvelope(env),
		workflowID:     workflowIDFromEnvelope(env),
		nodeID:         nodeIDFromEnvelope(env),
		sourceChunkIDs: sourceChunkIDsFromEnvelope(env),
		fields: map[string]any{
			"finish_reason": resp.FinishReason,
			"usage":         resp.Usage,
		},
	})
}

// IngestToolResult stores a tool result as a knowledge chunk.
func (ing *OutputIngester) IngestToolResult(ctx context.Context, toolName string, result []byte) (*KnowledgeChunk, error) {
	if len(result) == 0 {
		return nil, nil
	}
	env, _ := contextdata.EnvelopeFrom(ctx)
	return ing.ingestText(ctx, ingestTextInput{
		kind:           "tool_result",
		text:           string(result),
		sourceOrigin:   SourceOriginTool,
		trustClass:     agentspec.TrustClassToolResult,
		memoryClass:    MemoryClassWorking,
		storageMode:    storageModeForSize(len(result)),
		sessionID:      sessionIDFromEnvelope(env),
		workflowID:     workflowIDFromEnvelope(env),
		nodeID:         nodeIDFromEnvelope(env),
		sourceChunkIDs: sourceChunkIDsFromEnvelope(env),
		fields: map[string]any{
			"tool_name": toolName,
			"raw_bytes": len(result),
		},
	})
}

// IngestObservation stores a textual observation as a knowledge chunk.
func (ing *OutputIngester) IngestObservation(ctx context.Context, observation string) (*KnowledgeChunk, error) {
	if strings.TrimSpace(observation) == "" {
		return nil, nil
	}
	env, _ := contextdata.EnvelopeFrom(ctx)
	return ing.ingestText(ctx, ingestTextInput{
		kind:           "observation",
		text:           observation,
		sourceOrigin:   SourceOriginDerivation,
		trustClass:     agentspec.TrustClassLLMGenerated,
		memoryClass:    MemoryClassStreamed,
		storageMode:    StorageModeSummarized,
		sessionID:      sessionIDFromEnvelope(env),
		workflowID:     workflowIDFromEnvelope(env),
		nodeID:         nodeIDFromEnvelope(env),
		sourceChunkIDs: sourceChunkIDsFromEnvelope(env),
		fields: map[string]any{
			"observation": observation,
		},
	})
}

// IngestToolResultAsync schedules tool result ingestion without blocking the caller.
func IngestToolResultAsync(ctx context.Context, ing *OutputIngester, toolName string, result []byte) {
	if ing == nil || len(result) == 0 {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	go func() {
		timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		_, _ = ing.IngestToolResult(timeoutCtx, toolName, result)
	}()
}

// IngestObservationAsync schedules observation ingestion without blocking the caller.
func IngestObservationAsync(ctx context.Context, ing *OutputIngester, observation string) {
	if ing == nil || strings.TrimSpace(observation) == "" {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	go func() {
		timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		_, _ = ing.IngestObservation(timeoutCtx, observation)
	}()
}

type ingestTextInput struct {
	kind           string
	text           string
	sourceOrigin   SourceOrigin
	trustClass     agentspec.TrustClass
	memoryClass    MemoryClass
	storageMode    StorageMode
	sessionID      string
	workflowID     string
	nodeID         string
	sourceChunkIDs []ChunkID
	fields         map[string]any
}

func (ing *OutputIngester) ingestText(ctx context.Context, input ingestTextInput) (*KnowledgeChunk, error) {
	if ing == nil || ing.Store == nil {
		return nil, fmt.Errorf("knowledge: output ingester store is required")
	}
	text := strings.TrimSpace(input.text)
	if text == "" {
		return nil, nil
	}
	contentHash := contentHashForText(text)
	existing, err := ing.Store.FindByContentHash(contentHash)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	chunk := KnowledgeChunk{
		ID:                deterministicChunkID(input.kind, contentHash),
		WorkspaceID:       input.sessionID,
		ContentHash:       contentHash,
		TokenEstimate:     estimateTokens(text),
		MemoryClass:       input.memoryClass,
		StorageMode:       input.storageMode,
		SourceOrigin:      input.sourceOrigin,
		AcquisitionMethod: AcquisitionMethodRuntimeWrite,
		AcquiredAt:        now,
		TrustClass:        input.trustClass,
		DerivedFrom:       append([]ChunkID(nil), input.sourceChunkIDs...),
		Provenance: ChunkProvenance{
			Sources:    provenanceSourcesFromChunkIDs(input.sourceChunkIDs),
			SessionID:  input.sessionID,
			WorkflowID: input.workflowID,
			CompiledBy: CompilerLLMAssisted,
			Timestamp:  now,
		},
		Freshness: FreshnessValid,
		Body: ChunkBody{
			Raw:    text,
			Fields: cloneMap(input.fields),
		},
	}
	chunk.Body.Fields = ensureFields(chunk.Body.Fields)
	chunk.Body.Fields["kind"] = input.kind
	chunk.Body.Fields["content_hash"] = contentHash
	chunk.Body.Fields["session_id"] = input.sessionID
	chunk.Body.Fields["workflow_id"] = input.workflowID
	chunk.Body.Fields["node_id"] = input.nodeID
	chunk.Body.Fields["source_origin"] = string(input.sourceOrigin)
	chunk.Body.Fields["token_estimate"] = chunk.TokenEstimate

	if len(existing) > 0 {
		// Reuse the newest matching chunk rather than duplicating identical output.
		chunk.ID = existing[0].ID
		chunk.Version = existing[0].Version
		chunk.CreatedAt = existing[0].CreatedAt
	}

	saved, err := ing.Store.Save(chunk)
	if err != nil {
		return nil, err
	}
	for _, sourceID := range input.sourceChunkIDs {
		if sourceID == "" {
			continue
		}
		edgeKind := EdgeKindDerivesFrom
		fromID := sourceID
		toID := saved.ID
		if input.kind == "tool_result" {
			edgeKind = EdgeKindGrounds
			fromID = saved.ID
			toID = sourceID
		}
		_, _ = ing.Store.SaveEdge(ChunkEdge{
			FromChunk: fromID,
			ToChunk:   toID,
			Kind:      edgeKind,
			Weight:    1,
			Provenance: ChunkProvenance{
				Sources:    []ProvenanceSource{{Kind: "chunk", Ref: string(sourceID)}},
				SessionID:  input.sessionID,
				WorkflowID: input.workflowID,
				CompiledBy: CompilerLLMAssisted,
				Timestamp:  now,
			},
			CreatedAt: now,
		})
	}
	if ing.Events != nil {
		ing.Events.EmitChunkIngested(ChunkIngestedPayload{
			SessionID:      input.sessionID,
			WorkflowID:     input.workflowID,
			NodeID:         input.nodeID,
			ChunkID:        string(saved.ID),
			ContentHash:    saved.ContentHash,
			SourceOrigin:   string(saved.SourceOrigin),
			TokenEstimate:  saved.TokenEstimate,
			SourceChunkIDs: chunkIDsToStrings(input.sourceChunkIDs),
		})
	}
	return saved, nil
}

func contentHashForText(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:16])
}

func provenanceSourcesFromChunkIDs(ids []ChunkID) []ProvenanceSource {
	if len(ids) == 0 {
		return nil
	}
	out := make([]ProvenanceSource, 0, len(ids))
	for _, id := range ids {
		if id == "" {
			continue
		}
		out = append(out, ProvenanceSource{Kind: "chunk", Ref: string(id)})
	}
	return out
}

func sourceChunkIDsFromEnvelope(env *contextdata.Envelope) []ChunkID {
	if env == nil {
		return nil
	}
	refs := env.StreamedChunkIDs()
	if len(refs) == 0 {
		return nil
	}
	out := make([]ChunkID, 0, len(refs))
	for _, ref := range refs {
		out = append(out, ChunkID(ref))
	}
	return out
}

func sessionIDFromEnvelope(env *contextdata.Envelope) string {
	if env == nil {
		return ""
	}
	return env.SessionID
}

func workflowIDFromEnvelope(env *contextdata.Envelope) string {
	if env == nil {
		return ""
	}
	val, _ := env.GetWorkingValue("workflow.id")
	if s, ok := val.(string); ok {
		return s
	}
	return ""
}

func nodeIDFromEnvelope(env *contextdata.Envelope) string {
	if env == nil {
		return ""
	}
	return env.NodeID
}

func storageModeForSize(size int) StorageMode {
	if size > 8192 {
		return StorageModeExternal
	}
	return StorageModeInline
}

func ensureFields(fields map[string]any) map[string]any {
	if fields == nil {
		return map[string]any{}
	}
	return fields
}
