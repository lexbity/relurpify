package agenttest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/perfstats"
)

func TestBuildPhaseMetricsAllocatesAcrossRecordedDurations(t *testing.T) {
	snapshot := &core.ContextSnapshot{
		State: map[string]any{
			"euclo.interaction_state": map[string]any{
				"phases_executed": []any{"scope", "generate"},
			},
			"euclo.interaction_records": []any{
				map[string]any{"phase": "scope", "duration": "1s"},
				map[string]any{"phase": "generate", "duration": "3s"},
			},
		},
	}

	metrics := BuildPhaseMetrics(snapshot, TokenUsageReport{
		LLMCalls:    4,
		TotalTokens: 400,
	})
	if len(metrics) != 2 {
		t.Fatalf("expected 2 phase metrics, got %d", len(metrics))
	}
	if metrics[0].Phase != "scope" || metrics[1].Phase != "generate" {
		t.Fatalf("unexpected phase ordering: %+v", metrics)
	}
	if metrics[0].LLMCalls != 1 || metrics[1].LLMCalls != 3 {
		t.Fatalf("unexpected llm allocation: %+v", metrics)
	}
	if metrics[0].TokensUsed != 100 || metrics[1].TokensUsed != 300 {
		t.Fatalf("unexpected token allocation: %+v", metrics)
	}
}

func TestComparePerformanceBaselineWarnsOnRegression(t *testing.T) {
	warnings := ComparePerformanceBaseline(CaseReport{
		Name:          "edit_with_verification",
		DurationMS:    31000,
		FrameworkPerf: perfstats.Snapshot{BranchClones: 4},
		TokenUsage: TokenUsageReport{
			LLMCalls:    7,
			TotalTokens: 5000,
		},
	}, &PerformanceBaseline{
		LLMCalls:    4,
		TotalTokens: 2000,
		DurationMS:  10000,
		Framework:   perfstats.Snapshot{BranchClones: 1},
	})

	if len(warnings) != 4 {
		t.Fatalf("expected 4 warnings, got %+v", warnings)
	}
	if warnings[0].Metric != "llm_calls" {
		t.Fatalf("unexpected first warning: %+v", warnings[0])
	}
	if warnings[3].Metric != "framework_perf.branch_clones" {
		t.Fatalf("unexpected framework warning: %+v", warnings[3])
	}
}

func TestSummarizePerformanceCountsCases(t *testing.T) {
	summary := SummarizePerformance([]CaseReport{
		{BaselineFound: true},
		{BaselineFound: true, PerformanceWarnings: []PerformanceWarning{{Metric: "llm_calls"}}},
		{BaselineFound: false},
	})
	if summary.CasesWithBaseline != 2 || summary.CasesWithinBaseline != 1 || summary.CasesAboveBaseline != 1 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
}

func TestBuildAndWritePerformanceBaseline(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "basic.baseline.json")
	baseline := BuildPerformanceBaseline(CaseReport{
		Name:       "basic_edit_task",
		Model:      "qwen2.5-coder:14b",
		DurationMS: 12000,
		TokenUsage: TokenUsageReport{
			LLMCalls:    4,
			TotalTokens: 3200,
		},
		FrameworkPerf: perfstats.Snapshot{
			ContextBudgetRescanCount: 2,
		},
		PhaseMetrics: []PhaseMetric{{
			Phase:      "execute",
			DurationMS: 6000,
			LLMCalls:   2,
			TokensUsed: 1600,
		}},
	}, time.Date(2026, 3, 18, 0, 0, 0, 0, time.UTC))

	if err := WritePerformanceBaseline(path, baseline); err != nil {
		t.Fatalf("WritePerformanceBaseline: %v", err)
	}
	loaded, err := LoadPerformanceBaseline(path)
	if err != nil {
		t.Fatalf("LoadPerformanceBaseline: %v", err)
	}
	if loaded.RecordedAt != "2026-03-18" || loaded.Phases["execute"].Tokens != 1600 || loaded.Framework.ContextBudgetRescanCount != 2 {
		t.Fatalf("unexpected loaded baseline: %+v", loaded)
	}
}

func TestComparePerformanceBaselineWarnsOnIntroducedFrameworkMetric(t *testing.T) {
	warnings := ComparePerformanceBaseline(CaseReport{
		Name:          "retrieval_case",
		FrameworkPerf: perfstats.Snapshot{RetrievalCorpusStampCount: 1},
	}, &PerformanceBaseline{})
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %+v", warnings)
	}
	if warnings[0].Metric != "framework_perf.retrieval_corpus_stamp_count" {
		t.Fatalf("unexpected warning: %+v", warnings[0])
	}
}

func TestGoldenBaselineFilename(t *testing.T) {
	got := GoldenBaselineFilename("basic edit task", "qwen2.5-coder:14b")
	if !strings.HasSuffix(got, ".baseline.json") || !strings.Contains(got, "basic_edit_task__qwen2_5_coder_14b") {
		t.Fatalf("unexpected baseline filename %q", got)
	}
}

func TestBuildBenchmarkReportWritesArtifacts(t *testing.T) {
	workspace := t.TempDir()
	artifactsDir := filepath.Join(workspace, "artifacts", "case__model")
	if err := os.MkdirAll(artifactsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"tape.jsonl", "interaction.tape.jsonl", "model.provenance.json"} {
		if err := os.WriteFile(filepath.Join(artifactsDir, name), []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	suite := &Suite{
		SourcePath: "testsuite/agenttests/euclo.performance_context.testsuite.yaml",
		Metadata: SuiteMeta{
			Name:           "euclo.performance_context",
			Classification: "benchmark",
			Benchmark: BenchmarkMeta{
				ScoreFamily:       "context-pressure",
				ScoreDimensions:   []string{"completion", "context_pressure", "artifact_integrity", "stability"},
				ComparisonWindow:  "suite",
				VarianceThreshold: 0.15,
			},
		},
	}
	report, err := BuildBenchmarkReport(suite, &SuiteReport{
		Cases: []CaseReport{{
			Name:         "context_pressure_baseline",
			Model:        "qwen2.5-coder:14b",
			Endpoint:     "http://localhost:11434",
			Workspace:    workspace,
			ArtifactsDir: artifactsDir,
			Success:      true,
		}},
	})
	if err != nil {
		t.Fatalf("BuildBenchmarkReport: %v", err)
	}
	if report == nil || len(report.Cases) != 1 {
		t.Fatalf("unexpected benchmark report: %+v", report)
	}
	if report.ScoreFamily != "context-pressure" {
		t.Fatalf("unexpected score family: %+v", report)
	}
	if _, err := os.Stat(BenchmarkCaseScoreFilePath(artifactsDir)); err != nil {
		t.Fatalf("expected benchmark score artifact: %v", err)
	}
	if _, err := os.Stat(BenchmarkCaseComparisonFilePath(artifactsDir)); err != nil {
		t.Fatalf("expected benchmark comparison artifact: %v", err)
	}
}
