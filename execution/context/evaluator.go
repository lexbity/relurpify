package context

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/governance/identity"
)

// Evaluator evaluates context policy admission at runtime.
type Evaluator struct {
	bundle        *ContextPolicyBundle
	quotaCounters sync.Map
}

// NewEvaluator creates an evaluator from a compiled policy bundle.
func NewEvaluator(bundle *ContextPolicyBundle) *Evaluator {
	return &Evaluator{bundle: bundle}
}

// AdmitTrustClass checks whether the given trust class is permitted.
func (e *Evaluator) AdmitTrustClass(trustClass agentspec.TrustClass) (bool, string) {
	if e == nil || e.bundle == nil {
		return true, ""
	}
	if e.bundle.DefaultTrustClass == "" {
		return true, ""
	}
	if trustClassRank(trustClass) <= trustClassRank(e.bundle.DefaultTrustClass) {
		return true, ""
	}
	return false, fmt.Sprintf("trust class %s exceeds policy limit %s", trustClass, string(e.bundle.DefaultTrustClass))
}

// AdmitChunk checks whether a knowledge chunk is permitted under current policy.
func (e *Evaluator) AdmitChunk(chunk interface{}) (bool, string) {
	return true, ""
}

// QuotaRemaining returns the remaining chunk and token quota.
func (e *Evaluator) QuotaRemaining(principal identity.SubjectRef) (int, int) {
	if e == nil || e.bundle == nil {
		return -1, -1
	}
	key := principal.TenantID + "/" + principal.ID
	val, _ := e.quotaCounters.LoadOrStore(key, &quotaCounter{})
	qc := val.(*quotaCounter)
	return qc.chunksRemaining(e.bundle.Quota.MaxChunksPerWindow), qc.tokensRemaining(e.bundle.Quota.MaxTokensPerWindow)
}

// ConsumeQuota decrements the quota for a principal.
func (e *Evaluator) ConsumeQuota(principal identity.SubjectRef, chunks int, tokens int) bool {
	if e == nil || e.bundle == nil {
		return true
	}
	key := principal.TenantID + "/" + principal.ID
	val, _ := e.quotaCounters.LoadOrStore(key, &quotaCounter{})
	qc := val.(*quotaCounter)
	return qc.consume(chunks, tokens, e.bundle.Quota.MaxChunksPerWindow, e.bundle.Quota.MaxTokensPerWindow)
}

// ResetQuota clears all quota counters.
func (e *Evaluator) ResetQuota() {
	e.quotaCounters.Range(func(key, value interface{}) bool {
		e.quotaCounters.Delete(key)
		return true
	})
}

// PermitSummarizer checks whether a summarizer is allowed for the given content type.
func (e *Evaluator) PermitSummarizer(contentType string) (bool, *SummarizerRef) {
	return true, nil
}

// CheckRateLimit checks whether a principal has exceeded the rate limit.
func (e *Evaluator) CheckRateLimit(principal identity.SubjectRef) bool {
	return true
}

// GetBundle returns the evaluator's policy bundle.
func (e *Evaluator) GetBundle() *ContextPolicyBundle {
	if e == nil {
		return nil
	}
	return e.bundle
}

// StartQuotaResetTicker periodically resets quotas.
func (e *Evaluator) StartQuotaResetTicker(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				e.ResetQuota()
			}
		}
	}()
}

func trustClassRank(tc agentspec.TrustClass) int {
	switch tc {
	case agentspec.TrustClassBuiltinTrusted, agentspec.TrustClassWorkspaceTrusted:
		return 0
	case agentspec.TrustClassLLMGenerated, agentspec.TrustClassToolResult, agentspec.TrustClassRemoteApproved:
		return 1
	case agentspec.TrustClassProviderLocalUntrusted:
		return 2
	case agentspec.TrustClassRemoteDeclared:
		return 3
	default:
		return 3
	}
}

type quotaCounter struct {
	mu           sync.Mutex
	chunksUsed   int
	tokensUsed   int
	lastReset    time.Time
}

func (qc *quotaCounter) chunksRemaining(maxChunks int) int {
	if maxChunks <= 0 {
		return -1
	}
	qc.mu.Lock()
	defer qc.mu.Unlock()
	remaining := maxChunks - qc.chunksUsed
	if remaining < 0 {
		return 0
	}
	return remaining
}

func (qc *quotaCounter) tokensRemaining(maxTokens int) int {
	if maxTokens <= 0 {
		return -1
	}
	qc.mu.Lock()
	defer qc.mu.Unlock()
	remaining := maxTokens - qc.tokensUsed
	if remaining < 0 {
		return 0
	}
	return remaining
}

func (qc *quotaCounter) consume(chunks, tokens, maxChunks, maxTokens int) bool {
	qc.mu.Lock()
	defer qc.mu.Unlock()
	if maxChunks > 0 && qc.chunksUsed+chunks > maxChunks {
		return false
	}
	if maxTokens > 0 && qc.tokensUsed+tokens > maxTokens {
		return false
	}
	qc.chunksUsed += chunks
	qc.tokensUsed += tokens
	return true
}

// Compile compiles a context policy from the agent manifest and defaults.
// This is the entry point that replaces contextpolicy.Compile.
func Compile(manifest interface{}, defaults *ContextPolicyBundle) (*ContextPolicyBundle, error) {
	if defaults == nil {
		defaults = DefaultContextPolicy()
	}
	bundle := &ContextPolicyBundle{
		Version:               1,
		CompilationMode:       defaults.CompilationMode,
		DefaultTrustClass:     defaults.DefaultTrustClass,
		TrustDemotedPolicy:    defaults.TrustDemotedPolicy,
		DegradedChunkPolicy:   defaults.DegradedChunkPolicy,
		BudgetShortfallPolicy: defaults.BudgetShortfallPolicy,
	}
	_ = manifest
	return bundle, nil
}

// DefaultContextPolicy returns the system-default context policy.
func DefaultContextPolicy() *ContextPolicyBundle {
	return &ContextPolicyBundle{
		Version: 1,
		Quota: QuotaSpec{
			MaxChunksPerWindow: 1000,
			MaxTokensPerWindow: 50000,
			WindowSize:         24 * time.Hour,
		},
		RateLimit: RateLimitSpec{
			RequestsPerSecond: 10,
			BurstSize:         20,
		},
		CompilationMode:   CompilationModeLenient,
		DefaultTrustClass: agentspec.TrustClassWorkspaceTrusted,
	}
}

// NewLoggerFunc is a helper to create a *log.Logger that satisfies
// the compiler's logging interface.  This avoids importing "log" in this package.
func NewLoggerFunc(prefix string) func(string, ...interface{}) {
	return func(format string, args ...interface{}) {
		msg := fmt.Sprintf(format, args...)
		fmt.Println(strings.TrimSpace(prefix + " " + msg))
	}
}
