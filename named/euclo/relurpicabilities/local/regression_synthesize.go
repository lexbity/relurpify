package local

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
)

type regressionSynthesizeCapability struct {
	env agentenv.AgentEnvironment
}

func NewRegressionSynthesizeCapability(env agentenv.AgentEnvironment) euclotypes.EucloCodingCapability {
	return &regressionSynthesizeCapability{env: env}
}

func (c *regressionSynthesizeCapability) Descriptor() core.CapabilityDescriptor {
	return core.CapabilityDescriptor{
		ID:            "euclo:test.regression_synthesize",
		Name:          "Regression Synthesize",
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
		Tags:          []string{"coding", "testing", "regression", "tdd"},
		Annotations: map[string]any{
			"supported_profiles": []string{"test_driven_generation", "reproduce_localize_patch"},
		},
	}
}

func (c *regressionSynthesizeCapability) Contract() euclotypes.ArtifactContract {
	return euclotypes.ArtifactContract{
		RequiredInputs: []euclotypes.ArtifactRequirement{{Kind: euclotypes.ArtifactKindIntake, Required: true}},
		ProducedOutputs: []euclotypes.ArtifactKind{
			euclotypes.ArtifactKindReproduction,
			euclotypes.ArtifactKindRegressionAnalysis,
		},
	}
}

func (c *regressionSynthesizeCapability) Eligible(artifacts euclotypes.ArtifactState, snapshot euclotypes.CapabilitySnapshot) euclotypes.EligibilityResult {
	if !artifacts.Has(euclotypes.ArtifactKindIntake) {
		return euclotypes.EligibilityResult{
			Eligible:         false,
			Reason:           "regression synthesis requires intake",
			MissingArtifacts: []euclotypes.ArtifactKind{euclotypes.ArtifactKindIntake},
		}
	}
	if !snapshot.HasWriteTools {
		return euclotypes.EligibilityResult{Eligible: false, Reason: "regression synthesis requires write tools for reproducer-oriented testing"}
	}
	if !looksLikeBugfixOrRegressionRequest(instructionFromArtifacts(artifacts)) {
		return euclotypes.EligibilityResult{Eligible: false, Reason: "regression synthesis requires bugfix or regression-shaped intake"}
	}
	return euclotypes.EligibilityResult{Eligible: true, Reason: "bugfix-shaped intake can be synthesized into a reproducer"}
}

func (c *regressionSynthesizeCapability) Execute(_ context.Context, env euclotypes.ExecutionEnvelope) euclotypes.ExecutionResult {
	payload := buildSynthesizedRegressionPayload(env)
	reproductionArtifact := euclotypes.Artifact{
		ID:         "regression_synthesized_reproduction",
		Kind:       euclotypes.ArtifactKindReproduction,
		Summary:    firstNonEmpty(stringValue(payload["summary"]), "synthesized regression reproducer"),
		Payload:    payload,
		ProducerID: "euclo:test.regression_synthesize",
		Status:     "produced",
	}
	analysisPayload := map[string]any{
		"source":                "euclo:test.regression_synthesize",
		"synthesized":           true,
		"reproducer_available":  true,
		"symptom":               payload["symptom"],
		"expected_failure":      payload["expected_failure"],
		"acceptance_criteria":   payload["acceptance_criteria"],
		"existing_context_keys": payload["existing_context_keys"],
	}
	analysisArtifact := euclotypes.Artifact{
		ID:         "regression_synthesized_analysis",
		Kind:       euclotypes.ArtifactKindRegressionAnalysis,
		Summary:    "synthesized regression analysis from task symptom and existing artifacts",
		Payload:    analysisPayload,
		ProducerID: "euclo:test.regression_synthesize",
		Status:     "produced",
	}
	artifacts := []euclotypes.Artifact{reproductionArtifact, analysisArtifact}
	mergeStateArtifactsToContext(env.State, artifacts)
	return euclotypes.ExecutionResult{
		Status:    euclotypes.ExecutionStatusCompleted,
		Summary:   "regression reproducer synthesized",
		Artifacts: artifacts,
	}
}

func buildSynthesizedRegressionPayload(env euclotypes.ExecutionEnvelope) map[string]any {
	symptom := strings.TrimSpace(taskInstruction(env.Task))
	rootCause := mapPayloadFromState(env.State, "euclo.root_cause")
	regressionAnalysis := mapPayloadFromState(env.State, "euclo.regression_analysis")
	reproduction := mapPayloadFromState(env.State, "euclo.reproduction")
	contextKeys := synthesizedRegressionContextKeys(rootCause, regressionAnalysis, reproduction)
	acceptance := synthesizedAcceptanceCriteria(symptom, rootCause, reproduction)
	return map[string]any{
		"synthesized":           true,
		"reproduced":            false,
		"symptom":               symptom,
		"summary":               fmt.Sprintf("synthesized reproducer for: %s", symptom),
		"method":                "test_regression_synthesize",
		"expected_failure":      synthesizedFailureExpectation(symptom, rootCause, reproduction),
		"acceptance_criteria":   acceptance,
		"suggested_test_name":   synthesizedTestName(symptom, rootCause),
		"root_cause_hint":       firstNonEmpty(stringValue(rootCause["summary"]), stringValue(rootCause["function"]), stringValue(rootCause["file"])),
		"existing_context_keys": contextKeys,
		"source_artifacts": map[string]any{
			"root_cause":          cloneMapAny(rootCause),
			"regression_analysis": cloneMapAny(regressionAnalysis),
			"reproduction":        cloneMapAny(reproduction),
		},
	}
}

func looksLikeBugfixOrRegressionRequest(text string) bool {
	lowered := strings.ToLower(strings.TrimSpace(text))
	if lowered == "" {
		return false
	}
	for _, token := range []string{
		"bug", "bugfix", "fix", "broken", "fails", "failing", "failure", "regression",
		"stopped working", "no longer", "error", "panic", "incorrect", "wrong", "issue",
	} {
		if strings.Contains(lowered, token) {
			return true
		}
	}
	return false
}

func mapPayloadFromState(state *core.Context, key string) map[string]any {
	if state == nil {
		return nil
	}
	raw, ok := state.Get(key)
	if !ok || raw == nil {
		return nil
	}
	payload, _ := raw.(map[string]any)
	return payload
}

func synthesizedRegressionContextKeys(rootCause, regressionAnalysis, reproduction map[string]any) []string {
	keys := []string{}
	if len(rootCause) > 0 {
		keys = append(keys, "root_cause")
	}
	if len(regressionAnalysis) > 0 {
		keys = append(keys, "regression_analysis")
	}
	if len(reproduction) > 0 {
		keys = append(keys, "reproduction")
	}
	sort.Strings(keys)
	return keys
}

func synthesizedFailureExpectation(symptom string, rootCause, reproduction map[string]any) string {
	return firstNonEmpty(
		stringValue(reproduction["error"]),
		stringValue(rootCause["summary"]),
		strings.TrimSpace(symptom),
		"behavior should fail according to the reported bug symptom",
	)
}

func synthesizedAcceptanceCriteria(symptom string, rootCause, reproduction map[string]any) []string {
	items := []string{
		"capture the reported bug symptom in a regression test before implementation",
		"the synthesized regression test should fail during the red phase",
		"the implementation should make the regression test pass during the green phase",
	}
	if file := strings.TrimSpace(stringValue(rootCause["file"])); file != "" {
		items = append(items, "cover the affected area in "+file)
	}
	if function := strings.TrimSpace(stringValue(rootCause["function"])); function != "" {
		items = append(items, "exercise the impacted symbol "+function)
	}
	if errorText := strings.TrimSpace(stringValue(reproduction["error"])); errorText != "" {
		items = append(items, "assert on the observed failure mode: "+errorText)
	} else if strings.TrimSpace(symptom) != "" {
		items = append(items, "assert on the reported symptom: "+strings.TrimSpace(symptom))
	}
	return uniqueStrings(items)
}

func synthesizedTestName(symptom string, rootCause map[string]any) string {
	base := firstNonEmpty(stringValue(rootCause["function"]), stringValue(rootCause["file"]), symptom, "regression")
	base = strings.ToLower(base)
	replacer := strings.NewReplacer("/", "_", ".", "_", "-", "_", " ", "_", ":", "_")
	base = replacer.Replace(base)
	parts := strings.FieldsFunc(base, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_')
	})
	base = strings.Join(parts, "_")
	base = strings.Trim(base, "_")
	if base == "" {
		base = "regression"
	}
	return "test_" + base + "_regression"
}
