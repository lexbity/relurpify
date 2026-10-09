package persistence

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/context/knowledge"
	contextports "codeburg.org/lexbit/relurpify/context/ports"
	"codeburg.org/lexbit/relurpify/governance/classification"
	"codeburg.org/lexbit/relurpify/governance/identity"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

// Persist persists a single artifact to the knowledge store.
// Admission path: structural validation → trust class assignment → suspicion check → quota check → commit.
//
// A rejected, quarantined, or failed admission returns a non-nil error together
// with the populated result so callers cannot mistake a rejection for success.
func (w *Writer) Persist(ctx context.Context, req PersistenceRequest) (*PersistenceResult, error) {
	result := &PersistenceResult{}

	// 1. Structural validation
	if err := w.validateRequest(req); err != nil {
		result.Action = ActionRejected
		result.Error = fmt.Errorf("validation failed: %w", err)
		w.writeAuditRecord(req, result, "structural validation failed")
		return result, result.Error
	}

	// 2. Trust class assignment from source principal
	trustClass := w.determineTrustClass(req.SourcePrincipal)

	// 3. Suspicion check (lightweight)
	if suspicious, reason := w.suspicionCheck(req); suspicious {
		result.Action = ActionQuarantined
		result.Error = fmt.Errorf("suspicion check failed: %s", reason)
		w.writeAuditRecord(req, result, reason)
		return result, result.Error
	}

	// 4. Quota check
	if w.Evaluator != nil {
		remaining := w.Evaluator.QuotaRemaining(req.SourcePrincipal.ID)
		if remaining == 0 {
			result.Action = ActionQuarantined
			result.Error = fmt.Errorf("quota exceeded")
			w.writeAuditRecord(req, result, "quota exceeded")
			return result, result.Error
		}
	}

	// 5. Deduplication
	contentHash := w.computeContentHash(req.Content)

	// 6. Build and commit chunk
	chunk, alreadyExisted, err := w.buildChunk(ctx, req, trustClass, contentHash)
	if err != nil {
		result.Action = ActionRejected
		result.Error = fmt.Errorf("commit failed: %w", err)
		w.writeAuditRecord(req, result, "commit failed")
		return result, result.Error
	}
	savedChunk, err := w.Store.Save(ctx, *chunk)
	if err != nil {
		result.Action = ActionRejected
		result.Error = fmt.Errorf("commit failed: %w", err)
		w.writeAuditRecord(req, result, "commit failed")
		return result, result.Error
	}

	if savedChunk.Tombstoned {
		// Save preserved a retraction: the content matched a tombstoned chunk
		// with equal-or-older derivation. Nothing was committed.
		result.Action = ActionUpdated
		result.ChunkID = savedChunk.ID
		if w.Events != nil {
			w.Events.Emit(string(telemetry.EventTombstonePreserved), map[string]any{
				"chunk_id":     string(savedChunk.ID),
				"content_hash": contentHash,
				"origin":       "persistence",
			})
		}
		w.writeAuditRecord(req, result, "tombstone preserved")
		return result, nil
	}

	if alreadyExisted {
		result.Action = ActionUpdated
	} else {
		result.Action = ActionCreated
	}
	result.ChunkID = savedChunk.ID

	// 7. Emit event (only for a genuinely new chunk; dedup is not a commit)
	if w.Events != nil && !alreadyExisted {
		w.Events.Emit(string(telemetry.EventChunkCommitted), map[string]any{
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

// PersistBatch persists multiple artifacts with per-item error collection. It
// returns the per-item results and a non-nil error when any item was rejected.
func (w *Writer) PersistBatch(ctx context.Context, reqs []PersistenceRequest) ([]PersistenceResult, error) {
	results := make([]PersistenceResult, len(reqs))
	rejections := 0
	for i, req := range reqs {
		result, err := w.Persist(ctx, req)
		switch {
		case result != nil:
			results[i] = *result
		case err != nil:
			results[i] = PersistenceResult{Action: ActionRejected, Error: err}
		}
		if err != nil {
			rejections++
		}
	}
	if rejections > 0 {
		return results, fmt.Errorf("persist batch had %d rejections", rejections)
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
			Kind:                 req.Kind,
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
	if !req.Kind.Valid() {
		return fmt.Errorf("kind is required and must be one of the canonical chunk kinds")
	}
	if req.ContentType == "" {
		return fmt.Errorf("content_type is required")
	}
	if req.SourcePrincipal.ID == "" {
		return fmt.Errorf("source_principal is required")
	}

	// Check max content size from policy
	if w.Policy != nil && w.Policy.MaxTokensPerWindow > 0 {
		// Rough estimate: 1 token ≈ 4 bytes for text
		estimatedTokens := len(req.Content) / 4
		if estimatedTokens > w.Policy.MaxTokensPerWindow {
			return fmt.Errorf("content exceeds max size: %d tokens estimated", estimatedTokens)
		}
	}

	return nil
}

// determineTrustClass determines the trust class from source principal.
func (w *Writer) determineTrustClass(principal identity.SubjectRef) agentspec.TrustClass {
	if w.Policy != nil && w.Policy.DefaultTrustClass != "" {
		return agentspec.TrustClass(classification.NormalizeClassString(w.Policy.DefaultTrustClass))
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

// auditLogCap bounds the in-memory audit ring so a long-lived writer cannot
// grow without limit.
const auditLogCap = 1024

// buildChunk builds a KnowledgeChunk from a persistence request, reusing the
// identity of an existing live chunk when the content hash already matches.
// The returned bool reports whether the content was already present.
func (w *Writer) buildChunk(ctx context.Context, req PersistenceRequest, trustClass agentspec.TrustClass, contentHash string) (*knowledge.KnowledgeChunk, bool, error) {
	chunk := &knowledge.KnowledgeChunk{
		ID:                   knowledge.CanonicalChunkID(req.Kind, req.Content),
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
	existing, err := w.Store.FindByContentHash(contentHash)
	if err != nil {
		return nil, false, err
	}
	if len(existing) == 0 {
		return chunk, false, nil
	}
	// Route through the same identity the store already holds so the merge rule
	// in ChunkStore.Save is the only write semantics in the tree.
	chunk.ID = existing[0].ID
	chunk.Version = existing[0].Version
	chunk.CreatedAt = existing[0].CreatedAt
	chunk.Freshness = existing[0].Freshness
	return chunk, true, nil
}

// writeAuditRecord appends to the bounded audit ring under its mutex.
func (w *Writer) writeAuditRecord(req PersistenceRequest, result *PersistenceResult, reason string) {
	w.auditMu.Lock()
	defer w.auditMu.Unlock()
	w.auditSeq++
	record := PersistenceAuditRecord{
		AuditID:         fmt.Sprintf("audit_%d_%d", time.Now().UnixNano(), w.auditSeq),
		Action:          result.Action,
		ChunkID:         result.ChunkID,
		SourcePrincipal: req.SourcePrincipal,
		SourceOrigin:    req.SourceOrigin,
		Reason:          reason,
		CreatedAt:       time.Now().UTC(),
	}

	if w.Policy != nil {
		record.TrustClass = agentspec.TrustClass(classification.NormalizeClassString(w.Policy.DefaultTrustClass))
	}

	if len(w.AuditLog) < auditLogCap {
		w.AuditLog = append(w.AuditLog, record)
		return
	}
	w.AuditLog[w.auditNext] = record
	w.auditNext = (w.auditNext + 1) % auditLogCap
}

// NewWriter creates a new persistence writer.
func NewWriter(store *knowledge.ChunkStore, events EventLog, policy *contextports.PolicyBundle, evaluator contextports.PolicyEvaluator) *Writer {
	return &Writer{
		Store:     store,
		Events:    events,
		Policy:    policy,
		Evaluator: evaluator,
		AuditLog:  make([]PersistenceAuditRecord, 0, auditLogCap),
	}
}

// GetAuditLog returns a copy of the audit ring.
func (w *Writer) GetAuditLog() []PersistenceAuditRecord {
	w.auditMu.Lock()
	defer w.auditMu.Unlock()
	out := make([]PersistenceAuditRecord, len(w.AuditLog))
	copy(out, w.AuditLog)
	return out
}

// ClearAuditLog clears the audit ring.
func (w *Writer) ClearAuditLog() {
	w.auditMu.Lock()
	defer w.auditMu.Unlock()
	w.AuditLog = make([]PersistenceAuditRecord, 0, auditLogCap)
	w.auditNext = 0
	w.auditSeq = 0
}
