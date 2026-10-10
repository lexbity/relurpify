package knowledge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

const (
	// groundingKind is the canonical chunk kind for every grounded capture.
	groundingKind = ChunkKindCapture
	// groundingSchema is the canonical content schema version (§3.2).
	groundingSchema = 1
	// groundsEdgeCap bounds the grounds edge set per capture.
	groundsEdgeCap = 16
	// derivesEdgeCap bounds the derives_from edge set per capture.
	derivesEdgeCap = 8
)

// ErrGroundingFailed is the typed failure a grounding barrier surfaces after
// its bounded retry is exhausted. Callers match it with errors.Is.
var ErrGroundingFailed = errors.New("knowledge: grounding failed")

// Epistemics is the closed epistemic annotation vocabulary.
type Epistemics string

const (
	// EpistemicClaimed means the value is the agent's claim (default).
	EpistemicClaimed Epistemics = "claimed"
	// EpistemicGiven means the value is claimed to be verbatim user-provided.
	EpistemicGiven Epistemics = "given"
)

// Valid reports whether the epistemics is a member of the closed set.
func (e Epistemics) Valid() bool {
	return e == EpistemicClaimed || e == EpistemicGiven
}

// String returns the spelling used in persisted chunk props.
func (e Epistemics) String() string { return string(e) }

// QuotaChecker reports remaining admission quota for a principal.
type QuotaChecker interface {
	QuotaRemaining(principalID string) int
}

// GroundingItem is one capture enqueued for grounding at the epoch barrier.
type GroundingItem struct {
	Value          any
	TypeAnnotation string
	Epistemics     Epistemics
	Origin         contextdata.OriginClass
	StateKey       string // capturing destination, e.g. "state.findings"
	NodeID         string
	TaskID         string
	SessionID      string
	WorkspaceID    string
	RecipeID       string
	Epoch          uint64
	Kind           ChunkKind // capture by default; tool results use ChunkKindTool
	SourceChunkIDs []ChunkID // streamed context of the producing step (≤16)
	ForwardedFrom  []ChunkID // prior chunks when the capture forwards state
}

// GivenOriginLookup resolves whether a value is identity-equal to a
// user-origin envelope value. It is provided by the capture site; when
// absent, the capture-site check is trusted and no runtime re-check runs.
type GivenOriginLookup func(value any) (contextdata.OriginClass, bool)

// GroundingAction is the per-item outcome class.
type GroundingAction string

const (
	GroundingGrounded    GroundingAction = "grounded"
	GroundingQuarantined GroundingAction = "quarantined"
	GroundingSkipped     GroundingAction = "skipped"
)

// GroundingEntry is one per-item outcome line.
type GroundingEntry struct {
	Index          int
	ChunkID        ChunkID
	Action         GroundingAction
	TrustClass     agentspec.TrustClass
	Epistemics     Epistemics
	Reason         string
	AlreadyExisted bool
}

// GroundingReport is the retry unit returned by Ground.
type GroundingReport struct {
	Grounded    []GroundingEntry
	Skipped     []GroundingEntry
	Quarantined []GroundingEntry
}

// GroundingService is the synchronous, atomic, admission-gated write path for
// recipe captures. It is stateless between Ground calls, so a barrier may retry
// a failed batch safely.
type GroundingService struct {
	store     *ChunkStore
	events    *EventBus
	quota     QuotaChecker
	telemetry telemetry.Telemetry
	now       func() time.Time
	lookup    GivenOriginLookup
}

// NewGroundingService constructs the grounding service over a chunk store.
func NewGroundingService(store *ChunkStore, events *EventBus, quota QuotaChecker, tel telemetry.Telemetry) *GroundingService {
	return &GroundingService{
		store:     store,
		events:    events,
		quota:     quota,
		telemetry: tel,
		now:       time.Now,
	}
}

// SetClock installs a deterministic time source (tests).
func (g *GroundingService) SetClock(now func() time.Time) *GroundingService {
	if g != nil {
		g.now = now
	}
	return g
}

// SetGivenOriginLookup installs the defense-in-depth identity check for `given`
// captures.
func (g *GroundingService) SetGivenOriginLookup(lookup GivenOriginLookup) *GroundingService {
	if g != nil {
		g.lookup = lookup
	}
	return g
}

// ChunkIDForCaptureValue resolves the canonical chunk ID a previously grounded
// capture value carries, by recomputing its content-addressed ID (the same
// canonical encoding Ground uses) and loading it from the store. A missing,
// tombstoned, or stale chunk resolves to absent — (zero, false, nil) — never
// an error; only store failures surface as errors. It is the read-input
// resolver for authored blackboard sources (Wave 3 D6): a source's `read`
// context becomes derives_from provenance when the value was grounded, and is
// recorded as an absent input when it was not.
func (g *GroundingService) ChunkIDForCaptureValue(value any, typeAnnotation string) (ChunkID, bool, error) {
	if g == nil || g.store == nil {
		return "", false, nil
	}
	encoded, err := canonicalCaptureItem(GroundingItem{Value: value, TypeAnnotation: typeAnnotation})
	if err != nil {
		return "", false, fmt.Errorf("encode capture value: %w", err)
	}
	id := CanonicalChunkID(ChunkKindCapture, encoded)
	chunk, ok, err := g.store.LoadIncludingTombstoned(id)
	if err != nil || !ok {
		return "", false, err
	}
	if chunk.Freshness != FreshnessValid {
		return "", false, nil
	}
	return id, true, nil
}

// Ground commits every admitted item in one atomic store transaction and
// returns the per-item report. Admission (suspicion, quota) runs before the
// transaction; quarantined and skipped items never reach the store.
func (g *GroundingService) Ground(ctx context.Context, items []GroundingItem) (GroundingReport, error) {
	if g == nil || g.store == nil || g.store.Graph == nil {
		return GroundingReport{}, fmt.Errorf("%w: grounding store is required", ErrGroundingFailed)
	}
	report := GroundingReport{}
	prepared, err := g.prepare(ctx, items, &report)
	if err != nil {
		return report, fmt.Errorf("%w: %v", ErrGroundingFailed, err)
	}
	if len(prepared) > 0 {
		if err := g.store.GroundBatch(ctx, prepared); err != nil {
			return report, fmt.Errorf("%w: %w", ErrGroundingFailed, err)
		}
		g.emitGrounded(prepared)
	}
	return report, nil
}

// prepare builds the fully-merged chunks and their edges for one batch.
func (g *GroundingService) prepare(ctx context.Context, items []GroundingItem, report *GroundingReport) ([]preparedChunk, error) {
	out := make([]preparedChunk, 0, len(items))
	for i, item := range items {
		if !item.Epistemics.Valid() {
			item.Epistemics = EpistemicClaimed
		}
		if !item.Origin.Valid() {
			item.Origin = contextdata.OriginLLM
		}

		encoded, err := canonicalCaptureItem(item)
		if err != nil {
			return nil, fmt.Errorf("encode capture %d: %w", i, err)
		}
		if reason, suspicious := suspiciousGroundingValue(item.Value); suspicious {
			entry := GroundingEntry{Index: i, Action: GroundingQuarantined, Epistemics: item.Epistemics, Reason: reason}
			report.Quarantined = append(report.Quarantined, entry)
			g.emitQuarantined(item, reason)
			continue
		}
		if g.quota != nil && g.quota.QuotaRemaining(item.WorkspaceID) <= 0 {
			entry := GroundingEntry{Index: i, Action: GroundingSkipped, Epistemics: item.Epistemics, Reason: "quota exceeded"}
			report.Skipped = append(report.Skipped, entry)
			continue
		}

		trust, epistemics, downgraded := g.resolveTrust(item)
		if downgraded {
			g.emitDowngraded(item, trust)
		}

		id := CanonicalChunkID(kindFor(item), encoded)
		existing, _, err := g.store.LoadIncludingTombstoned(id)
		if err != nil {
			return nil, err
		}
		now := g.now().UTC()
		chunk := KnowledgeChunk{
			ID:                id,
			WorkspaceID:       item.WorkspaceID,
			ContentHash:       contentHashForText(string(encoded)),
			TokenEstimate:     estimateTokens(string(encoded)),
			MemoryClass:       MemoryClassWorking,
			StorageMode:       StorageModeInline,
			SourceOrigin:      sourceOriginForClass(item.Origin),
			AcquisitionMethod: AcquisitionMethodRuntimeWrite,
			AcquiredAt:        now,
			TrustClass:        trust,
			DerivedFrom:       append([]ChunkID(nil), item.ForwardedFrom...),
			OriginClass:       string(item.Origin),
			Epistemics:        string(epistemics),
			GroundedBy:        []GroundingRecord{{TaskID: item.TaskID, NodeID: item.NodeID, Epoch: item.Epoch, RecipeID: item.RecipeID}},
			Freshness:         FreshnessValid,
			Provenance:        ChunkProvenance{SessionID: item.SessionID, WorkflowID: item.TaskID, CompiledBy: CompilerDeterministic, Timestamp: now},
			Body:              ChunkBody{Raw: string(encoded), Fields: groundingFields(item)},
			CreatedAt:         now,
			UpdatedAt:         now,
		}
		srcGeneration, err := g.maxSourceGeneration(ctx, item.SourceChunkIDs)
		if err != nil {
			return nil, err
		}
		chunk.DerivationGeneration = srcGeneration + 1

		alreadyExisted := false
		if existing != nil {
			switch applyMergeRule(&chunk, existing) {
			case mergePreserved:
				entry := GroundingEntry{Index: i, ChunkID: id, Action: GroundingSkipped, TrustClass: trust, Epistemics: epistemics, Reason: "tombstone preserved"}
				report.Skipped = append(report.Skipped, entry)
				g.emitTombstonePreserved(item, id)
				continue
			case mergeMerged, mergeResurrected:
				// A resurrected chunk re-establishes a retracted fact, so its
				// grounding history spans the retraction: the prior records
				// stay in GroundedBy and the report marks the identity as
				// pre-existing, exactly as for a live merge.
				chunk.GroundedBy = append(append([]GroundingRecord(nil), existing.GroundedBy...), chunk.GroundedBy...)
				alreadyExisted = true
			}
		} else if chunk.Version <= 0 {
			chunk.Version = 1
		}

		entry := GroundingEntry{
			Index:          i,
			ChunkID:        chunk.ID,
			Action:         GroundingGrounded,
			TrustClass:     trust,
			Epistemics:     epistemics,
			AlreadyExisted: alreadyExisted,
		}
		report.Grounded = append(report.Grounded, entry)
		out = append(out, preparedChunk{
			chunk:   chunk,
			grounds: capChunkIDs(item.SourceChunkIDs, groundsEdgeCap),
			derives: capChunkIDs(item.ForwardedFrom, derivesEdgeCap),
		})
	}
	return out, nil
}

// resolveTrust computes the grounded trust class: the more restrictive of the
// epistemic annotation and the dataflow origin floor, with the capture-site
// given identity re-check as defense in depth.
func (g *GroundingService) resolveTrust(item GroundingItem) (agentspec.TrustClass, Epistemics, bool) {
	// Defense-in-depth: re-verify the given identity at grounding time when the
	// capture site supplied a lookup.
	if item.Epistemics == EpistemicGiven && g.lookup != nil {
		if origin, ok := g.lookup(item.Value); ok && origin != contextdata.OriginUser {
			return trustForOrigin(item.Origin), EpistemicClaimed, true
		}
	}
	return epistemicTrust(item.Epistemics, item.Origin)
}

// epistemicTrust is the D4 trust decision shared by grounding and restore: the
// more restrictive of the epistemic annotation and the dataflow origin floor.
func epistemicTrust(epistemics Epistemics, origin contextdata.OriginClass) (agentspec.TrustClass, Epistemics, bool) {
	if !epistemics.Valid() {
		epistemics = EpistemicClaimed
	}
	byAnnotation := map[Epistemics]agentspec.TrustClass{
		EpistemicClaimed: agentspec.TrustClassLLMGenerated,
		EpistemicGiven:   agentspec.TrustClassWorkspaceTrusted,
	}
	annotation := byAnnotation[epistemics]
	floor := trustForOrigin(origin)
	if trustRestrictiveness(floor) > trustRestrictiveness(annotation) {
		return floor, EpistemicClaimed, epistemics == EpistemicGiven
	}
	return annotation, epistemics, false
}

func trustForOrigin(origin contextdata.OriginClass) agentspec.TrustClass {
	byOrigin := map[contextdata.OriginClass]agentspec.TrustClass{
		contextdata.OriginUser: agentspec.TrustClassWorkspaceTrusted,
		contextdata.OriginTool: agentspec.TrustClassToolResult,
		contextdata.OriginLLM:  agentspec.TrustClassLLMGenerated,
	}
	if class, ok := byOrigin[origin]; ok {
		return class
	}
	return agentspec.TrustClassLLMGenerated
}

// trustRestrictiveness orders the decision's trust classes from least to most
// restrictive (§D4). Unknown classes are treated as fully restrictive so trust
// can never be elevated above a class the runtime does not model.
func trustRestrictiveness(class agentspec.TrustClass) int {
	switch class {
	case agentspec.TrustClassWorkspaceTrusted:
		return 1
	case agentspec.TrustClassLLMGenerated:
		return 2
	default:
		return 3
	}
}

// maxSourceGeneration returns the highest derivation generation among the
// bounded source set, or 0 when there are no sources.
func (g *GroundingService) maxSourceGeneration(ctx context.Context, sourceIDs []ChunkID) (int, error) {
	sources, err := g.store.LoadMany(capChunkIDs(sourceIDs, groundsEdgeCap))
	if err != nil {
		return 0, err
	}
	max := 0
	for _, source := range sources {
		if source.DerivationGeneration > max {
			max = source.DerivationGeneration
		}
	}
	return max, nil
}

func (g *GroundingService) emitGrounded(prepared []preparedChunk) {
	for _, item := range prepared {
		chunk := item.chunk
		groundedBy := chunk.GroundedBy
		nodeID := ""
		if len(groundedBy) > 0 {
			nodeID = groundedBy[0].NodeID
		}
		if g.events != nil {
			g.events.EmitChunkIngested(ChunkIngestedPayload{
				SessionID:     chunk.Provenance.SessionID,
				WorkflowID:    chunk.Provenance.WorkflowID,
				NodeID:        nodeID,
				ChunkID:       string(chunk.ID),
				ContentHash:   chunk.ContentHash,
				SourceOrigin:  string(chunk.SourceOrigin),
				TokenEstimate: chunk.TokenEstimate,
			})
		}
		if g.telemetry == nil {
			continue
		}
		g.telemetry.Emit(telemetry.Event{
			Type:      telemetry.EventCaptureGrounded,
			Message:   "capture grounded",
			Timestamp: chunk.UpdatedAt,
			Metadata: map[string]any{
				"node_id":     nodeID,
				"chunk_id":    string(chunk.ID),
				"trust_class": string(chunk.TrustClass),
				"epistemics":  chunk.Epistemics,
			},
		})
	}
}

func (g *GroundingService) emitQuarantined(item GroundingItem, reason string) {
	if g == nil || g.telemetry == nil {
		return
	}
	g.telemetry.Emit(telemetry.Event{
		Type:      telemetry.EventCaptureQuarantined,
		Message:   "capture quarantined",
		Timestamp: g.now(),
		Metadata: map[string]any{
			"node_id":   item.NodeID,
			"state_key": item.StateKey,
			"reason":    reason,
		},
	})
}

func (g *GroundingService) emitDowngraded(item GroundingItem, trust agentspec.TrustClass) {
	if g == nil || g.telemetry == nil {
		return
	}
	g.telemetry.Emit(telemetry.Event{
		Type:      telemetry.EventCaptureEpistemicsDowngraded,
		Message:   "capture epistemics downgraded",
		Timestamp: g.now(),
		Metadata: map[string]any{
			"node_id":     item.NodeID,
			"state_key":   item.StateKey,
			"trust_class": string(trust),
		},
	})
}

func (g *GroundingService) emitTombstonePreserved(item GroundingItem, id ChunkID) {
	if g == nil {
		return
	}
	if g.events != nil {
		g.events.EmitTombstonePreserved(TombstonePreservedPayload{
			ChunkID: string(id),
			Kind:    string(kindFor(item)),
		})
	}
}

// canonicalCaptureItem encodes a capture into the deterministic content that
// feeds CanonicalChunkID. Provenance lives in edges; identical values from two
// runs hash equal.
func canonicalCaptureItem(item GroundingItem) ([]byte, error) {
	encodedValue, err := json.Marshal(item.Value)
	if err != nil {
		return nil, err
	}
	return CanonicalJSON(captureEncoding{
		Schema: groundingSchema,
		Type:   item.TypeAnnotation,
		Value:  encodedValue,
	})
}

type captureEncoding struct {
	Schema int             `json:"schema"`
	Type   string          `json:"type"`
	Value  json.RawMessage `json:"value"`
}

func groundingFields(item GroundingItem) map[string]any {
	fields := make(map[string]any, 3)
	fields["kind"] = string(kindFor(item))
	fields["type"] = item.TypeAnnotation
	if item.StateKey != "" {
		fields["state_key"] = item.StateKey
	}
	if item.RecipeID != "" {
		fields["recipe_id"] = item.RecipeID
	}
	return fields
}

// kindFor resolves the canonical chunk kind for a grounding item, defaulting
// to capture so existing callers remain byte-identical.
func kindFor(item GroundingItem) ChunkKind {
	if item.Kind.Valid() {
		return item.Kind
	}
	return ChunkKindCapture
}

// CaptureKindForOrigin resolves the canonical chunk kind for a runtime
// capture from its dataflow origin floor. It is the grounding-side heir of
// the output ingester's input taxonomy (the ingester was absorbed into this
// commit boundary; see devdocs/plans/bkc-bidirectional-context-and-hermetic-
// dryrun-spec.md, Phase 3): a capture whose dataflow floor is tool output
// grounds as a tool fact (ChunkKindTool, the kind the capture encoding already
// documents for tool results), while agent claims and user-given values ground
// as ChunkKindCapture. Verbatim LLM-response and observation ingestion stay
// deferred by design: without a summarizer and a recipe-level policy, storing
// model output under a summarized-storage contract would be dishonest metadata.
func CaptureKindForOrigin(origin contextdata.OriginClass) ChunkKind {
	if origin == contextdata.OriginTool {
		return ChunkKindTool
	}
	return ChunkKindCapture
}

func sourceOriginForClass(origin contextdata.OriginClass) SourceOrigin {
	switch origin {
	case contextdata.OriginUser:
		return SourceOriginUser
	case contextdata.OriginTool:
		return SourceOriginTool
	default:
		return SourceOriginLLM
	}
}

func capChunkIDs(ids []ChunkID, cap int) []ChunkID {
	if cap <= 0 {
		return nil
	}
	out := make([]ChunkID, 0, cap)
	seen := make(map[ChunkID]struct{}, cap)
	for _, id := range ids {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
		if len(out) >= cap {
			break
		}
	}
	return out
}

// suspiciousGroundingValue runs the shared suspicion predicate over the string
// content carried by a capture value. JSON encoding escapes control bytes, so
// the check must inspect the value itself rather than its canonical encoding.
func suspiciousGroundingValue(value any) (string, bool) {
	switch typed := value.(type) {
	case string:
		return SuspicionReason([]byte(typed))
	case []byte:
		return SuspicionReason(typed)
	case []any:
		for _, element := range typed {
			if reason, suspicious := suspiciousGroundingValue(element); suspicious {
				return reason, true
			}
		}
	case map[string]any:
		for _, element := range typed {
			if reason, suspicious := suspiciousGroundingValue(element); suspicious {
				return reason, true
			}
		}
	}
	return "", false
}

// contentHashForText is the grounding boundary's content digest: stable,
// truncated SHA-256 over the encoded capture value.
func contentHashForText(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:16])
}
