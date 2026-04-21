package agentstate

import (
	"strconv"
	"strings"

	archaeodomain "codeburg.org/lexbit/relurpify/archaeo/domain"
	archaeolearning "codeburg.org/lexbit/relurpify/archaeo/learning"
	archaeoprojections "codeburg.org/lexbit/relurpify/archaeo/projections"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
	eucloruntime "codeburg.org/lexbit/relurpify/named/euclo/runtime"
	eucloarchaeomem "codeburg.org/lexbit/relurpify/named/euclo/runtime/archaeomem"
	euclopretask "codeburg.org/lexbit/relurpify/named/euclo/runtime/pretask"
	"codeburg.org/lexbit/relurpify/named/euclo/runtime/statebus"
)

func EnrichBundleWithContextKnowledge(bundle eucloruntime.SemanticInputBundle, state *core.Context) eucloruntime.SemanticInputBundle {
	if state == nil {
		return bundle
	}
	raw, ok := statebus.GetAny(state, "context.knowledge_items")
	if !ok || raw == nil {
		return bundle
	}
	switch items := raw.(type) {
	case []euclopretask.KnowledgeEvidenceItem:
		if len(items) == 0 {
			return bundle
		}
		for _, item := range items {
			finding := eucloruntime.SemanticFindingSummary{
				RefID:       item.RefID,
				Kind:        "context_retrieved_" + string(item.Kind),
				Status:      "retrieved",
				Title:       item.Title,
				Summary:     item.Summary,
				RelatedRefs: append([]string(nil), item.RelatedRefs...),
			}
			bundle.PatternFindings = append(bundle.PatternFindings, finding)
		}
	case []euclopretask.ContextKnowledgeItem:
		if len(items) == 0 {
			return bundle
		}
		for i, item := range items {
			finding := eucloruntime.SemanticFindingSummary{
				RefID:       item.Source + ":" + item.Content,
				Kind:        "context_retrieved_deferred_issue",
				Status:      "retrieved",
				Title:       item.Source,
				Summary:     item.Content,
				RelatedRefs: append([]string(nil), item.Tags...),
			}
			if finding.RefID == ":" {
				finding.RefID = "context_knowledge_item"
			}
			if finding.Title == "" {
				finding.Title = "Deferred issue"
			}
			finding.RefID = finding.RefID + "#" + strconv.Itoa(i)
			bundle.PatternFindings = append(bundle.PatternFindings, finding)
		}
	case []any:
		for _, rawItem := range items {
			switch item := rawItem.(type) {
			case euclopretask.KnowledgeEvidenceItem:
				finding := eucloruntime.SemanticFindingSummary{
					RefID:       item.RefID,
					Kind:        "context_retrieved_" + string(item.Kind),
					Status:      "retrieved",
					Title:       item.Title,
					Summary:     item.Summary,
					RelatedRefs: append([]string(nil), item.RelatedRefs...),
				}
				bundle.PatternFindings = append(bundle.PatternFindings, finding)
			case euclopretask.ContextKnowledgeItem:
				finding := eucloruntime.SemanticFindingSummary{
					RefID:       item.Source + ":" + item.Content,
					Kind:        "context_retrieved_deferred_issue",
					Status:      "retrieved",
					Title:       item.Source,
					Summary:     item.Content,
					RelatedRefs: append([]string(nil), item.Tags...),
				}
				if finding.Title == "" {
					finding.Title = "Deferred issue"
				}
				bundle.PatternFindings = append(bundle.PatternFindings, finding)
			}
		}
	default:
		return bundle
	}
	return bundle
}

func SeedInteractionPrepass(state *core.Context, task *core.Task, classification eucloruntime.TaskClassification, mode euclotypes.ModeResolution) {
	if state == nil {
		return
	}
	instruction := ""
	if task != nil {
		instruction = strings.ToLower(strings.TrimSpace(task.Instruction))
	}
	state.Set("requires_evidence_before_mutation", classification.RequiresEvidenceBeforeMutation)
	switch mode.ModeID {
	case "debug":
		if HasInstructionEvidence(instruction, classification.ReasonCodes) {
			state.Set("has_evidence", true)
			state.Set("evidence_in_instruction", true)
		}
	case "code":
		if strings.Contains(instruction, "just do it") {
			state.Set("just_do_it", true)
		}
	case "planning":
		if strings.Contains(instruction, "just plan it") || strings.Contains(instruction, "skip to plan") {
			state.Set("just_plan_it", true)
		}
	}
}

func HasInstructionEvidence(instruction string, reasonCodes []string) bool {
	for _, reason := range reasonCodes {
		if strings.HasPrefix(reason, "error_text:") {
			return true
		}
	}
	for _, token := range []string{"panic:", "stacktrace", "stack trace", "goroutine ", ".go:", "failing test", "runtime error"} {
		if strings.Contains(instruction, token) {
			return true
		}
	}
	return false
}

func InteractionEmitterForTask(task *core.Task) (interaction.FrameEmitter, bool) {
	script := InteractionScriptFromTask(task)
	if len(script) == 0 {
		return &interaction.NoopEmitter{}, false
	}
	return interaction.NewTestFrameEmitter(script...), true
}

func InteractionScriptFromTask(task *core.Task) []interaction.ScriptedResponse {
	if task == nil || task.Context == nil {
		return nil
	}
	raw, ok := task.Context["euclo.interaction_script"]
	if !ok || raw == nil {
		return nil
	}
	rows, ok := raw.([]any)
	if !ok {
		if typed, ok := raw.([]map[string]any); ok {
			rows = make([]any, 0, len(typed))
			for _, item := range typed {
				rows = append(rows, item)
			}
		} else {
			return nil
		}
	}
	script := make([]interaction.ScriptedResponse, 0, len(rows))
	for _, row := range rows {
		entry, ok := row.(map[string]any)
		if !ok {
			continue
		}
		action := stringValue(entry["action"])
		if action == "" {
			continue
		}
		script = append(script, interaction.ScriptedResponse{
			Phase:    stringValue(entry["phase"]),
			Kind:     stringValue(entry["kind"]),
			ActionID: action,
			Text:     stringValue(entry["text"]),
		})
	}
	return script
}

func InteractionMaxTransitions(task *core.Task) int {
	if task == nil || task.Context == nil {
		return 5
	}
	raw, ok := task.Context["euclo.max_interactive_transitions"]
	if !ok || raw == nil {
		return 5
	}
	switch typed := raw.(type) {
	case int:
		if typed > 0 {
			return typed
		}
	case int64:
		if typed > 0 {
			return int(typed)
		}
	case float64:
		if typed > 0 {
			return int(typed)
		}
	case string:
		if parsed, err := strconv.Atoi(strings.TrimSpace(typed)); err == nil && parsed > 0 {
			return parsed
		}
	}
	return 5
}

func WorkspaceIDFromTask(task *core.Task, state *core.Context) string {
	if state != nil {
		if value := strings.TrimSpace(state.GetString("workspace")); value != "" {
			return value
		}
		if value := strings.TrimSpace(state.GetString("euclo.workspace")); value != "" {
			return value
		}
	}
	if task != nil && task.Context != nil {
		if value := strings.TrimSpace(stringValue(task.Context["workspace"])); value != "" {
			return value
		}
	}
	return ""
}

func ExtractLearningResolutionPayload(task *core.Task, state *core.Context) (map[string]any, bool) {
	if state != nil {
		if raw, ok := statebus.GetAny(state, "euclo.learning_resolution"); ok {
			if payload, ok := raw.(map[string]any); ok {
				return payload, true
			}
		}
	}
	if task != nil && task.Context != nil {
		if raw, ok := task.Context["euclo.learning_resolution"]; ok {
			if payload, ok := raw.(map[string]any); ok {
				return payload, true
			}
		}
	}
	return nil, false
}

func BuildLearningResolutionInput(workflowID string, raw map[string]any) (archaeolearning.ResolveInput, bool) {
	if workflowID == "" || raw == nil {
		return archaeolearning.ResolveInput{}, false
	}
	input := archaeolearning.ResolveInput{
		WorkflowID:      workflowID,
		InteractionID:   strings.TrimSpace(stringValue(raw["interaction_id"])),
		ExpectedStatus:  archaeolearning.InteractionStatus(strings.TrimSpace(stringValue(raw["expected_status"]))),
		Kind:            archaeolearning.ResolutionKind(strings.TrimSpace(stringValue(raw["resolution_kind"]))),
		ChoiceID:        strings.TrimSpace(stringValue(raw["choice_id"])),
		RefinedPayload:  mapValue(raw["refined_payload"]),
		ResolvedBy:      strings.TrimSpace(stringValue(raw["resolved_by"])),
		BasedOnRevision: strings.TrimSpace(stringValue(raw["based_on_revision"])),
	}
	if comment := commentInputValue(raw["comment"]); comment != nil {
		input.Comment = comment
	}
	if input.InteractionID == "" || input.Kind == "" {
		return archaeolearning.ResolveInput{}, false
	}
	return input, true
}

func BuildSemanticInputBundle(
	workflowID string,
	activePlan *archaeodomain.VersionedLivingPlan,
	requests *archaeoprojections.RequestHistoryProjection,
	provenance *archaeoprojections.ProvenanceProjection,
	learning *archaeoprojections.LearningQueueProjection,
	convergence *archaeodomain.WorkspaceConvergenceProjection,
	state *core.Context,
	modeID string,
) eucloruntime.SemanticInputBundle {
	bundle := eucloarchaeomem.SemanticInputBundleFromSources(
		workflowID,
		activePlan,
		adaptSemanticRequestHistory(requests),
		adaptSemanticProvenance(provenance),
		adaptSemanticLearningQueue(learning),
		convergence,
	)
	if bundle.Source == "" {
		bundle.Source = "archaeo.projections"
	}
	switch modeID {
	case "planning":
		bundle.Source += "+planning_prepass"
	case "debug":
		bundle.Source += "+debug_prepass"
	case "review":
		bundle.Source += "+review_prepass"
	}
	enriched := eucloarchaeomem.EnrichSemanticInputBundle(bundle, state, eucloruntime.UnitOfWork{}, nil)
	return EnrichBundleWithContextKnowledge(enriched, state)
}

func adaptSemanticRequestHistory(history *archaeoprojections.RequestHistoryProjection) *eucloruntime.SemanticRequestHistory {
	if history == nil {
		return nil
	}
	return &eucloruntime.SemanticRequestHistory{
		Requests: append([]archaeodomain.RequestRecord(nil), history.Requests...),
	}
}

func adaptSemanticProvenance(provenance *archaeoprojections.ProvenanceProjection) *eucloruntime.SemanticProvenance {
	if provenance == nil {
		return nil
	}
	out := &eucloruntime.SemanticProvenance{
		ConvergenceRefs: append([]string(nil), provenance.ConvergenceRefs...),
		DecisionRefs:    append([]string(nil), provenance.DecisionRefs...),
	}
	for _, request := range provenance.Requests {
		out.Requests = append(out.Requests, eucloruntime.SemanticRequestProvenanceRef{RequestID: request.RequestID})
	}
	for _, learning := range provenance.Learning {
		out.Learning = append(out.Learning, eucloruntime.SemanticLearningRef{InteractionID: learning.InteractionID})
	}
	for _, tension := range provenance.Tensions {
		out.Tensions = append(out.Tensions, eucloruntime.SemanticTensionRef{
			TensionID:  tension.TensionID,
			PatternIDs: append([]string(nil), tension.PatternIDs...),
			AnchorRefs: append([]string(nil), tension.AnchorRefs...),
		})
	}
	for _, planVersion := range provenance.PlanVersions {
		out.PlanVersions = append(out.PlanVersions, eucloruntime.SemanticPlanVersionRef{
			PatternRefs:             append([]string(nil), planVersion.PatternRefs...),
			TensionRefs:             append([]string(nil), planVersion.TensionRefs...),
			FormationProvenanceRefs: append([]string(nil), planVersion.FormationProvenanceRefs...),
			FormationResultRef:      planVersion.FormationResultRef,
			SemanticSnapshotRef:     planVersion.SemanticSnapshotRef,
		})
	}
	return out
}

func adaptSemanticLearningQueue(queue *archaeoprojections.LearningQueueProjection) *eucloruntime.SemanticLearningQueue {
	if queue == nil {
		return nil
	}
	out := &eucloruntime.SemanticLearningQueue{}
	for _, interaction := range queue.PendingLearning {
		out.PendingLearning = append(out.PendingLearning, eucloruntime.SemanticLearningRef{InteractionID: interaction.ID})
	}
	return out
}

func stringValue(raw any) string {
	if raw == nil {
		return ""
	}
	if typed, ok := raw.(string); ok {
		return strings.TrimSpace(typed)
	}
	return ""
}

func mapValue(raw any) map[string]any {
	if raw == nil {
		return nil
	}
	if typed, ok := raw.(map[string]any); ok {
		return typed
	}
	return nil
}

func commentInputValue(raw any) *archaeolearning.CommentInput {
	payload, ok := raw.(map[string]any)
	if !ok || payload == nil {
		return nil
	}
	return &archaeolearning.CommentInput{
		IntentType:  strings.TrimSpace(stringValue(payload["intent_type"])),
		AuthorKind:  strings.TrimSpace(stringValue(payload["author_kind"])),
		Body:        strings.TrimSpace(stringValue(payload["body"])),
		TrustClass:  strings.TrimSpace(stringValue(payload["trust_class"])),
		CorpusScope: strings.TrimSpace(stringValue(payload["corpus_scope"])),
	}
}
