package capabilities

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
)

type reviewFindingsCapability struct {
	env agentenv.AgentEnvironment
}

type reviewCompatibilityCapability struct {
	env agentenv.AgentEnvironment
}

type reviewImplementIfSafeCapability struct {
	env agentenv.AgentEnvironment
}

func (c *reviewFindingsCapability) Descriptor() core.CapabilityDescriptor {
	return core.CapabilityDescriptor{
		ID:            "euclo:review.findings",
		Name:          "Review Findings",
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
		Tags:          []string{"coding", "review"},
		Annotations: map[string]any{
			"supported_profiles": []string{"review_suggest_implement"},
		},
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

func (c *reviewFindingsCapability) Eligible(artifacts euclotypes.ArtifactState, snapshot euclotypes.CapabilitySnapshot) euclotypes.EligibilityResult {
	if !snapshot.HasReadTools {
		return euclotypes.EligibilityResult{Eligible: false, Reason: "read tools required for review findings"}
	}
	if !looksLikeReviewRequest(artifacts) {
		return euclotypes.EligibilityResult{Eligible: false, Reason: "review findings requires review-like intake"}
	}
	return euclotypes.EligibilityResult{Eligible: true, Reason: "review-like intake with read tools"}
}

func (c *reviewFindingsCapability) Execute(_ context.Context, env euclotypes.ExecutionEnvelope) euclotypes.ExecutionResult {
	payload := buildReviewFindingsPayload(env)
	artifact := euclotypes.Artifact{
		ID:         "review_findings",
		Kind:       euclotypes.ArtifactKindReviewFindings,
		Summary:    summarizePayload(payload),
		Payload:    payload,
		ProducerID: "euclo:review.findings",
		Status:     "produced",
	}
	mergeStateArtifactsToContext(env.State, []euclotypes.Artifact{artifact})
	return euclotypes.ExecutionResult{
		Status:    euclotypes.ExecutionStatusCompleted,
		Summary:   "review findings produced",
		Artifacts: []euclotypes.Artifact{artifact},
	}
}

func (c *reviewCompatibilityCapability) Descriptor() core.CapabilityDescriptor {
	return core.CapabilityDescriptor{
		ID:            "euclo:review.compatibility",
		Name:          "Review Compatibility",
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
		Tags:          []string{"coding", "review", "compatibility"},
		Annotations: map[string]any{
			"supported_profiles": []string{"review_suggest_implement"},
		},
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
	payload := buildCompatibilityAssessmentPayload(env)
	artifact := euclotypes.Artifact{
		ID:         "compatibility_assessment",
		Kind:       euclotypes.ArtifactKindCompatibilityAssessment,
		Summary:    summarizePayload(payload),
		Payload:    payload,
		ProducerID: "euclo:review.compatibility",
		Status:     "produced",
	}
	mergeStateArtifactsToContext(env.State, []euclotypes.Artifact{artifact})
	return euclotypes.ExecutionResult{
		Status:    euclotypes.ExecutionStatusCompleted,
		Summary:   "compatibility assessment produced",
		Artifacts: []euclotypes.Artifact{artifact},
	}
}

func buildCompatibilityAssessmentPayload(env euclotypes.ExecutionEnvelope) map[string]any {
	before := extractAPISurface(reviewScopeFiles(env))
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

func (c *reviewImplementIfSafeCapability) Descriptor() core.CapabilityDescriptor {
	return core.CapabilityDescriptor{
		ID:            "euclo:review.implement_if_safe",
		Name:          "Review Implement If Safe",
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
		Tags:          []string{"coding", "review", "implementation"},
		Annotations: map[string]any{
			"supported_profiles": []string{"review_suggest_implement"},
		},
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
	reviewCap := &reviewFindingsCapability{env: c.env}
	reviewResult := reviewCap.Execute(ctx, env)
	if reviewResult.Status != euclotypes.ExecutionStatusCompleted || len(reviewResult.Artifacts) == 0 {
		return reviewResult
	}
	artifacts := append([]euclotypes.Artifact{}, reviewResult.Artifacts...)
	payload, _ := reviewResult.Artifacts[0].Payload.(map[string]any)
	stats, _ := payload["stats"].(map[string]any)
	criticalCount := intValue(stats["critical_count"])
	warningCount := intValue(stats["warning_count"])
	infoCount := intValue(stats["info_count"])

	switch {
	case criticalCount > 0:
		return euclotypes.ExecutionResult{
			Status:    euclotypes.ExecutionStatusCompleted,
			Summary:   "manual review required; critical findings present",
			Artifacts: artifacts,
			RecoveryHint: &euclotypes.RecoveryHint{
				Strategy: euclotypes.RecoveryStrategyModeEscalation,
				Context:  map[string]any{"manual_review_required": true},
			},
		}
	case warningCount == 0 && infoCount > 0:
		return euclotypes.ExecutionResult{
			Status:    euclotypes.ExecutionStatusCompleted,
			Summary:   "info-only findings; no automatic implementation needed",
			Artifacts: artifacts,
		}
	}

	task := env.Task
	if task == nil {
		task = &core.Task{ID: "review-implement", Instruction: "Apply safe review findings"}
	}
	fixTask := &core.Task{
		ID:          task.ID + "-implement-if-safe",
		Instruction: buildFixInstruction(payload),
		Type:        task.Type,
		Context:     task.Context,
	}
	editEnv := env
	editEnv.Task = fixTask
	editCap := &editVerifyRepairCapability{env: c.env}
	editResult := editCap.Execute(ctx, editEnv)
	artifacts = append(artifacts, editResult.Artifacts...)

	return euclotypes.ExecutionResult{
		Status:       editResult.Status,
		Summary:      "review findings implemented where safe",
		Artifacts:    artifacts,
		FailureInfo:  editResult.FailureInfo,
		RecoveryHint: editResult.RecoveryHint,
	}
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
		"findings": findings,
		"summary":  reviewFindingsSummary(findings),
		"stats":    stats,
	}
}

type reviewFile struct {
	Path    string
	Content string
}

func reviewScopeFiles(env euclotypes.ExecutionEnvelope) []reviewFile {
	var files []reviewFile
	if env.Task != nil && env.Task.Context != nil {
		if raw, ok := env.Task.Context["context_file_contents"].([]map[string]any); ok {
			for _, item := range raw {
				files = append(files, reviewFile{Path: stringValue(item["path"]), Content: stringValue(item["content"])})
			}
		}
		if raw, ok := env.Task.Context["context_file_contents"].([]any); ok {
			for _, item := range raw {
				record, ok := item.(map[string]any)
				if !ok {
					continue
				}
				files = append(files, reviewFile{Path: stringValue(record["path"]), Content: stringValue(record["content"])})
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
				findings = append(findings, reviewFinding("warning", fmt.Sprintf("%s:%d", file.Path, i+1), "leftover TODO/FIXME marker", "resolve or remove the pending marker", 0.8, "maintainability"))
			case strings.Contains(trimmed, "panic("):
				findings = append(findings, reviewFinding("critical", fmt.Sprintf("%s:%d", file.Path, i+1), "panic in production code path", "return an error or handle the failure explicitly", 0.92, "correctness"))
			case strings.Contains(trimmed, "fmt.Println("):
				findings = append(findings, reviewFinding("info", fmt.Sprintf("%s:%d", file.Path, i+1), "debug print left in code", "replace with structured logging or remove", 0.72, "style"))
			}
		}
	}
	if len(findings) == 0 {
		findings = append(findings, reviewFinding("info", "", "no obvious review issues found in provided scope", "", 0.6, "general"))
	}
	return findings
}

func reviewFinding(severity, location, description, suggestion string, confidence float64, category string) map[string]any {
	return map[string]any{
		"severity":    severity,
		"location":    location,
		"description": description,
		"suggestion":  suggestion,
		"confidence":  confidence,
		"category":    category,
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

func extractAPISurface(files []reviewFile) map[string]any {
	surface := map[string]any{
		"functions": []map[string]any{},
		"types":     []map[string]any{},
	}
	funcRe := regexp.MustCompile(`^func\s+([A-Z]\w*)\s*\(([^)]*)\)`)
	typeRe := regexp.MustCompile(`^type\s+([A-Z]\w*)\s+`)
	var functions []map[string]any
	var types []map[string]any
	for _, file := range files {
		for idx, line := range strings.Split(file.Content, "\n") {
			trimmed := strings.TrimSpace(line)
			if match := funcRe.FindStringSubmatch(trimmed); len(match) > 0 {
				functions = append(functions, map[string]any{
					"name":      match[1],
					"signature": trimmed,
					"location":  fmt.Sprintf("%s:%d", file.Path, idx+1),
				})
			}
			if match := typeRe.FindStringSubmatch(trimmed); len(match) > 0 {
				types = append(types, map[string]any{
					"name":     match[1],
					"location": fmt.Sprintf("%s:%d", file.Path, idx+1),
				})
			}
		}
	}
	surface["functions"] = functions
	surface["types"] = types
	return surface
}

func deriveAfterSurface(env euclotypes.ExecutionEnvelope, before map[string]any) map[string]any {
	after := map[string]any{
		"functions": append([]map[string]any{}, surfaceItems(before["functions"])...),
		"types":     append([]map[string]any{}, surfaceItems(before["types"])...),
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
			changes = append(changes, map[string]any{
				"kind":           "removal",
				"location":       name,
				"classification": "breaking",
				"risk":           "high",
				"mitigation":     "keep the exported symbol or add a compatibility shim",
			})
		}
	}
	for name := range afterNames {
		if _, ok := beforeNames[name]; !ok {
			changes = append(changes, map[string]any{
				"kind":           "addition",
				"location":       name,
				"classification": "backward-compatible",
				"risk":           "low",
				"mitigation":     "document the new API surface",
			})
		}
	}
	if len(changes) == 0 {
		changes = append(changes, map[string]any{
			"kind":           "none",
			"location":       "",
			"classification": "backward-compatible",
			"risk":           "low",
			"mitigation":     "",
		})
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
		return fmt.Sprintf("%d compatibility changes assessed; no breaking changes detected", len(changes))
	}
	return fmt.Sprintf("%d compatibility changes assessed; breaking changes present", len(changes))
}

func buildFixInstruction(reviewPayload map[string]any) string {
	findings := reviewPayload["findings"]
	var suggestions []string
	switch typed := findings.(type) {
	case []map[string]any:
		for _, finding := range typed {
			suggestion := stringValue(finding["suggestion"])
			if suggestion != "" {
				suggestions = append(suggestions, suggestion)
			}
		}
	case []any:
		for _, raw := range typed {
			record, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			suggestion := stringValue(record["suggestion"])
			if suggestion != "" {
				suggestions = append(suggestions, suggestion)
			}
		}
	}
	if len(suggestions) == 0 {
		return "Apply the safe review findings."
	}
	return "Apply these safe review findings:\n- " + strings.Join(suggestions, "\n- ")
}
