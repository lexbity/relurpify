package local

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/named/euclo/execution"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
)

type reviewFindingsCapability struct{ env agentenv.AgentEnvironment }
type reviewSemanticCapability struct{ env agentenv.AgentEnvironment }
type reviewCompatibilityCapability struct{ env agentenv.AgentEnvironment }
type reviewImplementIfSafeCapability struct{ env agentenv.AgentEnvironment }

func NewReviewFindingsCapability(env agentenv.AgentEnvironment) euclotypes.EucloCodingCapability {
	return &reviewFindingsCapability{env: env}
}

func NewReviewSemanticCapability(env agentenv.AgentEnvironment) euclotypes.EucloCodingCapability {
	return &reviewSemanticCapability{env: env}
}

func NewReviewCompatibilityCapability(env agentenv.AgentEnvironment) euclotypes.EucloCodingCapability {
	return &reviewCompatibilityCapability{env: env}
}

func NewReviewImplementIfSafeCapability(env agentenv.AgentEnvironment) euclotypes.EucloCodingCapability {
	return &reviewImplementIfSafeCapability{env: env}
}

func (c *reviewFindingsCapability) Descriptor() core.CapabilityDescriptor {
	return core.CapabilityDescriptor{
		ID:            "euclo:review.findings",
		Name:          "Review Findings",
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
		Tags:          []string{"coding", "review"},
		Annotations:   map[string]any{"supported_profiles": []string{"review_suggest_implement"}},
	}
}

func (c *reviewSemanticCapability) Descriptor() core.CapabilityDescriptor {
	return core.CapabilityDescriptor{
		ID:            "euclo:review.semantic",
		Name:          "Semantic Review",
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
		Tags:          []string{"coding", "review", "semantic"},
		Annotations:   map[string]any{"supported_profiles": []string{"review_suggest_implement", "edit_verify_repair", "test_driven_generation", "reproduce_localize_patch"}},
	}
}

func (c *reviewFindingsCapability) Contract() euclotypes.ArtifactContract {
	return euclotypes.ArtifactContract{
		RequiredInputs: []euclotypes.ArtifactRequirement{{Kind: euclotypes.ArtifactKindIntake, Required: true}},
		ProducedOutputs: []euclotypes.ArtifactKind{
			euclotypes.ArtifactKindReviewFindings,
		},
	}
}

func (c *reviewSemanticCapability) Contract() euclotypes.ArtifactContract {
	return euclotypes.ArtifactContract{
		RequiredInputs: []euclotypes.ArtifactRequirement{{Kind: euclotypes.ArtifactKindIntake, Required: true}},
		ProducedOutputs: []euclotypes.ArtifactKind{
			euclotypes.ArtifactKindReviewFindings,
			euclotypes.ArtifactKindCompatibilityAssessment,
		},
	}
}

func (c *reviewFindingsCapability) Eligible(artifacts euclotypes.ArtifactState, snapshot euclotypes.CapabilitySnapshot) euclotypes.EligibilityResult {
	if !snapshot.HasReadTools {
		return euclotypes.EligibilityResult{Eligible: false, Reason: "read tools required for review findings"}
	}
	if !looksLikeReviewRequest(artifacts) {
		return euclotypes.EligibilityResult{Eligible: false, Reason: "review findings requires review-like intake"}
	}
	return euclotypes.EligibilityResult{Eligible: true, Reason: "review-like intake with read tools"}
}

func (c *reviewSemanticCapability) Eligible(artifacts euclotypes.ArtifactState, snapshot euclotypes.CapabilitySnapshot) euclotypes.EligibilityResult {
	if !snapshot.HasReadTools {
		return euclotypes.EligibilityResult{Eligible: false, Reason: "read tools required for semantic review"}
	}
	if !looksLikeReviewRequest(artifacts) && !looksLikeReviewFixRequest(artifacts) && !looksLikeCompatibilityRequest(artifacts) {
		return euclotypes.EligibilityResult{Eligible: false, Reason: "semantic review requires review, compatibility, or review-fix intent"}
	}
	return euclotypes.EligibilityResult{Eligible: true, Reason: "semantic review can assess the current change surface"}
}

func (c *reviewFindingsCapability) Execute(_ context.Context, env euclotypes.ExecutionEnvelope) euclotypes.ExecutionResult {
	payload := buildReviewFindingsPayload(env)
	artifact := euclotypes.Artifact{ID: "review_findings", Kind: euclotypes.ArtifactKindReviewFindings, Summary: summarizePayload(payload), Payload: payload, ProducerID: "euclo:review.findings", Status: "produced"}
	mergeStateArtifactsToContext(env.State, []euclotypes.Artifact{artifact})
	return euclotypes.ExecutionResult{Status: euclotypes.ExecutionStatusCompleted, Summary: "review findings produced", Artifacts: []euclotypes.Artifact{artifact}}
}

func (c *reviewSemanticCapability) Execute(_ context.Context, env euclotypes.ExecutionEnvelope) euclotypes.ExecutionResult {
	reviewPayload := buildSemanticReviewPayload(env)
	compatPayload := semanticCompatibilityAssessment(reviewPayload)
	artifacts := []euclotypes.Artifact{
		{ID: "review_semantic", Kind: euclotypes.ArtifactKindReviewFindings, Summary: summarizePayload(reviewPayload), Payload: reviewPayload, ProducerID: "euclo:review.semantic", Status: "produced"},
		{ID: "review_semantic_compatibility", Kind: euclotypes.ArtifactKindCompatibilityAssessment, Summary: summarizePayload(compatPayload), Payload: compatPayload, ProducerID: "euclo:review.semantic", Status: "produced"},
	}
	mergeStateArtifactsToContext(env.State, artifacts)
	return euclotypes.ExecutionResult{Status: euclotypes.ExecutionStatusCompleted, Summary: "semantic review completed", Artifacts: artifacts}
}

func (c *reviewCompatibilityCapability) Descriptor() core.CapabilityDescriptor {
	return core.CapabilityDescriptor{
		ID:            "euclo:review.compatibility",
		Name:          "Review Compatibility",
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
		Tags:          []string{"coding", "review", "compatibility"},
		Annotations:   map[string]any{"supported_profiles": []string{"review_suggest_implement", "edit_verify_repair", "plan_stage_execute"}},
	}
}

func (c *reviewCompatibilityCapability) Contract() euclotypes.ArtifactContract {
	return euclotypes.ArtifactContract{
		RequiredInputs: []euclotypes.ArtifactRequirement{{Kind: euclotypes.ArtifactKindIntake, Required: true}},
		ProducedOutputs: []euclotypes.ArtifactKind{
			euclotypes.ArtifactKindCompatibilityAssessment,
		},
	}
}

func (c *reviewCompatibilityCapability) Eligible(artifacts euclotypes.ArtifactState, snapshot euclotypes.CapabilitySnapshot) euclotypes.EligibilityResult {
	if !snapshot.HasReadTools {
		return euclotypes.EligibilityResult{Eligible: false, Reason: "read tools required for compatibility review"}
	}
	if !looksLikeCompatibilityRequest(artifacts) {
		return euclotypes.EligibilityResult{Eligible: false, Reason: "compatibility capability requires explicit compatibility intent"}
	}
	return euclotypes.EligibilityResult{Eligible: true, Reason: "compatibility review requested"}
}

func (c *reviewCompatibilityCapability) Execute(_ context.Context, env euclotypes.ExecutionEnvelope) euclotypes.ExecutionResult {
	payload := semanticCompatibilityAssessment(buildSemanticReviewPayload(env))
	artifact := euclotypes.Artifact{ID: "compatibility_assessment", Kind: euclotypes.ArtifactKindCompatibilityAssessment, Summary: summarizePayload(payload), Payload: payload, ProducerID: "euclo:review.compatibility", Status: "produced"}
	mergeStateArtifactsToContext(env.State, []euclotypes.Artifact{artifact})
	return euclotypes.ExecutionResult{Status: euclotypes.ExecutionStatusCompleted, Summary: "compatibility assessment produced", Artifacts: []euclotypes.Artifact{artifact}}
}

func (c *reviewImplementIfSafeCapability) Descriptor() core.CapabilityDescriptor {
	return core.CapabilityDescriptor{
		ID:            "euclo:review.implement_if_safe",
		Name:          "Review Implement If Safe",
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
		Tags:          []string{"coding", "review", "implementation"},
		Annotations:   map[string]any{"supported_profiles": []string{"review_suggest_implement"}},
	}
}

func (c *reviewImplementIfSafeCapability) Contract() euclotypes.ArtifactContract {
	return euclotypes.ArtifactContract{
		RequiredInputs: []euclotypes.ArtifactRequirement{{Kind: euclotypes.ArtifactKindIntake, Required: true}},
		ProducedOutputs: []euclotypes.ArtifactKind{
			euclotypes.ArtifactKindReviewFindings,
			euclotypes.ArtifactKindEditIntent,
			euclotypes.ArtifactKindVerification,
		},
	}
}

func (c *reviewImplementIfSafeCapability) Eligible(artifacts euclotypes.ArtifactState, snapshot euclotypes.CapabilitySnapshot) euclotypes.EligibilityResult {
	if !snapshot.HasWriteTools {
		return euclotypes.EligibilityResult{Eligible: false, Reason: "write tools required for review implement-if-safe"}
	}
	if !looksLikeReviewFixRequest(artifacts) {
		return euclotypes.EligibilityResult{Eligible: false, Reason: "implement-if-safe requires explicit fix request"}
	}
	return euclotypes.EligibilityResult{Eligible: true, Reason: "review fix request with write tools"}
}

func (c *reviewImplementIfSafeCapability) Execute(ctx context.Context, env euclotypes.ExecutionEnvelope) euclotypes.ExecutionResult {
	reviewResult := (&reviewSemanticCapability{env: c.env}).Execute(ctx, env)
	artifacts := append([]euclotypes.Artifact{}, reviewResult.Artifacts...)
	payload, _ := reviewResult.Artifacts[0].Payload.(map[string]any)
	stats, _ := payload["stats"].(map[string]any)
	approvalDecision, _ := payload["approval_decision"].(map[string]any)
	if intValue(stats["critical_count"]) > 0 || strings.EqualFold(stringValue(approvalDecision["status"]), "blocked") {
		return euclotypes.ExecutionResult{
			Status:    euclotypes.ExecutionStatusCompleted,
			Summary:   "manual review required; semantic review blocked automatic mutation",
			Artifacts: artifacts,
			RecoveryHint: &euclotypes.RecoveryHint{
				Strategy: euclotypes.RecoveryStrategyModeEscalation,
				Context:  map[string]any{"manual_review_required": true, "approval_status": stringValue(approvalDecision["status"])},
			},
		}
	}
	if intValue(stats["warning_count"]) == 0 && intValue(stats["info_count"]) > 0 {
		return euclotypes.ExecutionResult{Status: euclotypes.ExecutionStatusCompleted, Summary: "info-only findings; no automatic implementation needed", Artifacts: artifacts}
	}
	task := &core.Task{
		ID:          firstNonEmpty(env.Task.ID, "review-implement") + "-implement-if-safe",
		Instruction: buildFixInstruction(payload),
		Type:        core.TaskTypeCodeModification,
		Context:     env.Task.Context,
	}
	stepEnv := env
	stepEnv.Task = task
	result, state, err := execution.ExecuteEnvelopeRecipe(ctx, stepEnv, execution.RecipeChatImplementEdit, task.ID, task.Instruction)
	if err != nil || result == nil || !result.Success {
		return euclotypes.ExecutionResult{
			Status:    euclotypes.ExecutionStatusPartial,
			Summary:   "review findings could not be implemented automatically",
			Artifacts: artifacts,
			FailureInfo: &euclotypes.CapabilityFailure{
				Code:         "review_implement_if_safe_failed",
				Message:      errMsg(err, result),
				Recoverable:  true,
				FailedPhase:  "implement",
				ParadigmUsed: "react",
			},
		}
	}
	execution.PropagateBehaviorTrace(env.State, state)
	editPayload := result.Data
	editArtifact := euclotypes.Artifact{ID: "review_safe_edit", Kind: euclotypes.ArtifactKindEditIntent, Summary: resultSummary(result), Payload: editPayload, ProducerID: "euclo:review.implement_if_safe", Status: "produced"}
	artifacts = append(artifacts, editArtifact)
	mergeStateArtifactsToContext(env.State, []euclotypes.Artifact{editArtifact})
	if verificationArtifacts, executed, execErr := ExecuteVerificationFlow(ctx, env, eucloruntime.SnapshotCapabilities(env.Registry)); execErr != nil {
		return euclotypes.ExecutionResult{
			Status:    euclotypes.ExecutionStatusFailed,
			Summary:   "review findings implementation failed verification",
			Artifacts: artifacts,
			FailureInfo: &euclotypes.CapabilityFailure{
				Code:         "review_implement_if_safe_verification_failed",
				Message:      execErr.Error(),
				Recoverable:  true,
				FailedPhase:  "verify",
				ParadigmUsed: "react",
			},
		}
	} else if executed {
		artifacts = append(artifacts, verificationArtifacts...)
		if verifyPayload, ok := verificationPayloadFromState(env.State); ok && verificationPayloadFailed(verifyPayload) {
			repairResult := NewFailedVerificationRepairCapability(c.env).Execute(ctx, env)
			artifacts = append(artifacts, repairResult.Artifacts...)
			if repairResult.Status == euclotypes.ExecutionStatusFailed {
				return repairResult
			}
			mergeStateArtifactsToContext(env.State, artifacts)
			return euclotypes.ExecutionResult{Status: euclotypes.ExecutionStatusCompleted, Summary: "review findings implemented and verification repaired", Artifacts: artifacts}
		}
	} else if existing, ok := state.Get("pipeline.verify"); ok && existing != nil {
		if verifyPayload, ok := existing.(map[string]any); ok {
			verifyArtifact := euclotypes.Artifact{ID: "review_safe_verification", Kind: euclotypes.ArtifactKindVerification, Summary: summarizePayload(verifyPayload), Payload: verifyPayload, ProducerID: "euclo:review.implement_if_safe", Status: "produced"}
			artifacts = append(artifacts, verifyArtifact)
			if verificationPayloadFailed(verifyPayload) {
				repairResult := NewFailedVerificationRepairCapability(c.env).Execute(ctx, env)
				artifacts = append(artifacts, repairResult.Artifacts...)
				if repairResult.Status == euclotypes.ExecutionStatusFailed {
					return repairResult
				}
				mergeStateArtifactsToContext(env.State, artifacts)
				return euclotypes.ExecutionResult{Status: euclotypes.ExecutionStatusCompleted, Summary: "review findings implemented and verification repaired", Artifacts: artifacts}
			}
		}
	}
	mergeStateArtifactsToContext(env.State, artifacts)
	return euclotypes.ExecutionResult{Status: euclotypes.ExecutionStatusCompleted, Summary: "review findings implemented where safe", Artifacts: artifacts}
}

func looksLikeReviewRequest(artifacts euclotypes.ArtifactState) bool {
	text := strings.ToLower(strings.TrimSpace(instructionFromArtifacts(artifacts)))
	for _, token := range []string{"review", "audit", "inspect", "findings"} {
		if strings.Contains(text, token) {
			return true
		}
	}
	return false
}

func looksLikeCompatibilityRequest(artifacts euclotypes.ArtifactState) bool {
	text := strings.ToLower(strings.TrimSpace(instructionFromArtifacts(artifacts)))
	for _, token := range []string{"compatibility", "breaking change", "backward compatible", "api surface"} {
		if strings.Contains(text, token) {
			return true
		}
	}
	return false
}

func looksLikeReviewFixRequest(artifacts euclotypes.ArtifactState) bool {
	text := strings.ToLower(strings.TrimSpace(instructionFromArtifacts(artifacts)))
	for _, token := range []string{"fix all critical", "fix findings", "implement if safe", "fix all warnings"} {
		if strings.Contains(text, token) {
			return true
		}
	}
	return false
}

func buildReviewFindingsPayload(env euclotypes.ExecutionEnvelope) map[string]any {
	files := reviewScopeFiles(env)
	findings := deriveReviewFindings(files)
	stats := map[string]any{
		"critical_count": countFindingsBySeverity(findings, "critical"),
		"warning_count":  countFindingsBySeverity(findings, "warning"),
		"info_count":     countFindingsBySeverity(findings, "info"),
	}
	return map[string]any{
		"scope": map[string]any{
			"files":        fileNames(files),
			"focus_lens":   reviewFocusLens(env),
			"change_range": "workspace",
		},
		"review_source": "euclo:review.findings",
		"findings":      findings,
		"summary":       reviewFindingsSummary(findings),
		"stats":         stats,
	}
}

func buildSemanticReviewPayload(env euclotypes.ExecutionEnvelope) map[string]any {
	files := reviewScopeFiles(env)
	before := extractAPISurfaceWithEnv(env.Environment, taskInstruction(env.Task), files)
	after := deriveAfterSurface(env, before)
	changeSummary := semanticChangeSummary(env)
	acceptance := semanticAcceptanceCriteria(env)
	verification := semanticVerificationContext(env)
	findings := deriveSemanticReviewFindings(env, files, before, after, acceptance, verification)
	stats := map[string]any{
		"critical_count": countFindingsBySeverity(findings, "critical"),
		"warning_count":  countFindingsBySeverity(findings, "warning"),
		"info_count":     countFindingsBySeverity(findings, "info"),
	}
	compatibility, _ := assessCompatibilityChanges(before, after)
	approval := semanticApprovalDecision(findings, verification, env)
	return map[string]any{
		"scope": map[string]any{
			"files":                fileNames(files),
			"focus_lens":           reviewFocusLens(env),
			"change_range":         semanticChangeRange(env, files),
			"public_surface":       before,
			"after_public_surface": after,
		},
		"review_source":              "euclo:review.semantic",
		"change_summary":             changeSummary,
		"acceptance_criteria":        acceptance,
		"verification":               verification,
		"findings":                   findings,
		"summary":                    semanticReviewSummary(findings, approval),
		"stats":                      stats,
		"approval_decision":          approval,
		"compatibility_risk_summary": semanticCompatibilitySummary(compatibility),
		"compatibility_changes":      compatibility,
		"confidence":                 semanticReviewConfidence(files, acceptance, verification),
	}
}

type reviewFile struct {
	Path    string
	Content string
}

func reviewScopeFiles(env euclotypes.ExecutionEnvelope) []reviewFile {
	var files []reviewFile
	if env.Task != nil && env.Task.Context != nil {
		switch raw := env.Task.Context["context_file_contents"].(type) {
		case []map[string]any:
			for _, item := range raw {
				files = append(files, reviewFile{Path: stringValue(item["path"]), Content: stringValue(item["content"])})
			}
		case []any:
			for _, item := range raw {
				if record, ok := item.(map[string]any); ok {
					files = append(files, reviewFile{Path: stringValue(record["path"]), Content: stringValue(record["content"])})
				}
			}
		}
	}
	if len(files) == 0 {
		for _, artifact := range euclotypes.ArtifactStateFromContext(env.State).OfKind(euclotypes.ArtifactKindExplore) {
			record, ok := artifact.Payload.(map[string]any)
			if !ok {
				continue
			}
			if path := stringValue(record["path"]); path != "" {
				files = append(files, reviewFile{Path: path, Content: stringValue(record["content"])})
			}
		}
	}
	return files
}

func deriveReviewFindings(files []reviewFile) []map[string]any {
	var findings []map[string]any
	for _, file := range files {
		lines := strings.Split(file.Content, "\n")
		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			switch {
			case strings.Contains(trimmed, "TODO") || strings.Contains(trimmed, "FIXME"):
				findings = append(findings, reviewFinding("warning", fmt.Sprintf("%s:%d", file.Path, i+1), "leftover TODO/FIXME marker", "resolve or remove the pending marker", 0.8, "maintainability", file.Path, nil, "line_scan"))
			case strings.Contains(trimmed, "panic("):
				findings = append(findings, reviewFinding("critical", fmt.Sprintf("%s:%d", file.Path, i+1), "panic in production code path", "return an error or handle the failure explicitly", 0.92, "correctness", file.Path, nil, "line_scan"))
			case strings.Contains(trimmed, "fmt.Println("):
				findings = append(findings, reviewFinding("info", fmt.Sprintf("%s:%d", file.Path, i+1), "debug print left in code", "replace with structured logging or remove", 0.72, "style", file.Path, nil, "line_scan"))
			}
		}
	}
	if len(findings) == 0 {
		findings = append(findings, reviewFinding("info", "", "no obvious review issues found in provided scope", "", 0.6, "general", "", nil, "workspace_scan"))
	}
	return findings
}

func deriveSemanticReviewFindings(env euclotypes.ExecutionEnvelope, files []reviewFile, before, after map[string]any, acceptance []string, verification map[string]any) []map[string]any {
	findings := []map[string]any{}
	for _, finding := range deriveReviewFindings(files) {
		finding["review_source"] = "euclo:review.semantic"
		traceability, _ := finding["traceability"].(map[string]any)
		if traceability == nil {
			traceability = map[string]any{}
		}
		traceability["source"] = "semantic_local_scan"
		finding["traceability"] = traceability
		findings = append(findings, finding)
	}
	compatibilityChanges, _ := assessCompatibilityChanges(before, after)
	for _, change := range compatibilityChanges {
		if stringValue(change["classification"]) != "breaking" {
			continue
		}
		findings = append(findings, reviewFinding("critical", stringValue(change["location"]), "public API surface change is breaking", stringValue(change["mitigation"]), 0.92, "compatibility", stringValue(change["location"]), nil, "semantic_api_surface"))
	}
	if verificationGapFinding(env, verification) != nil {
		findings = append(findings, verificationGapFinding(env, verification))
	}
	if acceptanceGapFinding(acceptance, verification) != nil {
		findings = append(findings, acceptanceGapFinding(acceptance, verification))
	}
	if len(findings) == 0 {
		findings = append(findings, map[string]any{
			"severity":         "info",
			"location":         "",
			"description":      "semantic review found no material correctness or compatibility issues in the current change surface",
			"suggestion":       "",
			"confidence":       0.7,
			"category":         "general",
			"rationale":        "review considered changed files, acceptance criteria, and available verification evidence",
			"impacted_files":   fileNames(files),
			"impacted_symbols": []string{},
			"review_source":    "euclo:review.semantic",
			"traceability": map[string]any{
				"source": "semantic_scope_review",
			},
		})
	}
	return findings
}

func reviewFinding(severity, location, description, suggestion string, confidence float64, category, filePath string, impactedSymbols []string, traceabilitySource string) map[string]any {
	impactedFiles := []string{}
	if strings.TrimSpace(filePath) != "" {
		impactedFiles = []string{strings.TrimSpace(filePath)}
	}
	return map[string]any{
		"severity":         severity,
		"location":         location,
		"description":      description,
		"suggestion":       suggestion,
		"confidence":       confidence,
		"category":         category,
		"rationale":        description,
		"impacted_files":   impactedFiles,
		"impacted_symbols": append([]string(nil), impactedSymbols...),
		"review_source":    "euclo:review.findings",
		"traceability": map[string]any{
			"source":   traceabilitySource,
			"location": location,
		},
	}
}

func countFindingsBySeverity(findings []map[string]any, severity string) int {
	count := 0
	for _, finding := range findings {
		if stringValue(finding["severity"]) == severity {
			count++
		}
	}
	return count
}

func semanticApprovalDecision(findings []map[string]any, verification map[string]any, env euclotypes.ExecutionEnvelope) map[string]any {
	status := "approved"
	reasons := []string{}
	if countFindingsBySeverity(findings, "critical") > 0 {
		status = "blocked"
		reasons = append(reasons, "critical semantic findings present")
	} else if countFindingsBySeverity(findings, "warning") > 0 {
		status = "conditional"
		reasons = append(reasons, "warnings require bounded follow-through")
	}
	if requireVerificationEvidence(env) && strings.TrimSpace(stringValue(verification["status"])) == "" {
		status = "blocked"
		reasons = append(reasons, "required verification evidence missing")
	}
	return map[string]any{
		"status":      status,
		"reasons":     uniqueStrings(reasons),
		"confidence":  semanticApprovalConfidence(findings, verification),
		"review_gate": true,
	}
}

func semanticApprovalConfidence(findings []map[string]any, verification map[string]any) float64 {
	confidence := 0.55
	if len(findings) > 0 {
		confidence += 0.1
	}
	if strings.TrimSpace(stringValue(verification["status"])) != "" {
		confidence += 0.15
	}
	if confidence > 0.95 {
		confidence = 0.95
	}
	return confidence
}

func semanticReviewSummary(findings []map[string]any, approval map[string]any) string {
	return fmt.Sprintf("semantic review found %d findings and returned %s approval", len(findings), stringValue(approval["status"]))
}

func semanticReviewConfidence(files []reviewFile, acceptance []string, verification map[string]any) float64 {
	confidence := 0.4
	if len(files) > 0 {
		confidence += 0.15
	}
	if len(acceptance) > 0 {
		confidence += 0.15
	}
	if strings.TrimSpace(stringValue(verification["status"])) != "" {
		confidence += 0.15
	}
	if confidence > 0.95 {
		confidence = 0.95
	}
	return confidence
}

func reviewFindingsSummary(findings []map[string]any) string {
	return fmt.Sprintf("%d findings identified", len(findings))
}

func reviewFocusLens(env euclotypes.ExecutionEnvelope) string {
	if env.Task == nil {
		return "general"
	}
	lower := strings.ToLower(env.Task.Instruction)
	for _, lens := range []string{"security", "performance", "correctness", "style", "compatibility"} {
		if strings.Contains(lower, lens) {
			return lens
		}
	}
	return "general"
}

func semanticChangeSummary(env euclotypes.ExecutionEnvelope) map[string]any {
	summary := map[string]any{
		"instruction": taskInstruction(env.Task),
	}
	if diff := mapPayloadFromState(env.State, "euclo.diff_summary"); len(diff) > 0 {
		summary["diff_summary"] = diff
	}
	if edit := mapPayloadFromState(env.State, "pipeline.code"); len(edit) > 0 {
		summary["edit_intent"] = edit
	}
	return summary
}

func semanticAcceptanceCriteria(env euclotypes.ExecutionEnvelope) []string {
	text := taskInstruction(env.Task)
	parts := strings.FieldsFunc(text, func(r rune) bool {
		return r == '.' || r == ';' || r == '\n'
	})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	if len(out) == 0 {
		out = append(out, text)
	}
	return uniqueStrings(out)
}

func semanticVerificationContext(env euclotypes.ExecutionEnvelope) map[string]any {
	payload := mapPayloadFromState(env.State, "pipeline.verify")
	if payload == nil {
		payload = map[string]any{}
	}
	if plan := mapPayloadFromState(env.State, "euclo.verification_plan"); len(plan) > 0 {
		payload["plan"] = plan
	}
	return payload
}

func semanticChangeRange(env euclotypes.ExecutionEnvelope, files []reviewFile) string {
	if len(files) == 0 {
		return "task_only"
	}
	if len(files) == 1 {
		return "single_file"
	}
	return "multi_file"
}

func fileNames(files []reviewFile) []string {
	out := make([]string, 0, len(files))
	for _, file := range files {
		if file.Path != "" {
			out = append(out, file.Path)
		}
	}
	sort.Strings(out)
	return out
}

func buildCompatibilityAssessmentPayload(env euclotypes.ExecutionEnvelope) map[string]any {
	before := extractAPISurfaceWithEnv(env.Environment, taskInstruction(env.Task), reviewScopeFiles(env))
	after := deriveAfterSurface(env, before)
	changes, overallCompatible := assessCompatibilityChanges(before, after)
	return map[string]any{
		"before_surface":     before,
		"after_surface":      after,
		"changes":            changes,
		"overall_compatible": overallCompatible,
		"breaking_changes":   breakingChanges(changes),
		"summary":            compatibilitySummary(changes, overallCompatible),
	}
}

func semanticCompatibilityAssessment(reviewPayload map[string]any) map[string]any {
	before, _ := reviewPayload["scope"].(map[string]any)
	changes := compatibilityChangesFromReview(reviewPayload)
	overallCompatible := len(breakingChanges(changes)) == 0
	out := map[string]any{
		"before_surface":     before["public_surface"],
		"after_surface":      before["after_public_surface"],
		"changes":            changes,
		"overall_compatible": overallCompatible,
		"breaking_changes":   breakingChanges(changes),
		"summary":            semanticCompatibilitySummary(changes),
		"review_source":      "euclo:review.semantic",
	}
	return out
}

func compatibilityChangesFromReview(reviewPayload map[string]any) []map[string]any {
	if raw, ok := reviewPayload["compatibility_changes"]; ok {
		if typed, ok := raw.([]map[string]any); ok {
			return typed
		}
		if typed, ok := raw.([]any); ok {
			out := make([]map[string]any, 0, len(typed))
			for _, item := range typed {
				if record, ok := item.(map[string]any); ok {
					out = append(out, record)
				}
			}
			return out
		}
	}
	return nil
}

func semanticCompatibilitySummary(changes []map[string]any) string {
	if len(breakingChanges(changes)) > 0 {
		return fmt.Sprintf("semantic review detected %d breaking compatibility changes", len(breakingChanges(changes)))
	}
	return fmt.Sprintf("semantic review detected %d compatibility-safe changes", len(changes))
}

func extractAPISurface(files []reviewFile) map[string]any {
	return extractAPISurfaceWithEnv(agentenv.AgentEnvironment{}, "", files)
}

func extractAPISurfaceWithEnv(env agentenv.AgentEnvironment, instruction string, files []reviewFile) map[string]any {
	if env.CompatibilitySurfaceExtractor != nil {
		request := agentenv.CompatibilitySurfaceRequest{
			TaskInstruction: strings.TrimSpace(instruction),
			Files:           fileNames(files),
			FileContents:    reviewFilesToRequest(files),
		}
		if surface, ok, err := env.CompatibilitySurfaceExtractor.ExtractSurface(context.Background(), request); err == nil && ok {
			return map[string]any{
				"functions": append([]map[string]any{}, surface.Functions...),
				"types":     append([]map[string]any{}, surface.Types...),
				"metadata":  cloneMapAny(surface.Metadata),
			}
		}
	}
	surface := map[string]any{"functions": []map[string]any{}, "types": []map[string]any{}}
	funcRe := regexp.MustCompile(`^func\s+([A-Z]\w*)\s*\(([^)]*)\)`)
	typeRe := regexp.MustCompile(`^type\s+([A-Z]\w*)\s+`)
	var functions []map[string]any
	var types []map[string]any
	for _, file := range files {
		for idx, line := range strings.Split(file.Content, "\n") {
			trimmed := strings.TrimSpace(line)
			if match := funcRe.FindStringSubmatch(trimmed); len(match) > 0 {
				functions = append(functions, map[string]any{"name": match[1], "signature": trimmed, "location": fmt.Sprintf("%s:%d", file.Path, idx+1)})
			}
			if match := typeRe.FindStringSubmatch(trimmed); len(match) > 0 {
				types = append(types, map[string]any{"name": match[1], "location": fmt.Sprintf("%s:%d", file.Path, idx+1)})
			}
		}
	}
	surface["functions"] = functions
	surface["types"] = types
	return surface
}

func reviewFilesToRequest(files []reviewFile) []map[string]any {
	out := make([]map[string]any, 0, len(files))
	for _, file := range files {
		out = append(out, map[string]any{"path": file.Path, "content": file.Content})
	}
	return out
}

func deriveAfterSurface(env euclotypes.ExecutionEnvelope, before map[string]any) map[string]any {
	after := map[string]any{
		"functions": append([]map[string]any{}, surfaceItems(before["functions"])...),
		"types":     append([]map[string]any{}, surfaceItems(before["types"])...),
	}
	if stateEdit := mapPayloadFromState(env.State, "pipeline.code"); len(stateEdit) > 0 {
		if proposed := normalizeSurface(stateEdit["compatibility_after_surface"]); proposed != nil {
			after = proposed
		}
		if summary := stringValue(stateEdit["summary"]); summary != "" {
			after["proposed_change"] = summary
		}
	}
	for _, artifact := range euclotypes.ArtifactStateFromContext(env.State).OfKind(euclotypes.ArtifactKindEditIntent) {
		record, ok := artifact.Payload.(map[string]any)
		if !ok {
			continue
		}
		if proposed := normalizeSurface(record["compatibility_after_surface"]); proposed != nil {
			after = proposed
		}
		if summary := stringValue(record["summary"]); summary != "" {
			after["proposed_change"] = summary
		}
	}
	return after
}

func normalizeSurface(raw any) map[string]any {
	record, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	return map[string]any{
		"functions": append([]map[string]any{}, surfaceItems(record["functions"])...),
		"types":     append([]map[string]any{}, surfaceItems(record["types"])...),
	}
}

func surfaceItems(raw any) []map[string]any {
	switch typed := raw.(type) {
	case []map[string]any:
		return typed
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

func assessCompatibilityChanges(before, after map[string]any) ([]map[string]any, bool) {
	beforeNames := map[string]struct{}{}
	for _, item := range surfaceItems(before["functions"]) {
		beforeNames[stringValue(item["name"])] = struct{}{}
	}
	afterNames := map[string]struct{}{}
	for _, item := range surfaceItems(after["functions"]) {
		afterNames[stringValue(item["name"])] = struct{}{}
	}
	var changes []map[string]any
	overallCompatible := true
	for name := range beforeNames {
		if _, ok := afterNames[name]; !ok {
			overallCompatible = false
			changes = append(changes, map[string]any{"kind": "removal", "location": name, "classification": "breaking", "risk": "high", "mitigation": "keep the exported symbol or add a compatibility shim"})
		}
	}
	for name := range afterNames {
		if _, ok := beforeNames[name]; !ok {
			changes = append(changes, map[string]any{"kind": "addition", "location": name, "classification": "backward-compatible", "risk": "low", "mitigation": "document the new API surface"})
		}
	}
	if len(changes) == 0 {
		changes = append(changes, map[string]any{"kind": "none", "location": "", "classification": "backward-compatible", "risk": "low", "mitigation": ""})
	}
	return changes, overallCompatible
}

func breakingChanges(changes []map[string]any) []map[string]any {
	var out []map[string]any
	for _, change := range changes {
		if stringValue(change["classification"]) == "breaking" {
			out = append(out, change)
		}
	}
	return out
}

func compatibilitySummary(changes []map[string]any, overallCompatible bool) string {
	if overallCompatible {
		return fmt.Sprintf("compatibility assessment found %d non-breaking changes", len(changes))
	}
	return fmt.Sprintf("compatibility assessment found %d changes with breaking impact", len(breakingChanges(changes)))
}

func buildFixInstruction(payload map[string]any) string {
	findings, _ := payload["findings"].([]map[string]any)
	if len(findings) == 0 {
		if raw, ok := payload["findings"].([]any); ok {
			for _, item := range raw {
				if finding, ok := item.(map[string]any); ok {
					findings = append(findings, finding)
				}
			}
		}
	}
	var suggestions []string
	for _, finding := range findings {
		if severity := stringValue(finding["severity"]); severity == "critical" || severity == "warning" {
			suggestions = append(suggestions, firstNonEmpty(stringValue(finding["suggestion"]), stringValue(finding["description"])))
		}
	}
	if len(suggestions) == 0 {
		return "Apply only clearly safe fixes from the review findings."
	}
	return "Apply the safe fixes from the review findings: " + strings.Join(uniqueStrings(suggestions), "; ")
}

func verificationGapFinding(env euclotypes.ExecutionEnvelope, verification map[string]any) map[string]any {
	status := strings.TrimSpace(fmt.Sprint(verification["status"]))
	if status != "" && status != "<nil>" {
		return nil
	}
	if !requireVerificationEvidence(env) && mapPayloadFromState(env.State, "pipeline.code") == nil {
		return nil
	}
	return map[string]any{
		"severity":         "warning",
		"location":         "",
		"description":      "semantic review could not find executed verification evidence for the current change surface",
		"suggestion":       "run focused verification for the touched scope before accepting the change",
		"confidence":       0.82,
		"category":         "verification_coverage",
		"rationale":        "verification evidence is required by the current execution policy but absent from review inputs",
		"impacted_files":   []string{},
		"impacted_symbols": []string{},
		"review_source":    "euclo:review.semantic",
		"traceability": map[string]any{
			"source": "verification_context",
		},
	}
}

func acceptanceGapFinding(acceptance []string, verification map[string]any) map[string]any {
	if len(acceptance) == 0 {
		return nil
	}
	verificationText := strings.ToLower(encodePayload(verification))
	for _, criterion := range acceptance {
		lowered := strings.ToLower(strings.TrimSpace(criterion))
		if lowered == "" {
			continue
		}
		if strings.Contains(lowered, "security") || strings.Contains(lowered, "performance") || strings.Contains(lowered, "compatibility") || strings.Contains(lowered, "correctness") {
			if !strings.Contains(verificationText, "pass") {
				return map[string]any{
					"severity":         "warning",
					"location":         "",
					"description":      "acceptance criteria mention high-sensitivity behavior without matching verification evidence",
					"suggestion":       "expand verification to cover the stated acceptance criteria",
					"confidence":       0.78,
					"category":         "acceptance_coverage",
					"rationale":        "sensitive acceptance criteria should be traceable to verification or review evidence",
					"impacted_files":   []string{},
					"impacted_symbols": []string{},
					"review_source":    "euclo:review.semantic",
					"traceability": map[string]any{
						"source":            "acceptance_criteria",
						"acceptance_inputs": acceptance,
					},
				}
			}
		}
	}
	return nil
}

func requireVerificationEvidence(env euclotypes.ExecutionEnvelope) bool {
	if env.State == nil {
		return false
	}
	raw, ok := env.State.Get("euclo.resolved_execution_policy")
	if !ok || raw == nil {
		return false
	}
	switch typed := raw.(type) {
	case eucloruntime.ResolvedExecutionPolicy:
		return typed.ReviewApprovalRules.RequireVerificationEvidence
	case map[string]any:
		if approval, ok := typed["review_approval_rules"].(map[string]any); ok {
			switch value := approval["require_verification_evidence"].(type) {
			case bool:
				return value
			case string:
				return strings.EqualFold(strings.TrimSpace(value), "true")
			}
		}
		switch value := typed["require_verification_evidence"].(type) {
		case bool:
			return value
		case string:
			return strings.EqualFold(strings.TrimSpace(value), "true")
		}
	}
	return false
}
