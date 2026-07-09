package agenttest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/platform/fs"
	"codeburg.org/lexbit/relurpify/telemetry/perfstats"
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
	Framework   perfstats.Snapshot       `json:"framework,omitempty"`
	// Latency tracking for tool execution.
	ToolLatencies   map[string]LatencyStats `json:"tool_latencies,omitempty"`
	TotalToolTimeMs int64                   `json:"total_tool_time_ms,omitempty"`
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
	return SanitizeName(caseName) + "__" + SanitizeName(modelName) + ".baseline.json"
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
		Framework:   cr.FrameworkPerf,
		// Latency tracking for tool execution.
		ToolLatencies:   cr.ToolLatencies,
		TotalToolTimeMs: cr.TotalToolTimeMs,
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
	return fs.WriteFileSecure(path, data)
}

func LoadPerformanceBaseline(path string) (*PerformanceBaseline, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	var baseline PerformanceBaseline
	if err := json.Unmarshal(data, &baseline); err != nil {
		return nil, err
	}
	return &baseline, nil
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


