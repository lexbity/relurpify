package bkc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	archaeodeferred "github.com/lexcodex/relurpify/archaeo/deferred"
	"github.com/lexcodex/relurpify/archaeo/internal/storeutil"
	archaeolearning "github.com/lexcodex/relurpify/archaeo/learning"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
)

const chunkCandidateArtifactKind = "archaeo_bkc_chunk_candidate"

type ChunkCandidateStatus string

const (
	ChunkCandidatePending   ChunkCandidateStatus = "pending"
	ChunkCandidateConfirmed ChunkCandidateStatus = "confirmed"
	ChunkCandidateRejected  ChunkCandidateStatus = "rejected"
	ChunkCandidateDeferred  ChunkCandidateStatus = "deferred"
)

type LLMCompileInput struct {
	WorkspaceID     string
	WorkflowID      string
	ExplorationID   string
	SnapshotID      string
	BasedOnRevision string
	SubjectRef      string
	Title           string
	Description     string
	Prompt          string
	RelatedChunkIDs []ChunkID
	SessionID       string
	Blocking        bool
}

type llmChunkPayload struct {
	Raw    string         `json:"raw"`
	Fields map[string]any `json:"fields,omitempty"`
}

type llmChunkView struct {
	Kind ViewKind `json:"kind"`
	Data any      `json:"data"`
}

type llmChunkProposal struct {
	Title       string          `json:"title"`
	Summary     string          `json:"summary"`
	Body        llmChunkPayload `json:"body"`
	Views       []llmChunkView  `json:"views,omitempty"`
	ContentHash string          `json:"content_hash,omitempty"`
}

type ChunkCandidate struct {
	ID              string               `json:"id"`
	WorkspaceID     string               `json:"workspace_id"`
	WorkflowID      string               `json:"workflow_id"`
	ExplorationID   string               `json:"exploration_id"`
	SnapshotID      string               `json:"snapshot_id,omitempty"`
	InteractionID   string               `json:"interaction_id,omitempty"`
	Chunk           KnowledgeChunk       `json:"chunk"`
	RelatedChunkIDs []ChunkID            `json:"related_chunk_ids,omitempty"`
	Status          ChunkCandidateStatus `json:"status"`
	CreatedAt       time.Time            `json:"created_at"`
	UpdatedAt       time.Time            `json:"updated_at"`
}

type CandidateResult struct {
	Candidate   ChunkCandidate              `json:"candidate"`
	Interaction archaeolearning.Interaction `json:"interaction"`
}

type ResolveCandidateInput struct {
	WorkflowID      string
	InteractionID   string
	Kind            archaeolearning.ResolutionKind
	ResolvedBy      string
	ChoiceID        string
	Comment         *archaeolearning.CommentInput
	BasedOnRevision string
}

// LLMCompiler proposes chunk candidates, routes them through archaeology
// learning, and finalizes them back into the durable chunk graph.
type LLMCompiler struct {
	Store         *ChunkStore
	WorkflowStore memory.WorkflowStateStore
	Learning      archaeolearning.Service
	Deferred      archaeodeferred.Service
	Model         core.LanguageModel
	Now           func() time.Time
	NewID         func(prefix string) string
}

func (c *LLMCompiler) Propose(ctx context.Context, input LLMCompileInput) (*CandidateResult, error) {
	if c == nil || c.Store == nil {
		return nil, errors.New("bkc: chunk store required")
	}
	if c.WorkflowStore == nil {
		return nil, errors.New("bkc: workflow store required")
	}
	if c.Model == nil {
		return nil, errors.New("bkc: language model required")
	}
	if strings.TrimSpace(input.WorkflowID) == "" || strings.TrimSpace(input.ExplorationID) == "" || strings.TrimSpace(input.WorkspaceID) == "" {
		return nil, errors.New("bkc: workspace, workflow, and exploration ids are required")
	}
	proposal, err := c.generateProposal(ctx, input)
	if err != nil {
		return nil, err
	}
	now := c.now()
	candidateID := c.newID("bkc-candidate")
	chunkID := ChunkID(firstNonEmpty(strings.TrimSpace(proposal.ContentHash), string(deterministicChunkID("llm_candidate", firstNonEmpty(input.SubjectRef, proposal.Title, candidateID)))))
	if !strings.HasPrefix(string(chunkID), "chunk:") {
		chunkID = ChunkID("chunk:llm:" + string(chunkID))
	}
	raw := strings.TrimSpace(firstNonEmpty(proposal.Body.Raw, proposal.Summary, input.Description, input.Prompt))
	bodyFields := cloneMap(proposal.Body.Fields)
	if bodyFields == nil {
		bodyFields = map[string]any{}
	}
	if strings.TrimSpace(proposal.Title) != "" {
		bodyFields["title"] = strings.TrimSpace(proposal.Title)
	}
	if strings.TrimSpace(input.SubjectRef) != "" {
		bodyFields["subject_ref"] = strings.TrimSpace(input.SubjectRef)
	}
	chunk := KnowledgeChunk{
		ID:            chunkID,
		WorkspaceID:   strings.TrimSpace(input.WorkspaceID),
		ContentHash:   hashStrings(raw, input.BasedOnRevision, input.SubjectRef),
		TokenEstimate: estimateTokens(raw),
		Provenance: ChunkProvenance{
			Sources: []ProvenanceSource{{
				Kind: "llm_compilation",
				Ref:  firstNonEmpty(strings.TrimSpace(input.SubjectRef), candidateID),
			}},
			SessionID:    strings.TrimSpace(input.SessionID),
			WorkflowID:   strings.TrimSpace(input.WorkflowID),
			CodeStateRef: strings.TrimSpace(input.BasedOnRevision),
			CompiledBy:   CompilerLLMAssisted,
			Timestamp:    now,
		},
		Freshness: FreshnessUnverified,
		Body: ChunkBody{
			Raw:    raw,
			Fields: bodyFields,
		},
		Views:     toChunkViews(proposal.Views),
		CreatedAt: now,
		UpdatedAt: now,
	}
	candidate := ChunkCandidate{
		ID:              candidateID,
		WorkspaceID:     strings.TrimSpace(input.WorkspaceID),
		WorkflowID:      strings.TrimSpace(input.WorkflowID),
		ExplorationID:   strings.TrimSpace(input.ExplorationID),
		SnapshotID:      strings.TrimSpace(input.SnapshotID),
		Chunk:           chunk,
		RelatedChunkIDs: append([]ChunkID(nil), input.RelatedChunkIDs...),
		Status:          ChunkCandidatePending,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	interaction, err := c.learningService().Create(ctx, archaeolearning.CreateInput{
		WorkflowID:      input.WorkflowID,
		ExplorationID:   input.ExplorationID,
		SnapshotID:      input.SnapshotID,
		Kind:            archaeolearning.InteractionKnowledgeProposal,
		SubjectType:     archaeolearning.SubjectExploration,
		SubjectID:       candidate.ID,
		Title:           firstNonEmpty(strings.TrimSpace(input.Title), strings.TrimSpace(proposal.Title), "Confirm BKC chunk candidate"),
		Description:     firstNonEmpty(strings.TrimSpace(input.Description), strings.TrimSpace(proposal.Summary), raw),
		Blocking:        input.Blocking,
		BasedOnRevision: strings.TrimSpace(input.BasedOnRevision),
		Evidence: []archaeolearning.EvidenceRef{{
			Kind:    "bkc_candidate",
			RefID:   candidate.ID,
			Title:   firstNonEmpty(strings.TrimSpace(proposal.Title), candidate.ID),
			Summary: firstNonEmpty(strings.TrimSpace(proposal.Summary), raw),
			Metadata: map[string]any{
				"chunk_id": string(chunk.ID),
			},
		}},
	})
	if err != nil {
		return nil, err
	}
	candidate.InteractionID = interaction.ID
	candidate.UpdatedAt = c.now()
	if err := c.saveCandidate(ctx, candidate); err != nil {
		return nil, err
	}
	return &CandidateResult{Candidate: candidate, Interaction: *interaction}, nil
}

func (c *LLMCompiler) ResolveCandidate(ctx context.Context, input ResolveCandidateInput) (*ChunkCandidate, *CompileResult, error) {
	if c == nil || c.Store == nil {
		return nil, nil, errors.New("bkc: chunk store required")
	}
	if c.WorkflowStore == nil {
		return nil, nil, errors.New("bkc: workflow store required")
	}
	interaction, ok, err := c.learningService().Load(ctx, input.WorkflowID, input.InteractionID)
	if err != nil {
		return nil, nil, err
	}
	if !ok || interaction == nil {
		return nil, nil, fmt.Errorf("bkc: learning interaction %s not found", input.InteractionID)
	}
	candidate, ok, err := c.loadCandidate(ctx, interaction.SubjectID)
	if err != nil {
		return nil, nil, err
	}
	if !ok || candidate == nil {
		return nil, nil, fmt.Errorf("bkc: chunk candidate %s not found", interaction.SubjectID)
	}
	resolveInput := archaeolearning.ResolveInput{
		WorkflowID:      input.WorkflowID,
		InteractionID:   input.InteractionID,
		Kind:            input.Kind,
		ChoiceID:        strings.TrimSpace(input.ChoiceID),
		Comment:         input.Comment,
		ResolvedBy:      strings.TrimSpace(input.ResolvedBy),
		BasedOnRevision: strings.TrimSpace(input.BasedOnRevision),
	}
	resolved, err := c.learningService().Resolve(ctx, resolveInput)
	if err != nil {
		return nil, nil, err
	}
	_ = resolved
	switch input.Kind {
	case archaeolearning.ResolutionConfirm:
		candidate.Chunk.Freshness = FreshnessValid
		candidate.Status = ChunkCandidateConfirmed
		result, err := (&Compiler{Store: c.Store, Now: c.Now}).saveCompiledChunk(candidate.Chunk, candidate.RelatedChunkIDs, true)
		if err != nil {
			return nil, nil, err
		}
		candidate.UpdatedAt = c.now()
		if err := c.saveCandidate(ctx, *candidate); err != nil {
			return nil, nil, err
		}
		return candidate, result, nil
	case archaeolearning.ResolutionDefer:
		candidate.Chunk.Freshness = FreshnessUnverified
		candidate.Status = ChunkCandidateDeferred
		result, err := (&Compiler{Store: c.Store, Now: c.Now}).saveCompiledChunk(candidate.Chunk, candidate.RelatedChunkIDs, false)
		if err != nil {
			return nil, nil, err
		}
		candidate.UpdatedAt = c.now()
		if err := c.saveCandidate(ctx, *candidate); err != nil {
			return nil, nil, err
		}
		return candidate, result, nil
	case archaeolearning.ResolutionReject:
		candidate.Status = ChunkCandidateRejected
		candidate.UpdatedAt = c.now()
		if err := c.saveCandidate(ctx, *candidate); err != nil {
			return nil, nil, err
		}
		_, err := c.deferredService().CreateOrUpdate(ctx, archaeodeferred.CreateInput{
			WorkspaceID:   candidate.WorkspaceID,
			WorkflowID:    candidate.WorkflowID,
			ExplorationID: candidate.ExplorationID,
			AmbiguityKey:  "bkc_candidate:" + candidate.ID,
			Title:         firstNonEmpty(interaction.Title, "Rejected BKC candidate"),
			Description:   firstNonEmpty(interaction.Description, candidate.Chunk.Body.Raw),
			Metadata: map[string]any{
				"candidate_id":   candidate.ID,
				"interaction_id": interaction.ID,
				"chunk_id":       string(candidate.Chunk.ID),
			},
		})
		return candidate, &CompileResult{}, err
	default:
		return nil, nil, fmt.Errorf("bkc: unsupported candidate resolution %q", input.Kind)
	}
}

func (c *LLMCompiler) generateProposal(ctx context.Context, input LLMCompileInput) (*llmChunkProposal, error) {
	response, err := c.Model.Chat(ctx, []core.Message{
		{Role: "system", Content: "Return only JSON for a proposed knowledge chunk. Include title, summary, body.raw, optional body.fields, and optional views."},
		{Role: "user", Content: strings.TrimSpace(input.Prompt)},
	}, &core.LLMOptions{Temperature: 0.1, MaxTokens: 800})
	if err != nil {
		return nil, err
	}
	var proposal llmChunkProposal
	if err := json.Unmarshal([]byte(strings.TrimSpace(response.Text)), &proposal); err != nil {
		return nil, fmt.Errorf("bkc: decode llm chunk proposal: %w", err)
	}
	return &proposal, nil
}

func (c *LLMCompiler) saveCandidate(ctx context.Context, candidate ChunkCandidate) error {
	raw, err := json.Marshal(candidate)
	if err != nil {
		return err
	}
	return c.WorkflowStore.UpsertWorkflowArtifact(ctx, memory.WorkflowArtifactRecord{
		ArtifactID:      "archaeo_bkc_chunk_candidate:" + candidate.ID,
		WorkflowID:      candidate.WorkflowID,
		Kind:            chunkCandidateArtifactKind,
		ContentType:     "application/json",
		StorageKind:     memory.ArtifactStorageInline,
		SummaryText:     firstNonEmpty(string(candidate.Status), candidate.ID),
		SummaryMetadata: map[string]any{"workspace_id": candidate.WorkspaceID, "candidate_id": candidate.ID, "interaction_id": candidate.InteractionID, "status": string(candidate.Status)},
		InlineRawText:   string(raw),
		RawSizeBytes:    int64(len(raw)),
	})
}

func (c *LLMCompiler) loadCandidate(ctx context.Context, candidateID string) (*ChunkCandidate, bool, error) {
	artifact, ok, err := storeutil.WorkflowArtifactByID(ctx, c.WorkflowStore, "archaeo_bkc_chunk_candidate:"+strings.TrimSpace(candidateID))
	if err != nil || !ok || artifact == nil {
		return nil, ok, err
	}
	var candidate ChunkCandidate
	if err := json.Unmarshal([]byte(artifact.InlineRawText), &candidate); err != nil {
		return nil, false, err
	}
	return &candidate, true, nil
}

func (c *LLMCompiler) learningService() archaeolearning.Service {
	service := c.Learning
	if service.Store == nil {
		service.Store = c.WorkflowStore
	}
	if service.Now == nil {
		service.Now = c.Now
	}
	if service.NewID == nil {
		service.NewID = c.NewID
	}
	return service
}

func (c *LLMCompiler) deferredService() archaeodeferred.Service {
	service := c.Deferred
	if service.Store == nil {
		service.Store = c.WorkflowStore
	}
	if service.Now == nil {
		service.Now = c.Now
	}
	if service.NewID == nil {
		service.NewID = c.NewID
	}
	return service
}

func (c *LLMCompiler) now() time.Time {
	if c != nil && c.Now != nil {
		return c.Now().UTC()
	}
	return time.Now().UTC()
}

func (c *LLMCompiler) newID(prefix string) string {
	if c != nil && c.NewID != nil {
		return c.NewID(prefix)
	}
	return fmt.Sprintf("%s-%d", strings.TrimSpace(prefix), c.now().UnixNano())
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
