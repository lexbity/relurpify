package runtime

import (
	"context"
	"time"

	"codeburg.org/lexbit/relurpify/app/relurpish/ports"
)

// evidencePortAdapter adapts the internal EvidenceRegistry to the ports.EvidencePort interface.
type evidencePortAdapter struct {
	registry *EvidenceRegistry
}

// newEvidencePortAdapter creates a new evidence port adapter.
func newEvidencePortAdapter(registry *EvidenceRegistry) ports.EvidencePort {
	return &evidencePortAdapter{registry: registry}
}

// GetPromptEvidence implements ports.EvidencePort.
func (a *evidencePortAdapter) GetPromptEvidence(ctx context.Context, promptID string, kind ports.EvidenceKind) (*ports.PromptEvidence, error) {
	internalEv, err := a.registry.GetPromptEvidence(ctx, promptID, EvidenceKind(kind))
	if err != nil {
		return nil, err
	}
	return a.toPortPromptEvidence(internalEv), nil
}

// GetModelAttemptEvidence implements ports.EvidencePort.
func (a *evidencePortAdapter) GetModelAttemptEvidence(ctx context.Context, callID string) (*ports.ModelAttemptEvidence, error) {
	internalEv, err := a.registry.GetModelAttemptEvidence(ctx, callID)
	if err != nil {
		return nil, err
	}
	return a.toPortModelAttemptEvidence(internalEv), nil
}

// GetKnowledgeFlowEvidence implements ports.EvidencePort.
func (a *evidencePortAdapter) GetKnowledgeFlowEvidence(ctx context.Context, operationID string) (*ports.KnowledgeFlowEvidence, error) {
	internalEv, err := a.registry.GetKnowledgeFlowEvidence(ctx, operationID)
	if err != nil {
		return nil, err
	}
	return a.toPortKnowledgeFlowEvidence(internalEv), nil
}

// GetEvidenceByCallID implements ports.EvidencePort.
func (a *evidencePortAdapter) GetEvidenceByCallID(ctx context.Context, callID string) (*ports.CallEvidenceBundle, error) {
	internalBundle, err := a.registry.GetEvidenceByCallID(ctx, callID)
	if err != nil {
		return nil, err
	}
	return a.toPortCallEvidenceBundle(internalBundle), nil
}

// QueryEvidence implements ports.EvidencePort.
func (a *evidencePortAdapter) QueryEvidence(ctx context.Context, query ports.EvidenceQuery) (*ports.EvidenceResult, error) {
	result := &ports.EvidenceResult{
		Prompts:   make([]*ports.PromptEvidence, 0),
		Calls:     make([]*ports.ModelAttemptEvidence, 0),
		Knowledge: make([]*ports.KnowledgeFlowEvidence, 0),
		Bundles:   make([]*ports.CallEvidenceBundle, 0),
	}

	// For now, implement simple queries
	// More complex queries can be added as needed

	if query.CallID != "" {
		bundle, err := a.GetEvidenceByCallID(ctx, query.CallID)
		if err == nil && bundle != nil {
			result.Bundles = append(result.Bundles, bundle)
			result.TotalCount++
		}
	}

	if query.PromptID != "" {
		promptEv, err := a.GetPromptEvidence(ctx, query.PromptID, query.Kind)
		if err == nil && promptEv != nil {
			result.Prompts = append(result.Prompts, promptEv)
			result.TotalCount++
		}
	}

	if query.OperationID != "" {
		knowledgeEv, err := a.GetKnowledgeFlowEvidence(ctx, query.OperationID)
		if err == nil && knowledgeEv != nil {
			result.Knowledge = append(result.Knowledge, knowledgeEv)
			result.TotalCount++
		}
	}

	// If no specific query, return all
	if query.CallID == "" && query.PromptID == "" && query.OperationID == "" {
		promptIDs, _ := a.registry.ListPromptEvidence(ctx)
		for _, id := range promptIDs {
			if query.Limit > 0 && len(result.Prompts) >= query.Limit {
				break
			}
			promptEv, _ := a.GetPromptEvidence(ctx, id, ports.EvidenceKindRuntime)
			if promptEv != nil {
				result.Prompts = append(result.Prompts, promptEv)
				result.TotalCount++
			}
		}

		callIDs, _ := a.registry.ListModelAttempts(ctx)
		for _, id := range callIDs {
			if query.Limit > 0 && len(result.Calls) >= query.Limit {
				break
			}
			callEv, _ := a.GetModelAttemptEvidence(ctx, id)
			if callEv != nil {
				result.Calls = append(result.Calls, callEv)
				result.TotalCount++
			}
		}

		knowledgeIDs, _ := a.registry.ListKnowledgeFlows(ctx)
		for _, id := range knowledgeIDs {
			if query.Limit > 0 && len(result.Knowledge) >= query.Limit {
				break
			}
			knowledgeEv, _ := a.GetKnowledgeFlowEvidence(ctx, id)
			if knowledgeEv != nil {
				result.Knowledge = append(result.Knowledge, knowledgeEv)
				result.TotalCount++
			}
		}
	}

	result.HasMore = result.TotalCount > query.Limit
	if query.Limit > 0 && result.TotalCount > query.Limit {
		result.TotalCount = query.Limit
	}

	return result, nil
}

// ListPromptEvidence implements ports.EvidencePort.
func (a *evidencePortAdapter) ListPromptEvidence(ctx context.Context) ([]string, error) {
	return a.registry.ListPromptEvidence(ctx)
}

// ListModelAttempts implements ports.EvidencePort.
func (a *evidencePortAdapter) ListModelAttempts(ctx context.Context) ([]string, error) {
	return a.registry.ListModelAttempts(ctx)
}

// ListKnowledgeFlows implements ports.EvidencePort.
func (a *evidencePortAdapter) ListKnowledgeFlows(ctx context.Context) ([]string, error) {
	return a.registry.ListKnowledgeFlows(ctx)
}

// Subscribe implements ports.EvidencePort.
func (a *evidencePortAdapter) Subscribe(bufferSize int) <-chan ports.EvidenceNotification {
	ch := a.registry.Subscribe(bufferSize)
	notificationCh := make(chan ports.EvidenceNotification, bufferSize)

	go func() {
		defer close(notificationCh)
		for ev := range ch {
			notification := ports.EvidenceNotification{
				Timestamp: ev.Timestamp.Format(time.RFC3339),
			}

			if ev.PromptEvidence != nil {
				notification.Kind = "prompt"
				notification.PromptID = ev.PromptEvidence.ID
			}
			if ev.ModelAttemptEvidence != nil {
				notification.Kind = "model_attempt"
				notification.CallID = ev.ModelAttemptEvidence.CallID
			}
			if ev.KnowledgeFlowEvidence != nil {
				notification.Kind = "knowledge_flow"
				notification.OperationID = ev.KnowledgeFlowEvidence.ID
			}

			select {
			case notificationCh <- notification:
			default:
				// Drop if channel is full
			}
		}
	}()

	return notificationCh
}

// toPortPromptEvidence converts internal PromptEvidence to port PromptEvidence.
func (a *evidencePortAdapter) toPortPromptEvidence(ev *PromptEvidence) *ports.PromptEvidence {
	if ev == nil {
		return nil
	}

	return &ports.PromptEvidence{
		ID:                ev.ID,
		Kind:              ports.EvidenceKind(ev.Kind),
		SourceReference:   ev.SourceReference,
		SourceRevision:    ev.SourceRevision,
		ResolvedAt:        ev.ResolvedAt.Format(time.RFC3339),
		IncludedBlockCount: len(ev.IncludedBlocks),
		ExcludedBlockCount: len(ev.ExcludedBlocks),
		OutputLength:      ev.OutputLength,
		DurationMS:        ev.DurationMS,
		Error:             ev.Error,
		CallID:            ev.CallID,
		AttemptID:         ev.AttemptID,
		ProvidersUsed:     ev.ProvidersUsed,
		CacheHit:          ev.CacheHit,
	}
}

// toPortModelAttemptEvidence converts internal ModelAttemptEvidence to port ModelAttemptEvidence.
func (a *evidencePortAdapter) toPortModelAttemptEvidence(ev *ModelAttemptEvidence) *ports.ModelAttemptEvidence {
	if ev == nil {
		return nil
	}

	return &ports.ModelAttemptEvidence{
		CallID:           ev.CallID,
		AttemptID:        ev.AttemptID,
		Kind:             ports.EvidenceKind(ev.Kind),
		Model:            ev.Model,
		Provider:         ev.Provider,
		ProfileRevision:  ev.ProfileRevision,
		PromptID:         ev.PromptID,
		InputTokens:      ev.InputTokens,
		OutputTokens:     ev.OutputTokens,
		FinishReason:     ev.FinishReason,
		ToolCallingMode:  ev.ToolCallingMode,
		DurationMS:       ev.DurationMS,
		StartedAt:        ev.StartedAt.Format(time.RFC3339),
		CompletedAt:      ev.CompletedAt.Format(time.RFC3339),
		Error:            ev.Error,
		TransportReclaimed: ev.TransportReclaimed,
		PromptPreview:    ev.PromptPreview,
		MessageCount:     ev.MessageCount,
	}
}

// toPortKnowledgeFlowEvidence converts internal KnowledgeFlowEvidence to port KnowledgeFlowEvidence.
func (a *evidencePortAdapter) toPortKnowledgeFlowEvidence(ev *KnowledgeFlowEvidence) *ports.KnowledgeFlowEvidence {
	if ev == nil {
		return nil
	}

	return &ports.KnowledgeFlowEvidence{
		ID:           ev.ID,
		Kind:         ev.Kind,
		Direction:    ev.Direction,
		Operation:    ev.Operation,
		ItemsSelected: ev.ItemsSelected,
		BudgetUsed:   ev.BudgetUsed,
		DurationMS:   ev.DurationMS,
		StartedAt:    ev.StartedAt.Format(time.RFC3339),
		CompletedAt:  ev.CompletedAt.Format(time.RFC3339),
		Error:        ev.Error,
		ChunkID:      ev.ChunkID,
		ContentHash:  ev.ContentHash,
		TokenEstimate: ev.TokenEstimate,
	}
}

// toPortCallEvidenceBundle converts internal CallEvidenceBundle to port CallEvidenceBundle.
func (a *evidencePortAdapter) toPortCallEvidenceBundle(bundle *CallEvidenceBundle) *ports.CallEvidenceBundle {
	if bundle == nil {
		return nil
	}

	portBundle := &ports.CallEvidenceBundle{
		CallID: bundle.CallID,
	}

	if bundle.ModelAttempt != nil {
		portBundle.ModelAttempt = a.toPortModelAttemptEvidence(bundle.ModelAttempt)
	}

	if bundle.PromptPreview != nil {
		portBundle.PromptPreview = a.toPortPromptEvidence(bundle.PromptPreview)
	}

	if bundle.PromptRuntime != nil {
		portBundle.PromptRuntime = a.toPortPromptEvidence(bundle.PromptRuntime)
	}

	if len(bundle.KnowledgeFlows) > 0 {
		portBundle.KnowledgeFlows = make([]*ports.KnowledgeFlowEvidence, 0, len(bundle.KnowledgeFlows))
		for _, flow := range bundle.KnowledgeFlows {
			portBundle.KnowledgeFlows = append(portBundle.KnowledgeFlows, a.toPortKnowledgeFlowEvidence(flow))
		}
	}

	return portBundle
}

// noOpEvidencePort is a no-op implementation of ports.EvidencePort.
type noOpEvidencePort struct{}

// GetPromptEvidence implements ports.EvidencePort.
func (n *noOpEvidencePort) GetPromptEvidence(ctx context.Context, promptID string, kind ports.EvidenceKind) (*ports.PromptEvidence, error) {
	return nil, ErrEvidenceNotFound
}

// GetModelAttemptEvidence implements ports.EvidencePort.
func (n *noOpEvidencePort) GetModelAttemptEvidence(ctx context.Context, callID string) (*ports.ModelAttemptEvidence, error) {
	return nil, ErrEvidenceNotFound
}

// GetKnowledgeFlowEvidence implements ports.EvidencePort.
func (n *noOpEvidencePort) GetKnowledgeFlowEvidence(ctx context.Context, operationID string) (*ports.KnowledgeFlowEvidence, error) {
	return nil, ErrEvidenceNotFound
}

// GetEvidenceByCallID implements ports.EvidencePort.
func (n *noOpEvidencePort) GetEvidenceByCallID(ctx context.Context, callID string) (*ports.CallEvidenceBundle, error) {
	return nil, ErrEvidenceNotFound
}

// QueryEvidence implements ports.EvidencePort.
func (n *noOpEvidencePort) QueryEvidence(ctx context.Context, query ports.EvidenceQuery) (*ports.EvidenceResult, error) {
	return &ports.EvidenceResult{
		Prompts:   make([]*ports.PromptEvidence, 0),
		Calls:     make([]*ports.ModelAttemptEvidence, 0),
		Knowledge: make([]*ports.KnowledgeFlowEvidence, 0),
		Bundles:   make([]*ports.CallEvidenceBundle, 0),
	}, nil
}

// ListPromptEvidence implements ports.EvidencePort.
func (n *noOpEvidencePort) ListPromptEvidence(ctx context.Context) ([]string, error) {
	return make([]string, 0), nil
}

// ListModelAttempts implements ports.EvidencePort.
func (n *noOpEvidencePort) ListModelAttempts(ctx context.Context) ([]string, error) {
	return make([]string, 0), nil
}

// ListKnowledgeFlows implements ports.EvidencePort.
func (n *noOpEvidencePort) ListKnowledgeFlows(ctx context.Context) ([]string, error) {
	return make([]string, 0), nil
}

// Subscribe implements ports.EvidencePort.
func (n *noOpEvidencePort) Subscribe(bufferSize int) <-chan ports.EvidenceNotification {
	ch := make(chan ports.EvidenceNotification, bufferSize)
	close(ch)
	return ch
}
