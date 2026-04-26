# Framework Rework Engineering Specification and Migration Plan

> Status: Approved for implementation.
> Scope: `framework/` package layer, `ayenitd/` composition root, selected moves into `archaeo/`, `agents/`, and `relurpnet/`.
> Agents and named packages are known-broken and out of scope until the framework API re-solidifies.

---

## Part I — Engineering Specification

### 1. Design Principles

Eight principles govern every decision in this rework. Where an implementation choice contradicts a principle, the principle wins or is explicitly revised — not silently overridden.

**1.1 Context assembly is deterministic code, not agent prompting.**
The compiler that assembles live context is a deterministic function of structured inputs. No language model runs in the assembly path.

**1.2 No language models in the compiler's assembly path.**
Ranking, fusion, trust filtering, freshness filtering, and budget fitting use only deterministic algorithms: BM25, graph proximity, AST distance, RRF, token counting. Language models participate in summarization only, which is a named bounded operation with a policy gate and a provenance record.

**1.3 Provenance is inherent, not bolted on.**
Every knowledge chunk carries the record of where it came from, what produced it, and what it was derived from as part of its identity — not optional metadata.

**1.4 The event log is the invalidation bus.**
State changes emit events. Subsystems that cache or derive from state subscribe to events and react. No separate invalidation protocol exists.

**1.5 Trust labels flow with content.**
Content is labeled at ingestion with the trust class of its source. That label travels through every derivation. A summary of untrusted content is itself untrusted unless policy explicitly records an elevation.

**1.6 Knowledge class, storage mode, and source origin are orthogonal axes.**
What a chunk represents, how it is held, and where it came from are independent. A chunk is located at a point in three-dimensional space and its runtime behavior follows from that position.

**1.7 Policy lives in compiled bundles.**
Context policy (ranker admission, trust thresholds, summarization permissions, scanner configuration, budget enforcement) is compiled once at manifest resolution into a bundle. Authorization policy is compiled into a parallel bundle. Both use the same compilation pattern.

**1.8 No SQLite in the framework layer.**
The framework's persistent state is backed by `framework/graphdb` (in-memory adjacency graph with append-only file durability). SQLite is a platform-layer concern only. Any existing SQLite usage in framework packages is removed as part of this rework.

**1.9 Ingestion is pre-runtime.**
The ingestion pipeline is invoked before agent execution begins to scan and index the workspace. It runs to completion. It is not a service. Runtime artifact durability (tool outputs, promoted working memory, compiler-produced summaries) routes through `framework/persistence/`, which is distinct from ingestion.

**1.10 Each package owns its concurrency.**
No shared locking utility. Standard library primitives (`sync.Mutex`, `sync.RWMutex`, `sync.Map`, `singleflight.Group`) are used internally per package. No package exposes a locking API to callers.

---

### 2. Final Package Map

#### 2.1 Framework packages — final state

| Package | Status | Notes |
|---|---|---|
| `framework/core/` | Modified | Significant cleanup; see Section 3 |
| `framework/agentgraph/` | Renamed from `graph/` | Core execution substrate; LLMNode removed |
| `framework/memory/` | Rebuilt | Working memory only; name reclaimed |
| `framework/persistence/` | New | Runtime artifact write path |
| `framework/retrieval/` | Rebuilt | Targeted knowledge query API; name reclaimed |
| `framework/knowledge/` | Modified | Three-axis schema extension; graphdb-backed |
| `framework/patterns/` | Modified | SQLite implementation replaced with graphdb |
| `framework/search/` | Unchanged | BM25 and text rankers |
| `framework/ast/` | Modified | Receives `code_index_types` from core |
| `framework/graphdb/` | Unchanged | Shared graph infrastructure |
| `framework/ingestion/` | New | Workspace scanner + six-stage pipeline |
| `framework/compiler/` | New | Bi-directional compiler; compilation cache |
| `framework/summarization/` | New | AST-aware, heading-aware, prose summarizers |
| `framework/contextpolicy/` | New | Context policy bundle compilation |
| `framework/contextmetric/` | Upgraded | Real budget/token implementation; receives `context_budget.go` from core |
| `framework/manifest/` | Consolidated | Absorbs `config/`, `contract/`, `policybundle/` |
| `framework/authorization/` | Unchanged | WorkflowExecutor dependency accepted for now |
| `framework/capability/` | Modified | Absorbs `capabilityplan/` |
| `framework/event/` | Modified | Additive new event types |
| `framework/jobs/` | Modified | Adds `BackgroundCapabilityWorker` adapter |
| `framework/agentenv/` | Modified | Receives `WorkspaceEnvironment` from `ayenitd/` |
| `framework/agentspec/` | Modified | Receives `agent_composition.go` from core |
| `framework/sandbox/` | Unchanged | |
| `framework/identity/` | Deleted | Moved to `relurpnet/identity/` |
| `framework/plan/` | Deleted | Moved to `archaeo/` (types only; store removed) |
| `framework/guidance/` | Deleted | Moved to `archaeo/guidance/` |
| `framework/pipeline/` | Deleted | Moved to `agents/pipeline/` |
| `framework/keylock/` | Deleted | Each package owns its own concurrency |
| `framework/biknowledgecc/` | Deleted | Absorbed into `framework/compiler/` |
| `framework/memory/` (old) | Deleted | Name reclaimed |
| `framework/retrieval/` (old) | Deleted | Name reclaimed |
| `framework/capabilityplan/` | Deleted | Absorbed into `framework/capability/` |
| `framework/config/` | Deleted | Absorbed into `framework/manifest/` |
| `framework/contract/` | Deleted | Absorbed into `framework/manifest/` |
| `framework/policybundle/` | Deleted | Absorbed into `framework/manifest/` |
| `framework/contextmetric/` (old aliases) | Deleted | Rebuilt with real implementation |
| `framework/skills/` | Modified | Extended for ingestion and contextpolicy contributions |
| `framework/telemetry/`, `perfstats/`, `templates/`, `guidance/` | Varies | See table above |

#### 2.2 Moves outside framework

| From | To | Notes |
|---|---|---|
| `framework/identity/` | `relurpnet/identity/` | Fixes inverted dependency; identity already imports relurpnet/channel |
| `framework/guidance/` | `archaeo/guidance/` | Archaeo-domain concern |
| `framework/plan/` (types only) | `archaeo/plan/` | Store removed; types are archaeo-domain |
| `framework/pipeline/` | `agents/pipeline/` | Agent execution paradigm, not framework primitive |
| `framework/core/fmp_types.go` | `relurpnet/fmp/` | Nexus gateway protocol types |
| `framework/core/fmp_event_types.go` | `relurpnet/fmp/` | Nexus gateway protocol events |
| `framework/core/identity_types.go` | `relurpnet/identity/` | SubjectRef, AuthenticatedPrincipal, etc. |
| `framework/core/node_types.go` | `relurpnet/` | NodeDescriptor, NodeHealth, NodeProvider |
| `framework/core/tenant_identity_records.go` | `relurpnet/identity/` | TenantRecord, SubjectRecord |
| `ayenitd/WorkspaceEnvironment` | `framework/agentenv/` | Runtime environment is a framework concern |

---

### 3. `framework/core/` Cleanup Taxonomy

Every file in `framework/core/` has one of four dispositions.

#### 3.1 Stays in core

| File | Key Types | Notes |
|---|---|---|
| `audit.go` | `AuditRecord`, `AuditLogger`, `InMemoryAuditLogger`, `AuditQuery` | Fundamental audit infrastructure |
| `capability_policy_eval.go` | `EffectiveInsertionDecision`, `SelectorMatchesDescriptor` | Capability evaluation logic |
| `capability_result_types.go` | `CapabilityResultEnvelope`, `InsertionDecision`, `ContentDisposition`, `ApprovalBinding` | Capability result handling |
| `capability_runtime.go` | `CapabilityHandler`, `InvocableCapabilityHandler`, `PromptCapabilityHandler`, `ResourceCapabilityHandler`, `AvailabilityAwareCapabilityHandler` | Runtime interfaces |
| `capability_selector_helpers.go` | `CloneCapabilitySelectors`, `MergeCapabilitySelectors`, `ValidateCapabilitySelector` | Selector helpers |
| `capability_types.go` | Type aliases to agentspec for `CapabilityKind`, `TrustClass`, `RiskClass`, `EffectClass`, `CapabilityRuntimeFamily`, `CapabilityScope` | Note: aliases may collapse when agentspec is canonical |
| `delegation_types.go` | `DelegationRequest`, `DelegationResult`, `DelegationSnapshot` | Delegation framework |
| `derivation.go` | `DerivationChain`, `DerivationStep`, `DerivationID` | Provenance tracking; used by knowledge and persistence |
| `event_types.go` | `FrameworkEvent`, event type constants | Gains new context subsystem event types in Phase 3 |
| `llm_types.go` | `LanguageModel`, `LLMResponse`, `Message`, `ToolCall`, `LLMOptions` | Fundamental LLM interface |
| `object_registry.go` | `ObjectRegistry` | General-purpose handle registry |
| `permission_helpers.go` + `permissions_types.go` | `PermissionSet`, `FileSystemPermission`, `ExecutablePermission`, `NetworkPermission` | Authorization foundation |
| `policy_duration.go` + `policy_types.go` | `PolicyRule`, `PolicyDecision`, `PolicyConditions`, `RateLimit` | Policy framework |
| `provider_types.go` | `ProviderDescriptor`, `ProviderSession`, `Provider`, `ProviderRuntime`, `CapabilityRegistrar` | Provider model |
| `runtime_safety.go` | `RuntimeSafetySpec`, `RedactMetadataMap`, `EstimatePayloadTokens` | Redaction and safety |
| `schema_validation.go` | `ValidateValueAgainstSchema` | Tool parameter validation |
| `session_types.go` + `session_policy_types.go` + `session_delegation_types.go` | `SessionBoundary`, `SessionScope`, `SessionPolicy`, `SessionDelegationRecord` | Session model |
| `state_boundaries.go` | `MemoryClass` (updated), `StateBoundaryPolicy`, `ArtifactReference`, `MemoryRecordEnvelope` | MemoryClass cutover: remove `declarative`/`procedural`, add `streamed` |
| `task_context.go` | `TaskContext` | Task context propagation |
| `telemetry_types.go` | `Telemetry`, `Event`, `EventType` | Telemetry interfaces |
| `token_utils.go` | `EstimateTokens`, `EstimateTextTokens`, `EstimateCodeTokens` | Token estimation utilities |
| `tool_types.go` | `Tool`, `ToolParameter`, `ToolResult`, `CapabilityExecutionResult` | Tool interface |
| `config_compat.go` | `Config` struct (keep), `Capability` string alias (delete) | Partial: keep `Config`, delete `Capability` |

#### 3.2 Delete — legacy context subsystem

| File | Types Deleted | Reason |
|---|---|---|
| `context_items.go` | `ContextItem`, `MemoryContextItem`, `RetrievalContextItem`, `CapabilityResultContextItem` | Legacy context assembly via prompting; replaced by compiler |
| `shared_context.go` | `SharedContext`, `WorkingSetReference`, `MutationRecord`, `FileContext`, `SharedContextTokenUsage` | Legacy in-process context manager |
| `compression.go` | `SimpleCompressionStrategy`, `Interaction`, `KeyFact`, `CompressedContext`, `CompressionStrategy` interface | Old compression and interaction model |
| `reference_compat.go` | `ContextReferenceKind`, `ContextReference` | Legacy context references; context assembly no longer produces these |
| `plan_compat.go` | `PlanStep`, `Plan` | Plan types move to `archaeo/plan/` |

#### 3.3 Delete — compat shims

| File | Action |
|---|---|
| `agentspec_compat.go` | Delete entirely; callers import `framework/agentspec` directly |
| `compat.go` | Delete entirely; non-alias types (`KeyFact`, `Interaction`, `CompressedContext`) are in the deletion category; `AgentDefinition` etc. are imported from agentspec directly |

#### 3.4 Move out of core

| File | Destination | Key Types Moved |
|---|---|---|
| `context_budget.go` | `framework/contextmetric/` | `ArtifactBudget`, `TokenUsage`, `BudgetState`, `AllocationPolicy`, `BudgetItem` |
| `code_index_types.go` | `framework/ast/` | `Symbol`, `SymbolKind`, `CodeChunk`, `CodeIndex`, `FileMetadata` |
| `identity_types.go` | `relurpnet/identity/` | `SubjectRef`, `AuthenticatedPrincipal`, `ExternalSessionBinding`, `EventActor` |
| `node_types.go` | `relurpnet/` | `NodeDescriptor`, `NodeHealth`, `NodeCredential`, `NodeProvider` |
| `tenant_identity_records.go` | `relurpnet/identity/` | `TenantRecord`, `SubjectRecord` |
| `fmp_types.go` | `relurpnet/fmp/` | All FMP protocol types |
| `fmp_event_types.go` | `relurpnet/fmp/` | All FMP event types |
| `summarization.go` | `framework/summarization/` | `FileSummary`, `DirectorySummary`, `ChunkSummary`, `Summarizer`, `SummaryLevel` (seed types) |
| `branch_delta.go` | `framework/agentgraph/` | `BranchDeltaSet`, `BranchContextDelta`, `BranchContextSideEffects` |
| `agent_composition.go` | `framework/agentspec/` | `MemoryMode`, `StateMode`, `AgentCompositionSpec`, `AgentInvocationPolicy` |
| `admin_types.go` | `framework/authorization/` | `AdminTokenRecord` |
| `backend_capabilities.go` | `platform/llm/` | `BackendClass`, `BackendCapabilities` |
| `provenance.go` | `framework/knowledge/` | `Provenance`, `ContentWithProvenance` |

---

### 4. The Three-Axis Knowledge Model

Every knowledge chunk is located at a point in three-dimensional space. All three axes are required fields on `KnowledgeChunk`.

**Axis 1 — `MemoryClass`** (what kind of knowledge)
- `working` — per-turn agent execution state; expires at checkpoint; lives in `framework/memory/`
- `streamed` — assembled by compiler for LLM consumption; never persisted as compiled form; ephemeral

**Axis 2 — `StorageMode`** (how it is held)
- `materialized` — stored in the knowledge graph; canonical form in graphdb; lookup by ID
- `compiled` — produced on demand; no persistent form; bounded by the request
- `cached` — semantically compiled but temporarily materialized; correctness depends on invalidation; cache key includes event-log sequence number

**Axis 3 — `SourceOrigin`** (where it came from)
Open-ended string enum. Initial values: `file`, `tool_output`, `user_input`, `capability_result`, `pattern_derivation`, `summary_derivation`, `ast_analysis`. New source types are added as new ingestion paths are developed.

#### 4.1 Extended `KnowledgeChunk` schema

```go
type KnowledgeChunk struct {
    // Identity
    ID            ChunkID        // stable artifact identifier
    Version       int            // incremented on re-ingestion of same source identity
    WorkspaceID   string

    // Three-axis model
    MemoryClass   MemoryClass    // working | streamed
    StorageMode   StorageMode    // materialized | compiled | cached
    SourceOrigin  SourceOrigin   // file | tool_output | user_input | ...

    // Provenance (inherent, not optional)
    SourcePrincipal   SubjectRef     // who/what produced this
    AcquisitionMethod string         // file_read | tool_output | user_input | ...
    AcquiredAt        time.Time
    TrustClass        TrustClass     // from source principal; propagates through derivation
    ContentSchemaVersion string      // for migration support

    // Summary provenance (non-zero only for summary_derivation chunks)
    DerivedFrom         []ChunkID   // edges to source chunks
    DerivationMethod    string      // summarizer ID + algorithm fingerprint
    DerivationGeneration int        // 0=original, 1=summary of original, 2=summary of summary
    CoverageHash        string      // hash of source chunk IDs+versions; stale when mismatched

    // Staleness
    Freshness     FreshnessState  // valid | stale | invalid | unverified
    Tombstoned    bool
    SupersededBy  ChunkID         // populated when tombstoned

    // Content
    ContentHash   string
    TokenEstimate int
    Body          ChunkBody
    Views         []ChunkView

    // Scanning metadata (populated by ingestion scanner stage)
    SuspicionScore float64
    SuspicionFlags []string

    // Legacy provenance (kept for graphdb compatibility during transition)
    Provenance    ChunkProvenance

    CreatedAt     time.Time
    UpdatedAt     time.Time
}
```

#### 4.2 Tombstoning

When a source identity is re-ingested, the prior chunk is tombstoned rather than deleted:
- `Tombstoned = true`, `SupersededBy = newChunkID`
- Content remains accessible for provenance resolution
- Tombstoned chunks are excluded from compilation and retrieval by default
- Tombstones are garbage-collected when no live provenance edges point to them (default: retain for workspace lifetime)

---

### 5. New Package Specifications

#### 5.1 `framework/contextpolicy/`

Compiles declarative context policy into a runtime-evaluable bundle. Parallel in structure to `framework/manifest/` (which compiles authorization policy). The bundle is computed at manifest resolution time and recomputed on manifest change.

**Key types:**

```go
type ContextPolicyBundle struct {
    // Ranker admission
    AdmittedRankers    []RankerRef       // rankers active for this manifest
    RequiredRankers    []RankerRef       // compilation fails if these are unavailable
    RankerWeights      map[string]float64 // non-uniform fusion weights; default 1.0

    // Trust thresholds
    MinAdmittedTrust   TrustClass        // chunks below this are excluded from live context
    TrustDemotedPolicy TrustDemotedPolicy // include-with-markers | exclude

    // Summarization permissions
    AutoSummarizeOnBudgetPressure bool
    SummarizationPermitted        bool
    DerivationGenerationCap       int    // default 2
    AdmittedSummarizers          []SummarizerRef
    ProseModelPermitted          bool
    AdmittedProseModels          []string

    // Scanner configuration
    AdmittedScanners    []ScannerRef
    QuarantineThreshold float64
    TrustDemoteThreshold float64

    // Freshness
    TimeBasedFreshnessEnabled bool
    FreshnessThresholds       map[SourceOrigin]time.Duration
    DegradedPolicy            DegradedChunkPolicy // show-with-markers | exclude | include-silent

    // Budget
    TokenBudget          int
    BudgetShortfallPolicy BudgetShortfallPolicy // fail | emit-partial
    SubstitutionPref      SubstitutionPreference // prefer-summary | prefer-tail-drop

    // Rate limits
    IngestionQuotas  map[string]QuotaSpec // keyed by source principal pattern
    CompilationRateLimit RateLimitSpec

    // Compilation behavior
    CompilationMode  CompilationMode // hybrid | per-turn | explicit
}

// Compile produces a ContextPolicyBundle from manifest declarations and system defaults.
func Compile(manifest *manifest.AgentManifest, skills []*manifest.SkillManifest, defaults SystemDefaults) (*ContextPolicyBundle, error)

// Evaluator wraps a compiled bundle and answers policy questions synchronously.
type Evaluator struct { bundle *ContextPolicyBundle }

func (e *Evaluator) AdmitRanker(ref RankerRef) bool
func (e *Evaluator) AdmitTrustClass(tc TrustClass) bool
func (e *Evaluator) AdmitChunk(chunk *knowledge.KnowledgeChunk) ChunkAdmissionDecision
func (e *Evaluator) PermitSummarization(kind SummarizerRef) bool
func (e *Evaluator) QuotaRemaining(principalPattern string) int
func (e *Evaluator) TokenBudget() int
```

#### 5.2 `framework/ingestion/`

Pre-runtime pipeline. Invoked once at workspace startup to scan and index. Also invoked incrementally when workspace files change (git diff integration, file watcher). Not a service.

**Six stages:**

```
Stage 1: Acquisition      RawIngestion record (bytes, source_principal, acquisition_method, acquired_at, mime_hint)
Stage 2: Parsing+Typing   TypedIngestion (structured representation, chunking boundaries, preliminary metadata)
Stage 3: Scanning         ScanResult per chunk (suspicion_score, flags)
Stage 4: Enrichment       CandidateEdges (call graphs, import relations, document links)
Stage 5: Admission        Disposition: Commit | Quarantine | Reject (via contextpolicy.Evaluator)
Stage 6: Commit           ChunkStore.Save(), edge persistence, ChunkCommitted event emission
```

**Entry points (public API):**

```go
func AcquireFromFile(ctx context.Context, path string, principal SubjectRef, policy *contextpolicy.ContextPolicyBundle) (*Pipeline, error)
func AcquireFromToolOutput(ctx context.Context, output []byte, principal SubjectRef, policy *contextpolicy.ContextPolicyBundle) (*Pipeline, error)
func AcquireFromUserInput(ctx context.Context, input string, principal SubjectRef, policy *contextpolicy.ContextPolicyBundle) (*Pipeline, error)

// Pipeline executes the six stages for a single raw ingestion.
type Pipeline struct { ... }
func (p *Pipeline) Run(ctx context.Context) (*IngestResult, error)

// WorkspaceScanner orchestrates bulk pre-runtime ingestion.
type WorkspaceScanner struct {
    Store        *knowledge.ChunkStore
    Events       event.Log
    Policy       *contextpolicy.ContextPolicyBundle
    Concurrency  int
    IncludeGlobs []string
    ExcludeGlobs []string
}
func (s *WorkspaceScanner) Scan(ctx context.Context, root string) (*ScanReport, error)
func (s *WorkspaceScanner) ScanIncremental(ctx context.Context, root string, since string) (*ScanReport, error) // since = git ref
```

**Scanner interface:**

```go
type Scanner interface {
    Name() string
    Scan(ctx context.Context, chunk TypedChunk) ScanResult
}

type ScanResult struct {
    SuspicionScore float64
    Flags          []string // "unicode_tag", "role_switch", "base64_payload", etc.
}
```

Built-in scanners: `SignatureScanner` (regex-based known patterns), `UnicodeTagScanner` (invisible tag injection), `StructuralHeuristicScanner` (imperative verb density, second-person AI addressing), `Base64BlobScanner`.

#### 5.3 `framework/persistence/`

Runtime artifact write path. Receives promoted working memory, compiler-produced artifacts, and agent-initiated durability requests. Lighter than ingestion (no workspace scanning, no enrichment) but enforces admission (trust labeling, provenance, basic structural validation) via `contextpolicy.Evaluator` before writing to `framework/knowledge/`.

```go
// PersistenceRequest is the caller-facing write contract.
type PersistenceRequest struct {
    Content         []byte
    ContentType     string          // mime type or structural type
    SourcePrincipal SubjectRef
    SourceOrigin    knowledge.SourceOrigin
    Reason          string
    Tags            []string
    DerivedFrom     []knowledge.ChunkID // for compiler-produced artifacts
    DerivationMethod string
    DerivationGeneration int
}

type PersistenceResult struct {
    ChunkID   knowledge.ChunkID
    Action    PersistenceAction // created | updated | deduplicated | quarantined | rejected
    AuditID   string
}

type PersistenceAuditRecord struct {
    AuditID         string
    Action          PersistenceAction
    ChunkID         knowledge.ChunkID
    SourcePrincipal SubjectRef
    SourceOrigin    knowledge.SourceOrigin
    TrustClass      TrustClass
    Reason          string
    PolicyName      string
    CreatedAt       time.Time
}

// Writer is the main entry point for runtime persistence.
type Writer struct {
    Store  *knowledge.ChunkStore
    Events event.Log
    Policy *contextpolicy.ContextPolicyBundle
}

func (w *Writer) Persist(ctx context.Context, req PersistenceRequest) (*PersistenceResult, error)
func (w *Writer) PersistBatch(ctx context.Context, reqs []PersistenceRequest) ([]PersistenceResult, error)
```

The admission path: structural validation → trust class assignment (from source principal) → suspicion check (simplified: no heavy scanning, but structural validation catches obvious issues) → quota check → commit to knowledge store → emit `ChunkCommitted` event.

#### 5.4 `framework/summarization/`

All summarization algorithms. Called by the compiler write direction and by explicit agent capability invocations. Does not decide whether summarization is permitted — that is `contextpolicy`'s job.

```go
type SummarizerKind string
const (
    SummarizerKindAST      SummarizerKind = "ast"       // code, deterministic
    SummarizerKindHeading  SummarizerKind = "heading"   // structured documents
    SummarizerKindProse    SummarizerKind = "prose"     // free text, model-based
)

type SummarizationRequest struct {
    Chunks          []knowledge.KnowledgeChunk
    Kind            SummarizerKind
    SourceOrigin    knowledge.SourceOrigin
    TargetTokenBudget int
    ModelID         string // for prose only; empty = use workspace default
}

type SummarizationResult struct {
    Summary          string
    TokenEstimate    int
    DerivationMethod string     // summarizer ID + fingerprint
    SourceCoverage   []knowledge.ChunkID
    CoverageHash     string
    UsedModel        bool       // true if a language model was invoked
}

type Summarizer interface {
    Kind() SummarizerKind
    Summarize(ctx context.Context, req SummarizationRequest) (*SummarizationResult, error)
    CanSummarize(chunks []knowledge.KnowledgeChunk) bool
}

// ASTSummarizer preserves function signatures, class declarations, doc comments;
// elides function bodies. Deterministic; no model call.
type ASTSummarizer struct { Detector *ast.LanguageDetector }

// HeadingSummarizer preserves heading structure and ledes; elides paragraph bodies.
type HeadingSummarizer struct{}

// ProseSummarizer is model-based. Requires an LLM. Records model ID in provenance.
type ProseSummarizer struct { Model core.LanguageModel }

// Router selects the appropriate summarizer for a chunk set and invokes it.
func Route(ctx context.Context, chunks []knowledge.KnowledgeChunk, budget int, summarizers []Summarizer, policy *contextpolicy.ContextPolicyBundle) (*SummarizationResult, error)
```

Generation cap enforcement: a chunk with `DerivationGeneration >= policy.DerivationGenerationCap` is not eligible as a source for further summarization. The `Route` function enforces this before selecting a summarizer.

#### 5.5 `framework/memory/` (rebuilt)

Working memory: per-turn, per-session in-memory state scoped by task ID. Expires at checkpoint boundary. No persistence.

```go
// WorkingMemoryStore holds per-task ephemeral state.
type WorkingMemoryStore struct {
    mu    sync.RWMutex
    tasks map[string]*taskMemory
}

func NewWorkingMemoryStore() *WorkingMemoryStore
func (s *WorkingMemoryStore) Scope(taskID string) *TaskMemory
func (s *WorkingMemoryStore) Evict(taskID string) // called at checkpoint

type TaskMemory struct {
    mu      sync.RWMutex
    entries map[string]MemoryEntry
}

func (m *TaskMemory) Set(key string, value any, class MemoryClass)
func (m *TaskMemory) Get(key string) (MemoryEntry, bool)
func (m *TaskMemory) Keys() []string
func (m *TaskMemory) Snapshot() map[string]MemoryEntry

type MemoryEntry struct {
    Value     any
    Class     core.MemoryClass
    CreatedAt time.Time
    UpdatedAt time.Time
}

// MemoryRetriever is the interface agentgraph nodes use to query memory.
// Defined here; implementations provided by WorkingMemoryStore.
type MemoryRetriever interface {
    Retrieve(ctx context.Context, query MemoryQuery) ([]MemoryRecordEnvelope, error)
}

// StateHydrator populates graph execution state from a memory retrieval result.
type StateHydrator interface {
    Hydrate(ctx context.Context, state *agentgraph.Context, results []MemoryRecordEnvelope) error
}

// PromotionRequest carries a working memory entry to be durably persisted.
// Callers submit this to framework/persistence.Writer after a turn completes.
type PromotionRequest struct {
    TaskID      string
    Key         string
    Destination knowledge.SourceOrigin
    Principal   core.SubjectRef
    Reason      string
}
```

#### 5.6 `framework/retrieval/` (rebuilt)

Targeted knowledge query API. The compiler uses it internally for scatter-gather; agents and graph nodes can call it directly for targeted queries without triggering full context compilation.

```go
// Ranker produces an ordered list of chunk IDs for a query. Rank position only;
// no scores (scores are not on the same scale across ranker types).
type Ranker interface {
    Name() string
    Rank(ctx context.Context, query RetrievalQuery, store *knowledge.ChunkStore) ([]knowledge.ChunkID, error)
}

// RankerRegistry holds admitted rankers for a compilation.
type RankerRegistry struct { rankers map[string]Ranker }
func (r *RankerRegistry) Register(ranker Ranker)
func (r *RankerRegistry) Admitted(policy *contextpolicy.ContextPolicyBundle) []Ranker

// RetrievalQuery is the caller-facing contract.
type RetrievalQuery struct {
    Text        string
    Scope       string
    SourceTypes []knowledge.SourceOrigin
    Limit       int
    AfterSeq    uint64 // event log sequence; for cache coherence
}

// RetrievalResult holds the RRF-merged ranked list with provenance.
type RetrievalResult struct {
    Chunks     []RankedChunk
    RankerOutputs map[string][]knowledge.ChunkID // per-ranker ranked list for replay
    FusedOrder []knowledge.ChunkID
}

type RankedChunk struct {
    Chunk    knowledge.KnowledgeChunk
    RRFScore float64
    Sources  []string // which rankers contributed
}

// Retriever is the main entry point.
type Retriever struct {
    Store    *knowledge.ChunkStore
    Registry *RankerRegistry
}

func (r *Retriever) Retrieve(ctx context.Context, query RetrievalQuery, policy *contextpolicy.ContextPolicyBundle) (*RetrievalResult, error)

// RRF performs Reciprocal Rank Fusion over multiple ranked lists.
// k is typically 60. Missing chunks contribute zero.
func RRF(lists [][]knowledge.ChunkID, weights []float64, k float64) []knowledge.ChunkID
```

Built-in rankers: `BM25Ranker` (wraps `framework/search/`), `ASTProximityRanker` (wraps `framework/ast/`), `GraphProximityRanker` (hop-count from anchor nodes in the knowledge graph), `RecencyRanker` (by `AcquiredAt` timestamp).

#### 5.7 `framework/compiler/`

The bi-directional compiler. Read direction assembles live context. Write direction triggers summarization and routes artifacts through persistence. Maintains a compilation cache keyed to event-log sequence numbers.

**Read direction:**

```go
type CompilationRequest struct {
    Query         string
    IntentSignal  string
    ManifestFingerprint string
    PolicyBundle  *contextpolicy.ContextPolicyBundle
    EventLogSeq   uint64  // current event log sequence; part of cache key
    TaskID        string
    SessionID     string
}

type CompilationResult struct {
    Chunks       []knowledge.KnowledgeChunk  // ordered; what the LLM will see
    Record       CompilationRecord           // full audit of the compilation
}

type CompilationRecord struct {
    ID               string
    QueryFingerprint string
    ManifestFingerprint string
    PolicyFingerprint string
    EventLogSeq      uint64
    AdmittedRankers  []string
    RankerOutputs    map[string][]knowledge.ChunkID
    FusedOrder       []knowledge.ChunkID
    TrustFiltered    []knowledge.ChunkID
    FreshnessFiltered []knowledge.ChunkID
    Substitutions    []SummarySubstitution     // which summaries replaced which sources
    BudgetUsed       int
    BudgetTotal      int
    ShortfallTokens  int
    EmittedChunkIDs  []knowledge.ChunkID
    ProducedAt       time.Time
}

type SummarySubstitution struct {
    ReplacedChunkIDs []knowledge.ChunkID
    SummaryChunkID   knowledge.ChunkID
}

type Compiler struct {
    Store      *knowledge.ChunkStore
    Retriever  *retrieval.Retriever
    Events     event.Log
    Persist    *persistence.Writer
    Summarize  summarization.Router
    cache      compilationCache
}

func NewCompiler(store *knowledge.ChunkStore, retriever *retrieval.Retriever, events event.Log, persist *persistence.Writer) *Compiler
func (c *Compiler) Start(ctx context.Context) error  // subscribe to invalidation events
func (c *Compiler) Stop()
func (c *Compiler) Compile(ctx context.Context, req CompilationRequest) (*CompilationResult, error)

// Replay re-runs compilation against the knowledge graph state at the original
// event log sequence (strict) or at current state (current).
func (c *Compiler) Replay(ctx context.Context, compilationID string, mode ReplayMode) (*CompilationResult, error)

// Diff produces a structured diff between two compilation records.
func (c *Compiler) Diff(ctx context.Context, idA, idB string) (*CompilationDiff, error)
```

**Cache coherence:**
Cache key: `(query_fingerprint, manifest_fingerprint, policy_bundle_fingerprint, event_log_seq)`. On each `ChunkCommitted` or `ContextPolicyReloaded` event, the cache evaluates whether the affected chunk IDs intersect the dependency bitmap of cached entries. Intersecting entries are evicted. Non-intersecting entries remain valid despite the sequence advance.

**Write direction:**
When `Compile` determines that summary substitution is needed and a qualifying summary does not exist:
1. Checks `contextpolicy.Evaluator.PermitSummarization()`
2. If permitted: calls `summarization.Route()` for the candidate chunks
3. Routes the result through `persistence.Writer.Persist()` with `SourceOrigin = summary_derivation`
4. Uses the resulting `ChunkID` for substitution in the `CompilationRecord`

---

### 6. `framework/agentgraph/` Changes

#### 6.1 Nodes retained
`CheckpointNode`, `ToolNode`, `ConditionalNode`, `HumanNode`, `TerminalNode`

#### 6.2 Nodes removed
`PersistenceWriterNode` — deleted entirely. All associated types (`RuntimePersistenceStore`, `DeclarativeRecord`, `ProceduralRecord`, `DeclarativeKind`, `ProceduralKind`, and all request/query types) are deleted. `ArtifactSink`/`ArtifactRecord` survive in simplified form as the mechanism for nodes to emit file-system-level outputs (not knowledge-store writes).

#### 6.3 Nodes added
`RetrievalNode` — performs targeted retrieval and writes results to graph state:

```go
type RetrievalNode struct {
    id        string
    Retriever *retrieval.Retriever
    Policy    *contextpolicy.ContextPolicyBundle
    Query     string       // static query, or resolved from StateKey
    QueryKey  string       // state key to resolve query dynamically
    OutputKey string       // where to write RetrievalResult in state
}
```

#### 6.4 `LLMNode` removed from framework
`LLMNode` moves to `agents/`. Each agent paradigm that needs LLM calls defines its own node. The framework substrate does not prescribe how LLM calls are structured.

#### 6.5 Compiler trigger injection
`LLMNode` (now in agents/) receives a `CompilationTrigger` interface. Graph construction code (in agents) injects a concrete `*compiler.Compiler`. The interface is defined in `framework/agentgraph/`:

```go
// CompilationTrigger is satisfied by *compiler.Compiler and injected into
// LLM-calling nodes in agents/.
type CompilationTrigger interface {
    Compile(ctx context.Context, req CompilerRequest) (*CompilerResult, error)
}

// CompilerRequest and CompilerResult are thin wrappers so agentgraph
// doesn't import framework/compiler directly.
type CompilerRequest struct {
    Query        string
    TaskID       string
    SessionID    string
    PolicyBundle any // *contextpolicy.ContextPolicyBundle; typed as any to avoid import
}
type CompilerResult struct {
    Chunks []knowledge.KnowledgeChunk
    Record any // *compiler.CompilationRecord
}
```

---

### 7. `framework/manifest/` Consolidation

The four packages `config/`, `contract/`, `policybundle/`, and `manifest/` consolidate into `framework/manifest/`. The consolidated package owns:

1. **YAML parsing** — `GlobalConfig`, `AgentManifest`, `SkillManifest` loading and validation
2. **Path resolution** — resolving relative paths against workspace root
3. **Skill expansion** — expanding skill references into capability descriptor sets
4. **Overlay merging** — applying workspace-local manifest overrides onto base templates
5. **Effective contract resolution** — combining manifest + skills + overlays into a single effective spec
6. **Authorization policy compilation** — `CompiledPolicyBundle` (authorization side; `contextpolicy/` handles context side)

**Public API surface:**

```go
// Load parses and validates a manifest from disk.
func Load(path string) (*AgentManifest, error)

// Resolve produces an effective runtime spec from a manifest with all paths
// resolved, skills expanded, and overlays applied.
func Resolve(baseFS string, manifest *AgentManifest, overlays []AgentSpecOverlay) (*ResolvedManifest, error)

// Compile produces a CompiledPolicyBundle from the resolved manifest.
func Compile(resolved *ResolvedManifest, manager *authorization.PermissionManager) (*CompiledPolicyBundle, error)

// CompileContext produces a ContextPolicyBundle from the resolved manifest.
// Delegates to framework/contextpolicy.
func CompileContext(resolved *ResolvedManifest) (*contextpolicy.ContextPolicyBundle, error)
```

---

### 8. `framework/agentenv/WorkspaceEnvironment`

`WorkspaceEnvironment` moves from `ayenitd/` to `framework/agentenv/`. The type is the canonical runtime environment shared by all agents in a workspace session.

**Before (in `ayenitd/environment.go`):**
```go
Memory          memory.MemoryStore        // old memory package
WorkflowStore   memory.WorkflowStateStore // old memory package
PlanStore       plan.PlanStore            // plan.PlanStore (deleted)
KnowledgeStore  memory.KnowledgeStore     // old memory package
GuidanceBroker  *guidance.GuidanceBroker  // framework/guidance (moved to archaeo)
Embedder        retrieval.Embedder        // old retrieval (deleted)
RetrievalDB     *sql.DB                   // SQLite (removed)
```

**After (in `framework/agentenv/`):**
```go
type WorkspaceEnvironment struct {
    // Identity + model
    Config        *core.Config
    Model         core.LanguageModel
    CommandPolicy sandbox.CommandPolicy
    Backend       string

    // Capability + permission
    Registry          *capability.Registry
    PermissionManager *authorization.PermissionManager

    // Code intelligence
    IndexManager *ast.IndexManager
    SearchEngine *search.SearchEngine

    // Knowledge + memory
    WorkingMemory  *memory.WorkingMemoryStore
    KnowledgeStore *knowledge.ChunkStore
    PatternStore   patterns.PatternStore

    // Retrieval + compilation
    Retriever *retrieval.Retriever
    Compiler  *compiler.Compiler

    // Event infrastructure
    EventLog   event.Log
    KnowledgeEvents *knowledge.EventBus

    // Scheduling + services
    Scheduler      *ServiceScheduler
    ServiceManager *ServiceManager

    // Optional agents (interfaces)
    VerificationPlanner           VerificationPlanner
    CompatibilitySurfaceExtractor CompatibilitySurfaceExtractor
}
```

`ayenitd/` becomes a thin composition root: `Open()` initializes platform services, constructs a `WorkspaceEnvironment`, and returns it. The `WorkspaceEnvironment` type and its `With*` scoping methods live in `framework/agentenv/`.

---

### 9. `framework/jobs/` Extension

`BackgroundCapabilityWorker` wraps a capability invocation as a job:

```go
// BackgroundCapabilityWorker adapts a CapabilityInvoker to the Worker interface.
// Agents submit long-running capabilities as background jobs via this adapter.
type BackgroundCapabilityWorker struct {
    Invoker   agentgraph.CapabilityInvoker
    Registry  *capability.Registry
}

func (w *BackgroundCapabilityWorker) Execute(ctx context.Context, job Job) (map[string]any, error)

// JobRef is returned to the agent when a capability is submitted as a background job.
type JobRef struct {
    JobID      string
    StatusKey  string
}
```

Two standard capabilities become available to agents using the jobs runtime: `submit_background(capability_id, args) → JobRef` and `poll_job(job_id) → JobStatus`.

---

## Part II — Rework, Consolidation, and Cleanup Plan

### Phase ordering overview

```
Phase 1:  framework/core surgery
Phase 2:  Package deletions and renames
Phase 3:  framework/manifest consolidation
Phase 4:  framework/knowledge schema extension
Phase 5:  framework/contextpolicy
Phase 6:  framework/ingestion
Phase 7:  framework/persistence
Phase 8:  framework/summarization
Phase 9:  framework/memory (rebuilt)
Phase 10: framework/retrieval (rebuilt)
Phase 11: framework/compiler (read direction)
Phase 12: framework/compiler (write direction)
Phase 13: framework/agentgraph integration
Phase 14: framework/agentenv / WorkspaceEnvironment migration
Phase 15: framework/skills extension
Phase 16: Compiler replay and diffing
```

---

### Phase 1 — `framework/core/` Surgery

**Goal:** Transform `framework/core/` from a soup of mixed concerns into a clean set of genuine framework primitives. This phase is the prerequisite for every other phase because every package imports `framework/core/`.

**Dependencies:** None. This is the first phase.

**Work items:**

1. **Delete legacy context subsystem files:**
   - `context_items.go` and all associated helper functions (`cloneDerivationChain`, `resultSummaryText`, `cloneResultMetadata`, `truncateParagraph`, `firstNonEmpty`)
   - `shared_context.go` and all associated methods (`AddFile`, `GetFile`, `DowngradeOldFiles`, `EnsureFileLevel`, `RecordMutation`, `RecentMutations`, `WorkingSetReferences`, `GetConversationSummary`, `RefreshConversationSummary`)
   - `compression.go` and all associated types and functions
   - `reference_compat.go`
   - `plan_compat.go`

2. **Delete compat shims:**
   - `agentspec_compat.go` — update all callers to import `framework/agentspec` directly
   - `compat.go` — update all callers; `AgentDefinition`/`AgentSemanticContext` callers import from `framework/agentspec`; `LoadAgentDefinition` callers call `agentspec.LoadAgentDefinition`
   - In `config_compat.go`: delete the `Capability` string type and its constants; keep `Config` struct

3. **Update `state_boundaries.go` — `MemoryClass` cutover:**
   - Remove `MemoryClassDeclarative = "declarative"`
   - Remove `MemoryClassProcedural = "procedural"`
   - Add `MemoryClassStreamed MemoryClass = "streamed"`
   - Update `ValidateStateBoundaryPolicy` switch to admit only `working` and `streamed`
   - Update `LintStateMap` and all associated logic

4. **Move types to destination packages** (create stub files in destinations; move content; update imports):
   - `context_budget.go` → `framework/contextmetric/budget.go` (replace the aliases file entirely)
   - `code_index_types.go` → `framework/ast/index_types.go`
   - `identity_types.go` → `relurpnet/identity/core_types.go`
   - `node_types.go` → `relurpnet/node_types.go`
   - `tenant_identity_records.go` → `relurpnet/identity/records.go`
   - `fmp_types.go` → `relurpnet/fmp/types.go`
   - `fmp_event_types.go` → `relurpnet/fmp/events.go`
   - `summarization.go` → `framework/summarization/seed_types.go` (temporary name; will be replaced in Phase 8)
   - `branch_delta.go` → `framework/agentgraph/branch_delta.go` (move after rename in Phase 2)
   - `agent_composition.go` → `framework/agentspec/composition.go`
   - `admin_types.go` → `framework/authorization/admin_types.go`
   - `backend_capabilities.go` → `platform/llm/backend_capabilities.go`
   - `provenance.go` → `framework/knowledge/provenance.go`

5. **Fix all call sites** across the entire codebase that imported the deleted or moved types. The build must pass (`go build ./...`) before this phase is considered complete — with the exception of the known-broken `agents/` and `named/` packages.

**Unit test cleanup:**

- Delete `state_boundaries_test.go` test cases that reference `MemoryClassDeclarative` or `MemoryClassProcedural`; rewrite to test the new two-value enum
- Update `capability_result_types_test.go` if any assertions use deleted types
- Delete test helpers in `context_items.go` test infrastructure if present
- Add new tests for the updated `ValidateStateBoundaryPolicy` rejecting `declarative` and `procedural`
- Add new tests for `MemoryClassStreamed` being a valid class

**Exit criteria:**
- `go build ./...` passes (excluding known-broken agents/named)
- `go test ./framework/core/...` passes
- `go test ./framework/agentspec/...` passes (receives moved types)
- `go test ./framework/contextmetric/...` passes (receives budget types)
- `go test ./relurpnet/...` passes (receives identity/node/fmp types)
- No file in `framework/core/` references `MemoryClassDeclarative` or `MemoryClassProcedural`
- No file in `framework/core/` references `ContextItem`, `SharedContext`, or `CompressionStrategy`
- No file in `framework/core/` contains a type alias pointing to `agentspec`

---

### Phase 2 — Package Deletions, Renames, and Consolidations (Structural)

**Goal:** Establish the new namespace. Delete dead packages. Rename `graph/` to `agentgraph/`. Absorb `capabilityplan/` into `capability/`. Move `guidance/`, `plan/`, `pipeline/`, `identity/` to their new homes. Delete `keylock/` and `biknowledgecc/`.

**Dependencies:** Phase 1 (core must be clean before restructuring dependents).

**Work items:**

1. **Rename `framework/graph/` → `framework/agentgraph/`:**
   - Update module import path in all files: `framework/graph` → `framework/agentgraph`
   - Move `branch_delta.go` (now in agentgraph stub) to final location
   - Update all importers across the codebase

2. **Delete `framework/keylock/`:**
   - Remove all usages in `archaeo/` — `archaeo/` internalizes its own `sync.Map`-based per-key mutex in `archaeo/internal/keylock/`
   - Delete the package

3. **Delete `framework/biknowledgecc/`:**
   - `archaeo/compiler/` is also deleted (it was a compat wrapper)
   - Any event type constants from `biknowledgecc/` that are still needed are redefined in `framework/compiler/` (Phase 11) and are currently unused

4. **Move `framework/guidance/` → `archaeo/guidance/`:**
   - Create `archaeo/guidance/` with the broker, frames, deferral, and types
   - Update all importers: `agents/relurpic/` → `archaeo/guidance/`, `named/euclo/` → `archaeo/guidance/`, `app/` → `archaeo/guidance/`
   - Delete `framework/guidance/`

5. **Move `framework/plan/` → `archaeo/plan/` (types only):**
   - Move `types.go`, `confidence.go`, `convergence.go`, `compatibility_surface.go`, `verification_planner.go`, `graph.go`, `invalidation.go`
   - Delete `sqlite.go` and `store.go` — no persistence layer
   - Delete `framework/plan/`
   - Update all importers

6. **Move `framework/pipeline/` → `agents/pipeline/`:**
   - Consolidate with existing `agents/pipeline/` code
   - `agents/chainer/`, `agents/htn/`, `named/euclo/` update imports
   - Delete `framework/pipeline/`

7. **Move `framework/identity/` → `relurpnet/identity/`:**
   - Merge resolver, contracts, and errors into existing `relurpnet/identity/`
   - Update all importers: `app/nexus/` → `relurpnet/identity/`
   - Delete `framework/identity/`

8. **Absorb `framework/capabilityplan/` → `framework/capability/`:**
   - Move `AdmissionResult`, `Candidate`, and admission logic into `framework/capability/`
   - Update all importers
   - Delete `framework/capabilityplan/`

9. **Delete old `framework/memory/` and `framework/retrieval/`:**
   - These are being rebuilt in Phases 9 and 10
   - All their importers are either in broken `agents/`/`named/` (acceptable build failure) or `ayenitd/` (updated in Phase 14)
   - Delete both packages; their names are reclaimed for the rebuilt versions

10. **Delete `framework/persistence_node.go` from agentgraph:**
    - Delete the entire file including `PersistenceWriterNode`, `RuntimePersistenceStore`, `DeclarativeRecord`, `ProceduralRecord`, and all associated types and functions
    - Retain `ArtifactSink` and `ArtifactRecord` in `framework/agentgraph/artifacts.go` (simplified: just the interface and struct, no persistence logic)
    - Retain `PersistenceAuditSink` interface as `framework/agentgraph/audit.go` — simplified; agents can implement it for custom audit recording

**Unit test cleanup:**

- Delete all tests in `framework/pipeline/` (moved with the package to agents)
- Delete `framework/plan/` tests that reference the SQLite store; keep and move the type-only tests
- Delete `framework/guidance/` tests (moved with the package)
- Delete `framework/identity/` tests (moved with the package)
- Delete `framework/capabilityplan/plan_test.go` — rewrite as part of `framework/capability/` tests
- Delete `framework/memory/` tests and `framework/retrieval/` tests (old packages deleted)
- Delete all tests in `framework/agentgraph/` that test `PersistenceWriterNode`; these test the old paradigm entirely

**Exit criteria:**
- `go build ./framework/...` passes
- `go build ./relurpnet/...` passes
- `go build ./archaeo/...` passes (guidance, plan imports updated)
- `go build ./app/...` passes
- No `framework/graph/` directory exists; `framework/agentgraph/` exists
- No `framework/keylock/`, `framework/biknowledgecc/`, `framework/pipeline/`, `framework/identity/`, `framework/capabilityplan/`, `framework/guidance/`, `framework/plan/`, `framework/memory/` (old), `framework/retrieval/` (old) directories exist
- `framework/agentgraph/` contains no `persistence_node.go`

---

### Phase 3 — `framework/manifest/` Consolidation

**Goal:** Collapse the four-package configuration pipeline (`config/`, `manifest/`, `contract/`, `policybundle/`) into a single `framework/manifest/` package with a coherent public API.

**Dependencies:** Phase 1 (core clean), Phase 2 (structural renames done so import paths are stable).

**Work items:**

1. **Absorb `framework/config/` into `framework/manifest/`:**
   - Move `GlobalConfig`, `ModelRef`, `FeatureFlags`, `ArtifactWindowConfig`, `LoggingConfig`, workspace path helpers
   - Update all importers

2. **Absorb `framework/contract/` into `framework/manifest/`:**
   - Move `EffectiveAgentContract` resolution logic — becomes an internal function called during `Resolve()`
   - The `ResolvedManifest` type is the public output of resolution

3. **Absorb `framework/policybundle/` into `framework/manifest/`:**
   - Move `CompiledPolicyBundle`, `BuildFromSpec` — becomes `manifest.Compile()`
   - The authorization policy compilation chain becomes fully internal to the package

4. **Establish clean public API** per Section 7 of this spec: `Load()`, `Resolve()`, `Compile()`, `CompileContext()` (the last delegates to `framework/contextpolicy/` which is built in Phase 5)

5. **Add context policy declaration sections to `AgentManifest`** (additive, with defaults for existing manifests):
   - `ContextPolicy.RankerAdmission []string`
   - `ContextPolicy.TrustThreshold string`
   - `ContextPolicy.SummarizationPermitted bool`
   - `ContextPolicy.DerivationGenerationCap int`
   - `ContextPolicy.ScannerConfig []ScannerConfigDecl`
   - `ContextPolicy.TokenBudget int`

**Unit test cleanup:**

- Merge test coverage from `config/`, `contract/`, `policybundle/` into `framework/manifest/`
- Delete test files from absorbed packages after merging
- Add integration tests for the full `Load → Resolve → Compile` pipeline
- Add tests verifying context policy section defaults for manifests that omit it

**Exit criteria:**
- `go build ./framework/manifest/...` passes
- `go test ./framework/manifest/...` passes
- No `framework/config/`, `framework/contract/`, `framework/policybundle/` directories exist
- All callers of the absorbed packages import from `framework/manifest/` with the new API

---

### Phase 4 — `framework/knowledge/` Schema Extension

**Goal:** Extend `KnowledgeChunk` with the three-axis model, full provenance, tombstoning, and freshness edge propagation. Replace `framework/patterns/sqlite.go` with a graphdb-backed implementation.

**Dependencies:** Phase 1 (core clean — `SubjectRef` must be in `relurpnet/identity/`; `TrustClass` must be accessible from capability types).

**Work items:**

1. **Extend `KnowledgeChunk`** per Section 4.1 of this spec:
   - Add `MemoryClass`, `StorageMode`, `SourceOrigin`
   - Add `SourcePrincipal`, `AcquisitionMethod`, `AcquiredAt`, `TrustClass`, `ContentSchemaVersion`
   - Add `DerivedFrom`, `DerivationMethod`, `DerivationGeneration`, `CoverageHash`
   - Add `Tombstoned`, `SupersededBy`
   - Add `SuspicionScore`, `SuspicionFlags`
   - Keep existing fields for backward compatibility with persisted graphdb data; new fields default to zero values on read

2. **Add tombstoning to `ChunkStore`:**
   - `Tombstone(ctx, id ChunkID, supersededBy ChunkID) error`
   - `LoadIncludingTombstoned(id ChunkID) (*KnowledgeChunk, bool, error)`
   - Reads via `Load` and `LoadMany` exclude tombstoned chunks by default

3. **Add freshness edge propagation to `ChunkStore`:**
   - `MarkStale(ctx, ids []ChunkID, reason string) error`
   - `MarkStaleByCoverageHash(ctx, coverageHash string) error` — marks all summary chunks whose source changed
   - Wire into `InvalidationPass.HandleRevisionChanged` to call `MarkStale` for all chunks whose `SourceOrigin = file` and path matches affected paths

4. **Add new event types to `framework/core/event_types.go`:**
   - `EventChunkCommitted` — emitted on every ingestion commit; payload includes chunk ID, source identity, tombstoned IDs
   - `EventSummaryCommitted` — specialization of ChunkCommitted for `summary_derivation` chunks; includes `CoverageHash` and `DerivedFrom`
   - `EventContextPolicyReloaded` — emitted when manifest changes; triggers compiler cache invalidation
   - `EventProviderSessionEnded` — existing provider lifecycle event; now formally defined here

5. **Replace `framework/patterns/sqlite.go`** with `framework/patterns/graphdb.go`:
   - Implement `PatternStore` interface using `graphdb.Engine`
   - Pattern records become graphdb nodes with kind `"pattern_record"`
   - Status updates become label updates on the node
   - `Supersede` creates a `EdgeKindSupersedes` edge between old and new pattern nodes
   - Delete `sqlite.go`

**Unit test cleanup:**

- Update `knowledge_test.go` to exercise new chunk fields and tombstoning
- Add tests for `MarkStale`, `MarkStaleByCoverageHash`, `Tombstone`
- Add tests verifying `Load` excludes tombstoned chunks
- Add tests for new event types in `events.go`
- Delete `framework/patterns/` SQLite integration tests; replace with graphdb-backed tests
- Add round-trip tests for patterns via the new graphdb implementation

**Exit criteria:**
- `go test ./framework/knowledge/...` passes
- `go test ./framework/patterns/...` passes
- `KnowledgeChunk` has all three-axis fields and all provenance fields per spec
- `ChunkStore` exposes `Tombstone`, `MarkStale`, `MarkStaleByCoverageHash`
- No `framework/patterns/sqlite.go` exists
- All four new event type constants exist in `framework/core/event_types.go`

---

### Phase 5 — `framework/contextpolicy/`

**Goal:** Implement the context policy bundle compiler. This is the admission authority for both ingestion and persistence; it must exist before those packages can be built.

**Dependencies:** Phase 3 (manifest package exists and can compile policy sections), Phase 4 (KnowledgeChunk has TrustClass field; event types defined).

**Work items:**

1. Implement all types from Section 5.1 of this spec: `ContextPolicyBundle`, `RankerRef`, `ScannerRef`, `SummarizerRef`, `QuotaSpec`, `RateLimitSpec`, `CompilationMode`, `TrustDemotedPolicy`, `DegradedChunkPolicy`, `BudgetShortfallPolicy`, `SubstitutionPreference`

2. Implement `Compile(manifest, skills, defaults) (*ContextPolicyBundle, error)` — reads the `ContextPolicy` section of the resolved manifest; applies system defaults for any missing fields

3. Implement `Evaluator` with all admission methods from Section 5.1

4. Wire `manifest.CompileContext()` to call `contextpolicy.Compile()`

5. Implement quota tracking: `Evaluator.QuotaRemaining()` uses an internal `sync.Map` of atomic counters keyed by principal pattern; counters reset on time-window tick

**Unit tests:**

- Test `Compile()` with a minimal manifest (all defaults applied)
- Test `Compile()` with all fields explicitly set
- Test `Evaluator.AdmitTrustClass()` for each trust level
- Test `Evaluator.AdmitChunk()` with suspicion scores above and below thresholds
- Test `Evaluator.QuotaRemaining()` decrement and reset
- Test `Evaluator.PermitSummarization()` with and without prose model permission
- Test that manifests without context policy sections get valid defaults

**Exit criteria:**
- `go test ./framework/contextpolicy/...` passes
- `go test ./framework/manifest/...` passes (including `CompileContext` integration)
- A manifest with no context policy sections compiles to a valid bundle with system defaults
- A manifest with all context policy sections compiles correctly and the evaluator makes correct decisions

---

### Phase 6 — `framework/ingestion/`

**Goal:** Implement the six-stage ingestion pipeline and the workspace scanner.

**Dependencies:** Phase 4 (knowledge schema extended), Phase 5 (contextpolicy exists for admission stage).

**Work items:**

1. Implement the six stage types per Section 5.2: `RawIngestion`, `TypedIngestion`, `ScanResult`, `CandidateEdges`, `IngestDisposition`, `IngestResult`

2. Implement `Pipeline.Run()` executing the six stages in sequence

3. Implement all four public entry points: `AcquireFromFile`, `AcquireFromToolOutput`, `AcquireFromUserInput`, `AcquireFromCapabilityResult`

4. Implement parsers for each content type:
   - Go, Python, Rust, JavaScript/TypeScript via `framework/ast/`
   - Markdown via heading-structure detection
   - Plain text via line-window chunking with configurable overlap
   - Binary content: reject with event emission

5. Implement all four built-in scanners: `SignatureScanner`, `UnicodeTagScanner`, `StructuralHeuristicScanner`, `Base64BlobScanner`

6. Implement the `Scanner` interface for pluggability

7. Implement `WorkspaceScanner`:
   - File discovery respecting `.gitignore` and configured include/exclude globs
   - Parallel processing with bounded concurrency (`Concurrency` field)
   - `ScanIncremental` via `git diff --name-only <since>` to enumerate changed files
   - `EventBootstrapComplete` emission on scan completion

8. Implement quarantine: write offending content to `relurpify_cfg/quarantine/<timestamp>_<hash>/` with a `reason.txt` alongside; emit telemetry event

**Unit tests:**

- Test each stage in isolation with synthetic inputs
- Test `AcquireFromFile` for Go, Markdown, and plain text content
- Test `SignatureScanner` with known injection payloads from a labeled corpus
- Test `UnicodeTagScanner` with Unicode tag characters
- Test admission stage: commit, quarantine, and reject dispositions
- Test quota enforcement: principal over quota triggers quarantine
- Test `WorkspaceScanner` with a synthetic directory tree; verify expected chunk count
- Test `ScanIncremental` with a mocked git diff output
- Test that `EventBootstrapComplete` is emitted exactly once on scan completion

**Exit criteria:**
- `go test ./framework/ingestion/...` passes
- A synthetic workspace with 10 Go files and 5 Markdown files can be fully scanned; chunks are written to a test `ChunkStore`
- All six stages run for each file type
- `EventBootstrapComplete` is emitted after a full scan
- Injection-attempt content is quarantined, not committed
- Binary content is rejected with an event

---

### Phase 7 — `framework/persistence/`

**Goal:** Implement the runtime artifact write path per Section 5.3.

**Dependencies:** Phase 4 (knowledge schema), Phase 5 (contextpolicy for admission).

**Work items:**

1. Implement all types from Section 5.3: `PersistenceRequest`, `PersistenceResult`, `PersistenceAction`, `PersistenceAuditRecord`

2. Implement `Writer.Persist()`:
   - Structural validation (required fields, max content size from policy)
   - Trust class assignment from source principal via capability `TrustClass`
   - Suspicion check (lightweight: no full scanning; structural validation only)
   - Quota check via `contextpolicy.Evaluator.QuotaRemaining()`
   - Deduplication: if a chunk with same `ContentHash` and `SourceOrigin` exists, return `deduplicated` action
   - Commit to `knowledge.ChunkStore`
   - Emit `EventChunkCommitted`
   - Write `PersistenceAuditRecord`

3. Implement `Writer.PersistBatch()` with per-item error collection

4. Implement `memory.PromotionRequest` consumption: `Writer.PromoteFromMemory(ctx, store *memory.WorkingMemoryStore, reqs []memory.PromotionRequest) error`

**Unit tests:**

- Test `Persist()` for each `PersistenceAction` outcome
- Test deduplication: identical content hash produces `deduplicated` not `created`
- Test trust class propagation from source principal
- Test quota enforcement: principal at quota produces quarantine
- Test `PromoteFromMemory` with synthetic working memory entries
- Test `PersistBatch` with mixed success and failure items; verify partial results

**Exit criteria:**
- `go test ./framework/persistence/...` passes
- `Persist()` writes chunks to a test `ChunkStore` with correct three-axis fields and provenance
- Deduplication works correctly
- `EventChunkCommitted` is emitted for every committed chunk

---

### Phase 8 — `framework/summarization/`

**Goal:** Implement all summarization algorithms per Section 5.4.

**Dependencies:** Phase 4 (KnowledgeChunk schema for provenance), Phase 5 (contextpolicy for generation cap enforcement).

**Work items:**

1. Migrate seed types from `framework/summarization/seed_types.go` (moved from core in Phase 1) into the full implementation; delete the seed file

2. Implement `ASTSummarizer`: use `framework/ast/` to parse code chunks; preserve function signatures, class/type declarations, doc comments; replace function bodies with `// ...`; track token delta

3. Implement `HeadingSummarizer`: parse heading structure; preserve headings and first 1-2 sentences per section (configurable); produce table-of-contents-plus-ledes form

4. Implement `ProseSummarizer`: model-based; requires injected `core.LanguageModel`; builds a structured prompt requesting a summary within token budget; records model ID and prompt fingerprint in `DerivationMethod`

5. Implement `Route()` per Section 5.4: select summarizer by source type and content structure; enforce generation cap before invocation; compute `CoverageHash`

6. Implement summary provenance: `SummarizationResult` fields fully populate `DerivationMethod`, `DerivedFrom`, `DerivationGeneration`, `CoverageHash` for persistence routing

**Unit tests:**

- Test `ASTSummarizer` on a synthetic Go file: verify function signatures preserved, bodies elided
- Test `ASTSummarizer` token reduction: summary must be < 50% of source token count
- Test `HeadingSummarizer` on synthetic Markdown: verify ledes preserved, paragraph bodies removed
- Test `Route()` selects AST summarizer for Go chunks, heading for Markdown
- Test generation cap: a chunk with `DerivationGeneration = 2` under `Cap = 2` is rejected
- Test `ProseSummarizer` with a mock `LanguageModel`: verify prompt structure and provenance recording
- Test `CoverageHash` changes when source chunk versions change

**Exit criteria:**
- `go test ./framework/summarization/...` passes
- AST summarizer produces valid Go code summaries (signatures only)
- Generation cap is enforced
- All `SummarizationResult` provenance fields are populated correctly

---

### Phase 9 — `framework/memory/` (Rebuilt)

**Goal:** Implement working memory per Section 5.5. The name was deleted in Phase 2; this creates the package fresh.

**Dependencies:** Phase 2 (old memory deleted), Phase 1 (MemoryClass updated to working/streamed).

**Work items:**

1. Create `framework/memory/` with all types from Section 5.5: `WorkingMemoryStore`, `TaskMemory`, `MemoryEntry`, `MemoryRetriever` interface, `StateHydrator` interface, `PromotionRequest`

2. Implement `WorkingMemoryStore` with task-scoped isolation and `Evict()` for checkpoint cleanup

3. Implement `MemoryRetriever` concretely: query by key prefix, by `MemoryClass`, or full scan within a task scope

4. Implement `StateHydrator` concretely: takes `[]MemoryRecordEnvelope`, writes to agentgraph `Context` via a provided key mapping

5. Implement `MemoryQuery`: `TaskID`, `KeyPrefix`, `Class`, `Limit`

**Unit tests:**

- Test `Set` and `Get` with different memory classes
- Test task isolation: entries in task A are not visible in task B
- Test `Evict`: entries are gone after eviction
- Test `MemoryRetriever.Retrieve` with key prefix and class filters
- Test `StateHydrator.Hydrate` populates agentgraph context correctly
- Test `Snapshot` captures point-in-time state

**Exit criteria:**
- `go test ./framework/memory/...` passes
- `framework/memory/` compiles with no dependency on the deleted old memory package
- `WorkingMemoryStore` satisfies `MemoryRetriever` and `StateHydrator` interfaces

---

### Phase 10 — `framework/retrieval/` (Rebuilt)

**Goal:** Implement the targeted knowledge query API per Section 5.6. The name was deleted in Phase 2; this creates the package fresh.

**Dependencies:** Phase 2 (old retrieval deleted), Phase 4 (KnowledgeChunk schema), Phase 5 (contextpolicy for ranker admission).

**Work items:**

1. Create `framework/retrieval/` with all types from Section 5.6: `Ranker` interface, `RankerRegistry`, `RetrievalQuery`, `RetrievalResult`, `RankedChunk`, `Retriever`

2. Implement `RRF()` function: given N ranked lists and weights, produce a single merged ranked list with RRF scores

3. Implement `BM25Ranker` wrapping `framework/search/SearchEngine`

4. Implement `ASTProximityRanker` wrapping `framework/ast/IndexManager` — computes structural distance from query-anchor symbols to candidate chunks

5. Implement `GraphProximityRanker` — BFS from anchor chunk IDs in the knowledge graph; ranks by hop count

6. Implement `RecencyRanker` — sorts by `AcquiredAt` descending

7. Implement `Retriever.Retrieve()`: admit rankers via policy → scatter in parallel → fuse with RRF → apply trust filter → apply freshness filter → return `RetrievalResult`

8. Add `RetrievalNode` to `framework/agentgraph/` using the `Retriever` interface

**Unit tests:**

- Test `RRF()` with two lists of known order; verify merged order and scores
- Test `RRF()` with unequal weights; verify higher-weighted ranker dominates
- Test `RRF()` with chunks missing from one list; verify zero contribution
- Test `BM25Ranker` with a synthetic `SearchEngine`; verify result order matches BM25 ranking
- Test `Retriever.Retrieve()` scatter-gather with two mock rankers
- Test trust filtering: chunks below trust threshold are excluded from result
- Test freshness filtering: stale chunks are excluded or marked degraded per policy
- Test `RetrievalNode.Execute()` writes results to graph state at the configured key

**Exit criteria:**
- `go test ./framework/retrieval/...` passes
- `go test ./framework/agentgraph/...` passes (includes `RetrievalNode`)
- `RRF()` is deterministic: same inputs always produce the same output
- All four built-in rankers compile and pass unit tests

---

### Phase 11 — `framework/compiler/` (Read Direction)

**Goal:** Implement live context assembly, compilation cache, and event-driven cache invalidation per Section 5.7 (read direction).

**Dependencies:** Phase 5 (contextpolicy), Phase 4 (knowledge schema, event types), Phase 10 (retrieval), Phase 7 (persistence for write direction prerequisites).

**Work items:**

1. Implement all types from Section 5.7: `CompilationRequest`, `CompilationResult`, `CompilationRecord`, `SummarySubstitution`, `ReplayMode`, `CompilationDiff`

2. Implement `Compiler.Compile()` with all seven pipeline stages:
   - Ranker admission (from policy bundle)
   - Scatter (parallel ranker invocations via `retrieval.Retriever`)
   - RRF fusion
   - Trust-class filtering
   - Freshness filtering
   - Budget fitting (tail-drop; summary substitution deferred to Phase 12)
   - Emission + `CompilationRecord` construction

3. Implement compilation cache:
   - Cache key: `(query_fingerprint, manifest_fingerprint, policy_bundle_fingerprint, event_log_seq)`
   - Per-entry dependency bitmap (set of chunk IDs the compilation used)
   - On `EventChunkCommitted`: update global `invalidated-IDs-since-seq` set; evict cache entries whose bitmap intersects
   - On `EventContextPolicyReloaded`: evict all cache entries

4. Implement `Compiler.Start(ctx)`: subscribe to `EventChunkCommitted` and `EventContextPolicyReloaded` via `event.Log`; run invalidation loop

5. Emit `CompilationRecord` as an event via `event.Log` after every successful compilation

6. Implement `CompilationTrigger` interface in `framework/agentgraph/` (Section 6.5); verify `*Compiler` satisfies it

**Unit tests:**

- Test each compilation stage in isolation with synthetic `ChunkStore` data
- Test full `Compile()` pipeline end-to-end: query → emit ranked chunks in correct order
- Test cache hit: identical `CompilationRequest` on same event-log sequence returns cached result (no ranker invocations)
- Test cache invalidation on `EventChunkCommitted` for a chunk in the dependency bitmap
- Test cache non-invalidation: `EventChunkCommitted` for a chunk NOT in dependency bitmap does not evict
- Test `EventContextPolicyReloaded` evicts all cache entries
- Test trust filtering in compilation: chunk below threshold absent from result
- Test freshness filtering: stale chunk excluded or degraded per policy
- Test budget tail-drop: excess chunks are dropped from end of ranked list
- Determinism test: compile the same request twice; assert identical `CompilationRecord`

**Exit criteria:**
- `go test ./framework/compiler/...` passes
- Determinism test passes: identical inputs produce identical `CompilationRecord`
- Cache invalidation is correct: after a `ChunkCommitted` event for a depended-upon chunk, the next `Compile()` call re-runs rankers
- `*compiler.Compiler` satisfies `agentgraph.CompilationTrigger`

---

### Phase 12 — `framework/compiler/` (Write Direction)

**Goal:** Implement compiler-initiated summarization and persistence. Complete budget fitting with summary substitution.

**Dependencies:** Phase 11 (compiler read direction), Phase 8 (summarization), Phase 7 (persistence).

**Work items:**

1. Complete budget fitting in `Compiler.Compile()`:
   - After tail-drop, check if included chunks still exceed budget
   - For each candidate for substitution: check `policy.PermitSummarization()`; look up existing summary via `CoverageHash`; if found and not stale, substitute; if not found and `AutoSummarizeOnBudgetPressure`, invoke `summarization.Route()` synchronously
   - Record all substitutions in `CompilationRecord.Substitutions`
   - If budget cannot be met: emit shortfall in `CompilationRecord`; apply `BudgetShortfallPolicy` (fail or emit partial)

2. Route compiler-produced summaries through `persistence.Writer.Persist()` with:
   - `SourceOrigin = summary_derivation`
   - `DerivedFrom` populated from source chunk IDs
   - `DerivationGeneration` incremented correctly
   - `CoverageHash` set from `SummarizationResult`
   - Emit `EventSummaryCommitted`

3. Implement `Compiler.Replay()` per Section 5.7:
   - `StrictReplay`: reconstruct knowledge graph state at original `EventLogSeq` via event log replay; re-run compilation; compare outputs
   - `CurrentReplay`: re-run compilation against current state; return both results for diff

4. Implement `Compiler.Diff()` per Section 5.7: structured diff of two `CompilationRecord` objects; identifies added/removed chunks, rank changes, ranker contribution differences, substitution changes

**Unit tests:**

- Test summary substitution: compilation with budget pressure and existing summary produces correct substitution record
- Test on-demand summarization: when no summary exists and policy permits, `summarization.Route()` is called and the result is persisted
- Test generation cap: source chunks at `DerivationGeneration >= cap` are not substituted by further summarization
- Test `StrictReplay`: replay of a recorded compilation against the same event-log state produces identical output
- Test `CurrentReplay`: replay after a new chunk commit produces a diff showing the new chunk
- Test `Diff()`: two compilations with one chunk difference produce a diff with exactly that chunk in added/removed

**Exit criteria:**
- `go test ./framework/compiler/...` passes with write direction tests included
- Summary substitution produces correct `CompilationRecord.Substitutions`
- `Replay(StrictReplay)` produces output matching the original compilation record
- `Diff()` produces correct structured differences

---

### Phase 13 — `framework/agentgraph/` Integration

**Goal:** Complete agentgraph integration with the compiler, retrieval, and persistence subsystems. Remove all remaining legacy coupling.

**Dependencies:** Phase 11 (compiler, including `CompilationTrigger`), Phase 10 (`RetrievalNode` already added), Phase 9 (memory interfaces).

**Work items:**

1. **Remove `LLMNode` from `framework/agentgraph/`:**
   - `LLMNode` moves to `agents/` — a shared location within agents such as `agents/llm/llm_node.go` that agent paradigms can use
   - The agentgraph `graph.go` file removes `NodeTypeLLM` and its handling
   - Agents that previously used `agentgraph.LLMNode` now use the agents-layer version

2. **Wire `CompilationTrigger` into the agents-layer `LLMNode`:**
   - The moved `LLMNode` accepts an optional `CompilationTrigger` field
   - Before each LLM call, if `CompilationTrigger` is set, `Compile()` is called and the result is added to the LLM context

3. **Fix `framework/authorization/runtime.go` coupling:**
   - Define `AgentExecutor` interface in `framework/core/` with `Initialize(*core.Config) error` and `Execute(context.Context, *core.Task, *core.Context) (*core.Result, error)`
   - `authorization.AgentRegistration.Execute()` takes `AgentExecutor` instead of `graph.WorkflowExecutor`
   - `*agentgraph.Graph` satisfies `AgentExecutor`
   - Remove `framework/agentgraph` import from `framework/authorization/`

4. **Verify `RetrievalNode` contract** — ensure `NodeContract` is correct; `RetrievalNode` reads knowledge store (side effect: none on graph state, external read only)

5. **Verify `CheckpointNode` evicts working memory** — at checkpoint, `WorkingMemoryStore.Evict(taskID)` is called; implement this hookup

**Unit tests:**

- Test that `framework/authorization/` does not import `framework/agentgraph/` (boundary check)
- Test `CheckpointNode.Execute()` triggers `WorkingMemoryStore.Evict()`
- Test `RetrievalNode.Execute()` writes retrieval results to state at configured key
- Regression tests for all existing `agentgraph` node types: verify no behavioral change

**Exit criteria:**
- `go build ./framework/...` passes
- `go test ./framework/agentgraph/...` passes
- `go test ./framework/authorization/...` passes
- `framework/authorization/runtime.go` does not import `framework/agentgraph/`
- No `LLMNode` type exists in `framework/agentgraph/`
- `scripts/check-framework-boundaries.sh` passes

---

### Phase 14 — `framework/agentenv/` and `WorkspaceEnvironment` Migration

**Goal:** Move `WorkspaceEnvironment` from `ayenitd/` to `framework/agentenv/`. Update all field types to new packages. Reduce `ayenitd/` to a thin composition root.

**Dependencies:** Phases 9, 10, 11 (memory, retrieval, compiler all exist with their final types).

**Work items:**

1. **Create `framework/agentenv/environment.go`** with `WorkspaceEnvironment` per Section 8 of this spec; include all `With*` scoping methods

2. **Update `ayenitd/environment.go`**: remove the `WorkspaceEnvironment` type definition; add a type alias `type WorkspaceEnvironment = agentenv.WorkspaceEnvironment` temporarily, then migrate callers and remove the alias

3. **Update `ayenitd/bootstrap_extract.go`**: populate the new field types:
   - `WorkingMemory` from `memory.NewWorkingMemoryStore()`
   - `KnowledgeStore` from `knowledge.ChunkStore` (already wired in existing ayenitd)
   - `Retriever` from `retrieval.NewRetriever(store, registry)`
   - `Compiler` from `compiler.NewCompiler(store, retriever, events, persist)`
   - `EventLog` wired from platform event log implementation
   - Remove `GuidanceBroker`, `Embedder`, `RetrievalDB`, `PlanStore` fields and their wiring

4. **Update all callers** of `WorkspaceEnvironment` fields that have been renamed or removed

5. **Simplify `ayenitd/`**: the package's role is now: load config → probe workspace → init graphdb → construct `WorkspaceEnvironment` → return. No framework-level types should be defined in `ayenitd/`

**Unit tests:**

- Add `framework/agentenv/` tests: `TestWorkspaceEnvironmentWithRegistry` (shallow copy with replaced registry), `TestWorkspaceEnvironmentWithMemory`, `TestWorkspaceEnvironmentWithService`
- Update `ayenitd/composition_coverage_test.go` to use the new field set; remove assertions on deleted fields
- Verify `ayenitd.Open()` returns a `WorkspaceEnvironment` with all non-optional fields populated

**Exit criteria:**
- `go test ./framework/agentenv/...` passes
- `go test ./ayenitd/...` passes
- `WorkspaceEnvironment` is defined in `framework/agentenv/`; `ayenitd/` does not redefine it
- `ayenitd/environment.go` contains no type definitions other than internal helpers
- `ayenitd/` does not import `framework/guidance/`, `framework/plan/`, or `framework/retrieval/` (old)

---

### Phase 15 — `framework/skills/` Extension

**Goal:** Extend the skills package so that skill manifests can declare ingestion sources, ranker admissions, and scanner configurations that feed into `contextpolicy` bundle compilation.

**Dependencies:** Phase 5 (contextpolicy), Phase 6 (ingestion — `WorkspaceScanner` needs to know which paths skills contribute), Phase 3 (manifest consolidation — `SkillManifest` is now defined in `framework/manifest/`).

**Work items:**

1. **Extend `SkillManifest`** (in `framework/manifest/`) with a new optional section:
   ```yaml
   context_contributions:
     ingestion_sources:
       - path: "./prompts/**/*.md"
         source_type: "skill_resource"
     ranker_admission:
       - "bm25"
       - "ast_proximity"
     scanner_config:
       additional_signatures:
         - pattern: "<skill-specific injection pattern>"
           flag: "skill_signature"
   ```

2. **Update `contextpolicy.Compile()`** to merge skill `ContextContributions` into the policy bundle: union of admitted rankers, union of scanner signature lists, override trust thresholds only when skill declares a more restrictive value

3. **Update `WorkspaceScanner`** to accept skill ingestion source paths: skills that declare `ingestion_sources` contribute those paths to the scanner's include glob list at startup

4. **Update `framework/skills/resolve.go`** to extract `ContextContributions` from resolved skill manifests and return them alongside `SkillCapabilityCandidate` results

**Unit tests:**

- Test skill ingestion source paths are added to scanner include globs
- Test ranker admissions from skills are merged into policy bundle
- Test conflicting trust thresholds: more restrictive skill value wins
- Test scanner signatures from skills are added to built-in scanner corpus

**Exit criteria:**
- `go test ./framework/skills/...` passes
- `go test ./framework/contextpolicy/...` passes (including skill contribution tests)
- A `SkillManifest` with `context_contributions` compiles into a policy bundle that includes those contributions

---

### Phase 16 — Compiler Replay and Diffing Capabilities

**Goal:** Expose replay and diffing as framework capabilities, completing the observability surface from Section 10 of the context subsystem spec.

**Dependencies:** Phase 12 (compiler write direction complete; `CompilationRecord` events emitted and persisted).

**Work items:**

1. **Persist `CompilationRecord` events to the knowledge store** via `persistence.Writer` so they are queryable by ID: `SourceOrigin = "compilation_record"`, with the full `CompilationRecord` JSON as content

2. **Implement `Compiler.Replay(ctx, compilationID, mode)`:**
   - Load `CompilationRecord` by ID from knowledge store
   - `StrictReplay`: replay event log to original `EventLogSeq`; re-run `Compile()` with original inputs; return both results
   - `CurrentReplay`: re-run `Compile()` against current state; return both results
   - For `StrictReplay`, assert output matches original (determinism check); any mismatch is logged as a bug

3. **Implement `Compiler.Diff(ctx, idA, idB)`** producing `CompilationDiff` per Section 5.7:
   - Chunks present in A not B, and vice versa
   - Chunks present in both but at different ranks
   - Ranker-output differences
   - Filter-decision differences
   - Substitution differences

4. **Register replay and diff as framework capabilities** in `framework/capability/` so agents can invoke them through the standard capability invocation path

**Unit tests:**

- Test `StrictReplay` on a recorded compilation: output matches original byte-for-byte
- Test `CurrentReplay` after a new chunk commit: diff identifies the new chunk
- Test `Diff()` with two compilations differing by one chunk: diff contains exactly one added and one removed entry
- Test `Diff()` with a rank change: diff identifies the changed rank without listing the chunk as added/removed
- Test that `Replay(StrictReplay)` on a compilation where a source chunk has been tombstoned produces a determinism mismatch log entry (not a panic)

**Exit criteria:**
- `go test ./framework/compiler/...` passes with replay and diff tests
- `Replay(StrictReplay)` returns identical `CompilationRecord` for the same event-log state
- `Diff()` produces correct structured differences for all diff categories
- Replay and diff are registered capabilities in `framework/capability/`
- `scripts/check-framework-boundaries.sh` passes
- `go build ./...` passes (excluding known-broken `agents/` and `named/`)
- `go test ./framework/...` passes in full

---

## Appendix A — Boundary Script Requirements

The following boundary checks must pass after every phase:

1. `scripts/check-framework-boundaries.sh` — no `framework/` package imports `agents/` or `named/`
2. `scripts/check-deprecated-agent-wrappers.sh` — `app/` and `testsuite/` do not call deprecated agent wrappers
3. New check (to be added): no `framework/` package imports `ayenitd/` — verify the layering rule from CLAUDE.md
4. New check (to be added): no SQLite import (`database/sql` with a SQLite driver) anywhere in `framework/`

---

## Appendix B — Global Test Invariants

These invariants must hold at the end of every phase, not just at the end of the full rework:

- `go build ./framework/...` passes
- `go test ./framework/...` passes (all framework unit tests; no integration tests requiring Ollama)
- No `MemoryClassDeclarative` or `MemoryClassProcedural` references anywhere in `framework/`
- No SQLite driver imports anywhere in `framework/`
- No `context_items.go` types (`ContextItem`, `MemoryContextItem`, `RetrievalContextItem`) referenced anywhere in `framework/`
- The boundary scripts pass

---

## Appendix C — Deferred Concerns

The following are explicitly deferred and not addressed by this plan:

- `agents/` and `named/` package rework — depends on framework API stabilization from this plan
- `archaeo/` internal rework — the guidance and plan moves in Phase 2 are the only archaeo-touching phases
- `ayenitd/` full deletion — ayenitd becomes thin in Phase 14 but is not deleted; deletion is a separate step
- Cross-workspace knowledge federation
- Embedding-based retrieval (out of scope by principle)
- UX for quarantine review, knowledge browsing, or pattern curation
- Pattern proposal flow
- `relurpnet/` internal rework beyond receiving the moved identity/node/fmp types
