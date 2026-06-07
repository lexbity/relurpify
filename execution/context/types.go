package context

import (
	"time"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
)

// ContextPolicyBundle is the compiled runtime context policy.  It governs how
// ingestion, persistence, retrieval, and the compiler treat context material.
type ContextPolicyBundle struct {
	Version               int                      `json:"version,omitempty"`
	CompilationMode       CompilationMode          `json:"compilation_mode,omitempty"`
	DefaultTrustClass     agentspec.TrustClass     `json:"default_trust_class,omitempty"`
	Rankers               []RankerRef              `json:"rankers,omitempty"`
	Scanners              []ScannerRef             `json:"scanners,omitempty"`
	Summarizers           []SummarizerRef          `json:"summarizers,omitempty"`
	Quota                 QuotaSpec                `json:"quota,omitempty"`
	RateLimit             RateLimitSpec            `json:"rate_limit,omitempty"`
	TrustDemotedPolicy    TrustDemotedPolicy       `json:"trust_demoted_policy,omitempty"`
	DegradedChunkPolicy   DegradedChunkPolicy      `json:"degraded_chunk_policy,omitempty"`
	BudgetShortfallPolicy BudgetShortfallPolicy    `json:"budget_shortfall_policy,omitempty"`
	SubstitutionPrefs     []SubstitutionPreference `json:"substitution_prefs,omitempty"`
	ContextAccessRules    []ContextAccessRule      `json:"context_access_rules,omitempty"`
	SkillContributions    SkillContributions       `json:"skill_contributions,omitempty"`
}

// SkillContributions records what the skill system contributed.
type SkillContributions struct {
	AdmittedRankers   []string           `json:"admitted_rankers,omitempty"`
	ScannerSignatures []ScannerSignature `json:"scanner_signatures,omitempty"`
	IngestionSources  []IngestionSource  `json:"ingestion_sources,omitempty"`
}

// ScannerSignature identifies a scanner by its pattern and flags.
type ScannerSignature struct {
	Pattern string `json:"pattern,omitempty"`
	Flag    string `json:"flag,omitempty"`
}

// IngestionSource records a path and its source type.
type IngestionSource struct {
	Path       string `json:"path,omitempty"`
	SourceType string `json:"source_type,omitempty"`
}

// CompilationMode controls strictness of context compilation.
type CompilationMode string

const (
	CompilationModeStrict   CompilationMode = "strict"
	CompilationModeLenient  CompilationMode = "lenient"
	CompilationModeFallback CompilationMode = "fallback"
)

// RankerRef references a ranker implementation by identifier.
type RankerRef struct {
	ID       string         `json:"id,omitempty"`
	Priority int            `json:"priority,omitempty"`
	Config   map[string]any `json:"config,omitempty"`
}

// ScannerRef references a scanner implementation by identifier.
type ScannerRef struct {
	ID       string         `json:"id,omitempty"`
	Priority int            `json:"priority,omitempty"`
	Config   map[string]any `json:"config,omitempty"`
}

// SummarizerRef references a summarizer implementation by identifier.
type SummarizerRef struct {
	ID          string         `json:"id,omitempty"`
	ModelRef    string         `json:"model_ref,omitempty"`
	ProseConfig map[string]any `json:"prose_config,omitempty"`
	CodeConfig  map[string]any `json:"code_config,omitempty"`
}

// QuotaSpec configures per-window context quotas.
type QuotaSpec struct {
	WindowSize         time.Duration `json:"window_size,string,omitempty"`
	MaxChunksPerWindow int           `json:"max_chunks_per_window,omitempty"`
	MaxTokensPerWindow int           `json:"max_tokens_per_window,omitempty"`
	PrincipalPattern   string        `json:"principal_pattern,omitempty"`
}

// RateLimitSpec configures per-second rate limiting.
type RateLimitSpec struct {
	RequestsPerSecond float64 `json:"requests_per_second,omitempty"`
	BurstSize         int     `json:"burst_size,omitempty"`
}

// TrustDemotedPolicy controls behaviour when trust is demoted.
type TrustDemotedPolicy string

const (
	TrustDemotedPolicyReject     TrustDemotedPolicy = "reject"
	TrustDemotedPolicyQuarantine TrustDemotedPolicy = "quarantine"
	TrustDemotedPolicyWarn       TrustDemotedPolicy = "warn"
)

// DegradedChunkPolicy controls behaviour for degraded chunks.
type DegradedChunkPolicy string

const (
	DegradedChunkPolicyDrop   DegradedChunkPolicy = "drop"
	DegradedChunkPolicyStale  DegradedChunkPolicy = "stale"
	DegradedChunkPolicyAccept DegradedChunkPolicy = "accept"
)

// BudgetShortfallPolicy controls behaviour when budget is exhausted.
type BudgetShortfallPolicy string

const (
	BudgetShortfallPolicyReject    BudgetShortfallPolicy = "reject"
	BudgetShortfallPolicyEvict     BudgetShortfallPolicy = "evict"
	BudgetShortfallPolicySummarize BudgetShortfallPolicy = "summarize"
)

// SubstitutionPreference declares a content-type substitution rule.
type SubstitutionPreference struct {
	SourceContentType string               `json:"source_content_type,omitempty"`
	TargetContentType string               `json:"target_content_type,omitempty"`
	Strategy          SubstitutionStrategy `json:"strategy,omitempty"`
}

// SubstitutionStrategy enumerates substitution approaches.
type SubstitutionStrategy string

const (
	SubstitutionStrategyInline    SubstitutionStrategy = "inline"
	SubstitutionStrategyReference SubstitutionStrategy = "reference"
	SubstitutionStrategySummarize SubstitutionStrategy = "summarize"
)

// ContextAccessTier identifies the access tier for a context rule.
type ContextAccessTier string

const (
	TierStreamedContext ContextAccessTier = "streamed_context"
	TierWorkingMemory   ContextAccessTier = "working_memory"
	TierRetrieval       ContextAccessTier = "retrieval"
)

// AccessMode controls read/write access.
type AccessMode string

const (
	AccessModeReadOnly  AccessMode = "read"
	AccessModeWriteOnly AccessMode = "write"
	AccessModeReadWrite AccessMode = "read_write"
)

// ContextAccessRule binds a tier to allowed node types.
type ContextAccessRule struct {
	Tier       ContextAccessTier `json:"tier,omitempty"`
	AccessMode AccessMode        `json:"access_mode,omitempty"`
	NodeTypes  []string          `json:"node_types,omitempty"`
}
