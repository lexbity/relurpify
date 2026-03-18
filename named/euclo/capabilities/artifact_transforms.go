package capabilities

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
)

type diffSummaryCapability struct {
	env agentenv.AgentEnvironment
}

type traceToRootCauseCapability struct {
	env agentenv.AgentEnvironment
}

type verificationSummaryCapability struct {
	env agentenv.AgentEnvironment
}

func (c *diffSummaryCapability) Descriptor() core.CapabilityDescriptor {
	return core.CapabilityDescriptor{
		ID:            "euclo:artifact.diff_summary",
		Name:          "Artifact Diff Summary",
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
		Tags:          []string{"coding", "artifact", "summary"},
		Annotations: map[string]any{
			"supported_profiles": allPhase7Profiles(),
		},
	}
}

func (c *diffSummaryCapability) Contract() euclotypes.ArtifactContract {
	return euclotypes.ArtifactContract{
		RequiredInputs: []euclotypes.ArtifactRequirement{{Kind: euclotypes.ArtifactKindEditIntent, Required: true}},
		ProducedOutputs: []euclotypes.ArtifactKind{
			euclotypes.ArtifactKindDiffSummary,
		},
	}
}

func (c *diffSummaryCapability) Eligible(artifacts euclotypes.ArtifactState, _ euclotypes.CapabilitySnapshot) euclotypes.EligibilityResult {
	if !artifacts.Has(euclotypes.ArtifactKindEditIntent) {
		return euclotypes.EligibilityResult{
			Eligible:         false,
			Reason:           "diff summary requires edit intent",
			MissingArtifacts: []euclotypes.ArtifactKind{euclotypes.ArtifactKindEditIntent},
		}
	}
	return euclotypes.EligibilityResult{Eligible: true, Reason: "edit intent artifact available"}
}

func (c *diffSummaryCapability) Execute(_ context.Context, env euclotypes.ExecutionEnvelope) euclotypes.ExecutionResult {
	artifacts := euclotypes.ArtifactStateFromContext(env.State).OfKind(euclotypes.ArtifactKindEditIntent)
	payload := buildDiffSummaryPayload(artifacts)
	artifact := euclotypes.Artifact{
		ID:         "diff_summary",
		Kind:       euclotypes.ArtifactKindDiffSummary,
		Summary:    summarizePayload(payload),
		Payload:    payload,
		ProducerID: "euclo:artifact.diff_summary",
		Status:     "produced",
	}
	mergeStateArtifactsToContext(env.State, []euclotypes.Artifact{artifact})
	return euclotypes.ExecutionResult{
		Status:    euclotypes.ExecutionStatusCompleted,
		Summary:   "diff summary produced",
		Artifacts: []euclotypes.Artifact{artifact},
	}
}

func (c *traceToRootCauseCapability) Descriptor() core.CapabilityDescriptor {
	return core.CapabilityDescriptor{
		ID:            "euclo:artifact.trace_to_root_cause",
		Name:          "Artifact Trace To Root Cause",
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
		Tags:          []string{"coding", "artifact", "trace"},
		Annotations: map[string]any{
			"supported_profiles": []string{"trace_execute_analyze", "reproduce_localize_patch"},
		},
	}
}

func (c *traceToRootCauseCapability) Contract() euclotypes.ArtifactContract {
	return euclotypes.ArtifactContract{
		RequiredInputs: []euclotypes.ArtifactRequirement{{Kind: euclotypes.ArtifactKindTrace, Required: true}},
		ProducedOutputs: []euclotypes.ArtifactKind{
			euclotypes.ArtifactKindRootCauseCandidates,
		},
	}
}

func (c *traceToRootCauseCapability) Eligible(artifacts euclotypes.ArtifactState, _ euclotypes.CapabilitySnapshot) euclotypes.EligibilityResult {
	if !artifacts.Has(euclotypes.ArtifactKindTrace) {
		return euclotypes.EligibilityResult{
			Eligible:         false,
			Reason:           "trace-to-root-cause requires trace artifact",
			MissingArtifacts: []euclotypes.ArtifactKind{euclotypes.ArtifactKindTrace},
		}
	}
	return euclotypes.EligibilityResult{Eligible: true, Reason: "trace artifact available"}
}

func (c *traceToRootCauseCapability) Execute(_ context.Context, env euclotypes.ExecutionEnvelope) euclotypes.ExecutionResult {
	traceArtifacts := euclotypes.ArtifactStateFromContext(env.State).OfKind(euclotypes.ArtifactKindTrace)
	payload := buildRootCauseCandidatesPayload(traceArtifacts)
	artifact := euclotypes.Artifact{
		ID:         "root_cause_candidates",
		Kind:       euclotypes.ArtifactKindRootCauseCandidates,
		Summary:    summarizePayload(payload),
		Payload:    payload,
		ProducerID: "euclo:artifact.trace_to_root_cause",
		Status:     "produced",
	}
	mergeStateArtifactsToContext(env.State, []euclotypes.Artifact{artifact})
	return euclotypes.ExecutionResult{
		Status:    euclotypes.ExecutionStatusCompleted,
		Summary:   "root-cause candidates produced from trace",
		Artifacts: []euclotypes.Artifact{artifact},
	}
}

func (c *verificationSummaryCapability) Descriptor() core.CapabilityDescriptor {
	return core.CapabilityDescriptor{
		ID:            "euclo:artifact.verification_summary",
		Name:          "Artifact Verification Summary",
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
		Tags:          []string{"coding", "artifact", "verification"},
		Annotations: map[string]any{
			"supported_profiles": allPhase7Profiles(),
		},
	}
}

func (c *verificationSummaryCapability) Contract() euclotypes.ArtifactContract {
	return euclotypes.ArtifactContract{
		RequiredInputs: []euclotypes.ArtifactRequirement{{Kind: euclotypes.ArtifactKindVerification, Required: true}},
		ProducedOutputs: []euclotypes.ArtifactKind{
			euclotypes.ArtifactKindVerificationSummary,
		},
	}
}

func (c *verificationSummaryCapability) Eligible(artifacts euclotypes.ArtifactState, _ euclotypes.CapabilitySnapshot) euclotypes.EligibilityResult {
	if !artifacts.Has(euclotypes.ArtifactKindVerification) {
		return euclotypes.EligibilityResult{
			Eligible:         false,
			Reason:           "verification summary requires verification artifact",
			MissingArtifacts: []euclotypes.ArtifactKind{euclotypes.ArtifactKindVerification},
		}
	}
	return euclotypes.EligibilityResult{Eligible: true, Reason: "verification artifact available"}
}

func (c *verificationSummaryCapability) Execute(_ context.Context, env euclotypes.ExecutionEnvelope) euclotypes.ExecutionResult {
	verificationArtifacts := euclotypes.ArtifactStateFromContext(env.State).OfKind(euclotypes.ArtifactKindVerification)
	payload := buildVerificationSummaryPayload(verificationArtifacts)
	artifact := euclotypes.Artifact{
		ID:         "verification_summary",
		Kind:       euclotypes.ArtifactKindVerificationSummary,
		Summary:    summarizePayload(payload),
		Payload:    payload,
		ProducerID: "euclo:artifact.verification_summary",
		Status:     "produced",
	}
	mergeStateArtifactsToContext(env.State, []euclotypes.Artifact{artifact})
	return euclotypes.ExecutionResult{
		Status:    euclotypes.ExecutionStatusCompleted,
		Summary:   "verification summary produced",
		Artifacts: []euclotypes.Artifact{artifact},
	}
}

func allPhase7Profiles() []string {
	return []string{
		"edit_verify_repair",
		"reproduce_localize_patch",
		"test_driven_generation",
		"review_suggest_implement",
		"plan_stage_execute",
		"trace_execute_analyze",
	}
}

func buildDiffSummaryPayload(editArtifacts []euclotypes.Artifact) map[string]any {
	files := map[string]map[string]any{}
	var overall []string
	totalChanges := 0
	for _, artifact := range editArtifacts {
		record, ok := artifact.Payload.(map[string]any)
		if !ok {
			continue
		}
		summary := firstNonEmpty(stringValue(record["summary"]), artifact.Summary, "code changes proposed")
		overall = append(overall, summary)
		for _, path := range diffPathsFromPayload(record) {
			entry := files[path]
			if entry == nil {
				entry = map[string]any{
					"path":          path,
					"change_type":   classifyChangeType(summary),
					"description":   summary,
					"impact":        classifyImpact(summary),
					"lines_changed": estimateLinesChanged(summary),
				}
				files[path] = entry
			}
			totalChanges += intValue(entry["lines_changed"])
		}
	}
	rows := mapValuesSorted(files)
	scope := "narrow"
	switch {
	case len(rows) > 4:
		scope = "broad"
	case len(rows) > 1:
		scope = "moderate"
	}
	risk := "low"
	switch {
	case len(rows) > 4 || totalChanges > 80:
		risk = "high"
	case len(rows) > 1 || totalChanges > 20:
		risk = "medium"
	}
	if len(rows) == 0 {
		rows = []map[string]any{{
			"path":          "",
			"change_type":   "unknown",
			"description":   "no scoped file changes were available",
			"impact":        "unable to assess",
			"lines_changed": 0,
		}}
	}
	return map[string]any{
		"files":            rows,
		"overall_summary":  strings.Join(uniqueStrings(overall), "; "),
		"scope_assessment": scope,
		"risk_level":       risk,
	}
}

func buildRootCauseCandidatesPayload(traceArtifacts []euclotypes.Artifact) map[string]any {
	var candidates []map[string]any
	for _, artifact := range traceArtifacts {
		record, ok := artifact.Payload.(map[string]any)
		if !ok {
			continue
		}
		for index, frame := range traceFrames(record) {
			location := firstNonEmpty(stringValue(frame["function"]), stringValue(frame["location"]), stringValue(frame["file"]))
			if location == "" {
				location = fmt.Sprintf("trace_frame_%d", index+1)
			}
			evidence := firstNonEmpty(stringValue(frame["detail"]), stringValue(frame["message"]), artifact.Summary)
			confidence := traceCandidateConfidence(index, len(traceFrames(record)), evidence)
			candidates = append(candidates, map[string]any{
				"location":                location,
				"description":             fmt.Sprintf("Trace suggests investigation near %s", location),
				"evidence":                evidence,
				"confidence":              confidence,
				"suggested_investigation": fmt.Sprintf("Inspect %s and its immediate callers for the failing state transition", location),
			})
		}
	}
	if len(candidates) == 0 {
		candidates = append(candidates, map[string]any{
			"location":                "unknown",
			"description":             "No structured frames found in trace payload",
			"evidence":                "Trace artifact lacked frame-level detail",
			"confidence":              0.25,
			"suggested_investigation": "Collect a fuller stack trace or execution trace",
		})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return floatValue(candidates[i]["confidence"]) > floatValue(candidates[j]["confidence"])
	})
	return map[string]any{
		"candidates":    candidates,
		"top_candidate": candidates[0],
	}
}

func buildVerificationSummaryPayload(verificationArtifacts []euclotypes.Artifact) map[string]any {
	var checks []map[string]any
	var gaps []string
	overallStatus := "pass"
	confidence := 0.9
	for _, artifact := range verificationArtifacts {
		record, ok := artifact.Payload.(map[string]any)
		if !ok {
			continue
		}
		if summary := firstNonEmpty(stringValue(record["summary"]), artifact.Summary); summary != "" {
			status := classifyVerificationStatus(summary)
			checks = append(checks, map[string]any{
				"kind":     classifyVerificationKind(summary),
				"status":   status,
				"detail":   summary,
				"duration": stringValue(record["duration"]),
			})
			switch status {
			case "fail":
				overallStatus = "fail"
				confidence = 0.35
			case "partial":
				if overallStatus != "fail" {
					overallStatus = "partial"
				}
				if confidence > 0.6 {
					confidence = 0.6
				}
			}
		}
		if rawGaps, ok := record["gaps"].([]string); ok {
			gaps = append(gaps, rawGaps...)
		}
		if rawGaps, ok := record["gaps"].([]any); ok {
			for _, gap := range rawGaps {
				if s, ok := gap.(string); ok {
					gaps = append(gaps, s)
				}
			}
		}
	}
	if len(checks) == 0 {
		checks = append(checks, map[string]any{
			"kind":     "verification",
			"status":   "partial",
			"detail":   "No structured verification checks were available",
			"duration": "",
		})
		overallStatus = "partial"
		confidence = 0.4
	}
	if len(gaps) == 0 && overallStatus != "pass" {
		gaps = append(gaps, "verification output did not identify which checks were skipped")
	}
	return map[string]any{
		"overall_status": overallStatus,
		"checks":         checks,
		"gaps":           uniqueStrings(gaps),
		"confidence":     confidence,
		"recommendation": verificationRecommendation(overallStatus),
	}
}

func diffPathsFromPayload(record map[string]any) []string {
	var files []string
	for _, key := range []string{"files", "paths"} {
		for _, item := range uniqueStringsFromAny(record[key]) {
			files = append(files, item)
		}
	}
	if len(files) == 0 {
		if path := stringValue(record["path"]); path != "" {
			files = append(files, path)
		}
	}
	return uniqueStrings(files)
}

func classifyChangeType(summary string) string {
	lower := strings.ToLower(summary)
	switch {
	case strings.Contains(lower, "rename"), strings.Contains(lower, "refactor"):
		return "refactor"
	case strings.Contains(lower, "fix"), strings.Contains(lower, "patch"):
		return "bugfix"
	case strings.Contains(lower, "add"), strings.Contains(lower, "create"):
		return "addition"
	default:
		return "modification"
	}
}

func classifyImpact(summary string) string {
	lower := strings.ToLower(summary)
	switch {
	case strings.Contains(lower, "api"), strings.Contains(lower, "public"):
		return "possible external caller impact"
	case strings.Contains(lower, "test"):
		return "verification-only impact"
	default:
		return "localized implementation impact"
	}
}

func estimateLinesChanged(summary string) int {
	words := len(strings.Fields(summary))
	if words < 4 {
		return 5
	}
	return words * 2
}

func mapValuesSorted(values map[string]map[string]any) []map[string]any {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]map[string]any, 0, len(keys))
	for _, key := range keys {
		out = append(out, values[key])
	}
	return out
}

func traceFrames(record map[string]any) []map[string]any {
	if raw, ok := record["frames"].([]map[string]any); ok {
		return raw
	}
	if raw, ok := record["frames"].([]any); ok {
		out := make([]map[string]any, 0, len(raw))
		for _, item := range raw {
			if typed, ok := item.(map[string]any); ok {
				out = append(out, typed)
			}
		}
		return out
	}
	if frame, ok := record["root_frame"].(map[string]any); ok {
		return []map[string]any{frame}
	}
	return nil
}

func traceCandidateConfidence(index, total int, evidence string) float64 {
	confidence := 0.8 - float64(index)*0.15
	if total == 1 {
		confidence += 0.1
	}
	if strings.Contains(strings.ToLower(evidence), "error") {
		confidence += 0.05
	}
	if confidence < 0.2 {
		confidence = 0.2
	}
	if confidence > 0.98 {
		confidence = 0.98
	}
	return confidence
}

func classifyVerificationKind(summary string) string {
	lower := strings.ToLower(summary)
	switch {
	case strings.Contains(lower, "test"):
		return "test"
	case strings.Contains(lower, "build"), strings.Contains(lower, "compile"):
		return "build"
	case strings.Contains(lower, "lint"):
		return "lint"
	case strings.Contains(lower, "type"):
		return "typecheck"
	default:
		return "verification"
	}
}

func classifyVerificationStatus(summary string) string {
	lower := strings.ToLower(summary)
	switch {
	case strings.Contains(lower, "fail"), strings.Contains(lower, "error"):
		return "fail"
	case strings.Contains(lower, "partial"), strings.Contains(lower, "skipped"), strings.Contains(lower, "not run"):
		return "partial"
	default:
		return "pass"
	}
}

func verificationRecommendation(status string) string {
	switch status {
	case "fail":
		return "reject"
	case "partial":
		return "investigate"
	default:
		return "accept"
	}
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func floatValue(raw any) float64 {
	switch typed := raw.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	default:
		return 0
	}
}
