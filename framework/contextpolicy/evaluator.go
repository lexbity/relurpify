package contextpolicy

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/framework/knowledge"
	"codeburg.org/lexbit/relurpify/relurpnet/identity"
)

// Evaluator provides admission decisions based on context policy.
type Evaluator struct {
	bundle *ContextPolicyBundle
	// quotaCounters tracks quota usage per principal pattern
	quotaCounters sync.Map // map[string]*atomic.Int64
}

// NewEvaluator creates a new policy evaluator from a compiled bundle.
func NewEvaluator(bundle *ContextPolicyBundle) *Evaluator {
	return &Evaluator{
		bundle: bundle,
	}
}

// AdmitTrustClass determines if a chunk with the given trust class should be admitted.
func (e *Evaluator) AdmitTrustClass(trustClass agentspec.TrustClass) (bool, string) {
	if e.bundle == nil {
		return true, "" // No policy, admit everything
	}

	// Check if trust class is at or above minimum
	if trustClassRank(trustClass) < trustClassRank(e.bundle.DefaultTrustClass) {
		switch e.bundle.TrustDemotedPolicy {
		case TrustDemotedPolicyReject:
			return false, fmt.Sprintf("trust class %s below minimum %s", trustClass, e.bundle.DefaultTrustClass)
		case TrustDemotedPolicyQuarantine:
			return true, fmt.Sprintf("trust class %s quarantined", trustClass)
		case TrustDemotedPolicyWarn:
			return true, fmt.Sprintf("trust class %s warning", trustClass)
		}
	}

	return true, ""
}

// AdmitChunk determines if a chunk should be admitted based on suspicion scores and other checks.
func (e *Evaluator) AdmitChunk(chunk *knowledge.KnowledgeChunk) (bool, string) {
	if e.bundle == nil {
		return true, ""
	}

	// Check suspicion score
	if chunk.SuspicionScore > 0.7 { // High suspicion threshold
		switch e.bundle.DegradedChunkPolicy {
		case DegradedChunkPolicyDrop:
			return false, fmt.Sprintf("suspicion score %.2f exceeds threshold", chunk.SuspicionScore)
		case DegradedChunkPolicyStale:
			// Mark as stale but admit
			return true, "marked stale due to suspicion"
		case DegradedChunkPolicyAccept:
			// Accept anyway
		}
	}

	// Check tombstone status
	if chunk.Tombstoned {
		return false, "chunk is tombstoned"
	}

	return true, ""
}

// QuotaRemaining returns the remaining quota for a principal.
func (e *Evaluator) QuotaRemaining(principal identity.SubjectRef) (int, int) {
	if e.bundle == nil || e.bundle.Quota.MaxChunksPerWindow == 0 {
		return -1, -1 // Unlimited
	}

	key := fmt.Sprintf(e.bundle.Quota.PrincipalPattern, principal)

	// Get or create counter
	counterRaw, _ := e.quotaCounters.LoadOrStore(key, &atomic.Int64{})
	counter := counterRaw.(*atomic.Int64)

	used := int(counter.Load())
	remainingChunks := e.bundle.Quota.MaxChunksPerWindow - used
	if remainingChunks < 0 {
		remainingChunks = 0
	}

	// Estimate tokens (simplified - in real impl would track actual tokens)
	remainingTokens := e.bundle.Quota.MaxTokensPerWindow - (used * 100) // rough estimate
	if remainingTokens < 0 {
		remainingTokens = 0
	}

	return remainingChunks, remainingTokens
}

// ConsumeQuota decrements quota for a principal.
func (e *Evaluator) ConsumeQuota(principal identity.SubjectRef, chunks int, tokens int) bool {
	if e.bundle == nil || e.bundle.Quota.MaxChunksPerWindow == 0 {
		return true // Unlimited
	}

	key := fmt.Sprintf(e.bundle.Quota.PrincipalPattern, principal)
	counterRaw, _ := e.quotaCounters.LoadOrStore(key, &atomic.Int64{})
	counter := counterRaw.(*atomic.Int64)

	current := int(counter.Load())
	if current+chunks > e.bundle.Quota.MaxChunksPerWindow {
		return false // Quota exceeded
	}

	counter.Add(int64(chunks))
	return true
}

// ResetQuota resets quota counters (should be called on window tick).
func (e *Evaluator) ResetQuota() {
	e.quotaCounters.Range(func(key, value interface{}) bool {
		if counter, ok := value.(*atomic.Int64); ok {
			counter.Store(0)
		}
		return true
	})
}

// PermitSummarization determines if summarization is permitted for the given content type.
func (e *Evaluator) PermitSummarizer(contentType string) (bool, *SummarizerRef) {
	if e.bundle == nil {
		return false, nil
	}

	for _, s := range e.bundle.Summarizers {
		// Simple matching - in real impl would check content type compatibility
		if s.ID != "" {
			return true, &s
		}
	}

	return false, nil
}

// trustClassRank returns a numeric rank for trust class comparison.
func trustClassRank(tc agentspec.TrustClass) int {
	switch tc {
	case agentspec.TrustClassBuiltinTrusted:
		return 4
	case agentspec.TrustClassWorkspaceTrusted:
		return 3
	case agentspec.TrustClassRemoteApproved:
		return 2
	case agentspec.TrustClassRemoteDeclared:
		return 1
	case agentspec.TrustClassProviderLocalUntrusted:
		return 0
	default:
		return 0
	}
}

// CheckRateLimit checks if a request should be rate limited.
func (e *Evaluator) CheckRateLimit(principal identity.SubjectRef) bool {
	if e.bundle == nil || e.bundle.RateLimit.RequestsPerSecond == 0 {
		return true // No rate limiting
	}

	// Simplified rate limiting - in real impl would use token bucket
	return true
}

// GetBundle returns the underlying policy bundle.
func (e *Evaluator) GetBundle() *ContextPolicyBundle {
	return e.bundle
}

// StartQuotaResetTicker starts a ticker to reset quotas periodically.
func (e *Evaluator) StartQuotaResetTicker(ctx context.Context) {
	if e.bundle == nil || e.bundle.Quota.WindowSize == 0 {
		return
	}

	ticker := time.NewTicker(e.bundle.Quota.WindowSize)
	go func() {
		for {
			select {
			case <-ctx.Done():
				ticker.Stop()
				return
			case <-ticker.C:
				e.ResetQuota()
			}
		}
	}()
}
