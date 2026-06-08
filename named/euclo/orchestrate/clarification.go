package orchestrate

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/capability"
	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/contextstream"
	"codeburg.org/lexbit/relurpify/context/knowledge/retrieval"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/prompt"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	intentcontext "codeburg.org/lexbit/relurpify/named/euclo/intentcontext"
	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
	"codeburg.org/lexbit/relurpify/named/euclo/reporting"
	euclostate "codeburg.org/lexbit/relurpify/named/euclo/state"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

const (
	clarificationThoughtRecipeID = "euclo.thoughtrecipe.intent.clarify"
	clarificationCapabilityID    = "euclo:cap.intent.clarify"
	clarificationRequestKey      = "euclo.clarification.request"
	clarificationGroundingKey    = "euclo.clarification.grounding"
	clarificationProjectionKey   = "euclo.clarification.projection"
	clarificationRequeryKey      = "euclo.clarification.requery_request"
	clarificationActionKey       = "action"
	clarificationActionRequest   = "request"
	clarificationActionGround    = "ground"
	clarificationActionProject   = "project"
	clarificationActionRequery   = "requery"
	clarificationActionHandoff   = "handoff"
)

func needsClarificationRoute(env *contextdata.Envelope) bool {
	if v, ok := env.GetWorkingValue(intentcontext.ClarificationStateKey); ok {
		if state, ok := v.(*intentcontext.ClarificationState); ok && state != nil {
			if strings.TrimSpace(state.ActiveThoughtRecipeID) != "" {
				return true
			}
			if len(state.PendingQuestions) > 0 || len(state.PendingProjection) > 0 {
				return true
			}
		}
	}
	if evidence, ok := euclostate.GetIntentEvidence(env); ok && evidence != nil {
		if evidence.RequiresClarification || len(evidence.MissingFields) > 0 {
			return true
		}
	}
	if interpretation, ok := euclostate.GetIntentInterpretation(env); ok && interpretation != nil {
		if len(interpretation.MissingInfo) > 0 {
			return true
		}
	}
	return false
}

func clarificationRouteRequested(env *contextdata.Envelope) bool {
	if _, ok := env.GetWorkingValue(clarificationRequestKey); ok {
		return true
	}
	if needsClarificationRoute(env) {
		return true
	}
	if selection, ok := euclostate.GetRouteSelection(env); ok && selection != nil {
		return strings.TrimSpace(selection.ThoughtRecipeID) == clarificationThoughtRecipeID
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

func (h *clarificationCapabilityHandler) Descriptor(context.Context, *contextdata.Envelope) capability.CapabilityDescriptor {
	return capability.CapabilityDescriptor{
		ID:            clarificationCapabilityID,
		Name:          "intent clarification",
		Kind:          agentspec.CapabilityKindTool,
		RuntimeFamily: agentspec.CapabilityRuntimeFamilyProvider,
		Category:      "clarification",
		Availability:  capability.AvailabilitySpec{Available: true},
	}
}

func (h *clarificationCapabilityHandler) Invoke(ctx context.Context, env *contextdata.Envelope, args map[string]interface{}) (*ports.ToolResult, error) {
	_ = ctx
	state, err := intentcontext.NewStateStore().Read(context.Background(), env)
	if err != nil {
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
		state.ActiveThoughtRecipeID = clarificationThoughtRecipeID
		state.LastUpdatedAt = time.Now().UTC()
		state.Normalize()
		if err := intentcontext.NewStateStore().Write(context.Background(), env, state); err != nil {
			return nil, err
		}
		if env != nil {
			env.SetWorkingValue(clarificationRequestKey, req, contextdata.MemoryClassTask)
			env.SetWorkingValue(intentcontext.ClarificationActiveThoughtRecipeKey, clarificationThoughtRecipeID, contextdata.MemoryClassTask)
			setRouteSelectionContinuation(env, euclotypes.RouteKindIntent, clarificationThoughtRecipeID, euclotypes.RouteKindIntent, clarificationThoughtRecipeID)
			frame := clarificationFrameFromState(state, req, nil)
			if frame != nil {
				env.SetWorkingValue(euclostate.KeyInteractionClarificationFrame, frame, contextdata.MemoryClassTask)
				if interactionFrame := frame.ToInteractionFrame(); interactionFrame != nil {
					_ = interaction.EmitFrame(ctx, interactionFrame, env, telemetry.TelemetryFromContext(ctx))
				}
			}
		}
		emitClarificationGateResult(ctx, env, state, false, "clarify", "follow-up clarification required")
		emitClarificationStarted(ctx, env, state, req)
		result["request"] = req
	case clarificationActionGround:
		grounding, anchors, validationErrs := buildGroundingFromState(state, args)
		if len(validationErrs) > 0 {
			emitClarificationAnswered(ctx, env, state, grounding, validationErrs)
			return &ports.ToolResult{
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
			env.SetWorkingValue(intentcontext.ClarificationActiveThoughtRecipeKey, clarificationThoughtRecipeID, contextdata.MemoryClassTask)
			setRouteSelectionContinuation(env, euclotypes.RouteKindIntent, clarificationThoughtRecipeID, euclotypes.RouteKindIntent, clarificationThoughtRecipeID)
			frame := clarificationFrameFromState(state, req, grounding)
			if frame != nil {
				env.SetWorkingValue(euclostate.KeyInteractionClarificationFrame, frame, contextdata.MemoryClassTask)
				if interactionFrame := frame.ToInteractionFrame(); interactionFrame != nil {
					_ = interaction.EmitFrame(ctx, interactionFrame, env, telemetry.TelemetryFromContext(ctx))
				}
			}
		}
		result["grounding"] = grounding
		result["requery"] = req
	case clarificationActionProject:
		plan, planErr := buildProjectionPlanFromState(state)
		if planErr != nil {
			return &ports.ToolResult{
				Success: false,
				Error:   planErr.Error(),
				Data:    result,
			}, planErr
		}
		state.ActiveThoughtRecipeID = clarificationThoughtRecipeID
		state.LastUpdatedAt = time.Now().UTC()
		state.Normalize()
		if err := intentcontext.NewStateStore().Write(context.Background(), env, state); err != nil {
			return nil, err
		}
		if env != nil {
			env.SetWorkingValue(clarificationProjectionKey, plan, contextdata.MemoryClassTask)
			env.SetWorkingValue(intentcontext.ClarificationActiveThoughtRecipeKey, clarificationThoughtRecipeID, contextdata.MemoryClassTask)
			setRouteSelectionContinuation(env, euclotypes.RouteKindIntent, clarificationThoughtRecipeID, euclotypes.RouteKindIntent, clarificationThoughtRecipeID)
		}
		emitClarificationProjected(ctx, env, state, plan)
		result["projection_plan"] = plan
	case clarificationActionRequery:
		req := buildClarificationRequestFromState(state, taskInstruction, maxTokens, mode)
		if env != nil {
			env.SetWorkingValue(clarificationRequeryKey, req, contextdata.MemoryClassTask)
			env.SetWorkingValue(intentcontext.ClarificationActiveThoughtRecipeKey, clarificationThoughtRecipeID, contextdata.MemoryClassTask)
			setRouteSelectionContinuation(env, euclotypes.RouteKindIntent, clarificationThoughtRecipeID, euclotypes.RouteKindIntent, clarificationThoughtRecipeID)
			frame := clarificationFrameFromState(state, req, nil)
			if frame != nil {
				env.SetWorkingValue(euclostate.KeyInteractionClarificationFrame, frame, contextdata.MemoryClassTask)
				if interactionFrame := frame.ToInteractionFrame(); interactionFrame != nil {
					_ = interaction.EmitFrame(ctx, interactionFrame, env, telemetry.TelemetryFromContext(ctx))
				}
			}
		}
		result["requery"] = req
	case clarificationActionHandoff:
		nextThoughtRecipeID := clarificationThoughtRecipeForState(state, args)
		if nextThoughtRecipeID != "" && env != nil {
			state.ActiveThoughtRecipeID = nextThoughtRecipeID
			state.LastUpdatedAt = time.Now().UTC()
			state.Normalize()
			if err := intentcontext.NewStateStore().Write(context.Background(), env, state); err != nil {
				return nil, err
			}
			euclostate.SetClarificationNextThoughtRecipeID(env, nextThoughtRecipeID)
			env.SetWorkingValue(intentcontext.ClarificationActiveThoughtRecipeKey, nextThoughtRecipeID, contextdata.MemoryClassTask)
			routeKind := euclotypes.RouteKindForThoughtRecipeID(nextThoughtRecipeID)
			setRouteSelectionContinuation(env, routeKind, nextThoughtRecipeID, euclotypes.RouteKindIntent, clarificationThoughtRecipeID)
		}
		if env != nil && nextThoughtRecipeID == "" {
			euclostate.SetClarificationNextThoughtRecipeID(env, "")
			euclostate.SetClarificationUnresolved(env, true)
			euclostate.SetClarificationUnresolvedReason(env, "missing handoff target")
			env.SetWorkingValue(intentcontext.ClarificationActiveThoughtRecipeKey, clarificationThoughtRecipeID, contextdata.MemoryClassTask)
			setRouteSelectionContinuation(env, euclotypes.RouteKindIntent, clarificationThoughtRecipeID, euclotypes.RouteKindIntent, clarificationThoughtRecipeID)
		}
		if nextThoughtRecipeID == "" {
			emitClarificationGateResult(ctx, env, state, false, "unresolved", "missing handoff target")
			result["next_thoughtrecipe_id"] = ""
			result["unresolved"] = true
			return &ports.ToolResult{
				Success: false,
				Error:   "clarification handoff requires a next thoughtrecipe id",
				Data:    result,
			}, fmt.Errorf("clarification handoff requires a next thoughtrecipe id")
		}
		emitClarificationCompleted(ctx, env, state, nextThoughtRecipeID)
		result["next_thoughtrecipe_id"] = nextThoughtRecipeID
	default:
		return &ports.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("unsupported clarification action %q", action),
			Data:    result,
		}, fmt.Errorf("unsupported clarification action %q", action)
	}

	return &ports.ToolResult{
		Success: true,
		Data:    result,
	}, nil
}

func instructionFromEnvelope(env *contextdata.Envelope) string {
	if v, ok := env.GetWorkingValue(euclostate.KeyTaskInputLegacy); ok {
		if task, ok := v.(*execution.Task); ok && task != nil {
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
			"task_id":                 state.TaskID,
			"session_id":              state.SessionID,
			"state_version":           state.StateVersion,
			"active_thoughtrecipe_id": state.ActiveThoughtRecipeID,
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

func clarificationThoughtRecipeForState(state *intentcontext.ClarificationState, args map[string]interface{}) string {
	if thoughtrecipeID := strings.TrimSpace(stringArg(args, "thoughtrecipe_id")); thoughtrecipeID != "" {
		return thoughtrecipeID
	}
	if familyID := strings.TrimSpace(stringArg(args, "family_id")); familyID != "" {
		if thoughtrecipeID := clarificationThoughtRecipeForFamily(familyID); thoughtrecipeID != "" {
			return thoughtrecipeID
		}
	}
	return ""
}

func setRouteSelectionContinuation(env *contextdata.Envelope, targetRouteKind, targetRouteID, sourceRouteKind, sourceRouteID string) {
	routeKind := strings.TrimSpace(targetRouteKind)
	routeID := strings.TrimSpace(targetRouteID)
	sourceKind := strings.TrimSpace(sourceRouteKind)
	sourceID := strings.TrimSpace(sourceRouteID)
	euclostate.SetRouteSelection(env, &euclotypes.RouteSelection{
		RouteKind:       routeKind,
		ThoughtRecipeID: routeID,
	})
	euclostate.SetRouteContinuation(env, &euclotypes.RouteContinuation{
		SharedContext:         true,
		SourceRouteKind:       sourceKind,
		SourceRouteID:         sourceID,
		TargetRouteKind:       routeKind,
		TargetRouteID:         routeID,
		ActiveThoughtRecipeID: routeID,
	})
}

func clarificationFrameFromState(state *intentcontext.ClarificationState, req *contextstream.Request, grounding map[string]any) *ClarificationFrame {
	if state == nil || req == nil {
		return nil
	}
	choices := clarificationFrameChoicesFromState(state)
	missingFields := []string{}
	if grounding != nil {
		if grounded, ok := grounding["grounded_anchor_ids"].([]string); ok && len(grounded) == 0 {
			missingFields = append(missingFields, "grounding")
		}
	}
	resume := &interaction.ClarificationResumeMetadata{
		ActiveThoughtRecipeID: clarificationThoughtRecipeID,
		RouteKind:             euclotypes.RouteKindIntent,
		RouteID:               clarificationThoughtRecipeID,
		StateVersion:          state.StateVersion,
		Unresolved:            len(choices) == 0 || len(missingFields) > 0,
		MissingFields:         append([]string(nil), missingFields...),
	}
	return NewClarificationFrame(state.TaskID, state.SessionID, clarificationThoughtRecipeID, strings.TrimSpace(req.Query.Text), choices, resume.MissingFields, resume)
}

func clarificationFrameChoicesFromState(state *intentcontext.ClarificationState) []string {
	if state == nil || state.Ambiguity == nil {
		return nil
	}
	return interaction.NormalizeChoices(state.Ambiguity.CandidateFamilies)
}

func seedClarificationAmbiguityFromEnvelope(state *intentcontext.ClarificationState, env *contextdata.Envelope) {
	if state == nil {
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
	if classification, ok := euclostate.GetIntentClassification(env); ok && classification != nil {
		for _, candidate := range classification.FamilyCandidates {
			addFamily(candidate.FamilyID)
		}
		addFamily(classification.WinningFamily)
	}
	if v, ok := env.GetWorkingValue(euclostate.KeyFamilySelection); ok {
		switch value := v.(type) {
		case map[string]any:
			if winning, ok := value["winning_family"].(string); ok {
				addFamily(winning)
			}
		case string:
			addFamily(value)
		}
	}
	addFamily(euclostate.GetFamilySelected(env))
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

func clarificationThoughtRecipeForFamily(family string) string {
	switch strings.ToLower(strings.TrimSpace(family)) {
	case "review":
		return "euclo.thoughtrecipe.code_review"
	case "investigation":
		return "euclo.thoughtrecipe.investigation"
	case "debug":
		return "euclo.thoughtrecipe.debug_tdd_repair"
	case "migration":
		return "euclo.thoughtrecipe.dep_upgrade"
	case "implementation":
		return "euclo.thoughtrecipe.test_synthesis"
	case "refactor":
		return "euclo.thoughtrecipe.extract_func"
	case "architecture":
		return "euclo.thoughtrecipe.investigation"
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
		"euclo.thoughtrecipe.intent.clarify",
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

func emitClarificationGateResult(ctx context.Context, env *contextdata.Envelope, state *intentcontext.ClarificationState, passed bool, decision, reason string) {
	tel := reporting.NewEucloTelemetry(telemetry.TelemetryFromContext(ctx))
	if tel == nil {
		return
	}
	taskID := ""
	sessionID := ""
	if state != nil {
		taskID = state.TaskID
		sessionID = state.SessionID
	} else if env != nil {
		taskID = env.TaskID
		sessionID = env.SessionID
	}
	tel.EmitGateResult(ctx, reporting.EventGateResult{
		EventHeader: reporting.EventHeader{
			TaskID:     taskID,
			SessionID:  sessionID,
			Seq:        0,
			OccurredAt: time.Now().UTC(),
		},
		GateID:   clarificationCapabilityID,
		Passed:   passed,
		Decision: strings.TrimSpace(decision),
	})
	euclostate.SetClarificationGateResult(env, map[string]any{
		"gate_id":    clarificationCapabilityID,
		"passed":     passed,
		"decision":   strings.TrimSpace(decision),
		"reason":     strings.TrimSpace(reason),
		"task_id":    taskID,
		"session_id": sessionID,
	})
}

func emitClarificationStarted(ctx context.Context, env *contextdata.Envelope, state *intentcontext.ClarificationState, req *contextstream.Request) {
	_ = env
	tel := reporting.NewEucloTelemetry(telemetry.TelemetryFromContext(ctx))
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
		ThoughtRecipeID:   clarificationThoughtRecipeID,
		StateVersion:      state.StateVersion,
		AmbiguityKind:     ambiguityKind,
		CandidateFamilies: candidateFamilies,
		Question:          question,
	})
}

func emitClarificationAnswered(ctx context.Context, env *contextdata.Envelope, state *intentcontext.ClarificationState, grounding map[string]any, validationErrs []string) {
	_ = env
	tel := reporting.NewEucloTelemetry(telemetry.TelemetryFromContext(ctx))
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
	tel := reporting.NewEucloTelemetry(telemetry.TelemetryFromContext(ctx))
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
	tel := reporting.NewEucloTelemetry(telemetry.TelemetryFromContext(ctx))
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

func emitClarificationCompleted(ctx context.Context, env *contextdata.Envelope, state *intentcontext.ClarificationState, nextThoughtRecipeID string) {
	_ = env
	tel := reporting.NewEucloTelemetry(telemetry.TelemetryFromContext(ctx))
	if tel == nil || state == nil {
		return
	}
	completion := "completed"
	if strings.TrimSpace(nextThoughtRecipeID) != "" {
		completion = "handoff:" + strings.TrimSpace(nextThoughtRecipeID)
	}
	tel.EmitClarificationCompleted(ctx, reporting.EventClarificationCompleted{
		EventHeader: reporting.EventHeader{
			TaskID:     state.TaskID,
			SessionID:  state.SessionID,
			Seq:        0,
			OccurredAt: time.Now().UTC(),
		},
		ThoughtRecipeID: clarificationThoughtRecipeID,
		StateVersion:    state.StateVersion,
		PlanID:          "",
		Completion:      completion,
	})
}
