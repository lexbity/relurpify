package orchestrate

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/contextstream"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/prompt"
	"codeburg.org/lexbit/relurpify/framework/retrieval"
	"codeburg.org/lexbit/relurpify/named/euclo/intake"
	intentcontext "codeburg.org/lexbit/relurpify/named/euclo/intentcontext"
	"codeburg.org/lexbit/relurpify/named/euclo/reporting"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

const (
	clarificationRecipeID      = "euclo.recipe.intent.clarify"
	clarificationCapabilityID  = "euclo:cap.intent.clarify"
	clarificationRequestKey    = "euclo.clarification.request"
	clarificationGroundingKey  = "euclo.clarification.grounding"
	clarificationProjectionKey = "euclo.clarification.projection"
	clarificationRequeryKey    = "euclo.clarification.requery_request"
	clarificationActionKey     = "action"
	clarificationActionRequest = "request"
	clarificationActionGround  = "ground"
	clarificationActionProject = "project"
	clarificationActionRequery = "requery"
	clarificationActionHandoff = "handoff"
)

func needsClarificationRoute(env *contextdata.Envelope) bool {
	if env == nil {
		return false
	}
	if v, ok := env.GetWorkingValue("euclo.intent_classification"); ok {
		if cls, ok := v.(*intake.IntentClassification); ok && cls != nil {
			return cls.Ambiguous || cls.Confidence < 0.7
		}
		if ambiguous, confidence, ok := ambiguityFromValue(v); ok {
			return ambiguous || confidence < 0.7
		}
	}
	return false
}

func clarificationRouteRequested(env *contextdata.Envelope) bool {
	if env == nil {
		return false
	}
	if needsClarificationRoute(env) {
		return true
	}
	if v, ok := env.GetWorkingValue("euclo.route_selection"); ok {
		if selection, ok := v.(*RouteSelection); ok && selection != nil {
			return strings.TrimSpace(selection.RecipeID) == clarificationRecipeID
		}
	}
	return false
}

func registerClarificationCapability(reg *capability.CapabilityRegistry) error {
	if reg == nil {
		return nil
	}
	if _, ok := reg.GetCapability(clarificationCapabilityID); ok {
		return nil
	}
	return reg.RegisterInvocableCapability(&clarificationCapabilityHandler{})
}

type clarificationCapabilityHandler struct{}

func (h *clarificationCapabilityHandler) Descriptor(context.Context, *contextdata.Envelope) core.CapabilityDescriptor {
	return core.CapabilityDescriptor{
		ID:            clarificationCapabilityID,
		Name:          "intent clarification",
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyProvider,
		Category:      "clarification",
		Availability:  core.AvailabilitySpec{Available: true},
	}
}

func (h *clarificationCapabilityHandler) Invoke(ctx context.Context, env *contextdata.Envelope, args map[string]interface{}) (*contracts.CapabilityExecutionResult, error) {
	_ = ctx
	state, err := intentcontext.NewStateStore().Read(context.Background(), env)
	if err != nil {
		if env == nil {
			return nil, err
		}
		state = intentcontext.NewState(env.TaskID, env.SessionID)
	}
	if state == nil {
		state = intentcontext.NewState(taskID(env), sessionID(env))
	}

	action := strings.ToLower(strings.TrimSpace(stringArg(args, clarificationActionKey)))
	if action == "" {
		action = clarificationActionRequest
	}

	taskInstruction := instructionFromEnvelope(env)
	maxTokens := 1024
	if v, ok := args["max_tokens"].(int); ok && v > 0 {
		maxTokens = v
	}
	if v, ok := args["max_tokens"].(float64); ok && int(v) > 0 {
		maxTokens = int(v)
	}
	mode := contextstream.ModeBlocking
	if v := strings.TrimSpace(fmt.Sprint(args["mode"])); v != "" {
		mode = contextstream.Mode(v)
	}

	result := map[string]any{
		"capability_id": clarificationCapabilityID,
		"action":        action,
	}

	switch action {
	case clarificationActionRequest:
		seedClarificationAmbiguityFromEnvelope(state, env)
		req := buildClarificationRequestFromState(state, taskInstruction, maxTokens, mode)
		state.ActiveRecipeID = clarificationRecipeID
		state.LastUpdatedAt = time.Now().UTC()
		state.Normalize()
		if err := intentcontext.NewStateStore().Write(context.Background(), env, state); err != nil {
			return nil, err
		}
		if env != nil {
			env.SetWorkingValue(clarificationRequestKey, req, contextdata.MemoryClassTask)
			env.SetWorkingValue(intentcontext.ClarificationActiveRecipeKey, clarificationRecipeID, contextdata.MemoryClassTask)
		}
		emitClarificationStarted(ctx, env, state, req)
		result["request"] = req
	case clarificationActionGround:
		grounding, anchors, validationErrs := buildGroundingFromState(state, args)
		if len(validationErrs) > 0 {
			emitClarificationAnswered(ctx, env, state, grounding, validationErrs)
			return &contracts.CapabilityExecutionResult{
				Success: false,
				Error:   strings.Join(validationErrs, "; "),
				Data:    result,
			}, fmt.Errorf("clarification grounding validation failed: %s", strings.Join(validationErrs, "; "))
		}
		state.GroundedAnchors = anchors
		state.StateVersion = intentcontext.NextStateVersion(state.StateVersion)
		state.LastUpdatedAt = time.Now().UTC()
		state.Normalize()
		if err := intentcontext.NewStateStore().Write(context.Background(), env, state); err != nil {
			return nil, err
		}
		emitClarificationAnsweredAndGrounded(ctx, env, state, grounding, nil)
		req := buildClarificationRequestFromState(state, taskInstruction, maxTokens, mode)
		if env != nil {
			env.SetWorkingValue(clarificationGroundingKey, grounding, contextdata.MemoryClassTask)
			env.SetWorkingValue(clarificationRequeryKey, req, contextdata.MemoryClassTask)
			env.SetWorkingValue(intentcontext.ClarificationActiveRecipeKey, clarificationRecipeID, contextdata.MemoryClassTask)
		}
		result["grounding"] = grounding
		result["requery"] = req
	case clarificationActionProject:
		plan, planErr := buildProjectionPlanFromState(state)
		if planErr != nil {
			return &contracts.CapabilityExecutionResult{
				Success: false,
				Error:   planErr.Error(),
				Data:    result,
			}, planErr
		}
		state.ActiveRecipeID = clarificationRecipeID
		state.LastUpdatedAt = time.Now().UTC()
		state.Normalize()
		if err := intentcontext.NewStateStore().Write(context.Background(), env, state); err != nil {
			return nil, err
		}
		if env != nil {
			env.SetWorkingValue(clarificationProjectionKey, plan, contextdata.MemoryClassTask)
			env.SetWorkingValue(intentcontext.ClarificationActiveRecipeKey, clarificationRecipeID, contextdata.MemoryClassTask)
		}
		emitClarificationProjected(ctx, env, state, plan)
		result["projection_plan"] = plan
	case clarificationActionRequery:
		req := buildClarificationRequestFromState(state, taskInstruction, maxTokens, mode)
		if env != nil {
			env.SetWorkingValue(clarificationRequeryKey, req, contextdata.MemoryClassTask)
			env.SetWorkingValue(intentcontext.ClarificationActiveRecipeKey, clarificationRecipeID, contextdata.MemoryClassTask)
		}
		result["requery"] = req
	case clarificationActionHandoff:
		nextRecipeID := clarificationRecipeForState(state, args)
		if nextRecipeID != "" && env != nil {
			env.SetWorkingValue("euclo.clarification.next_recipe_id", nextRecipeID, contextdata.MemoryClassTask)
			env.SetWorkingValue(intentcontext.ClarificationActiveRecipeKey, nextRecipeID, contextdata.MemoryClassTask)
			env.SetWorkingValue("euclo.route_selection", &RouteSelection{RouteKind: "recipe", RecipeID: nextRecipeID}, contextdata.MemoryClassTask)
		}
		if env != nil && nextRecipeID == "" {
			env.SetWorkingValue("euclo.clarification.next_recipe_id", "", contextdata.MemoryClassTask)
			env.SetWorkingValue("euclo.execution.completed", true, contextdata.MemoryClassTask)
		}
		emitClarificationCompleted(ctx, env, state, nextRecipeID)
		result["next_recipe_id"] = nextRecipeID
	default:
		return &contracts.CapabilityExecutionResult{
			Success: false,
			Error:   fmt.Sprintf("unsupported clarification action %q", action),
			Data:    result,
		}, fmt.Errorf("unsupported clarification action %q", action)
	}

	return &contracts.CapabilityExecutionResult{
		Success: true,
		Data:    result,
	}, nil
}

func instructionFromEnvelope(env *contextdata.Envelope) string {
	if env == nil {
		return ""
	}
	if v, ok := env.GetWorkingValue("task.input"); ok {
		if task, ok := v.(*core.Task); ok && task != nil {
			return strings.TrimSpace(task.Instruction)
		}
	}
	return ""
}

func buildClarificationRequestFromState(state *intentcontext.ClarificationState, instruction string, maxTokens int, mode contextstream.Mode) *contextstream.Request {
	anchors := append([]retrieval.AnchorRef(nil), state.GroundedAnchors...)
	traversal := buildTraversalFromAnchors(anchors)
	req := &contextstream.Request{
		Query: retrieval.RetrievalQuery{
			Text:      strings.TrimSpace(instruction),
			Anchors:   anchors,
			Traversal: traversal,
		},
		MaxTokens:   maxTokens,
		Mode:        mode,
		RequestedAt: time.Now().UTC(),
		Metadata: map[string]any{
			"task_id":          state.TaskID,
			"session_id":       state.SessionID,
			"state_version":    state.StateVersion,
			"active_recipe_id": state.ActiveRecipeID,
		},
	}
	if state.Ambiguity != nil {
		req.Metadata["ambiguity_kind"] = string(state.Ambiguity.Kind)
		req.Metadata["ambiguity_confidence"] = state.Ambiguity.Confidence
		req.Metadata["ambiguity_rationale"] = state.Ambiguity.Rationale
	}
	return req
}

func buildTraversalFromAnchors(anchors []retrieval.AnchorRef) *retrieval.TraversalSpec {
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

func ambiguityFromValue(value any) (bool, float64, bool) {
	if value == nil {
		return false, 0, false
	}
	rv := reflect.ValueOf(value)
	if rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return false, 0, false
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		if m, ok := value.(map[string]any); ok {
			ambiguous, _ := m["ambiguous"].(bool)
			confidence := confidenceFromAny(m["confidence"])
			return ambiguous, confidence, true
		}
		return false, 0, false
	}
	ambiguous := fieldBool(rv, "Ambiguous")
	confidence := fieldFloat(rv, "Confidence")
	return ambiguous, confidence, true
}

func fieldBool(rv reflect.Value, name string) bool {
	f := rv.FieldByName(name)
	if !f.IsValid() || !f.CanInterface() || f.Kind() != reflect.Bool {
		return false
	}
	return f.Bool()
}

func fieldFloat(rv reflect.Value, name string) float64 {
	f := rv.FieldByName(name)
	if !f.IsValid() || !f.CanInterface() {
		return 0
	}
	switch f.Kind() {
	case reflect.Float32, reflect.Float64:
		return f.Float()
	default:
		return confidenceFromAny(f.Interface())
	}
}

func confidenceFromAny(value any) float64 {
	switch v := value.(type) {
	case float32:
		return float64(v)
	case float64:
		return v
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case int32:
		return float64(v)
	case uint:
		return float64(v)
	case uint64:
		return float64(v)
	case uint32:
		return float64(v)
	default:
		return 0
	}
}

func clarificationRecipeForState(state *intentcontext.ClarificationState, args map[string]interface{}) string {
	if recipeID := strings.TrimSpace(stringArg(args, "recipe_id")); recipeID != "" {
		return recipeID
	}
	if familyID := strings.TrimSpace(stringArg(args, "family_id")); familyID != "" {
		if recipeID := clarificationRecipeForFamily(familyID); recipeID != "" {
			return recipeID
		}
	}
	return ""
}

func seedClarificationAmbiguityFromEnvelope(state *intentcontext.ClarificationState, env *contextdata.Envelope) {
	if state == nil || env == nil {
		return
	}
	candidateFamilies := stateCandidateFamiliesFromEnvelope(env)
	if len(candidateFamilies) == 0 && state.Ambiguity != nil {
		candidateFamilies = append([]string(nil), state.Ambiguity.CandidateFamilies...)
	}
	if len(candidateFamilies) == 0 {
		return
	}
	if state.Ambiguity == nil {
		state.Ambiguity = &intentcontext.AmbiguityCharacterization{}
	}
	if len(state.Ambiguity.CandidateFamilies) == 0 {
		state.Ambiguity.CandidateFamilies = candidateFamilies
	}
	if state.Ambiguity.Kind == "" || state.Ambiguity.Kind == intentcontext.AmbiguityKindUnknown {
		state.Ambiguity.Kind = intentcontext.AmbiguityKindMultiMatch
	}
	if state.Ambiguity.Confidence == 0 {
		state.Ambiguity.Confidence = 0.5
	}
	if strings.TrimSpace(state.Ambiguity.Rationale) == "" {
		state.Ambiguity.Rationale = "seeded from intake family candidates"
	}
}

func stateCandidateFamiliesFromEnvelope(env *contextdata.Envelope) []string {
	if env == nil {
		return nil
	}
	seen := make(map[string]struct{})
	familiesOut := make([]string, 0, 4)
	addFamily := func(family string) {
		family = strings.TrimSpace(family)
		if family == "" {
			return
		}
		if _, ok := seen[family]; ok {
			return
		}
		seen[family] = struct{}{}
		familiesOut = append(familiesOut, family)
	}
	if v, ok := env.GetWorkingValue("euclo.intent_classification"); ok {
		if classification, ok := v.(*intake.IntentClassification); ok && classification != nil {
			for _, candidate := range classification.FamilyCandidates {
				addFamily(candidate.FamilyID)
			}
			addFamily(classification.WinningFamily)
		}
	}
	if v, ok := env.GetWorkingValue("euclo.family_selection"); ok {
		switch value := v.(type) {
		case map[string]any:
			if winning, ok := value["winning_family"].(string); ok {
				addFamily(winning)
			}
		case string:
			addFamily(value)
		}
	}
	if v, ok := env.GetWorkingValue("euclo.family.selected"); ok {
		if family, ok := v.(string); ok {
			addFamily(family)
		}
	}
	return familiesOut
}

func stringArg(args map[string]interface{}, key string) string {
	if len(args) == 0 {
		return ""
	}
	value, ok := args[key]
	if !ok || value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return v
	default:
		return fmt.Sprint(v)
	}
}

func clarificationRecipeForFamily(family string) string {
	switch strings.ToLower(strings.TrimSpace(family)) {
	case "review":
		return "euclo.recipe.code_review"
	case "investigation":
		return "euclo.recipe.investigation"
	case "debug":
		return "euclo.recipe.debug_tdd_repair"
	case "migration":
		return "euclo.recipe.dep_upgrade"
	case "implementation":
		return "euclo.recipe.test_synthesis"
	case "refactor":
		return "euclo.recipe.extract_func"
	case "architecture":
		return "euclo.recipe.investigation"
	default:
		return ""
	}
}

func buildGroundingFromState(state *intentcontext.ClarificationState, args map[string]interface{}) (map[string]any, []retrieval.AnchorRef, []string) {
	grounding := map[string]any{
		"grounded_anchor_ids": []string{},
	}
	if state == nil {
		return grounding, nil, validateStructuredGrounding(grounding)
	}
	anchors := make([]retrieval.AnchorRef, 0)
	for _, scope := range state.ConfirmedScopes {
		for _, entity := range scope.Entities {
			anchor := anchorFromEntity(state, entity, scope.Name, string(scope.AnchorClass))
			if anchor.AnchorID != "" {
				anchors = append(anchors, anchor)
			}
		}
	}
	for _, entity := range state.ConfirmedEntities {
		anchor := anchorFromEntity(state, entity, entity.Name, "clarified_entity")
		if anchor.AnchorID != "" {
			anchors = append(anchors, anchor)
		}
	}
	anchors = dedupeAnchors(anchors)
	ids := make([]string, 0, len(anchors))
	for _, anchor := range anchors {
		ids = append(ids, anchor.AnchorID)
	}
	grounding["grounded_anchor_ids"] = ids
	grounding["confirmed_entities"] = len(state.ConfirmedEntities)
	grounding["confirmed_scopes"] = len(state.ConfirmedScopes)
	if scopeName, ok := args["scope_name"].(string); ok && strings.TrimSpace(scopeName) != "" {
		grounding["scope_name"] = strings.TrimSpace(scopeName)
	}
	return grounding, anchors, validateStructuredGrounding(grounding)
}

func anchorFromEntity(state *intentcontext.ClarificationState, entity intentcontext.ConfirmedEntity, term, class string) retrieval.AnchorRef {
	entity.EntityRef.Normalize()
	chunkID := strings.TrimSpace(entity.EntityRef.ChunkID)
	if chunkID == "" {
		chunkID = strings.TrimSpace(entity.EntityRef.EntityID)
	}
	if chunkID == "" {
		chunkID = strings.TrimSpace(entity.Name)
	}
	anchorID := intentcontext.StableID(state.TaskID, state.SessionID, "anchor", entity.StableID, chunkID, term, class)
	return retrieval.AnchorRef{
		AnchorID:   anchorID,
		ChunkID:    chunkID,
		Term:       term,
		Definition: "clarified anchor",
		Class:      class,
		Active:     true,
		CreatedAt:  time.Now().UTC().Format(time.RFC3339),
	}
}

func dedupeAnchors(anchors []retrieval.AnchorRef) []retrieval.AnchorRef {
	if len(anchors) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(anchors))
	out := make([]retrieval.AnchorRef, 0, len(anchors))
	for _, anchor := range anchors {
		if strings.TrimSpace(anchor.AnchorID) == "" {
			continue
		}
		if _, ok := seen[anchor.AnchorID]; ok {
			continue
		}
		seen[anchor.AnchorID] = struct{}{}
		out = append(out, anchor)
	}
	return out
}

func buildProjectionPlanFromState(state *intentcontext.ClarificationState) (*intentcontext.ProjectionPlan, error) {
	if state == nil {
		return nil, fmt.Errorf("clarification state is nil")
	}
	env := contextdata.NewEnvelope(state.TaskID, state.SessionID)
	env.SetWorkingValue(intentcontext.ClarificationStateKey, state.Clone(), contextdata.MemoryClassTask)
	plan, err := intentcontext.NewIntentCore(nil, nil).BuildProjectionPlan(context.Background(), env)
	if err != nil {
		return &intentcontext.ProjectionPlan{
			PlanID:       intentcontext.StableID(state.TaskID, state.SessionID, "projection_plan", fmt.Sprint(state.StateVersion), state.CurrentTurnID),
			StateVersion: state.StateVersion,
		}, nil
	}
	if plan == nil {
		return &intentcontext.ProjectionPlan{
			PlanID:       intentcontext.StableID(state.TaskID, state.SessionID, "projection_plan", fmt.Sprint(state.StateVersion), state.CurrentTurnID),
			StateVersion: state.StateVersion,
		}, nil
	}
	return plan, nil
}

func validateStructuredGrounding(value map[string]any) []string {
	issues := prompt.ValidateStructuredMap(
		"euclo.recipe.intent.clarify",
		"grounding",
		value,
		[]string{"grounded_anchor_ids", "confirmed_entities", "confirmed_scopes"},
	)
	if len(issues) == 0 {
		return nil
	}
	errs := make([]string, 0, len(issues))
	for _, issue := range issues {
		errs = append(errs, issue.Error())
	}
	return errs
}

func emitClarificationStarted(ctx context.Context, env *contextdata.Envelope, state *intentcontext.ClarificationState, req *contextstream.Request) {
	_ = env
	tel := reporting.NewEucloTelemetry(core.TelemetryFromContext(ctx))
	if tel == nil || state == nil {
		return
	}
	ambiguityKind := ""
	candidateFamilies := []string(nil)
	if state.Ambiguity != nil {
		ambiguityKind = string(state.Ambiguity.Kind)
		candidateFamilies = append([]string(nil), state.Ambiguity.CandidateFamilies...)
	}
	question := ""
	if req != nil {
		question = strings.TrimSpace(req.Query.Text)
	}
	tel.EmitClarificationStarted(ctx, reporting.EventClarificationStarted{
		EventHeader: reporting.EventHeader{
			TaskID:     state.TaskID,
			SessionID:  state.SessionID,
			Seq:        0,
			OccurredAt: time.Now().UTC(),
		},
		RecipeID:          clarificationRecipeID,
		StateVersion:      state.StateVersion,
		AmbiguityKind:     ambiguityKind,
		CandidateFamilies: candidateFamilies,
		Question:          question,
	})
}

func emitClarificationAnswered(ctx context.Context, env *contextdata.Envelope, state *intentcontext.ClarificationState, grounding map[string]any, validationErrs []string) {
	_ = env
	tel := reporting.NewEucloTelemetry(core.TelemetryFromContext(ctx))
	if tel == nil || state == nil {
		return
	}
	answerText := ""
	if v, ok := grounding["answer_text"].(string); ok {
		answerText = strings.TrimSpace(v)
	}
	responseKind := ""
	if v, ok := grounding["response_kind"].(string); ok {
		responseKind = strings.TrimSpace(v)
	}
	tel.EmitClarificationAnswered(ctx, reporting.EventClarificationAnswered{
		EventHeader: reporting.EventHeader{
			TaskID:     state.TaskID,
			SessionID:  state.SessionID,
			Seq:        0,
			OccurredAt: time.Now().UTC(),
		},
		TurnID:         state.CurrentTurnID,
		AnswerText:     answerText,
		ResponseKind:   responseKind,
		StateVersion:   state.StateVersion,
		ValidationErrs: append([]string(nil), validationErrs...),
	})
}

func emitClarificationAnsweredAndGrounded(ctx context.Context, env *contextdata.Envelope, state *intentcontext.ClarificationState, grounding map[string]any, validationErrs []string) {
	emitClarificationAnswered(ctx, env, state, grounding, validationErrs)
	emitClarificationGrounded(ctx, env, state, grounding)
}

func emitClarificationGrounded(ctx context.Context, env *contextdata.Envelope, state *intentcontext.ClarificationState, grounding map[string]any) {
	_ = env
	tel := reporting.NewEucloTelemetry(core.TelemetryFromContext(ctx))
	if tel == nil || state == nil {
		return
	}
	anchorIDs := []string{}
	if ids, ok := grounding["grounded_anchor_ids"].([]string); ok {
		anchorIDs = append(anchorIDs, ids...)
	}
	confirmedEntities := 0
	if v, ok := grounding["confirmed_entities"].(int); ok {
		confirmedEntities = v
	}
	confirmedScopes := 0
	if v, ok := grounding["confirmed_scopes"].(int); ok {
		confirmedScopes = v
	}
	tel.EmitClarificationGrounded(ctx, reporting.EventClarificationGrounded{
		EventHeader: reporting.EventHeader{
			TaskID:     state.TaskID,
			SessionID:  state.SessionID,
			Seq:        0,
			OccurredAt: time.Now().UTC(),
		},
		StateVersion:      state.StateVersion,
		GroundedAnchorIDs: anchorIDs,
		ConfirmedEntities: confirmedEntities,
		ConfirmedScopes:   confirmedScopes,
	})
}

func emitClarificationProjected(ctx context.Context, env *contextdata.Envelope, state *intentcontext.ClarificationState, plan *intentcontext.ProjectionPlan) {
	_ = env
	tel := reporting.NewEucloTelemetry(core.TelemetryFromContext(ctx))
	if tel == nil || state == nil || plan == nil {
		return
	}
	tel.EmitClarificationProjected(ctx, reporting.EventClarificationProjected{
		EventHeader: reporting.EventHeader{
			TaskID:     state.TaskID,
			SessionID:  state.SessionID,
			Seq:        0,
			OccurredAt: time.Now().UTC(),
		},
		PlanID:       plan.PlanID,
		StateVersion: state.StateVersion,
		Details: map[string]any{
			"intent_count":   len(plan.Intents),
			"conflict_count": len(plan.Conflicts),
		},
	})
}

func emitClarificationCompleted(ctx context.Context, env *contextdata.Envelope, state *intentcontext.ClarificationState, nextRecipeID string) {
	_ = env
	tel := reporting.NewEucloTelemetry(core.TelemetryFromContext(ctx))
	if tel == nil || state == nil {
		return
	}
	completion := "completed"
	if strings.TrimSpace(nextRecipeID) != "" {
		completion = "handoff:" + strings.TrimSpace(nextRecipeID)
	}
	tel.EmitClarificationCompleted(ctx, reporting.EventClarificationCompleted{
		EventHeader: reporting.EventHeader{
			TaskID:     state.TaskID,
			SessionID:  state.SessionID,
			Seq:        0,
			OccurredAt: time.Now().UTC(),
		},
		RecipeID:     clarificationRecipeID,
		StateVersion: state.StateVersion,
		PlanID:       "",
		Completion:   completion,
	})
}
