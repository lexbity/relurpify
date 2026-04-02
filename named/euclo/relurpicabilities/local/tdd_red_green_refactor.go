package local

import (
	"context"
	"strings"

	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/named/euclo/execution"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
)

type tddRedGreenRefactorCapability struct{ env agentenv.AgentEnvironment }

func NewTDDRedGreenRefactorCapability(env agentenv.AgentEnvironment) euclotypes.EucloCodingCapability {
	return &tddRedGreenRefactorCapability{env: env}
}

func (c *tddRedGreenRefactorCapability) Descriptor() core.CapabilityDescriptor {
	return core.CapabilityDescriptor{
		ID:            "euclo:tdd.red_green_refactor",
		Name:          "TDD Red Green Refactor",
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
		Tags:          []string{"coding", "tdd", "verification"},
		Annotations: map[string]any{
			"supported_profiles": []string{"test_driven_generation"},
		},
	}
}

func (c *tddRedGreenRefactorCapability) Contract() euclotypes.ArtifactContract {
	return euclotypes.ArtifactContract{
		RequiredInputs: []euclotypes.ArtifactRequirement{{Kind: euclotypes.ArtifactKindIntake, Required: true}},
		ProducedOutputs: []euclotypes.ArtifactKind{
			euclotypes.ArtifactKindPlan,
			euclotypes.ArtifactKindEditIntent,
			euclotypes.ArtifactKindVerificationPlan,
			euclotypes.ArtifactKindVerification,
			euclotypes.ArtifactKindTDDLifecycle,
		},
	}
}

func (c *tddRedGreenRefactorCapability) Eligible(artifacts euclotypes.ArtifactState, snapshot euclotypes.CapabilitySnapshot) euclotypes.EligibilityResult {
	if !snapshot.HasWriteTools {
		return euclotypes.EligibilityResult{Eligible: false, Reason: "write tools required for TDD execution"}
	}
	if !snapshot.HasExecuteTools {
		return euclotypes.EligibilityResult{Eligible: false, Reason: "execute tools required for TDD execution"}
	}
	if !artifacts.Has(euclotypes.ArtifactKindIntake) {
		return euclotypes.EligibilityResult{Eligible: false, Reason: "TDD execution requires intake", MissingArtifacts: []euclotypes.ArtifactKind{euclotypes.ArtifactKindIntake}}
	}
	return euclotypes.EligibilityResult{Eligible: true, Reason: "TDD execution is available"}
}

func (c *tddRedGreenRefactorCapability) Execute(ctx context.Context, env euclotypes.ExecutionEnvelope) euclotypes.ExecutionResult {
	if env.Environment.Config == nil || env.Environment.Model == nil {
		return tddFailure("tdd_runtime_unavailable", "TDD execution runtime unavailable", "TDD red/green execution requires a configured model runtime", nil)
	}
	requestedRefactor := shouldRunTDDRefactor(env.Task)
	if env.State != nil {
		env.State.Set("euclo.tdd.phase", "plan_tests")
	}
	lifecycle := initializeTDDLifecycle(env.State, requestedRefactor)
	artifacts := []euclotypes.Artifact{{
		ID:      "tdd_plan",
		Kind:    euclotypes.ArtifactKindPlan,
		Summary: "TDD lifecycle planned: tests first, then implementation, then green verification, with optional refactor verification",
		Payload: map[string]any{
			"mode":               "tdd",
			"phases":             []string{"plan_tests", "red", "implement", "green", "refactor"},
			"refactor_requested": requestedRefactor,
		},
		ProducerID: "euclo:tdd.red_green_refactor",
		Status:     "produced",
	}, buildTDDLifecycleArtifact(lifecycle)}
	updateTDDLifecycle(env.State, "plan_tests", "completed", map[string]any{
		"artifact_ids": []string{"tdd_plan"},
	})
	artifacts[1] = buildTDDLifecycleArtifact(tddLifecycleFromState(env.State))
	mergeStateArtifactsToContext(env.State, artifacts)

	if shouldSynthesizeRegressionForTDD(env) {
		synthResult := NewRegressionSynthesizeCapability(c.env).Execute(ctx, env)
		if synthResult.Status != euclotypes.ExecutionStatusCompleted {
			return tddFailure("tdd_regression_synthesis_failed", "TDD regression synthesis failed", firstNonEmpty(synthResult.Summary, "failed to synthesize regression reproducer"), artifacts)
		}
		artifacts = append(artifacts, synthResult.Artifacts...)
		updateTDDLifecycle(env.State, "plan_tests", "completed", map[string]any{
			"artifact_ids": append([]string{"tdd_plan"}, artifactIDs(synthResult.Artifacts)...),
			"summary":      "regression reproducer synthesized before red phase",
		})
		artifacts[1] = buildTDDLifecycleArtifact(tddLifecycleFromState(env.State))
	}

	testTask := &core.Task{
		ID:          firstNonEmpty(taskIdentifier(env.Task), "tdd") + "-tests-first",
		Instruction: tddRedPhaseInstruction(env),
		Type:        core.TaskTypeCodeModification,
		Context:     taskContextFromEnvelope(env),
	}
	testEnv := env
	testEnv.Task = testTask
	testResult, testState, err := execution.ExecuteEnvelopeRecipe(ctx, testEnv, execution.RecipeChatImplementEdit, testTask.ID, testTask.Instruction)
	if err != nil || testResult == nil || !testResult.Success {
		return tddFailure("tdd_red_test_edit_failed", "TDD test-first edit failed", errMsg(err, testResult), artifacts)
	}
	execution.PropagateBehaviorTrace(env.State, testState)
	testEditPayload := firstNonNilMap(testResult.Data, map[string]any{"summary": resultSummary(testResult)})
	testArtifact := euclotypes.Artifact{
		ID:         "tdd_test_edit",
		Kind:       euclotypes.ArtifactKindEditIntent,
		Summary:    firstNonEmpty(resultSummary(testResult), "TDD tests drafted"),
		Payload:    testEditPayload,
		ProducerID: "euclo:tdd.red_green_refactor",
		Status:     "produced",
	}
	artifacts = append(artifacts, testArtifact)
	updateTDDLifecycle(env.State, "plan_tests", "completed", map[string]any{
		"artifact_ids": []string{"tdd_plan", testArtifact.ID},
		"summary":      testArtifact.Summary,
	})
	artifacts[1] = buildTDDLifecycleArtifact(tddLifecycleFromState(env.State))
	mergeStateArtifactsToContext(env.State, []euclotypes.Artifact{testArtifact})

	if env.State != nil {
		env.State.Set("euclo.tdd.phase", "red")
	}
	redArtifacts, executed, redErr := ExecuteVerificationFlow(ctx, env, eucloruntime.SnapshotCapabilities(env.Registry))
	if redErr != nil {
		return tddFailure("tdd_red_verification_failed", "TDD red phase verification failed to execute", redErr.Error(), artifacts)
	}
	artifacts = append(artifacts, redArtifacts...)
	if !executed {
		return tddFailure("tdd_red_verification_missing", "TDD red phase requires executed failing evidence", "no executed verification plan was available", artifacts)
	}
	redPayload, _ := verificationPayloadFromState(env.State)
	if !verificationPayloadFailed(redPayload) {
		return tddFailure("tdd_red_not_failing", "TDD red phase did not produce failing evidence", "tests did not fail before implementation", artifacts)
	}
	redArtifact := buildTDDVerificationArtifact("red", redPayload)
	artifacts = append(artifacts, redArtifact)
	if env.State != nil {
		env.State.Set("euclo.tdd.red_evidence", cloneMapAny(redPayload))
		env.State.Set("euclo.tdd.phase", "implement")
	}
	updateTDDLifecycle(env.State, "red", "completed", map[string]any{
		"status":       stringValue(redPayload["status"]),
		"artifact_ids": []string{redArtifact.ID},
		"check_count":  len(checkRecords(redPayload)),
		"summary":      stringValue(redPayload["summary"]),
	})
	artifacts[1] = buildTDDLifecycleArtifact(tddLifecycleFromState(env.State))
	mergeStateArtifactsToContext(env.State, []euclotypes.Artifact{redArtifact})

	implementTask := &core.Task{
		ID:          firstNonEmpty(taskIdentifier(env.Task), "tdd") + "-implement",
		Instruction: "TDD green phase: implement the production change necessary to satisfy the failing tests. " + taskInstruction(env.Task),
		Type:        core.TaskTypeCodeModification,
		Context:     taskContextFromEnvelope(env),
	}
	implEnv := env
	implEnv.Task = implementTask
	implResult, implState, err := execution.ExecuteEnvelopeRecipe(ctx, implEnv, execution.RecipeChatImplementEdit, implementTask.ID, implementTask.Instruction)
	if err != nil || implResult == nil || !implResult.Success {
		return tddFailure("tdd_green_implementation_failed", "TDD implementation step failed", errMsg(err, implResult), artifacts)
	}
	execution.PropagateBehaviorTrace(env.State, implState)
	implEditPayload := firstNonNilMap(implResult.Data, map[string]any{"summary": resultSummary(implResult)})
	implArtifact := euclotypes.Artifact{
		ID:         "tdd_implementation_edit",
		Kind:       euclotypes.ArtifactKindEditIntent,
		Summary:    firstNonEmpty(resultSummary(implResult), "TDD implementation applied"),
		Payload:    implEditPayload,
		ProducerID: "euclo:tdd.red_green_refactor",
		Status:     "produced",
	}
	artifacts = append(artifacts, implArtifact)
	updateTDDLifecycle(env.State, "implement", "completed", map[string]any{
		"artifact_ids": []string{implArtifact.ID},
		"summary":      implArtifact.Summary,
	})
	artifacts[1] = buildTDDLifecycleArtifact(tddLifecycleFromState(env.State))
	mergeStateArtifactsToContext(env.State, []euclotypes.Artifact{implArtifact})

	if env.State != nil {
		env.State.Set("euclo.tdd.phase", "green")
	}
	greenArtifacts, executed, greenErr := ExecuteVerificationFlow(ctx, env, eucloruntime.SnapshotCapabilities(env.Registry))
	if greenErr != nil {
		return tddFailure("tdd_green_verification_failed", "TDD green phase verification failed to execute", greenErr.Error(), artifacts)
	}
	artifacts = append(artifacts, greenArtifacts...)
	if !executed {
		return tddFailure("tdd_green_verification_missing", "TDD green phase requires executed passing evidence", "no executed verification plan was available", artifacts)
	}
	greenPayload, _ := verificationPayloadFromState(env.State)
	if !verificationPayloadPassed(greenPayload) {
		return tddFailure("tdd_green_not_passing", "TDD green phase did not pass", "tests did not pass after implementation", artifacts)
	}
	greenArtifact := buildTDDVerificationArtifact("green", greenPayload)
	artifacts = append(artifacts, greenArtifact)
	if env.State != nil {
		env.State.Set("euclo.tdd.green_evidence", cloneMapAny(greenPayload))
	}
	updateTDDLifecycle(env.State, "green", "completed", map[string]any{
		"status":       stringValue(greenPayload["status"]),
		"artifact_ids": []string{greenArtifact.ID},
		"check_count":  len(checkRecords(greenPayload)),
		"summary":      stringValue(greenPayload["summary"]),
	})
	artifacts[1] = buildTDDLifecycleArtifact(tddLifecycleFromState(env.State))
	mergeStateArtifactsToContext(env.State, []euclotypes.Artifact{greenArtifact})

	if requestedRefactor {
		if env.State != nil {
			env.State.Set("euclo.tdd.phase", "refactor")
		}
		refactorTask := &core.Task{
			ID:          firstNonEmpty(taskIdentifier(env.Task), "tdd") + "-refactor",
			Instruction: "TDD refactor phase: improve the implementation structure while preserving the now-green behavior and keeping tests green. " + taskInstruction(env.Task),
			Type:        core.TaskTypeCodeModification,
			Context:     taskContextFromEnvelope(env),
		}
		refactorEnv := env
		refactorEnv.Task = refactorTask
		refactorResult, refactorState, err := execution.ExecuteEnvelopeRecipe(ctx, refactorEnv, execution.RecipeChatImplementEdit, refactorTask.ID, refactorTask.Instruction)
		if err != nil || refactorResult == nil || !refactorResult.Success {
			return tddFailure("tdd_refactor_edit_failed", "TDD refactor step failed", errMsg(err, refactorResult), artifacts)
		}
		execution.PropagateBehaviorTrace(env.State, refactorState)
		refactorPayload := firstNonNilMap(refactorResult.Data, map[string]any{"summary": resultSummary(refactorResult)})
		refactorArtifact := euclotypes.Artifact{
			ID:         "tdd_refactor_edit",
			Kind:       euclotypes.ArtifactKindEditIntent,
			Summary:    firstNonEmpty(resultSummary(refactorResult), "TDD refactor applied"),
			Payload:    refactorPayload,
			ProducerID: "euclo:tdd.red_green_refactor",
			Status:     "produced",
		}
		artifacts = append(artifacts, refactorArtifact)
		updateTDDLifecycle(env.State, "refactor", "in_progress", map[string]any{
			"artifact_ids": []string{refactorArtifact.ID},
			"summary":      refactorArtifact.Summary,
		})
		artifacts[1] = buildTDDLifecycleArtifact(tddLifecycleFromState(env.State))
		mergeStateArtifactsToContext(env.State, []euclotypes.Artifact{refactorArtifact})

		refactorVerifyArtifacts, refactorExecuted, refactorErr := ExecuteVerificationFlow(ctx, env, eucloruntime.SnapshotCapabilities(env.Registry))
		if refactorErr != nil {
			return tddFailure("tdd_refactor_verification_failed", "TDD refactor verification failed to execute", refactorErr.Error(), artifacts)
		}
		artifacts = append(artifacts, refactorVerifyArtifacts...)
		if !refactorExecuted {
			return tddFailure("tdd_refactor_verification_missing", "TDD refactor requires executed passing evidence", "no executed verification plan was available", artifacts)
		}
		refactorVerifyPayload, _ := verificationPayloadFromState(env.State)
		if !verificationPayloadPassed(refactorVerifyPayload) {
			return tddFailure("tdd_refactor_not_passing", "TDD refactor verification did not stay green", "tests did not remain green after refactor", artifacts)
		}
		refactorEvidenceArtifact := buildTDDVerificationArtifact("refactor", refactorVerifyPayload)
		artifacts = append(artifacts, refactorEvidenceArtifact)
		if env.State != nil {
			env.State.Set("euclo.tdd.refactor_evidence", cloneMapAny(refactorVerifyPayload))
		}
		updateTDDLifecycle(env.State, "refactor", "completed", map[string]any{
			"status":       stringValue(refactorVerifyPayload["status"]),
			"artifact_ids": []string{refactorArtifact.ID, refactorEvidenceArtifact.ID},
			"check_count":  len(checkRecords(refactorVerifyPayload)),
			"summary":      stringValue(refactorVerifyPayload["summary"]),
		})
		artifacts[1] = buildTDDLifecycleArtifact(tddLifecycleFromState(env.State))
		mergeStateArtifactsToContext(env.State, []euclotypes.Artifact{refactorEvidenceArtifact})
	}
	if env.State != nil {
		env.State.Set("euclo.tdd.phase", "complete")
	}
	updateTDDLifecycle(env.State, "complete", "completed", map[string]any{
		"summary": "TDD lifecycle completed",
	})
	artifacts[1] = buildTDDLifecycleArtifact(tddLifecycleFromState(env.State))
	mergeStateArtifactsToContext(env.State, artifacts)
	return euclotypes.ExecutionResult{Status: euclotypes.ExecutionStatusCompleted, Summary: tddCompletionSummary(requestedRefactor), Artifacts: artifacts}
}

func tddFailure(code, summary, message string, artifacts []euclotypes.Artifact) euclotypes.ExecutionResult {
	return euclotypes.ExecutionResult{
		Status:    euclotypes.ExecutionStatusFailed,
		Summary:   summary,
		Artifacts: artifacts,
		FailureInfo: &euclotypes.CapabilityFailure{
			Code:         code,
			Message:      strings.TrimSpace(message),
			Recoverable:  true,
			FailedPhase:  "tdd",
			ParadigmUsed: "react",
		},
	}
}

func cloneMapAny(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func firstNonNilMap(value map[string]any, fallback map[string]any) map[string]any {
	if value != nil {
		return value
	}
	return fallback
}

func taskIdentifier(task *core.Task) string {
	if task == nil {
		return ""
	}
	return strings.TrimSpace(task.ID)
}

func shouldRunTDDRefactor(task *core.Task) bool {
	text := strings.ToLower(taskInstruction(task))
	if strings.Contains(text, "refactor") {
		return true
	}
	if task != nil && task.Context != nil {
		if value, ok := task.Context["tdd_refactor_requested"]; ok {
			switch typed := value.(type) {
			case bool:
				return typed
			case string:
				return strings.EqualFold(strings.TrimSpace(typed), "true") || strings.EqualFold(strings.TrimSpace(typed), "yes")
			}
		}
	}
	return false
}

func initializeTDDLifecycle(state *core.Context, requestedRefactor bool) map[string]any {
	lifecycle := map[string]any{
		"current_phase":       "plan_tests",
		"status":              "in_progress",
		"requested_refactor":  requestedRefactor,
		"completed_refactor":  false,
		"phase_history":       []map[string]any{},
		"required_phases":     []string{"plan_tests", "red", "implement", "green"},
		"verification_phases": []string{"red", "green"},
	}
	if requestedRefactor {
		lifecycle["required_phases"] = []string{"plan_tests", "red", "implement", "green", "refactor"}
		lifecycle["verification_phases"] = []string{"red", "green", "refactor"}
	}
	if state != nil {
		state.Set("euclo.tdd.lifecycle", lifecycle)
	}
	return lifecycle
}

func updateTDDLifecycle(state *core.Context, phase, status string, details map[string]any) {
	if state == nil {
		return
	}
	lifecycle := tddLifecycleFromState(state)
	if lifecycle == nil {
		lifecycle = initializeTDDLifecycle(state, false)
	}
	entry := map[string]any{
		"phase":  strings.TrimSpace(phase),
		"status": strings.TrimSpace(status),
		"run_id": strings.TrimSpace(state.GetString("euclo.run_id")),
	}
	for key, value := range details {
		entry[key] = value
	}
	history := phaseHistory(lifecycle["phase_history"])
	history = append(history, entry)
	lifecycle["phase_history"] = history
	lifecycle["current_phase"] = strings.TrimSpace(phase)
	lifecycle["status"] = strings.TrimSpace(status)
	if phase == "refactor" && status == "completed" {
		lifecycle["completed_refactor"] = true
	}
	if phase == "complete" {
		lifecycle["status"] = "completed"
	}
	state.Set("euclo.tdd.lifecycle", lifecycle)
}

func tddLifecycleFromState(state *core.Context) map[string]any {
	if state == nil {
		return nil
	}
	raw, ok := state.Get("euclo.tdd.lifecycle")
	if !ok || raw == nil {
		return nil
	}
	payload, _ := raw.(map[string]any)
	return payload
}

func buildTDDLifecycleArtifact(payload map[string]any) euclotypes.Artifact {
	return euclotypes.Artifact{
		ID:         "tdd_lifecycle",
		Kind:       euclotypes.ArtifactKindTDDLifecycle,
		Summary:    tddLifecycleSummary(payload),
		Payload:    cloneMapAny(payload),
		ProducerID: "euclo:tdd.red_green_refactor",
		Status:     "produced",
	}
}

func buildTDDVerificationArtifact(phase string, payload map[string]any) euclotypes.Artifact {
	phase = strings.TrimSpace(strings.ToLower(phase))
	cloned := cloneMapAny(payload)
	if cloned == nil {
		cloned = map[string]any{}
	}
	cloned["tdd_phase"] = phase
	return euclotypes.Artifact{
		ID:           "tdd_" + phase + "_evidence",
		Kind:         euclotypes.ArtifactKindVerification,
		Summary:      "TDD " + phase + " evidence: " + firstNonEmpty(stringValue(cloned["summary"]), stringValue(cloned["status"]), phase),
		Payload:      cloned,
		Metadata:     map[string]any{"tdd_phase": phase, "tdd_evidence": true},
		ProducerID:   "euclo:tdd.red_green_refactor",
		Status:       "produced",
		EvidenceRefs: verificationEvidenceRefs(cloned),
	}
}

func verificationEvidenceRefs(payload map[string]any) []string {
	refs := make([]string, 0, len(checkRecords(payload)))
	for _, record := range checkRecords(payload) {
		name := strings.TrimSpace(stringValue(record["name"]))
		command := strings.TrimSpace(stringValue(record["command"]))
		if name == "" && command == "" {
			continue
		}
		refs = append(refs, firstNonEmpty(name, command))
	}
	return uniqueStrings(refs)
}

func checkRecords(payload map[string]any) []map[string]any {
	raw := payload["checks"]
	switch typed := raw.(type) {
	case []map[string]any:
		return append([]map[string]any{}, typed...)
	case []any:
		out := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if record, ok := item.(map[string]any); ok {
				out = append(out, record)
			}
		}
		return out
	default:
		return nil
	}
}

func phaseHistory(raw any) []map[string]any {
	switch typed := raw.(type) {
	case []map[string]any:
		return append([]map[string]any{}, typed...)
	case []any:
		out := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if entry, ok := item.(map[string]any); ok {
				out = append(out, entry)
			}
		}
		return out
	default:
		return nil
	}
}

func tddLifecycleSummary(payload map[string]any) string {
	if payload == nil {
		return "TDD lifecycle tracked"
	}
	current := firstNonEmpty(stringValue(payload["current_phase"]), stringValue(payload["status"]))
	if current == "" {
		current = "tracked"
	}
	return "TDD lifecycle " + current
}

func tddCompletionSummary(refactored bool) string {
	if refactored {
		return "TDD red/green/refactor execution completed"
	}
	return "TDD red/green execution completed"
}

func shouldSynthesizeRegressionForTDD(env euclotypes.ExecutionEnvelope) bool {
	if !looksLikeBugfixOrRegressionRequest(taskInstruction(env.Task)) {
		return false
	}
	if state := env.State; state != nil {
		if payload := mapPayloadFromState(state, "euclo.reproduction"); len(payload) > 0 && !valueTruthy(payload["synthesized"]) {
			return false
		}
	}
	return true
}

func tddRedPhaseInstruction(env euclotypes.ExecutionEnvelope) string {
	base := "TDD red phase: write or update failing tests first for this request without implementing the production fix yet. " + taskInstruction(env.Task)
	reproducer := mapPayloadFromState(env.State, "euclo.reproduction")
	if len(reproducer) == 0 {
		return base
	}
	return base + " Reproducer target: " + tddReproducerPrompt(reproducer)
}

func tddReproducerPrompt(reproducer map[string]any) string {
	parts := []string{}
	if symptom := strings.TrimSpace(stringValue(reproducer["symptom"])); symptom != "" {
		parts = append(parts, "symptom="+symptom)
	}
	if failure := strings.TrimSpace(stringValue(reproducer["expected_failure"])); failure != "" {
		parts = append(parts, "expected_failure="+failure)
	}
	if suggested := strings.TrimSpace(stringValue(reproducer["suggested_test_name"])); suggested != "" {
		parts = append(parts, "suggested_test_name="+suggested)
	}
	if criteria := stringSliceFromAny(reproducer["acceptance_criteria"]); len(criteria) > 0 {
		parts = append(parts, "acceptance_criteria="+strings.Join(criteria, "; "))
	}
	if len(parts) == 0 {
		return "capture the bug symptom as a failing regression test"
	}
	return strings.Join(parts, " | ")
}

func artifactIDs(artifacts []euclotypes.Artifact) []string {
	out := make([]string, 0, len(artifacts))
	for _, artifact := range artifacts {
		if id := strings.TrimSpace(artifact.ID); id != "" {
			out = append(out, id)
		}
	}
	return out
}

func valueTruthy(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		trimmed := strings.TrimSpace(strings.ToLower(typed))
		return trimmed == "true" || trimmed == "yes" || trimmed == "1"
	default:
		return false
	}
}

func stringSliceFromAny(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			text := strings.TrimSpace(stringValue(item))
			if text != "" {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}
