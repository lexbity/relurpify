package agenttest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
)

const (
	performanceLLMCallThreshold  = 1.5
	performanceTokenThreshold    = 2.0
	performanceDurationThreshold = 3.0
)

type PhaseMetric struct {
	Phase      string `json:"phase"`
	DurationMS int64  `json:"duration_ms"`
	LLMCalls   int    `json:"llm_calls"`
	TokensUsed int    `json:"tokens_used"`
}

type PhaseBaseline struct {
	LLMCalls   int   `json:"llm_calls"`
	Tokens     int   `json:"tokens"`
	DurationMS int64 `json:"duration_ms,omitempty"`
}

type PerformanceBaseline struct {
	Model       string                   `json:"model"`
	RecordedAt  string                   `json:"recorded_at"`
	LLMCalls    int                      `json:"llm_calls"`
	TotalTokens int                      `json:"total_tokens"`
	DurationMS  int64                    `json:"duration_ms"`
	Phases      map[string]PhaseBaseline `json:"phases,omitempty"`
}

type PerformanceWarning struct {
	Metric   string `json:"metric"`
	Actual   int64  `json:"actual"`
	Baseline int64  `json:"baseline"`
	Detail   string `json:"detail"`
}

type PerformanceSummary struct {
	CasesWithBaseline   int                  `json:"cases_with_baseline,omitempty"`
	CasesWithinBaseline int                  `json:"cases_within_baseline,omitempty"`
	CasesAboveBaseline  int                  `json:"cases_above_baseline,omitempty"`
	Warnings            []PerformanceWarning `json:"warnings,omitempty"`
}

func GoldenBaselineFilename(caseName, modelName string) string {
	return sanitizeName(caseName) + "__" + sanitizeName(modelName) + ".baseline.json"
}

func BaselineFilePath(workspace, suiteName, caseName, modelName string) string {
	return filepath.Join(workspace, "testsuite", "agenttests", "tapes", suiteName, GoldenBaselineFilename(caseName, modelName))
}

func BuildPerformanceBaseline(cr CaseReport, recordedAt time.Time) *PerformanceBaseline {
	baseline := &PerformanceBaseline{
		Model:       strings.TrimSpace(cr.Model),
		RecordedAt:  recordedAt.UTC().Format("2006-01-02"),
		LLMCalls:    cr.TokenUsage.LLMCalls,
		TotalTokens: cr.TokenUsage.TotalTokens,
		DurationMS:  cr.DurationMS,
	}
	if len(cr.PhaseMetrics) > 0 {
		baseline.Phases = make(map[string]PhaseBaseline, len(cr.PhaseMetrics))
		for _, phase := range cr.PhaseMetrics {
			if strings.TrimSpace(phase.Phase) == "" {
				continue
			}
			baseline.Phases[phase.Phase] = PhaseBaseline{
				LLMCalls:   phase.LLMCalls,
				Tokens:     phase.TokensUsed,
				DurationMS: phase.DurationMS,
			}
		}
	}
	return baseline
}

func WritePerformanceBaseline(path string, baseline *PerformanceBaseline) error {
	if baseline == nil {
		return fmt.Errorf("baseline required")
	}
	data, err := json.MarshalIndent(baseline, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func LoadPerformanceBaseline(path string) (*PerformanceBaseline, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var baseline PerformanceBaseline
	if err := json.Unmarshal(data, &baseline); err != nil {
		return nil, err
	}
	return &baseline, nil
}

func BuildPhaseMetrics(snapshot *core.ContextSnapshot, total TokenUsageReport) []PhaseMetric {
	phases := collectPerformancePhases(snapshot)
	if len(phases) == 0 {
		return nil
	}

	metrics := make([]PhaseMetric, 0, len(phases))
	durationWeights := make([]int64, 0, len(phases))
	var totalDuration int64
	for _, phase := range phases {
		durationMS := phaseDurationMS(snapshot, phase)
		metrics = append(metrics, PhaseMetric{
			Phase:      phase,
			DurationMS: durationMS,
		})
		durationWeights = append(durationWeights, durationMS)
		totalDuration += durationMS
	}

	if len(metrics) == 1 {
		metrics[0].LLMCalls = total.LLMCalls
		metrics[0].TokensUsed = total.TotalTokens
		return metrics
	}
	if totalDuration > 0 {
		llmAlloc := allocateIntAcrossPhases(durationWeights, total.LLMCalls)
		tokenAlloc := allocateIntAcrossPhases(durationWeights, total.TotalTokens)
		for i := range metrics {
			metrics[i].LLMCalls = llmAlloc[i]
			metrics[i].TokensUsed = tokenAlloc[i]
		}
		return metrics
	}

	if len(metrics) > 0 {
		metrics[len(metrics)-1].LLMCalls = total.LLMCalls
		metrics[len(metrics)-1].TokensUsed = total.TotalTokens
	}
	return metrics
}

func ComparePerformanceBaseline(actual CaseReport, baseline *PerformanceBaseline) []PerformanceWarning {
	if baseline == nil {
		return nil
	}
	var warnings []PerformanceWarning
	if exceededThreshold(actual.TokenUsage.LLMCalls, baseline.LLMCalls, performanceLLMCallThreshold) {
		warnings = append(warnings, PerformanceWarning{
			Metric:   "llm_calls",
			Actual:   int64(actual.TokenUsage.LLMCalls),
			Baseline: int64(baseline.LLMCalls),
			Detail:   fmt.Sprintf("%s: %d vs baseline %d", actual.Name, actual.TokenUsage.LLMCalls, baseline.LLMCalls),
		})
	}
	if exceededThreshold(actual.TokenUsage.TotalTokens, baseline.TotalTokens, performanceTokenThreshold) {
		warnings = append(warnings, PerformanceWarning{
			Metric:   "total_tokens",
			Actual:   int64(actual.TokenUsage.TotalTokens),
			Baseline: int64(baseline.TotalTokens),
			Detail:   fmt.Sprintf("%s: %d vs baseline %d", actual.Name, actual.TokenUsage.TotalTokens, baseline.TotalTokens),
		})
	}
	if exceededThreshold64(actual.DurationMS, baseline.DurationMS, performanceDurationThreshold) {
		warnings = append(warnings, PerformanceWarning{
			Metric:   "duration_ms",
			Actual:   actual.DurationMS,
			Baseline: baseline.DurationMS,
			Detail:   fmt.Sprintf("%s: %dms vs baseline %dms", actual.Name, actual.DurationMS, baseline.DurationMS),
		})
	}
	return warnings
}

func SummarizePerformance(cases []CaseReport) PerformanceSummary {
	var summary PerformanceSummary
	for _, cr := range cases {
		if !cr.BaselineFound {
			continue
		}
		summary.CasesWithBaseline++
		if len(cr.PerformanceWarnings) == 0 {
			summary.CasesWithinBaseline++
			continue
		}
		summary.CasesAboveBaseline++
		summary.Warnings = append(summary.Warnings, cr.PerformanceWarnings...)
	}
	return summary
}

func collectPerformancePhases(snapshot *core.ContextSnapshot) []string {
	if snapshot == nil {
		return nil
	}
	seen := map[string]struct{}{}
	var phases []string
	appendPhase := func(phase string) {
		phase = strings.TrimSpace(phase)
		if phase == "" {
			return
		}
		if _, ok := seen[phase]; ok {
			return
		}
		seen[phase] = struct{}{}
		phases = append(phases, phase)
	}

	interactionState := toStringAnyMap(snapshot.State["euclo.interaction_state"])
	for _, phase := range toStringSlice(interactionState["phases_executed"]) {
		appendPhase(phase)
	}
	for _, raw := range toAnySlice(snapshot.State["euclo.profile_phase_records"]) {
		record := toStringAnyMap(raw)
		appendPhase(toString(record["phase"]))
	}
	for _, raw := range toAnySlice(snapshot.State["euclo.interaction_records"]) {
		record := toStringAnyMap(raw)
		appendPhase(toString(record["phase"]))
	}
	return phases
}

func shouldComparePerformanceBaseline(recordingMode string) bool {
	switch strings.ToLower(strings.TrimSpace(recordingMode)) {
	case "replay", "replay-only":
		return false
	default:
		return true
	}
}

func phaseDurationMS(snapshot *core.ContextSnapshot, phase string) int64 {
	if snapshot == nil || strings.TrimSpace(phase) == "" {
		return 0
	}
	for _, raw := range toAnySlice(snapshot.State["euclo.interaction_records"]) {
		record := toStringAnyMap(raw)
		if !strings.EqualFold(strings.TrimSpace(toString(record["phase"])), strings.TrimSpace(phase)) {
			continue
		}
		duration := strings.TrimSpace(toString(record["duration"]))
		if duration == "" {
			return 0
		}
		if parsed, err := time.ParseDuration(duration); err == nil {
			return parsed.Milliseconds()
		}
	}
	return 0
}

func allocateIntAcrossPhases(weights []int64, total int) []int {
	out := make([]int, len(weights))
	if len(weights) == 0 || total <= 0 {
		return out
	}
	var weightSum int64
	for _, weight := range weights {
		weightSum += weight
	}
	if weightSum <= 0 {
		out[len(out)-1] = total
		return out
	}

	type remainder struct {
		index     int
		remainder int64
	}
	remainders := make([]remainder, 0, len(weights))
	assigned := 0
	for i, weight := range weights {
		scaled := int64(total) * weight
		value := int(scaled / weightSum)
		out[i] = value
		assigned += value
		remainders = append(remainders, remainder{index: i, remainder: scaled % weightSum})
	}
	sort.Slice(remainders, func(i, j int) bool {
		if remainders[i].remainder == remainders[j].remainder {
			return remainders[i].index < remainders[j].index
		}
		return remainders[i].remainder > remainders[j].remainder
	})
	for remaining := total - assigned; remaining > 0 && len(remainders) > 0; remaining-- {
		out[remainders[0].index]++
		remainders = append(remainders[1:], remainders[0])
	}
	return out
}

func exceededThreshold(actual, baseline int, multiplier float64) bool {
	if baseline <= 0 || actual <= baseline {
		return false
	}
	return float64(actual) > float64(baseline)*multiplier
}

func exceededThreshold64(actual, baseline int64, multiplier float64) bool {
	if baseline <= 0 || actual <= baseline {
		return false
	}
	return float64(actual) > float64(baseline)*multiplier
}
