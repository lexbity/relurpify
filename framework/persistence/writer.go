package persistence

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/framework/contextpolicy"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/knowledge"
	"codeburg.org/lexbit/relurpify/relurpnet/identity"
)

// Persist persists a single artifact to the knowledge store.
// Admission path: structural validation → trust class assignment → suspicion check → quota check → commit.
func (w *Writer) Persist(ctx context.Context, req PersistenceRequest) (*PersistenceResult, error) {
	result := &PersistenceResult{}

	// 1. Structural validation
	if err := w.validateRequest(req); err != nil {
		result.Action = ActionRejected
		result.Error = fmt.Errorf("validation failed: %w", err)
		w.writeAuditRecord(req, result, "structural validation failed")
		return result, nil
	}

	// 2. Trust class assignment from source principal
	trustClass := w.determineTrustClass(req.SourcePrincipal)

	// 3. Suspicion check (lightweight)
	if suspicious, reason := w.suspicionCheck(req); suspicious {
		result.Action = ActionQuarantined
		result.Error = fmt.Errorf("suspicion check failed: %s", reason)
		w.writeAuditRecord(req, result, reason)
		return result, nil
	}

	// 4. Quota check
	if w.Evaluator != nil {
		remaining, _ := w.Evaluator.QuotaRemaining(req.SourcePrincipal)
		if remaining == 0 {
			result.Action = ActionQuarantined
			result.Error = fmt.Errorf("quota exceeded")
			w.writeAuditRecord(req, result, "quota exceeded")
			return result, nil
		}
	}

	// 5. Deduplication
	contentHash := w.computeContentHash(req.Content)
	if existing := w.checkDuplicate(contentHash, req.SourceOrigin); existing != "" {
		result.Action = ActionDeduplicated
		result.ChunkID = existing
		w.writeAuditRecord(req, result, "duplicate detected")
		return result, nil
	}

	// 6. Build and commit chunk
	chunk := w.buildChunk(req, trustClass, contentHash)
	savedChunk, err := w.Store.Save(*chunk)
	if err != nil {
		result.Action = ActionRejected
		result.Error = fmt.Errorf("commit failed: %w", err)
		w.writeAuditRecord(req, result, "commit failed")
		return result, nil
	}

	result.Action = ActionCreated
	result.ChunkID = savedChunk.ID

	// 7. Emit event
	if w.Events != nil {
		w.Events.Emit(string(core.EventChunkCommitted), map[string]any{
			"chunk_id":         string(savedChunk.ID),
			"content_hash":     contentHash,
			"source_principal": req.SourcePrincipal.ID,
			"source_origin":    req.SourceOrigin,
			"trust_class":      trustClass,
		})
	}

	// 8. Write audit record
	w.writeAuditRecord(req, result, "successfully committed")

	return result, nil
}

// PersistBatch persists multiple artifacts with per-item error collection.
func (w *Writer) PersistBatch(ctx context.Context, reqs []PersistenceRequest) ([]PersistenceResult, error) {
	results := make([]PersistenceResult, len(reqs))

	for i, req := range reqs {
		result, err := w.Persist(ctx, req)
		if err != nil {
			results[i] = PersistenceResult{
				Action: ActionRejected,
				Error:  err,
			}
		} else {
			results[i] = *result
		}
	}

	return results, nil
}

// PromoteFromMemory promotes content from working memory to persistent storage.
func (w *Writer) PromoteFromMemory(ctx context.Context, store WorkingMemoryStore, reqs []PromotionRequest) error {
	for _, req := range reqs {
		content, found := store.Get(req.Key)
		if !found {
			continue // Skip missing entries
		}

		// Convert promotion request to persistence request
		persistReq := PersistenceRequest{
			Content:              content,
			ContentType:          req.ContentType,
			SourcePrincipal:      req.SourcePrincipal,
			SourceOrigin:         req.SourceOrigin,
			Reason:               req.Reason,
			Tags:                 req.Tags,
			DerivedFrom:          req.DerivedFrom,
			DerivationMethod:     req.DerivationMethod,
			DerivationGeneration: req.DerivationGeneration,
		}

		_, err := w.Persist(ctx, persistReq)
		if err != nil {
			// Log error but continue processing other requests
			// In production, this might want different error handling
			continue
		}
	}

	return nil
}

// validateRequest performs structural validation on the request.
func (w *Writer) validateRequest(req PersistenceRequest) error {
	// Check required fields
	if len(req.Content) == 0 {
		return fmt.Errorf("content is required")
	}
	if req.ContentType == "" {
		return fmt.Errorf("content_type is required")
	}
	if req.SourcePrincipal.ID == "" {
		return fmt.Errorf("source_principal is required")
	}

	// Check max content size from policy
	if w.Policy != nil && w.Policy.Quota.MaxTokensPerWindow > 0 {
		// Rough estimate: 1 token ≈ 4 bytes for text
		estimatedTokens := len(req.Content) / 4
		if estimatedTokens > w.Policy.Quota.MaxTokensPerWindow {
			return fmt.Errorf("content exceeds max size: %d tokens estimated", estimatedTokens)
		}
	}

	return nil
}

// determineTrustClass determines the trust class from source principal.
func (w *Writer) determineTrustClass(principal identity.SubjectRef) agentspec.TrustClass {
	if w.Policy != nil && w.Policy.DefaultTrustClass != "" {
		return w.Policy.DefaultTrustClass
	}
	return agentspec.TrustClassWorkspaceTrusted
}

// suspicionCheck performs lightweight suspicion detection.
func (w *Writer) suspicionCheck(req PersistenceRequest) (bool, string) {
	// Check for obviously suspicious patterns (lightweight version)
	content := string(req.Content)

	// Check for null bytes (binary content)
	for _, b := range req.Content {
		if b == 0 {
			return true, "binary content detected"
		}
	}

	// Check for non-printable character ratio
	nonPrintable := 0
	for _, r := range content {
		if r < 32 && r != '\n' && r != '\r' && r != '\t' {
			nonPrintable++
		}
	}
	if len(content) > 0 && float64(nonPrintable)/float64(len(content)) > 0.1 {
		return true, "high non-printable character ratio"
	}

	return false, ""
}

// computeContentHash computes a content hash for deduplication.
func (w *Writer) computeContentHash(content []byte) string {
	hash := sha256.Sum256(content)
	return fmt.Sprintf("%x", hash[:16]) // Use first 16 bytes
}

// checkDuplicate checks if a chunk with the same content hash and source origin exists.
func (w *Writer) checkDuplicate(contentHash string, sourceOrigin knowledge.SourceOrigin) knowledge.ChunkID {
	// In a real implementation, this would query the ChunkStore
	// For now, return empty string (no duplicate found)
	// This is a placeholder - actual implementation would search by content hash
	return ""
}

// buildChunk builds a KnowledgeChunk from a persistence request.
func (w *Writer) buildChunk(req PersistenceRequest, trustClass agentspec.TrustClass, contentHash string) *knowledge.KnowledgeChunk {
	return &knowledge.KnowledgeChunk{
		ID:                   knowledge.ChunkID(fmt.Sprintf("chunk_%d", time.Now().UnixNano())),
		ContentHash:          contentHash,
		SourceOrigin:         req.SourceOrigin,
		SourcePrincipal:      req.SourcePrincipal,
		AcquisitionMethod:    knowledge.AcquisitionMethodRuntimeWrite,
		AcquiredAt:           time.Now().UTC(),
		TrustClass:           trustClass,
		DerivedFrom:          req.DerivedFrom,
		DerivationMethod:     knowledge.DerivationMethod(req.DerivationMethod),
		DerivationGeneration: req.DerivationGeneration,
		Body: knowledge.ChunkBody{
			Raw: string(req.Content),
			Fields: map[string]any{
				"content_type": req.ContentType,
				"tags":         req.Tags,
				"reason":       req.Reason,
			},
		},
	}
}

// writeAuditRecord writes an audit record for the operation.
func (w *Writer) writeAuditRecord(req PersistenceRequest, result *PersistenceResult, reason string) {
	record := PersistenceAuditRecord{
		AuditID:         w.generateAuditID(),
		Action:          result.Action,
		ChunkID:         result.ChunkID,
		SourcePrincipal: req.SourcePrincipal,
		SourceOrigin:    req.SourceOrigin,
		Reason:          reason,
		CreatedAt:       time.Now().UTC(),
	}

	if w.Policy != nil {
		record.TrustClass = w.Policy.DefaultTrustClass
	}

	w.AuditLog = append(w.AuditLog, record)
}

// generateAuditID generates a unique audit ID.
func (w *Writer) generateAuditID() string {
	return fmt.Sprintf("audit_%d_%d", time.Now().UnixNano(), len(w.AuditLog))
}

// NewWriter creates a new persistence writer.
func NewWriter(store *knowledge.ChunkStore, events EventLog, policy *contextpolicy.ContextPolicyBundle) *Writer {
	return &Writer{
		Store:     store,
		Events:    events,
		Policy:    policy,
		Evaluator: contextpolicy.NewEvaluator(policy),
		AuditLog:  make([]PersistenceAuditRecord, 0),
	}
}

// GetAuditLog returns the audit log.
func (w *Writer) GetAuditLog() []PersistenceAuditRecord {
	return w.AuditLog
}

// ClearAuditLog clears the audit log.
func (w *Writer) ClearAuditLog() {
	w.AuditLog = make([]PersistenceAuditRecord, 0)
}
