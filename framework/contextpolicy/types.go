// Package contextpolicy implements the context policy bundle compiler.
// This is the admission authority for both ingestion and persistence.
package contextpolicy

import (
	"time"

	"codeburg.org/lexbit/relurpify/framework/agentspec"
)

// ContextPolicyBundle is the compiled, runtime-facing policy configuration.
type ContextPolicyBundle struct {
	Version               int
	CompilationMode       CompilationMode
	DefaultTrustClass     agentspec.TrustClass
	Rankers               []RankerRef
	Scanners              []ScannerRef
	Summarizers           []SummarizerRef
	Quota                 QuotaSpec
	RateLimit             RateLimitSpec
	TrustDemotedPolicy    TrustDemotedPolicy
	DegradedChunkPolicy   DegradedChunkPolicy
	BudgetShortfallPolicy BudgetShortfallPolicy
	SubstitutionPrefs     []SubstitutionPreference

	// ContextAccessRules defines which tiers (streamed, working, retrieval) can be
	// accessed by which node types and in what mode (read/write).
	ContextAccessRules []ContextAccessRule

	// SkillContributions contains merged contributions from resolved skills.
	SkillContributions SkillContributions
}

// SkillContributions stores merged skill contributions to context policy.
type SkillContributions struct {
	// AdmittedRankers is the union of ranker IDs admitted by skills.
	AdmittedRankers []string
	// ScannerSignatures contains additional signatures contributed by skills.
	ScannerSignatures []ScannerSignature
	// IngestionSources contains paths from skills that should be scanned.
	IngestionSources []IngestionSource
}

// ScannerSignature defines a pattern-based signature for scanning.
type ScannerSignature struct {
	Pattern string
	Flag    string
}

// IngestionSource defines a path pattern for skill resource ingestion.
type IngestionSource struct {
	Path       string
	SourceType string
}

// CompilationMode indicates how the policy bundle was compiled.
type CompilationMode string

const (
	CompilationModeStrict   CompilationMode = "strict"
	CompilationModeLenient  CompilationMode = "lenient"
	CompilationModeFallback CompilationMode = "fallback"
)

// RankerRef references a registered ranker capability.
type RankerRef struct {
	ID       string
	Priority int
	Config   map[string]any
}

// ScannerRef references a registered scanner capability.
type ScannerRef struct {
	ID       string
	Priority int
	Config   map[string]any
}

// SummarizerRef references a registered summarizer capability.
type SummarizerRef struct {
	ID          string
	ModelRef    string
	ProseConfig map[string]any
	CodeConfig  map[string]any
}

// QuotaSpec defines quota limits.
type QuotaSpec struct {
	WindowSize         time.Duration
	MaxChunksPerWindow int
	MaxTokensPerWindow int
	PrincipalPattern   string // e.g., "tenant:{tenant_id}"
}

// RateLimitSpec defines rate limiting configuration.
type RateLimitSpec struct {
	RequestsPerSecond float64
	BurstSize         int
}

// TrustDemotedPolicy determines handling when trust class is demoted.
type TrustDemotedPolicy string

const (
	TrustDemotedPolicyReject     TrustDemotedPolicy = "reject"
	TrustDemotedPolicyQuarantine TrustDemotedPolicy = "quarantine"
	TrustDemotedPolicyWarn       TrustDemotedPolicy = "warn"
)

// DegradedChunkPolicy determines handling when chunk quality degrades.
type DegradedChunkPolicy string

const (
	DegradedChunkPolicyDrop   DegradedChunkPolicy = "drop"
	DegradedChunkPolicyStale  DegradedChunkPolicy = "stale"
	DegradedChunkPolicyAccept DegradedChunkPolicy = "accept"
)

// BudgetShortfallPolicy determines handling when budget is exceeded.
type BudgetShortfallPolicy string

const (
	BudgetShortfallPolicyReject    BudgetShortfallPolicy = "reject"
	BudgetShortfallPolicyEvict     BudgetShortfallPolicy = "evict"
	BudgetShortfallPolicySummarize BudgetShortfallPolicy = "summarize"
)

// SubstitutionPreference defines how to substitute content.
type SubstitutionPreference struct {
	SourceContentType string
	TargetContentType string
	Strategy          SubstitutionStrategy
}

// SubstitutionStrategy determines substitution behavior.
type SubstitutionStrategy string

const (
	SubstitutionStrategyInline    SubstitutionStrategy = "inline"
	SubstitutionStrategyReference SubstitutionStrategy = "reference"
	SubstitutionStrategySummarize SubstitutionStrategy = "summarize"
)

// ContextAccessTier indicates which context tier is being accessed.
type ContextAccessTier string

const (
	// TierStreamedContext is read-only streamed context (managed by compiler).
	TierStreamedContext ContextAccessTier = "streamed"
	// TierWorkingMemory is mutable working memory (graph nodes can write).
	TierWorkingMemory ContextAccessTier = "working_memory"
	// TierRetrieval is controlled retrieval state.
	TierRetrieval ContextAccessTier = "retrieval"
)

// ContextAccessRule defines access permissions for a context tier.
type ContextAccessRule struct {
	Tier       ContextAccessTier
	AccessMode AccessMode
	NodeTypes  []string // Node type patterns allowed (empty = all)
}

// AccessMode indicates how a tier can be accessed.
type AccessMode string

const (
	AccessModeReadOnly  AccessMode = "read_only"
	AccessModeWriteOnly AccessMode = "write_only"
	AccessModeReadWrite AccessMode = "read_write"
)

// ContextPolicy defines the context policy section in a manifest.
type ContextPolicy struct {
	CompilationMode       CompilationMode          `json:"compilation_mode,omitempty"`
	DefaultTrustClass     agentspec.TrustClass     `json:"default_trust_class,omitempty"`
	Rankers               []RankerRef              `json:"rankers,omitempty"`
	Scanners              []ScannerRef             `json:"scanners,omitempty"`
	Summarizers           []SummarizerRef          `json:"summarizers,omitempty"`
	Quota                 *QuotaSpec               `json:"quota,omitempty"`
	RateLimit             *RateLimitSpec           `json:"rate_limit,omitempty"`
	TrustDemotedPolicy    TrustDemotedPolicy       `json:"trust_demoted_policy,omitempty"`
	DegradedChunkPolicy   DegradedChunkPolicy      `json:"degraded_chunk_policy,omitempty"`
	BudgetShortfallPolicy BudgetShortfallPolicy    `json:"budget_shortfall_policy,omitempty"`
	SubstitutionPrefs     []SubstitutionPreference `json:"substitution_preferences,omitempty"`
}

// DefaultContextPolicy returns the system default context policy.
func DefaultContextPolicy() *ContextPolicy {
	return &ContextPolicy{
		CompilationMode:       CompilationModeStrict,
		DefaultTrustClass:     agentspec.TrustClassBuiltinTrusted,
		TrustDemotedPolicy:    TrustDemotedPolicyQuarantine,
		DegradedChunkPolicy:   DegradedChunkPolicyStale,
		BudgetShortfallPolicy: BudgetShortfallPolicySummarize,
		Quota: &QuotaSpec{
			WindowSize:         time.Hour,
			MaxChunksPerWindow: 10000,
			MaxTokensPerWindow: 1000000,
			PrincipalPattern:   "tenant:{tenant_id}",
		},
		RateLimit: &RateLimitSpec{
			RequestsPerSecond: 10.0,
			BurstSize:         100,
		},
	}
}
