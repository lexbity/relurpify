package intentcontext

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/contextstream"
	"codeburg.org/lexbit/relurpify/framework/graphdb"
	"codeburg.org/lexbit/relurpify/framework/knowledge"
	"codeburg.org/lexbit/relurpify/framework/retrieval"
)

// EucloIntent is the Euclo-owned clarification core surface.
type EucloIntent interface {
	BuildClarificationRequest(ctx context.Context, env *contextdata.Envelope, instruction string, maxTokens int, mode contextstream.Mode) (*contextstream.Request, error)
	GroundConfirmed(ctx context.Context, env *contextdata.Envelope, scope ScopeDeclaration) (*GroundingResult, error)
	ResolveEntity(ctx context.Context, name string, kind EntityKind) (*EntityRef, error)
	BuildProjectionPlan(ctx context.Context, env *contextdata.Envelope) (*ProjectionPlan, error)
	ApplyProjection(ctx context.Context, env *contextdata.Envelope, plan *ProjectionPlan) (*ProjectionResult, error)
	GroundedAnchors(env *contextdata.Envelope) []retrieval.AnchorRef
}

// IntentCore is the default Euclo clarification core implementation.
type IntentCore struct {
	Store      StateStore
	ChunkStore *knowledge.ChunkStore
	Graph      *graphdb.Engine
}

// NewIntentCore creates a new clarification core.
func NewIntentCore(chunkStore *knowledge.ChunkStore, graph *graphdb.Engine) *IntentCore {
	return &IntentCore{
		Store:      NewStateStore(),
		ChunkStore: chunkStore,
		Graph:      graph,
	}
}

// BuildClarificationRequest creates a traversal-aware context stream request.
func (c *IntentCore) BuildClarificationRequest(ctx context.Context, env *contextdata.Envelope, instruction string, maxTokens int, mode contextstream.Mode) (*contextstream.Request, error) {
	_ = ctx
	state, err := c.readState(env)
	if err != nil {
		return nil, err
	}
	anchors := cloneAnchors(state.GroundedAnchors)
	traversal := traversalFromAnchors(anchors)
	req := &contextstream.Request{
		Query: retrieval.RetrievalQuery{
			Text:      strings.TrimSpace(instruction),
			Anchors:   anchors,
			Traversal: traversal,
		},
		MaxTokens: maxTokens,
		Mode:      mode,
		Metadata: map[string]any{
			"task_id":          state.TaskID,
			"session_id":       state.SessionID,
			"state_version":    state.StateVersion,
			"current_turn_id":  state.CurrentTurnID,
			"active_recipe_id": state.ActiveRecipeID,
		},
		RequestedAt: time.Now().UTC(),
	}
	if state.Ambiguity != nil {
		req.Metadata["ambiguity_kind"] = string(state.Ambiguity.Kind)
		req.Metadata["ambiguity_confidence"] = state.Ambiguity.Confidence
		req.Metadata["ambiguity_rationale"] = state.Ambiguity.Rationale
	}
	return req, nil
}

// GroundConfirmed resolves a scope declaration into grounded anchors and writes them back.
func (c *IntentCore) GroundConfirmed(ctx context.Context, env *contextdata.Envelope, scope ScopeDeclaration) (*GroundingResult, error) {
	_ = ctx
	state, err := c.readState(env)
	if err != nil {
		return nil, err
	}
	scope.Normalize(state.TaskID, state.SessionID)
	resolution, err := c.ResolveScope(context.Background(), scope)
	if err != nil {
		return nil, err
	}

	existing := make(map[string]retrieval.AnchorRef, len(state.GroundedAnchors))
	for _, anchor := range state.GroundedAnchors {
		if strings.TrimSpace(anchor.AnchorID) == "" {
			continue
		}
		existing[anchor.AnchorID] = anchor
	}
	seenThisCall := make(map[string]struct{})

	result := &GroundingResult{
		StateVersion: state.StateVersion,
	}
	addAnchor := func(anchor retrieval.AnchorRef) {
		if strings.TrimSpace(anchor.AnchorID) == "" {
			return
		}
		if _, ok := seenThisCall[anchor.AnchorID]; ok {
			return
		}
		seenThisCall[anchor.AnchorID] = struct{}{}
		if prior, ok := existing[anchor.AnchorID]; ok {
			result.Reused = append(result.Reused, prior)
			return
		}
		existing[anchor.AnchorID] = anchor
		result.Added = append(result.Added, anchor)
		state.GroundedAnchors = append(state.GroundedAnchors, anchor)
	}

	for _, id := range resolution.AnchorIDs {
		addAnchor(retrieval.AnchorRef{
			AnchorID:   StableID(state.TaskID, state.SessionID, "anchor", id, scope.StableID),
			ChunkID:    id,
			Term:       scope.Name,
			Definition: "clarified scope anchor",
			Class:      string(scope.AnchorClass),
			Active:     true,
			CreatedAt:  time.Now().UTC().Format(time.RFC3339),
		})
	}
	for _, chunkID := range resolution.ChunkIDs {
		addAnchor(retrieval.AnchorRef{
			AnchorID:   StableID(state.TaskID, state.SessionID, "anchor", chunkID, scope.StableID),
			ChunkID:    chunkID,
			Term:       scope.Name,
			Definition: "clarified chunk anchor",
			Class:      string(scope.AnchorClass),
			Active:     true,
			CreatedAt:  time.Now().UTC().Format(time.RFC3339),
		})
	}

	state.StateVersion = NextStateVersion(state.StateVersion)
	result.StateVersion = state.StateVersion
	if err := c.writeState(env, state); err != nil {
		return nil, err
	}
	return result, nil
}

// ResolveEntity resolves an entity name against the knowledge chunk store.
func (c *IntentCore) ResolveEntity(ctx context.Context, name string, kind EntityKind) (*EntityRef, error) {
	_ = ctx
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("entity name is required")
	}
	if c == nil || c.ChunkStore == nil {
		return nil, errors.New("entity resolution requires a chunk store")
	}

	chunks, err := c.ChunkStore.FindAll()
	if err != nil {
		return nil, err
	}

	matches := make([]EntityRef, 0)
	for _, chunk := range chunks {
		ref := entityRefFromChunk(chunk, kind)
		if ref == nil {
			continue
		}
		if entityMatches(ref, chunk, name, kind) {
			matches = append(matches, *ref)
		}
	}

	if len(matches) == 0 {
		return nil, fmt.Errorf("entity %q not found", name)
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].FilePath == matches[j].FilePath {
			return matches[i].Name < matches[j].Name
		}
		return matches[i].FilePath < matches[j].FilePath
	})
	if len(matches) > 1 {
		return nil, fmt.Errorf("entity %q is ambiguous: %d matches", name, len(matches))
	}
	out := matches[0]
	out.Normalize()
	return &out, nil
}

// ResolveScope resolves a scope declaration into chunk and anchor ids.
func (c *IntentCore) ResolveScope(ctx context.Context, decl ScopeDeclaration) (*ScopeResolution, error) {
	_ = ctx
	if c == nil || c.ChunkStore == nil {
		return nil, errors.New("scope resolution requires a chunk store")
	}
	decl.Normalize("", "")
	if strings.TrimSpace(decl.Name) == "" && strings.TrimSpace(decl.Selector) == "" {
		return nil, errors.New("scope declaration requires a name or selector")
	}
	resolution := &ScopeResolution{Decl: decl, Confidence: 1.0}

	chunks, err := c.ChunkStore.FindAll()
	if err != nil {
		return nil, err
	}
	for _, chunk := range chunks {
		if scopeMatchesChunk(decl, chunk) {
			resolution.ChunkIDs = append(resolution.ChunkIDs, string(chunk.ID))
			resolution.AnchorIDs = append(resolution.AnchorIDs, string(chunk.ID))
		}
	}
	if len(resolution.ChunkIDs) == 0 {
		return nil, fmt.Errorf("scope %q not found", decl.Name)
	}
	resolution.ScopeID = StableID(decl.Name, decl.Selector, string(decl.AnchorClass), string(decl.Kind))
	resolution.Normalize("", "")
	return resolution, nil
}

// BuildProjectionPlan constructs projection intents from clarification state.
func (c *IntentCore) BuildProjectionPlan(ctx context.Context, env *contextdata.Envelope) (*ProjectionPlan, error) {
	_ = ctx
	state, err := c.readState(env)
	if err != nil {
		return nil, err
	}
	plan := &ProjectionPlan{
		PlanID:       StableID(state.TaskID, state.SessionID, "projection_plan", fmt.Sprint(state.StateVersion), state.CurrentTurnID),
		StateVersion: state.StateVersion,
	}
	for _, intent := range state.PendingProjection {
		plan.Intents = append(plan.Intents, intent)
	}
	if len(plan.Intents) == 0 {
		for _, relation := range state.PendingRelationIntents {
			intent := ProjectionIntent{
				RevisionRootID: relation.StableID,
				MutationKind:   "upsert_edge",
				SubjectIDs:     []string{relation.SourceEntityID},
				ObjectIDs:      []string{relation.TargetEntityID},
				EdgeKind:       relation.RelationKind,
				Provenance: ProjectionProvenance{
					TaskID:       state.TaskID,
					SessionID:    state.SessionID,
					TurnID:       relation.SourceTurnID,
					StateVersion: state.StateVersion,
					Extractor:    "intentcontext",
				},
				IdempotencyKey: StableID(state.TaskID, state.SessionID, relation.StableID, relation.SourceEntityID, relation.TargetEntityID, relation.RelationKind),
			}
			intent.Normalize(state.TaskID, state.SessionID)
			plan.Intents = append(plan.Intents, intent)
		}
	}
	plan.Normalize(state.TaskID, state.SessionID)
	if err := plan.Validate(); err != nil {
		return nil, err
	}
	return plan, nil
}

// ApplyProjection applies a projection plan to the graph and updates clarification state.
func (c *IntentCore) ApplyProjection(ctx context.Context, env *contextdata.Envelope, plan *ProjectionPlan) (*ProjectionResult, error) {
	_ = ctx
	if c == nil || c.Graph == nil {
		return nil, errors.New("projection requires a graph engine")
	}
	state, err := c.readState(env)
	if err != nil {
		return nil, err
	}
	if plan == nil {
		return nil, errors.New("projection plan is required")
	}
	plan.Normalize(state.TaskID, state.SessionID)
	if err := plan.Validate(); err != nil {
		return nil, err
	}

	result := &ProjectionResult{PlanID: plan.PlanID}
	mutationResult := &graphdb.MutationResult{
		Scope:        graphdb.MutationScopeProjection,
		Status:       graphdb.MutationStatusNoop,
		AppliedAt:    time.Now().UTC(),
		TaskID:       state.TaskID,
		SessionID:    state.SessionID,
		TurnID:       state.CurrentTurnID,
		Details:      map[string]any{"plan_id": plan.PlanID},
		StateVersion: state.StateVersion,
	}
	for _, intent := range plan.Intents {
		graphIDs, applied, mutationStatus, err := c.applyProjectionIntent(state, intent)
		if err != nil {
			result.Conflicts = append(result.Conflicts, ProjectionConflict{
				Reason:             err.Error(),
				ProposedMutationID: intent.StableID,
			})
			mutationResult.Status = graphdb.MutationStatusConflict
			mutationResult.ConflictIDs = append(mutationResult.ConflictIDs, intent.StableID)
			continue
		}
		if !applied {
			result.Skipped = append(result.Skipped, intent)
			if mutationResult.Status == graphdb.MutationStatusNoop {
				mutationResult.Status = graphdb.MutationStatusMatched
			}
			mutationResult.MatchedIDs = append(mutationResult.MatchedIDs, intent.StableID)
			continue
		}
		mutationResult.Status = summarizeMutationStatus(mutationResult.Status, mutationStatus)
		mutationResult.RecordIDs = append(mutationResult.RecordIDs, graphIDs...)
		switch mutationStatus {
		case graphdb.MutationStatusCreated:
			mutationResult.CreatedIDs = append(mutationResult.CreatedIDs, intent.StableID)
		case graphdb.MutationStatusUpdated:
			mutationResult.UpdatedIDs = append(mutationResult.UpdatedIDs, intent.StableID)
		case graphdb.MutationStatusAnnotated:
			mutationResult.AnnotatedIDs = append(mutationResult.AnnotatedIDs, intent.StableID)
		case graphdb.MutationStatusSuperseded:
			mutationResult.SupersededIDs = append(mutationResult.SupersededIDs, intent.StableID)
		case graphdb.MutationStatusMatched:
			mutationResult.MatchedIDs = append(mutationResult.MatchedIDs, intent.StableID)
		}
		record := ProjectionRecord{
			RevisionRootID: intent.RevisionRootID,
			IdempotencyKey: intent.IdempotencyKey,
			GraphRecordIDs: graphIDs,
			AppliedAt:      time.Now().UTC(),
			AppliedBy:      "intentcontext",
			Result:         ProjectionStatusApplied,
			RevisionOf:     intent.RevisionOf,
		}
		record.Normalize(state.TaskID, state.SessionID)
		result.Applied = append(result.Applied, record)
		state.AppliedMutations = append(state.AppliedMutations, record)
	}

	state.PendingProjection = nil
	if len(result.Applied) > 0 {
		state.StateVersion = NextStateVersion(state.StateVersion)
	}
	state.LastUpdatedAt = time.Now().UTC()
	if err := c.writeState(env, state); err != nil {
		return nil, err
	}
	if mutationResult.Status == graphdb.MutationStatusNoop {
		mutationResult.Status = graphdb.MutationStatusMatched
	}
	mutationResult.Reason = "projection pass completed"
	mutationResult.Normalize(state.TaskID, state.SessionID)
	if err := c.Graph.RecordMutationResult(*mutationResult); err != nil {
		if mutationResult.Details == nil {
			mutationResult.Details = map[string]any{}
		}
		mutationResult.Details["record_error"] = err.Error()
	}
	if env != nil {
		env.SetWorkingValue("euclo.projection.plan_id", plan.PlanID, contextdata.MemoryClassTask)
		env.SetWorkingValue("euclo.projection.mutation_result", mutationResult, contextdata.MemoryClassTask)
	}
	result.Mutation = mutationResult
	result.Normalize(state.TaskID, state.SessionID)
	if err := result.Validate(); err != nil {
		return nil, err
	}
	return result, nil
}

// GroundedAnchors returns the current grounded anchors in working memory.
func (c *IntentCore) GroundedAnchors(env *contextdata.Envelope) []retrieval.AnchorRef {
	state, err := c.readState(env)
	if err != nil || state == nil {
		return nil
	}
	return cloneAnchors(state.GroundedAnchors)
}

func (c *IntentCore) readState(env *contextdata.Envelope) (*ClarificationState, error) {
	if c != nil && c.Store != nil {
		return c.Store.Read(context.Background(), env)
	}
	return NewStateStore().Read(context.Background(), env)
}

func (c *IntentCore) writeState(env *contextdata.Envelope, state *ClarificationState) error {
	if c != nil && c.Store != nil {
		return c.Store.Write(context.Background(), env, state)
	}
	return NewStateStore().Write(context.Background(), env, state)
}

func traversalFromAnchors(anchors []retrieval.AnchorRef) *retrieval.TraversalSpec {
	if len(anchors) == 0 {
		return nil
	}
	ids := make([]string, 0, len(anchors))
	for _, anchor := range anchors {
		if strings.TrimSpace(anchor.ChunkID) != "" {
			ids = append(ids, strings.TrimSpace(anchor.ChunkID))
		}
	}
	if len(ids) == 0 {
		return nil
	}
	return &retrieval.TraversalSpec{
		AnchorIDs:    ids,
		Direction:    retrieval.TraversalDirectionBoth,
		MaxDepth:     2,
		PreferLatest: true,
	}
}

func entityRefFromChunk(chunk knowledge.KnowledgeChunk, kind EntityKind) *EntityRef {
	if chunk.ID == "" {
		return nil
	}
	var name string
	if chunk.Body.Fields != nil {
		if raw, ok := chunk.Body.Fields["name"].(string); ok {
			name = strings.TrimSpace(raw)
		}
		if name == "" {
			if raw, ok := chunk.Body.Fields["symbol"].(string); ok {
				name = strings.TrimSpace(raw)
			}
		}
		if name == "" {
			if raw, ok := chunk.Body.Fields["file_path"].(string); ok {
				name = filepath.Base(strings.TrimSpace(raw))
			}
		}
	}
	if name == "" {
		name = string(chunk.ID)
	}
	resolvedKind := kind
	if resolvedKind == "" {
		resolvedKind = EntityKindUnknown
	}
	ref := &EntityRef{
		EntityID: string(chunk.ID),
		ChunkID:  string(chunk.ID),
		Kind:     resolvedKind,
		Name:     name,
		Package:  strings.TrimSpace(chunk.WorkspaceID),
		StableID: StableID(chunk.WorkspaceID, string(chunk.ID), name, string(resolvedKind)),
	}
	if chunk.Body.Fields != nil {
		if path, ok := chunk.Body.Fields["file_path"].(string); ok {
			ref.FilePath = normalizePath(path)
		}
	}
	return ref
}

func entityMatches(ref *EntityRef, chunk knowledge.KnowledgeChunk, name string, kind EntityKind) bool {
	if ref == nil {
		return false
	}
	if kind != "" && kind != EntityKindUnknown && ref.Kind != kind {
		return false
	}
	needle := strings.ToLower(strings.TrimSpace(name))
	if needle == "" {
		return false
	}
	if strings.EqualFold(ref.Name, name) || strings.Contains(strings.ToLower(ref.Name), needle) {
		return true
	}
	if chunk.Body.Raw != "" && strings.Contains(strings.ToLower(chunk.Body.Raw), needle) {
		return true
	}
	if chunk.Body.Fields != nil {
		for _, value := range chunk.Body.Fields {
			if strings.Contains(strings.ToLower(fmt.Sprint(value)), needle) {
				return true
			}
		}
	}
	if strings.Contains(strings.ToLower(ref.FilePath), needle) {
		return true
	}
	return false
}

func scopeMatchesChunk(decl ScopeDeclaration, chunk knowledge.KnowledgeChunk) bool {
	if chunk.ID == "" {
		return false
	}
	if strings.TrimSpace(decl.Selector) != "" {
		selector := strings.ToLower(strings.TrimSpace(decl.Selector))
		if chunk.Body.Fields != nil {
			for _, value := range chunk.Body.Fields {
				if strings.Contains(strings.ToLower(fmt.Sprint(value)), selector) {
					return true
				}
			}
		}
		if strings.Contains(strings.ToLower(chunk.Body.Raw), selector) {
			return true
		}
	}
	name := strings.ToLower(strings.TrimSpace(decl.Name))
	if name == "" {
		return false
	}
	if chunk.Body.Fields != nil {
		if filePath, ok := chunk.Body.Fields["file_path"].(string); ok && strings.Contains(strings.ToLower(normalizePath(filePath)), name) {
			return true
		}
		if pkg, ok := chunk.Body.Fields["package"].(string); ok && strings.Contains(strings.ToLower(pkg), name) {
			return true
		}
		if symbol, ok := chunk.Body.Fields["symbol"].(string); ok && strings.Contains(strings.ToLower(symbol), name) {
			return true
		}
	}
	return strings.Contains(strings.ToLower(chunk.Body.Raw), name)
}

func normalizePath(path string) string {
	return filepath.ToSlash(filepath.Clean(strings.TrimSpace(path)))
}

func (c *IntentCore) applyProjectionIntent(state *ClarificationState, intent ProjectionIntent) ([]string, bool, graphdb.MutationStatus, error) {
	intent.Normalize(state.TaskID, state.SessionID)
	if intent.MutationKind == "" {
		return nil, false, graphdb.MutationStatusRejected, errors.New("projection intent missing mutation kind")
	}

	props := map[string]any{
		"provenance":       intent.Provenance,
		"idempotency_key":  intent.IdempotencyKey,
		"revision_root_id": intent.RevisionRootID,
		"revision_of":      intent.RevisionOf,
		"task_id":          state.TaskID,
		"session_id":       state.SessionID,
		"turn_id":          intent.Provenance.TurnID,
		"state_version":    intent.Provenance.StateVersion,
	}
	rawProps, err := json.Marshal(props)
	if err != nil {
		return nil, false, graphdb.MutationStatusRejected, err
	}

	switch intent.MutationKind {
	case "upsert_node":
		if len(intent.SubjectIDs) == 0 {
			return nil, false, graphdb.MutationStatusRejected, errors.New("projection intent missing subject ids")
		}
		nodes := make([]graphdb.NodeRecord, 0, len(intent.SubjectIDs))
		status := graphdb.MutationStatusCreated
		applied := false
		for _, subjectID := range intent.SubjectIDs {
			node := graphdb.NodeRecord{
				ID:             subjectID,
				Kind:           graphdb.NodeKind(intent.NodeKind),
				StableID:       intent.StableID,
				RevisionRootID: intent.RevisionRootID,
				RevisionOf:     intent.RevisionOf,
				IdempotencyKey: intent.IdempotencyKey,
				TaskID:         state.TaskID,
				SessionID:      state.SessionID,
				TurnID:         intent.Provenance.TurnID,
				StateVersion:   intent.Provenance.StateVersion,
				Props:          rawProps,
			}
			if existing, ok := c.Graph.GetNode(subjectID); ok {
				if semanticNodeEqual(existing, node) {
					status = graphdb.MutationStatusMatched
					continue
				} else {
					status = graphdb.MutationStatusUpdated
				}
			}
			applied = true
			nodes = append(nodes, node)
		}
		if len(nodes) > 0 {
			if err := c.Graph.UpsertNodes(nodes); err != nil {
				return nil, false, graphdb.MutationStatusRejected, err
			}
		}
		ids := make([]string, 0, len(nodes))
		for _, node := range nodes {
			ids = append(ids, node.ID)
		}
		return ids, applied, status, nil
	case "upsert_edge":
		if len(intent.SubjectIDs) == 0 || len(intent.ObjectIDs) == 0 {
			return nil, false, graphdb.MutationStatusRejected, errors.New("projection intent missing endpoint ids")
		}
		edge := graphdb.EdgeRecord{
			SourceID:       intent.SubjectIDs[0],
			TargetID:       intent.ObjectIDs[0],
			Kind:           graphdb.EdgeKind(intent.EdgeKind),
			StableID:       intent.StableID,
			RevisionRootID: intent.RevisionRootID,
			RevisionOf:     intent.RevisionOf,
			IdempotencyKey: intent.IdempotencyKey,
			TaskID:         state.TaskID,
			SessionID:      state.SessionID,
			TurnID:         intent.Provenance.TurnID,
			StateVersion:   intent.Provenance.StateVersion,
			Props:          rawProps,
		}
		status := graphdb.MutationStatusCreated
		applied := true
		if existing := findActiveEdgeRecord(c.Graph, edge.SourceID, edge.TargetID, edge.Kind); existing != nil {
			if semanticEdgeEqual(*existing, edge) {
				status = graphdb.MutationStatusMatched
				applied = false
			} else {
				status = graphdb.MutationStatusUpdated
			}
		}
		if applied {
			if err := c.Graph.LinkEdges([]graphdb.EdgeRecord{edge}); err != nil {
				return nil, false, graphdb.MutationStatusRejected, err
			}
		}
		return []string{StableID(edge.SourceID, edge.TargetID, string(edge.Kind), intent.RevisionRootID)}, applied, status, nil
	default:
		return nil, false, graphdb.MutationStatusRejected, fmt.Errorf("unsupported projection mutation kind %q", intent.MutationKind)
	}
}

func summarizeMutationStatus(existing, next graphdb.MutationStatus) graphdb.MutationStatus {
	if next == "" {
		return existing
	}
	if existing == graphdb.MutationStatusConflict || existing == graphdb.MutationStatusRejected {
		return existing
	}
	if next == graphdb.MutationStatusConflict || next == graphdb.MutationStatusRejected {
		return next
	}
	if existing == graphdb.MutationStatusNoop {
		return next
	}
	if existing == next {
		return existing
	}
	switch {
	case existing == graphdb.MutationStatusCreated || next == graphdb.MutationStatusCreated:
		return graphdb.MutationStatusCreated
	case existing == graphdb.MutationStatusUpdated || next == graphdb.MutationStatusUpdated:
		return graphdb.MutationStatusUpdated
	case existing == graphdb.MutationStatusAnnotated || next == graphdb.MutationStatusAnnotated:
		return graphdb.MutationStatusAnnotated
	case existing == graphdb.MutationStatusSuperseded || next == graphdb.MutationStatusSuperseded:
		return graphdb.MutationStatusSuperseded
	case existing == graphdb.MutationStatusMatched || next == graphdb.MutationStatusMatched:
		return graphdb.MutationStatusMatched
	default:
		return next
	}
}

func semanticNodeEqual(a, b graphdb.NodeRecord) bool {
	return a.Kind == b.Kind &&
		a.SourceID == b.SourceID &&
		a.StableID == b.StableID &&
		a.RevisionRootID == b.RevisionRootID &&
		a.RevisionOf == b.RevisionOf &&
		a.IdempotencyKey == b.IdempotencyKey &&
		a.TaskID == b.TaskID &&
		a.SessionID == b.SessionID &&
		a.TurnID == b.TurnID &&
		a.StateVersion == b.StateVersion &&
		reflect.DeepEqual(a.Labels, b.Labels) &&
		reflect.DeepEqual(a.Props, b.Props)
}

func semanticEdgeEqual(a, b graphdb.EdgeRecord) bool {
	return a.SourceID == b.SourceID &&
		a.TargetID == b.TargetID &&
		a.Kind == b.Kind &&
		a.StableID == b.StableID &&
		a.RevisionRootID == b.RevisionRootID &&
		a.RevisionOf == b.RevisionOf &&
		a.IdempotencyKey == b.IdempotencyKey &&
		a.TaskID == b.TaskID &&
		a.SessionID == b.SessionID &&
		a.TurnID == b.TurnID &&
		a.StateVersion == b.StateVersion &&
		a.Weight == b.Weight &&
		reflect.DeepEqual(a.Props, b.Props)
}

func findActiveEdgeRecord(engine *graphdb.Engine, sourceID, targetID string, kind graphdb.EdgeKind) *graphdb.EdgeRecord {
	if engine == nil {
		return nil
	}
	edges := engine.GetOutEdges(sourceID, kind)
	for i := range edges {
		if edges[i].SourceID == sourceID && edges[i].TargetID == targetID && edges[i].Kind == kind {
			edge := edges[i]
			return &edge
		}
	}
	return nil
}
