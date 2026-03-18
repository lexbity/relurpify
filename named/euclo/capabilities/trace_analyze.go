package capabilities

import (
	"context"
	"fmt"
	"strings"

	"github.com/lexcodex/relurpify/agents/blackboard"
	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
)

type traceAnalyzeCapability struct {
	env agentenv.AgentEnvironment
}

func (c *traceAnalyzeCapability) Descriptor() core.CapabilityDescriptor {
	return core.CapabilityDescriptor{
		ID:            "euclo:trace.analyze",
		Name:          "Trace Analyze",
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
		Tags:          []string{"coding", "debugging", "trace"},
		Annotations: map[string]any{
			"supported_profiles": []string{"trace_execute_analyze", "reproduce_localize_patch"},
		},
	}
}

func (c *traceAnalyzeCapability) Contract() euclotypes.ArtifactContract {
	return euclotypes.ArtifactContract{
		RequiredInputs: []euclotypes.ArtifactRequirement{
			{Kind: euclotypes.ArtifactKindIntake, Required: true},
		},
		ProducedOutputs: []euclotypes.ArtifactKind{
			euclotypes.ArtifactKindTrace,
			euclotypes.ArtifactKindAnalyze,
		},
	}
}

func (c *traceAnalyzeCapability) Eligible(artifacts euclotypes.ArtifactState, snapshot euclotypes.CapabilitySnapshot) euclotypes.EligibilityResult {
	if !looksLikeTraceRequest(artifacts) {
		return euclotypes.EligibilityResult{Eligible: false, Reason: "trace analysis requires explicit tracing intent"}
	}
	if !snapshot.HasExecuteTools && !snapshot.HasVerificationTools {
		return euclotypes.EligibilityResult{Eligible: false, Reason: "execute or verification tools required for trace analysis"}
	}
	return euclotypes.EligibilityResult{Eligible: true, Reason: "trace request with executable evidence tools"}
}

func (c *traceAnalyzeCapability) Execute(ctx context.Context, env euclotypes.ExecutionEnvelope) euclotypes.ExecutionResult {
	if env.State == nil {
		env.State = core.NewContext()
	}
	env.State.Set("euclo.blackboard_seed_facts", map[string]any{
		"trace:symptom": capTaskInstruction(env.Task),
	})

	result, err := ExecuteBlackboard(ctx, env, traceKnowledgeSources(), 6, func(bb *blackboard.Blackboard) bool {
		return boardHasEntry(bb, "trace:correlations")
	})
	if err != nil {
		return euclotypes.ExecutionResult{
			Status:  euclotypes.ExecutionStatusFailed,
			Summary: "trace analysis failed",
			FailureInfo: &euclotypes.CapabilityFailure{
				Code:         "trace_analyze_failed",
				Message:      err.Error(),
				Recoverable:  true,
				FailedPhase:  "trace_analyze",
				ParadigmUsed: "blackboard",
			},
			RecoveryHint: &euclotypes.RecoveryHint{
				Strategy:          euclotypes.RecoveryStrategyParadigmSwitch,
				SuggestedParadigm: "react",
			},
		}
	}

	tracePayload := map[string]any{
		"raw_output": defaultValueFromBoard(result.Board, "trace:raw_output", ""),
	}
	if analysis, ok := boardEntryValue(result.Board, "trace:analysis"); ok {
		if record, ok := analysis.(map[string]any); ok {
			for _, key := range []string{"call_chain", "hot_paths", "anomalies", "timing"} {
				if value, exists := record[key]; exists {
					tracePayload[key] = value
				}
			}
		}
	}

	correlations, _ := boardEntryValue(result.Board, "trace:correlations")
	analyzePayload := map[string]any{
		"correlations":     defaultValue(correlations, []any{}),
		"summary":          traceSummary(tracePayload, correlations),
		"actionable_items": traceActionableItems(correlations),
	}

	artifacts := []euclotypes.Artifact{
		{
			ID:         "trace_artifact",
			Kind:       euclotypes.ArtifactKindTrace,
			Summary:    summarizePayload(tracePayload),
			Payload:    tracePayload,
			ProducerID: "euclo:trace.analyze",
			Status:     "produced",
		},
		{
			ID:         "trace_analysis",
			Kind:       euclotypes.ArtifactKindAnalyze,
			Summary:    summarizePayload(analyzePayload),
			Payload:    analyzePayload,
			ProducerID: "euclo:trace.analyze",
			Status:     "produced",
		},
	}
	mergeStateArtifactsToContext(env.State, artifacts)

	return euclotypes.ExecutionResult{
		Status:    euclotypes.ExecutionStatusCompleted,
		Summary:   "trace collected and analyzed",
		Artifacts: artifacts,
	}
}

func traceKnowledgeSources() []blackboard.KnowledgeSource {
	return []blackboard.KnowledgeSource{
		NewAnalysisKnowledgeSource(
			"Trace Collector",
			"not trace:raw_output exists",
			[]string{"cli_go", "file_read", "file_write"},
			`Collect runtime trace output for the target scenario.
Goal: {{goal}}
Context: {{entries}}
Return JSON with:
- facts: [{"key":"trace:raw_output","value":"... raw trace output ..."}]
- summary: short string`,
		),
		NewAnalysisKnowledgeSource(
			"Trace Analyzer",
			"trace:raw_output exists",
			[]string{"file_read"},
			`Parse the raw trace into structured findings.
Context: {{entries}}
Return JSON with:
- facts: [{"key":"trace:analysis","value":{"call_chain":[{"function":"...","location":"..."}],"hot_paths":[{"path":"...","count":1}],"anomalies":[{"description":"...","severity":"..."}],"timing":{"slowest_path":"..."}}}]
- summary: short string`,
		),
		NewSynthesisKnowledgeSource(
			"Code Correlator",
			"not trace:correlations exists",
			[]string{"trace:analysis", "trace:symptom"},
			`Map trace findings back to code and assess severity.
Inputs: {{input_entries}}
Return JSON with:
- facts: [{"key":"trace:correlations","value":[{"location":"...","finding":"...","assessment":"...","severity":"..."}]}]
- summary: short string`,
		),
	}
}

func looksLikeTraceRequest(artifacts euclotypes.ArtifactState) bool {
	text := strings.ToLower(strings.TrimSpace(instructionFromArtifacts(artifacts)))
	for _, token := range []string{"show trace", "trace this", "run with tracing", "trace", "stacktrace"} {
		if strings.Contains(text, token) {
			return true
		}
	}
	return false
}

func defaultValueFromBoard(board *blackboard.Blackboard, key string, fallback any) any {
	if value, ok := boardEntryValue(board, key); ok {
		return value
	}
	return fallback
}

func traceSummary(tracePayload map[string]any, correlations any) string {
	rawOutput, _ := tracePayload["raw_output"].(string)
	correlationCount := len(asSlice(correlations))
	return fmt.Sprintf("captured %d chars of trace output and produced %d code correlations", len(strings.TrimSpace(rawOutput)), correlationCount)
}

func traceActionableItems(correlations any) []string {
	items := asSlice(correlations)
	out := make([]string, 0, len(items))
	for _, item := range items {
		record, ok := item.(map[string]any)
		if !ok {
			continue
		}
		location := stringField(record, "location")
		finding := stringField(record, "finding")
		if location == "" && finding == "" {
			continue
		}
		out = append(out, strings.TrimSpace(location+": "+finding))
	}
	return out
}
