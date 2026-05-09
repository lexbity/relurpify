package intentcontext

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/graphdb"
	"codeburg.org/lexbit/relurpify/framework/retrieval"
)

type EntityKind string

const (
	EntityKindUnknown   EntityKind = "unknown"
	EntityKindFunction  EntityKind = "function"
	EntityKindMethod    EntityKind = "method"
	EntityKindType      EntityKind = "type"
	EntityKindInterface EntityKind = "interface"
	EntityKindPackage   EntityKind = "package"
	EntityKindFile      EntityKind = "file"
	EntityKindModule    EntityKind = "module"
	EntityKindSymbol    EntityKind = "symbol"
	EntityKindScope     EntityKind = "scope"
)

type AnchorClass string

const (
	AnchorClassClarifiedEntity   AnchorClass = "clarified_entity"
	AnchorClassClarifiedModule   AnchorClass = "clarified_module"
	AnchorClassClarifiedScope    AnchorClass = "clarified_scope"
	AnchorClassClarifiedRelation AnchorClass = "clarified_relation_root"
)

type AmbiguityKind string

const (
	AmbiguityKindUnknown        AmbiguityKind = "unknown"
	AmbiguityKindUnderspecified AmbiguityKind = "underspecified"
	AmbiguityKindWorkspaceBound AmbiguityKind = "workspace_bound"
	AmbiguityKindMultiMatch     AmbiguityKind = "multi_match"
	AmbiguityKindContradictory  AmbiguityKind = "contradictory"
	AmbiguityKindScopeAmbiguous AmbiguityKind = "scope_ambiguous"
)

type ResponseKind string

const (
	ResponseKindUnknown  ResponseKind = "unknown"
	ResponseKindFreeText ResponseKind = "free_text"
	ResponseKindConfirm  ResponseKind = "confirm"
	ResponseKindChoice   ResponseKind = "choice"
	ResponseKindSymbol   ResponseKind = "symbol"
	ResponseKindScope    ResponseKind = "scope"
	ResponseKindRelation ResponseKind = "relation"
)

type ScopeKind string

const (
	ScopeKindUnknown         ScopeKind = "unknown"
	ScopeKindPackageSubtree  ScopeKind = "package_subtree"
	ScopeKindServiceBoundary ScopeKind = "service_boundary"
	ScopeKindDirectory       ScopeKind = "directory"
	ScopeKindSymbolCluster   ScopeKind = "symbol_cluster"
	ScopeKindGraphComponent  ScopeKind = "graph_component"
)

type ProjectionStatus string

const (
	ProjectionStatusPending  ProjectionStatus = "pending"
	ProjectionStatusApplied  ProjectionStatus = "applied"
	ProjectionStatusSkipped  ProjectionStatus = "skipped"
	ProjectionStatusConflict ProjectionStatus = "conflict"
	ProjectionStatusRejected ProjectionStatus = "rejected"
)

type ClarificationStepStatus string

const (
	ClarificationStepStatusPending    ClarificationStepStatus = "pending"
	ClarificationStepStatusSucceeded  ClarificationStepStatus = "succeeded"
	ClarificationStepStatusRetryable  ClarificationStepStatus = "retryable"
	ClarificationStepStatusFailed     ClarificationStepStatus = "failed"
	ClarificationStepStatusContradict ClarificationStepStatus = "contradictory"
)

type ValidationMode string

const (
	ValidationModeStrict  ValidationMode = "strict"
	ValidationModePartial ValidationMode = "partial"
	ValidationModeRepair  ValidationMode = "repair"
)

type EntityRef struct {
	EntityID string
	ChunkID  string
	Kind     EntityKind
	Name     string
	FilePath string
	Package  string
	StableID string
}

type ConfirmedEntity struct {
	StableID     string
	Name         string
	Kind         EntityKind
	ResolverKey  string
	EntityRef    EntityRef
	SourceTurnID string
	ConfirmedAt  time.Time
}

type ConfirmedScope struct {
	StableID     string
	Name         string
	AnchorClass  AnchorClass
	Entities     []ConfirmedEntity
	Rationale    string
	SourceTurnID string
	ConfirmedAt  time.Time
}

type RelationIntent struct {
	StableID       string
	SourceEntityID string
	TargetEntityID string
	RelationKind   string
	Direction      string
	Evidence       string
	SourceTurnID   string
	Status         ProjectionStatus
}

type ProjectionProvenance struct {
	TaskID         string
	SessionID      string
	TurnID         string
	StateVersion   uint64
	AnswerText     string
	SourcePromptID string
	Extractor      string
}

type ProjectionIntent struct {
	StableID       string
	RevisionRootID string
	MutationKind   string
	SubjectIDs     []string
	ObjectIDs      []string
	EdgeKind       string
	NodeKind       string
	Provenance     ProjectionProvenance
	IdempotencyKey string
	RevisionOf     string
}

type ProjectionRecord struct {
	StableID       string
	RevisionRootID string
	IdempotencyKey string
	GraphRecordIDs []string
	AppliedAt      time.Time
	AppliedBy      string
	Result         ProjectionStatus
	RevisionOf     string
}

type ProjectionConflict struct {
	StableID           string
	Reason             string
	ExistingMutationID string
	ProposedMutationID string
}

type ProjectionPlan struct {
	PlanID       string
	StateVersion uint64
	Intents      []ProjectionIntent
	Conflicts    []ProjectionConflict
}

type ProjectionResult struct {
	PlanID    string
	Applied   []ProjectionRecord
	Skipped   []ProjectionIntent
	Conflicts []ProjectionConflict
	Mutation  *graphdb.MutationResult
}

type ClarificationTurn struct {
	StableID     string
	TurnID       string
	PromptID     string
	PromptFamily string
	Question     string
	Answer       string
	ResponseKind ResponseKind
	StateVersion uint64
	SourceTurnID string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type AmbiguityCharacterization struct {
	Kind               AmbiguityKind
	Confidence         float64
	Rationale          string
	CandidateFamilies  []string
	CandidateScopes    []ScopeDeclaration
	CandidateEntities  []EntityRef
	NeedsClarification bool
}

type ClarificationQuestion struct {
	StableID     string
	PromptID     string
	PromptFamily string
	Text         string
	ExpectedKind ResponseKind
	AnswerSchema string
	TurnID       string
}

type ScopeDeclaration struct {
	StableID    string
	Kind        ScopeKind
	Name        string
	AnchorClass AnchorClass
	Selector    string
	ResolverKey string
}

type ScopeResolution struct {
	Decl       ScopeDeclaration
	ScopeID    string
	ChunkIDs   []string
	AnchorIDs  []string
	Confidence float64
	Rationale  string
}

type GroundingResult struct {
	StateVersion uint64
	Anchors      []retrieval.AnchorRef
	Added        []retrieval.AnchorRef
	Reused       []retrieval.AnchorRef
	Conflicts    []string
}

type ClarificationStepResult struct {
	StepID            string
	StateVersion      uint64
	TurnID            string
	Status            ClarificationStepStatus
	PromptID          string
	ValidationErrors  []string
	ResolvedEntities  []EntityRef
	ResolvedScopes    []ScopeResolution
	Grounding         GroundingResult
	Projection        *ProjectionResult
	Retrieval         *retrieval.RetrievalResult
	ResidualAmbiguity *AmbiguityCharacterization
}

type ClarificationStepConfig struct {
	OutputSchemaID   string
	ValidationMode   ValidationMode
	RequiredFields   []string
	AllowedStatuses  []ClarificationStepStatus
	StateWriteKeys   []string
	ProjectionPolicy string
	RequeryOnSuccess bool
}

type ClarificationState struct {
	TaskID                 string
	SessionID              string
	StateVersion           uint64
	CurrentTurnID          string
	ActiveThoughtRecipeID  string
	Turns                  []ClarificationTurn
	Ambiguity              *AmbiguityCharacterization
	ConfirmedEntities      []ConfirmedEntity
	ConfirmedScopes        []ConfirmedScope
	PendingQuestions       []ClarificationQuestion
	PendingRelationIntents []RelationIntent
	GroundedAnchors        []retrieval.AnchorRef
	PendingProjection      []ProjectionIntent
	AppliedMutations       []ProjectionRecord
	LastCheckpointID       string
	LastCheckpointSeq      uint64
	LastUpdatedAt          time.Time
}

// IntentEvidence captures the structured semantic evidence extracted from a request.
type IntentEvidence struct {
	ActionType            string
	Target                string
	Scope                 string
	RiskLevel             string
	ExpectedVerb          string
	ExplicitFiles         []string
	UserFiles             []string
	WorkspaceScopes       []string
	SessionPins           []string
	ContextHints          []string
	SessionContinuation   string
	FollowUp              string
	NegativeConstraints   []string
	RequiresClarification bool
	MissingFields         []string
	ReasonCodes           []string
}

// Clone returns a deep copy of the evidence record.
func (e *IntentEvidence) Clone() *IntentEvidence {
	if e == nil {
		return nil
	}
	out := *e
	out.ExplicitFiles = cloneStrings(e.ExplicitFiles)
	out.UserFiles = cloneStrings(e.UserFiles)
	out.WorkspaceScopes = cloneStrings(e.WorkspaceScopes)
	out.SessionPins = cloneStrings(e.SessionPins)
	out.ContextHints = cloneStrings(e.ContextHints)
	out.NegativeConstraints = cloneStrings(e.NegativeConstraints)
	out.MissingFields = cloneStrings(e.MissingFields)
	out.ReasonCodes = cloneStrings(e.ReasonCodes)
	return &out
}

// Normalize trims whitespace and removes empty list entries.
func (e *IntentEvidence) Normalize() {
	if e == nil {
		return
	}
	e.ActionType = strings.TrimSpace(e.ActionType)
	e.Target = strings.TrimSpace(e.Target)
	e.Scope = strings.TrimSpace(e.Scope)
	e.RiskLevel = strings.TrimSpace(e.RiskLevel)
	e.ExpectedVerb = strings.TrimSpace(e.ExpectedVerb)
	e.SessionContinuation = strings.TrimSpace(e.SessionContinuation)
	e.FollowUp = strings.TrimSpace(e.FollowUp)
	e.ExplicitFiles = normalizeStrings(e.ExplicitFiles)
	e.UserFiles = normalizeStrings(e.UserFiles)
	e.WorkspaceScopes = normalizeStrings(e.WorkspaceScopes)
	e.SessionPins = normalizeStrings(e.SessionPins)
	e.ContextHints = normalizeStrings(e.ContextHints)
	e.NegativeConstraints = normalizeStrings(e.NegativeConstraints)
	e.MissingFields = normalizeStrings(e.MissingFields)
	e.ReasonCodes = normalizeStrings(e.ReasonCodes)
}

// IntentInterpretation is the structured interpretation of the evidence set.
type IntentInterpretation struct {
	ActionType     string
	Target         string
	Scope          string
	RiskLevel      string
	MissingInfo    []string
	Rationale      string
	ConfidenceNote string
	ReasonCodes    []string
}

// Clone returns a deep copy of the interpretation record.
func (i *IntentInterpretation) Clone() *IntentInterpretation {
	if i == nil {
		return nil
	}
	out := *i
	out.MissingInfo = cloneStrings(i.MissingInfo)
	out.ReasonCodes = cloneStrings(i.ReasonCodes)
	return &out
}

// Normalize trims whitespace and removes empty list entries.
func (i *IntentInterpretation) Normalize() {
	if i == nil {
		return
	}
	i.ActionType = strings.TrimSpace(i.ActionType)
	i.Target = strings.TrimSpace(i.Target)
	i.Scope = strings.TrimSpace(i.Scope)
	i.RiskLevel = strings.TrimSpace(i.RiskLevel)
	i.Rationale = strings.TrimSpace(i.Rationale)
	i.ConfidenceNote = strings.TrimSpace(i.ConfidenceNote)
	i.MissingInfo = normalizeStrings(i.MissingInfo)
	i.ReasonCodes = normalizeStrings(i.ReasonCodes)
}

// RouteResolution records the final route selected for execution.
type RouteResolution struct {
	RouteKind                 string
	ThoughtRecipeID           string
	CapabilityID              string
	ResolutionSource          string
	FallbackTaken             bool
	ClarificationStateVersion uint64
	ReasonCodes               []string
}

// Clone returns a deep copy of the route resolution record.
func (r *RouteResolution) Clone() *RouteResolution {
	if r == nil {
		return nil
	}
	out := *r
	out.ReasonCodes = cloneStrings(r.ReasonCodes)
	return &out
}

// Normalize trims whitespace and removes empty list entries.
func (r *RouteResolution) Normalize() {
	if r == nil {
		return
	}
	r.RouteKind = strings.TrimSpace(r.RouteKind)
	r.ThoughtRecipeID = strings.TrimSpace(r.ThoughtRecipeID)
	r.CapabilityID = strings.TrimSpace(r.CapabilityID)
	r.ResolutionSource = strings.TrimSpace(r.ResolutionSource)
	r.ReasonCodes = normalizeStrings(r.ReasonCodes)
}

// RouteID returns the selected route identifier if one is present.
func (r *RouteResolution) RouteID() string {
	if r == nil {
		return ""
	}
	if trimmed := strings.TrimSpace(r.ThoughtRecipeID); trimmed != "" {
		return trimmed
	}
	return strings.TrimSpace(r.CapabilityID)
}

// Validate checks that the clarification state is internally consistent enough
// to be safely persisted or replayed.
func (s *ClarificationState) Validate() error {
	if s == nil {
		return fmt.Errorf("clarification state is nil")
	}
	if strings.TrimSpace(s.TaskID) == "" {
		return fmt.Errorf("clarification state missing task id")
	}
	if strings.TrimSpace(s.SessionID) == "" {
		return fmt.Errorf("clarification state missing session id")
	}
	if s.StateVersion == 0 {
		return fmt.Errorf("clarification state missing state version")
	}
	if err := validateStableIDs("turn", stableIDsFromTurns(s.Turns)); err != nil {
		return err
	}
	if err := validateStableIDs("entity", stableIDsFromEntities(s.ConfirmedEntities)); err != nil {
		return err
	}
	if err := validateStableIDs("scope", stableIDsFromScopes(s.ConfirmedScopes)); err != nil {
		return err
	}
	if err := validateStableIDs("question", stableIDsFromQuestions(s.PendingQuestions)); err != nil {
		return err
	}
	if err := validateStableIDs("relation intent", stableIDsFromRelations(s.PendingRelationIntents)); err != nil {
		return err
	}
	if err := validateStableIDs("projection intent", stableIDsFromProjectionIntents(s.PendingProjection)); err != nil {
		return err
	}
	if err := validateStableIDs("projection record", stableIDsFromProjectionRecords(s.AppliedMutations)); err != nil {
		return err
	}
	if err := validateStableIDs("anchor", stableIDsFromAnchors(s.GroundedAnchors)); err != nil {
		return err
	}
	return nil
}

// NewState creates a normalized clarification state for the task/session pair.
func NewState(taskID, sessionID string) *ClarificationState {
	return &ClarificationState{
		TaskID:                 strings.TrimSpace(taskID),
		SessionID:              strings.TrimSpace(sessionID),
		StateVersion:           1,
		Turns:                  []ClarificationTurn{},
		ConfirmedEntities:      []ConfirmedEntity{},
		ConfirmedScopes:        []ConfirmedScope{},
		PendingQuestions:       []ClarificationQuestion{},
		PendingRelationIntents: []RelationIntent{},
		GroundedAnchors:        []retrieval.AnchorRef{},
		PendingProjection:      []ProjectionIntent{},
		AppliedMutations:       []ProjectionRecord{},
		LastUpdatedAt:          time.Now().UTC(),
	}
}

// Clone creates a deep copy of the clarification state.
func (s *ClarificationState) Clone() *ClarificationState {
	if s == nil {
		return nil
	}
	out := *s
	out.Turns = cloneTurns(s.Turns)
	out.Ambiguity = cloneAmbiguity(s.Ambiguity)
	out.ConfirmedEntities = cloneConfirmedEntities(s.ConfirmedEntities)
	out.ConfirmedScopes = cloneConfirmedScopes(s.ConfirmedScopes)
	out.PendingQuestions = cloneQuestions(s.PendingQuestions)
	out.PendingRelationIntents = cloneRelationIntents(s.PendingRelationIntents)
	out.GroundedAnchors = cloneAnchors(s.GroundedAnchors)
	out.PendingProjection = cloneProjectionIntents(s.PendingProjection)
	out.AppliedMutations = cloneProjectionRecords(s.AppliedMutations)
	return &out
}

// Normalize trims whitespace and ensures stable IDs are populated.
func (s *ClarificationState) Normalize() {
	if s == nil {
		return
	}
	s.TaskID = strings.TrimSpace(s.TaskID)
	s.SessionID = strings.TrimSpace(s.SessionID)
	s.CurrentTurnID = strings.TrimSpace(s.CurrentTurnID)
	s.ActiveThoughtRecipeID = strings.TrimSpace(s.ActiveThoughtRecipeID)
	s.LastCheckpointID = strings.TrimSpace(s.LastCheckpointID)
	for i := range s.Turns {
		s.Turns[i].Normalize(s.TaskID, s.SessionID)
	}
	if s.Ambiguity != nil {
		s.Ambiguity.Normalize()
	}
	for i := range s.ConfirmedEntities {
		s.ConfirmedEntities[i].Normalize(s.TaskID, s.SessionID)
	}
	for i := range s.ConfirmedScopes {
		s.ConfirmedScopes[i].Normalize(s.TaskID, s.SessionID)
	}
	for i := range s.PendingQuestions {
		s.PendingQuestions[i].Normalize(s.TaskID, s.SessionID)
	}
	for i := range s.PendingRelationIntents {
		s.PendingRelationIntents[i].Normalize(s.TaskID, s.SessionID)
	}
	for i := range s.PendingProjection {
		s.PendingProjection[i].Normalize(s.TaskID, s.SessionID)
	}
	for i := range s.AppliedMutations {
		s.AppliedMutations[i].Normalize(s.TaskID, s.SessionID)
	}
	for i := range s.GroundedAnchors {
		s.GroundedAnchors[i].AnchorID = strings.TrimSpace(s.GroundedAnchors[i].AnchorID)
		s.GroundedAnchors[i].ChunkID = strings.TrimSpace(s.GroundedAnchors[i].ChunkID)
		s.GroundedAnchors[i].Class = strings.TrimSpace(s.GroundedAnchors[i].Class)
		s.GroundedAnchors[i].Term = strings.TrimSpace(s.GroundedAnchors[i].Term)
		s.GroundedAnchors[i].Definition = strings.TrimSpace(s.GroundedAnchors[i].Definition)
		s.GroundedAnchors[i].CreatedAt = strings.TrimSpace(s.GroundedAnchors[i].CreatedAt)
	}
	s.LastUpdatedAt = normalizeTime(s.LastUpdatedAt)
}

func validateStableIDs(label string, ids []string) error {
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			return fmt.Errorf("clarification state has empty %s stable id", label)
		}
		if _, ok := seen[id]; ok {
			return fmt.Errorf("clarification state has duplicate %s stable id: %s", label, id)
		}
		seen[id] = struct{}{}
	}
	return nil
}

func stableIDsFromTurns(turns []ClarificationTurn) []string {
	out := make([]string, 0, len(turns))
	for _, turn := range turns {
		out = append(out, turn.StableID)
	}
	return out
}

func stableIDsFromEntities(entities []ConfirmedEntity) []string {
	out := make([]string, 0, len(entities))
	for _, entity := range entities {
		out = append(out, entity.StableID)
	}
	return out
}

func stableIDsFromScopes(scopes []ConfirmedScope) []string {
	out := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		out = append(out, scope.StableID)
	}
	return out
}

func stableIDsFromQuestions(questions []ClarificationQuestion) []string {
	out := make([]string, 0, len(questions))
	for _, question := range questions {
		out = append(out, question.StableID)
	}
	return out
}

func stableIDsFromRelations(relations []RelationIntent) []string {
	out := make([]string, 0, len(relations))
	for _, relation := range relations {
		out = append(out, relation.StableID)
	}
	return out
}

func stableIDsFromProjectionIntents(intents []ProjectionIntent) []string {
	out := make([]string, 0, len(intents))
	for _, intent := range intents {
		out = append(out, intent.StableID)
	}
	return out
}

func stableIDsFromProjectionRecords(records []ProjectionRecord) []string {
	out := make([]string, 0, len(records))
	for _, record := range records {
		out = append(out, record.StableID)
	}
	return out
}

func stableIDsFromAnchors(anchors []retrieval.AnchorRef) []string {
	out := make([]string, 0, len(anchors))
	for _, anchor := range anchors {
		out = append(out, anchor.AnchorID)
	}
	return out
}

// NextStateVersion returns the next monotonic state version.
func NextStateVersion(version uint64) uint64 {
	if version == 0 {
		return 1
	}
	return version + 1
}

// StableID returns a deterministic identifier for the supplied parts.
func StableID(parts ...string) string {
	h := sha256.New()
	for i, part := range parts {
		if i > 0 {
			_, _ = h.Write([]byte{0x1f})
		}
		_, _ = h.Write([]byte(strings.TrimSpace(part)))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (t *ClarificationTurn) Normalize(taskID, sessionID string) {
	if t == nil {
		return
	}
	t.StableID = strings.TrimSpace(t.StableID)
	t.TurnID = strings.TrimSpace(t.TurnID)
	t.PromptID = strings.TrimSpace(t.PromptID)
	t.PromptFamily = strings.TrimSpace(t.PromptFamily)
	t.Question = strings.TrimSpace(t.Question)
	t.Answer = strings.TrimSpace(t.Answer)
	t.SourceTurnID = strings.TrimSpace(t.SourceTurnID)
	if t.StableID == "" {
		t.StableID = StableID(taskID, sessionID, "turn", t.TurnID, t.PromptID, t.Question, t.Answer)
	}
	t.CreatedAt = normalizeTime(t.CreatedAt)
	t.UpdatedAt = normalizeTime(t.UpdatedAt)
}

func (e *EntityRef) Normalize() {
	if e == nil {
		return
	}
	e.EntityID = strings.TrimSpace(e.EntityID)
	e.ChunkID = strings.TrimSpace(e.ChunkID)
	e.Name = strings.TrimSpace(e.Name)
	e.FilePath = strings.TrimSpace(e.FilePath)
	e.Package = strings.TrimSpace(e.Package)
	e.StableID = strings.TrimSpace(e.StableID)
}

func (a *AmbiguityCharacterization) Normalize() {
	if a == nil {
		return
	}
	a.Rationale = strings.TrimSpace(a.Rationale)
	for i := range a.CandidateFamilies {
		a.CandidateFamilies[i] = strings.TrimSpace(a.CandidateFamilies[i])
	}
	for i := range a.CandidateScopes {
		a.CandidateScopes[i].Normalize("", "")
	}
	for i := range a.CandidateEntities {
		a.CandidateEntities[i].Normalize()
	}
}

func (e *ConfirmedEntity) Normalize(taskID, sessionID string) {
	if e == nil {
		return
	}
	e.StableID = strings.TrimSpace(e.StableID)
	e.Name = strings.TrimSpace(e.Name)
	e.ResolverKey = strings.TrimSpace(e.ResolverKey)
	e.SourceTurnID = strings.TrimSpace(e.SourceTurnID)
	e.EntityRef.Normalize()
	if e.StableID == "" {
		e.StableID = StableID(taskID, sessionID, "entity", e.Name, string(e.Kind), e.ResolverKey, e.EntityRef.StableID, e.EntityRef.EntityID)
	}
	e.ConfirmedAt = normalizeTime(e.ConfirmedAt)
}

func (s *ConfirmedScope) Normalize(taskID, sessionID string) {
	if s == nil {
		return
	}
	s.StableID = strings.TrimSpace(s.StableID)
	s.Name = strings.TrimSpace(s.Name)
	s.Rationale = strings.TrimSpace(s.Rationale)
	s.SourceTurnID = strings.TrimSpace(s.SourceTurnID)
	for i := range s.Entities {
		s.Entities[i].Normalize(taskID, sessionID)
	}
	if s.StableID == "" {
		names := make([]string, 0, len(s.Entities))
		for _, entity := range s.Entities {
			names = append(names, entity.StableID, entity.Name, string(entity.Kind))
		}
		s.StableID = StableID(taskID, sessionID, "scope", s.Name, string(s.AnchorClass), strings.Join(names, ","))
	}
	s.ConfirmedAt = normalizeTime(s.ConfirmedAt)
}

func (r *RelationIntent) Normalize(taskID, sessionID string) {
	if r == nil {
		return
	}
	r.StableID = strings.TrimSpace(r.StableID)
	r.SourceEntityID = strings.TrimSpace(r.SourceEntityID)
	r.TargetEntityID = strings.TrimSpace(r.TargetEntityID)
	r.RelationKind = strings.TrimSpace(r.RelationKind)
	r.Direction = strings.TrimSpace(r.Direction)
	r.Evidence = strings.TrimSpace(r.Evidence)
	r.SourceTurnID = strings.TrimSpace(r.SourceTurnID)
	if r.StableID == "" {
		r.StableID = StableID(taskID, sessionID, "relation", r.SourceEntityID, r.TargetEntityID, r.RelationKind, r.Direction, r.Evidence)
	}
}

func (p *ProjectionIntent) Normalize(taskID, sessionID string) {
	if p == nil {
		return
	}
	p.StableID = strings.TrimSpace(p.StableID)
	p.RevisionRootID = strings.TrimSpace(p.RevisionRootID)
	p.MutationKind = strings.TrimSpace(p.MutationKind)
	p.EdgeKind = strings.TrimSpace(p.EdgeKind)
	p.NodeKind = strings.TrimSpace(p.NodeKind)
	p.IdempotencyKey = strings.TrimSpace(p.IdempotencyKey)
	p.RevisionOf = strings.TrimSpace(p.RevisionOf)
	for i := range p.SubjectIDs {
		p.SubjectIDs[i] = strings.TrimSpace(p.SubjectIDs[i])
	}
	for i := range p.ObjectIDs {
		p.ObjectIDs[i] = strings.TrimSpace(p.ObjectIDs[i])
	}
	p.Provenance.Normalize()
	if p.StableID == "" {
		p.StableID = StableID(taskID, sessionID, "projection", p.RevisionRootID, p.MutationKind, p.EdgeKind, p.NodeKind, strings.Join(p.SubjectIDs, ","), strings.Join(p.ObjectIDs, ","), p.IdempotencyKey, p.RevisionOf)
	}
}

func (r *ProjectionRecord) Normalize(taskID, sessionID string) {
	if r == nil {
		return
	}
	r.StableID = strings.TrimSpace(r.StableID)
	r.RevisionRootID = strings.TrimSpace(r.RevisionRootID)
	r.IdempotencyKey = strings.TrimSpace(r.IdempotencyKey)
	r.AppliedBy = strings.TrimSpace(r.AppliedBy)
	r.RevisionOf = strings.TrimSpace(r.RevisionOf)
	if r.StableID == "" {
		r.StableID = StableID(taskID, sessionID, "record", r.RevisionRootID, r.IdempotencyKey, r.RevisionOf, string(r.Result), r.AppliedBy)
	}
	r.AppliedAt = normalizeTime(r.AppliedAt)
	for i := range r.GraphRecordIDs {
		r.GraphRecordIDs[i] = strings.TrimSpace(r.GraphRecordIDs[i])
	}
}

func (q *ClarificationQuestion) Normalize(taskID, sessionID string) {
	if q == nil {
		return
	}
	q.StableID = strings.TrimSpace(q.StableID)
	q.PromptID = strings.TrimSpace(q.PromptID)
	q.PromptFamily = strings.TrimSpace(q.PromptFamily)
	q.Text = strings.TrimSpace(q.Text)
	q.AnswerSchema = strings.TrimSpace(q.AnswerSchema)
	q.TurnID = strings.TrimSpace(q.TurnID)
	if q.StableID == "" {
		q.StableID = StableID(taskID, sessionID, "question", q.PromptID, q.PromptFamily, q.Text, q.AnswerSchema, string(q.ExpectedKind), q.TurnID)
	}
}

func (d *ScopeDeclaration) Normalize(taskID, sessionID string) {
	if d == nil {
		return
	}
	d.StableID = strings.TrimSpace(d.StableID)
	d.Name = strings.TrimSpace(d.Name)
	d.Selector = strings.TrimSpace(d.Selector)
	d.ResolverKey = strings.TrimSpace(d.ResolverKey)
	if d.StableID == "" {
		d.StableID = StableID(taskID, sessionID, "scope_decl", d.Name, string(d.Kind), string(d.AnchorClass), d.Selector, d.ResolverKey)
	}
}

func (r *ScopeResolution) Normalize(taskID, sessionID string) {
	if r == nil {
		return
	}
	r.Decl.Normalize(taskID, sessionID)
	r.ScopeID = strings.TrimSpace(r.ScopeID)
	r.Rationale = strings.TrimSpace(r.Rationale)
	for i := range r.ChunkIDs {
		r.ChunkIDs[i] = strings.TrimSpace(r.ChunkIDs[i])
	}
	for i := range r.AnchorIDs {
		r.AnchorIDs[i] = strings.TrimSpace(r.AnchorIDs[i])
	}
}

func (c *ClarificationStepResult) Normalize(taskID, sessionID string) {
	if c == nil {
		return
	}
	c.StepID = strings.TrimSpace(c.StepID)
	c.TurnID = strings.TrimSpace(c.TurnID)
	c.PromptID = strings.TrimSpace(c.PromptID)
	for i := range c.ValidationErrors {
		c.ValidationErrors[i] = strings.TrimSpace(c.ValidationErrors[i])
	}
	for i := range c.ResolvedEntities {
		c.ResolvedEntities[i].Normalize()
	}
	for i := range c.ResolvedScopes {
		c.ResolvedScopes[i].Normalize(taskID, sessionID)
	}
	c.Grounding.Normalize()
	if c.Projection != nil {
		c.Projection.Normalize(taskID, sessionID)
	}
	if c.ResidualAmbiguity != nil {
		c.ResidualAmbiguity.Normalize()
	}
}

func (p *ProjectionPlan) Normalize(taskID, sessionID string) {
	if p == nil {
		return
	}
	p.PlanID = strings.TrimSpace(p.PlanID)
	for i := range p.Intents {
		p.Intents[i].Normalize(taskID, sessionID)
	}
	for i := range p.Conflicts {
		p.Conflicts[i].Normalize(taskID, sessionID)
	}
}

func (p *ProjectionPlan) Validate() error {
	if p == nil {
		return fmt.Errorf("projection plan is nil")
	}
	if strings.TrimSpace(p.PlanID) == "" {
		return fmt.Errorf("projection plan missing plan id")
	}
	if len(p.Intents) == 0 {
		return fmt.Errorf("projection plan %s has no intents", p.PlanID)
	}
	seenIntentIDs := make(map[string]string, len(p.Intents))
	seenIdempotency := make(map[string]string, len(p.Intents))
	for i := range p.Intents {
		intent := &p.Intents[i]
		if err := intent.Validate(); err != nil {
			return fmt.Errorf("projection plan %s intent %d: %w", p.PlanID, i, err)
		}
		if prev, ok := seenIntentIDs[intent.StableID]; ok {
			return fmt.Errorf("projection plan %s has duplicate intent stable_id %q (%s)", p.PlanID, intent.StableID, prev)
		}
		seenIntentIDs[intent.StableID] = intent.MutationKind
		if key := strings.TrimSpace(intent.IdempotencyKey); key != "" {
			if prev, ok := seenIdempotency[key]; ok {
				return fmt.Errorf("projection plan %s has duplicate idempotency key %q (%s)", p.PlanID, key, prev)
			}
			seenIdempotency[key] = intent.StableID
		}
	}
	for i := range p.Conflicts {
		conflict := &p.Conflicts[i]
		if conflict.Reason == "" {
			return fmt.Errorf("projection plan %s conflict %d missing reason", p.PlanID, i)
		}
	}
	return nil
}

func (p *ProjectionResult) Normalize(taskID, sessionID string) {
	if p == nil {
		return
	}
	p.PlanID = strings.TrimSpace(p.PlanID)
	for i := range p.Applied {
		p.Applied[i].Normalize(taskID, sessionID)
	}
	for i := range p.Skipped {
		p.Skipped[i].Normalize(taskID, sessionID)
	}
	for i := range p.Conflicts {
		p.Conflicts[i].Normalize(taskID, sessionID)
	}
	if p.Mutation != nil {
		p.Mutation.Normalize(taskID, sessionID)
	}
}

func (p *ProjectionResult) Validate() error {
	if p == nil {
		return fmt.Errorf("projection result is nil")
	}
	if strings.TrimSpace(p.PlanID) == "" {
		return fmt.Errorf("projection result missing plan id")
	}
	if p.Mutation != nil {
		if strings.TrimSpace(p.Mutation.StableID) == "" {
			return fmt.Errorf("projection result missing mutation stable id")
		}
		if p.Mutation.Scope == "" {
			return fmt.Errorf("projection result missing mutation scope")
		}
	}
	return nil
}

func (c *ProjectionConflict) Normalize(taskID, sessionID string) {
	if c == nil {
		return
	}
	c.StableID = strings.TrimSpace(c.StableID)
	c.Reason = strings.TrimSpace(c.Reason)
	c.ExistingMutationID = strings.TrimSpace(c.ExistingMutationID)
	c.ProposedMutationID = strings.TrimSpace(c.ProposedMutationID)
	if c.StableID == "" {
		c.StableID = StableID(taskID, sessionID, "projection_conflict", c.Reason, c.ExistingMutationID, c.ProposedMutationID)
	}
}

func (p *ProjectionIntent) Validate() error {
	if p == nil {
		return fmt.Errorf("projection intent is nil")
	}
	if strings.TrimSpace(p.StableID) == "" {
		return fmt.Errorf("projection intent missing stable id")
	}
	if strings.TrimSpace(p.MutationKind) == "" {
		return fmt.Errorf("projection intent missing mutation kind")
	}
	switch strings.TrimSpace(p.MutationKind) {
	case "upsert_node":
		if len(p.SubjectIDs) == 0 {
			return fmt.Errorf("projection intent %s missing subject ids", p.StableID)
		}
		if strings.TrimSpace(p.NodeKind) == "" {
			return fmt.Errorf("projection intent %s missing node kind", p.StableID)
		}
	case "upsert_edge":
		if len(p.SubjectIDs) == 0 || len(p.ObjectIDs) == 0 {
			return fmt.Errorf("projection intent %s missing edge endpoints", p.StableID)
		}
		if strings.TrimSpace(p.EdgeKind) == "" {
			return fmt.Errorf("projection intent %s missing edge kind", p.StableID)
		}
	}
	if strings.TrimSpace(p.NodeKind) != "" && strings.TrimSpace(p.EdgeKind) != "" && p.MutationKind != "annotate" {
		return fmt.Errorf("projection intent %s cannot set both node and edge kinds", p.StableID)
	}
	return nil
}

func (p *ProjectionProvenance) Normalize() {
	if p == nil {
		return
	}
	p.TaskID = strings.TrimSpace(p.TaskID)
	p.SessionID = strings.TrimSpace(p.SessionID)
	p.TurnID = strings.TrimSpace(p.TurnID)
	p.SourcePromptID = strings.TrimSpace(p.SourcePromptID)
	p.Extractor = strings.TrimSpace(p.Extractor)
	p.AnswerText = strings.TrimSpace(p.AnswerText)
}

func (g *GroundingResult) Normalize() {
	if g == nil {
		return
	}
	for i := range g.Anchors {
		g.Anchors[i].AnchorID = strings.TrimSpace(g.Anchors[i].AnchorID)
		g.Anchors[i].ChunkID = strings.TrimSpace(g.Anchors[i].ChunkID)
		g.Anchors[i].Term = strings.TrimSpace(g.Anchors[i].Term)
		g.Anchors[i].Definition = strings.TrimSpace(g.Anchors[i].Definition)
		g.Anchors[i].Class = strings.TrimSpace(g.Anchors[i].Class)
		g.Anchors[i].CreatedAt = strings.TrimSpace(g.Anchors[i].CreatedAt)
	}
}

func normalizeTime(t time.Time) time.Time {
	if t.IsZero() {
		return time.Time{}
	}
	return t.UTC()
}

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, len(values))
	copy(out, values)
	return out
}

func normalizeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func cloneAnchors(in []retrieval.AnchorRef) []retrieval.AnchorRef {
	if len(in) == 0 {
		return nil
	}
	out := make([]retrieval.AnchorRef, len(in))
	copy(out, in)
	return out
}

func cloneTurns(in []ClarificationTurn) []ClarificationTurn {
	if len(in) == 0 {
		return nil
	}
	out := make([]ClarificationTurn, len(in))
	copy(out, in)
	return out
}

func cloneConfirmedEntities(in []ConfirmedEntity) []ConfirmedEntity {
	if len(in) == 0 {
		return nil
	}
	out := make([]ConfirmedEntity, len(in))
	copy(out, in)
	return out
}

func cloneConfirmedScopes(in []ConfirmedScope) []ConfirmedScope {
	if len(in) == 0 {
		return nil
	}
	out := make([]ConfirmedScope, len(in))
	for i := range in {
		out[i] = in[i]
		out[i].Entities = cloneConfirmedEntities(in[i].Entities)
	}
	return out
}

func cloneQuestions(in []ClarificationQuestion) []ClarificationQuestion {
	if len(in) == 0 {
		return nil
	}
	out := make([]ClarificationQuestion, len(in))
	copy(out, in)
	return out
}

func cloneRelationIntents(in []RelationIntent) []RelationIntent {
	if len(in) == 0 {
		return nil
	}
	out := make([]RelationIntent, len(in))
	copy(out, in)
	return out
}

func cloneProjectionIntents(in []ProjectionIntent) []ProjectionIntent {
	if len(in) == 0 {
		return nil
	}
	out := make([]ProjectionIntent, len(in))
	for i := range in {
		out[i] = in[i]
		out[i].SubjectIDs = append([]string(nil), in[i].SubjectIDs...)
		out[i].ObjectIDs = append([]string(nil), in[i].ObjectIDs...)
	}
	return out
}

func cloneProjectionRecords(in []ProjectionRecord) []ProjectionRecord {
	if len(in) == 0 {
		return nil
	}
	out := make([]ProjectionRecord, len(in))
	for i := range in {
		out[i] = in[i]
		out[i].GraphRecordIDs = append([]string(nil), in[i].GraphRecordIDs...)
	}
	return out
}

func cloneAmbiguity(in *AmbiguityCharacterization) *AmbiguityCharacterization {
	if in == nil {
		return nil
	}
	out := *in
	out.CandidateFamilies = append([]string(nil), in.CandidateFamilies...)
	out.CandidateScopes = append([]ScopeDeclaration(nil), in.CandidateScopes...)
	out.CandidateEntities = append([]EntityRef(nil), in.CandidateEntities...)
	return &out
}

// String returns a concise summary for logging.
func (s *ClarificationState) String() string {
	if s == nil {
		return "<nil clarification state>"
	}
	return fmt.Sprintf("ClarificationState{TaskID:%s SessionID:%s Version:%d Turn:%s Entities:%d Scopes:%d Anchors:%d}",
		s.TaskID, s.SessionID, s.StateVersion, s.CurrentTurnID,
		len(s.ConfirmedEntities), len(s.ConfirmedScopes), len(s.GroundedAnchors))
}
