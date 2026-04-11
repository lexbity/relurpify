package agenttest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type BenchmarkBaseline struct {
	SuiteName         string             `json:"suite_name"`
	RecordedAt        string             `json:"recorded_at,omitempty"`
	ScoreFamily       string             `json:"score_family,omitempty"`
	ScoreDimensions   []string           `json:"score_dimensions,omitempty"`
	ComparisonWindow  string             `json:"comparison_window,omitempty"`
	VarianceThreshold float64            `json:"variance_threshold,omitempty"`
	OverallScore      float64            `json:"overall_score"`
	DimensionScores   map[string]float64 `json:"dimension_scores,omitempty"`
}

type BenchmarkComparisonReport struct {
	RunClass          string  `json:"run_class"`
	SuiteName         string  `json:"suite_name,omitempty"`
	BaselinePath      string  `json:"baseline_path,omitempty"`
	BaselineFound     bool    `json:"baseline_found"`
	ComparisonWindow  string  `json:"comparison_window,omitempty"`
	VarianceThreshold float64 `json:"variance_threshold,omitempty"`
	BaselineScore     float64 `json:"baseline_score,omitempty"`
	ActualScore       float64 `json:"actual_score,omitempty"`
	Delta             float64 `json:"delta,omitempty"`
	WithinVariance    bool    `json:"within_variance"`
	Message           string  `json:"message,omitempty"`
}

type BenchmarkCaseDimensionScore struct {
	Name   string  `json:"name"`
	Weight float64 `json:"weight"`
	Score  float64 `json:"score"`
	Detail string  `json:"detail,omitempty"`
}

type BenchmarkCaseScore struct {
	RunClass        string                        `json:"run_class"`
	Name            string                        `json:"name"`
	Model           string                        `json:"model,omitempty"`
	Provider        string                        `json:"provider,omitempty"`
	Endpoint        string                        `json:"endpoint,omitempty"`
	ScoreFamily     string                        `json:"score_family,omitempty"`
	OverallScore    float64                       `json:"overall_score"`
	DimensionScores []BenchmarkCaseDimensionScore `json:"dimension_scores,omitempty"`
	ArtifactPaths   []string                      `json:"artifact_paths,omitempty"`
	ArtifactChecks  map[string]bool               `json:"artifact_checks,omitempty"`
	BaselinePath    string                        `json:"baseline_path,omitempty"`
	BaselineFound   bool                          `json:"baseline_found"`
	BaselineScore   float64                       `json:"baseline_score,omitempty"`
	ScoreDelta      float64                       `json:"score_delta,omitempty"`
	WithinVariance  bool                          `json:"within_variance"`
	Message         string                        `json:"message,omitempty"`
}

type BenchmarkSummary struct {
	TotalCases       int                `json:"total_cases"`
	ScoredCases      int                `json:"scored_cases"`
	PassedCases      int                `json:"passed_cases"`
	FailedCases      int                `json:"failed_cases"`
	SuccessRate      float64            `json:"success_rate,omitempty"`
	OverallScore     float64            `json:"overall_score,omitempty"`
	DimensionScores  map[string]float64 `json:"dimension_scores,omitempty"`
	UniqueModels     []string           `json:"unique_models,omitempty"`
	UniqueProviders  []string           `json:"unique_providers,omitempty"`
	ArtifactCoverage map[string]float64 `json:"artifact_coverage,omitempty"`
}

type BenchmarkReport struct {
	RunClass          string                    `json:"run_class"`
	Workspace         string                    `json:"workspace,omitempty"`
	SuitePath         string                    `json:"suite_path,omitempty"`
	SuiteName         string                    `json:"suite_name,omitempty"`
	ScoreFamily       string                    `json:"score_family,omitempty"`
	ScoreDimensions   []string                  `json:"score_dimensions,omitempty"`
	ComparisonWindow  string                    `json:"comparison_window,omitempty"`
	VarianceThreshold float64                   `json:"variance_threshold,omitempty"`
	Summary           BenchmarkSummary          `json:"summary"`
	Cases             []BenchmarkCaseScore      `json:"cases,omitempty"`
	Comparison        BenchmarkComparisonReport `json:"comparison"`
	Success           bool                      `json:"success"`
}

var benchmarkDimensionWeights = map[string]map[string]float64{
	"capability": {
		"completion":         0.50,
		"artifact_integrity": 0.20,
		"stability":          0.30,
	},
	"journey": {
		"completion":         0.35,
		"recovery":           0.25,
		"artifact_integrity": 0.20,
		"stability":          0.20,
	},
	"artifact": {
		"artifact_integrity": 0.55,
		"completion":         0.20,
		"stability":          0.25,
	},
	"recovery": {
		"recovery":   0.50,
		"completion": 0.25,
		"stability":  0.25,
	},
	"context-pressure": {
		"context_pressure":   0.55,
		"completion":         0.20,
		"artifact_integrity": 0.15,
		"stability":          0.10,
	},
	"provider-stability": {
		"provider_stability": 0.55,
		"completion":         0.20,
		"artifact_integrity": 0.10,
		"recovery":           0.15,
	},
}

func BenchmarkBaselineFilePath(workspace, suiteName string) string {
	return filepath.Join(workspace, "testsuite", "agenttests", "tapes", suiteName, "benchmark.baseline.json")
}

func BenchmarkReportFilePath(outDir string) string {
	return filepath.Join(outDir, "benchmark_report.json")
}

func BenchmarkCaseScoreFilePath(artifactsDir string) string {
	return filepath.Join(artifactsDir, "benchmark_score.json")
}

func BenchmarkCaseComparisonFilePath(artifactsDir string) string {
	return filepath.Join(artifactsDir, "benchmark_comparison.json")
}

func BuildBenchmarkReport(suite *Suite, suiteReport *SuiteReport) (*BenchmarkReport, error) {
	if suite == nil {
		return nil, fmt.Errorf("suite required")
	}
	if suiteReport == nil {
		return nil, fmt.Errorf("suite report required")
	}
	meta := suite.Metadata.Benchmark
	family := strings.TrimSpace(meta.ScoreFamily)
	if family == "" {
		family = benchmarkFamilyForSuite(suite)
	}
	if family == "" {
		family = "capability"
	}
	dimensions := normalizedBenchmarkDimensions(meta.ScoreDimensions)
	if len(dimensions) == 0 {
		dimensions = benchmarkDefaultDimensions(family)
	}
	if len(dimensions) == 0 {
		dimensions = []string{"completion"}
	}
	weights := benchmarkWeightsForFamily(family, dimensions)
	artifactsByName := benchmarkArtifactNames()

	report := &BenchmarkReport{
		RunClass:          "benchmark",
		Workspace:         benchmarkReportWorkspace(suiteReport),
		SuitePath:         suite.SourcePath,
		SuiteName:         suite.Metadata.Name,
		ScoreFamily:       family,
		ScoreDimensions:   append([]string(nil), dimensions...),
		ComparisonWindow:  meta.ComparisonWindow,
		VarianceThreshold: meta.VarianceThreshold,
		Success:           true,
	}
	report.Summary.DimensionScores = map[string]float64{}
	report.Summary.ArtifactCoverage = map[string]float64{}
	report.Comparison = BenchmarkComparisonReport{
		RunClass:          "benchmark",
		SuiteName:         suite.Metadata.Name,
		BaselinePath:      BenchmarkBaselineFilePath(report.Workspace, suite.Metadata.Name),
		ComparisonWindow:  meta.ComparisonWindow,
		VarianceThreshold: meta.VarianceThreshold,
		WithinVariance:    true,
	}

	var totalScore float64
	dimensionTotals := map[string]float64{}
	artifactTotals := map[string]int{}
	modelSet := map[string]struct{}{}
	providerSet := map[string]struct{}{}
	for _, cr := range suiteReport.Cases {
		caseScore := scoreBenchmarkCase(family, weights, artifactsByName, cr)
		report.Cases = append(report.Cases, caseScore)
		if data, err := json.MarshalIndent(caseScore, "", "  "); err == nil {
			_ = os.WriteFile(BenchmarkCaseScoreFilePath(cr.ArtifactsDir), data, 0o644)
		}
		if data, err := json.MarshalIndent(caseScore.comparison(), "", "  "); err == nil {
			_ = os.WriteFile(BenchmarkCaseComparisonFilePath(cr.ArtifactsDir), data, 0o644)
		}
		if cr.Model != "" {
			modelSet[cr.Model] = struct{}{}
		}
		if cr.Provider != "" {
			providerSet[cr.Provider] = struct{}{}
		}
		if !cr.Skipped {
			report.Summary.ScoredCases++
		}
		if cr.Success {
			report.Summary.PassedCases++
		} else {
			report.Summary.FailedCases++
		}
		totalScore += caseScore.OverallScore
		for _, dim := range caseScore.DimensionScores {
			dimensionTotals[dim.Name] += dim.Score
		}
		for name, ok := range caseScore.ArtifactChecks {
			if ok {
				artifactTotals[name]++
			}
		}
		if !cr.Success {
			// The benchmark report records correctness separately from scoring.
		}
	}
	report.Summary.TotalCases = len(suiteReport.Cases)
	if len(report.Cases) > 0 {
		report.Summary.OverallScore = totalScore / float64(len(report.Cases))
		for name, total := range dimensionTotals {
			report.Summary.DimensionScores[name] = total / float64(len(report.Cases))
		}
		for name, total := range artifactTotals {
			report.Summary.ArtifactCoverage[name] = float64(total) / float64(len(report.Cases))
		}
		report.Summary.SuccessRate = float64(report.Summary.PassedCases) / float64(len(report.Cases))
	}
	report.Summary.UniqueModels = sortedSetKeys(modelSet)
	report.Summary.UniqueProviders = sortedSetKeys(providerSet)
	report.Comparison.ActualScore = report.Summary.OverallScore
	if baseline, err := LoadBenchmarkBaseline(report.Comparison.BaselinePath); err == nil && baseline != nil {
		report.Comparison.BaselineFound = true
		report.Comparison.BaselineScore = baseline.OverallScore
		report.Comparison.Delta = report.Comparison.ActualScore - baseline.OverallScore
		if meta.VarianceThreshold > 0 {
			report.Comparison.WithinVariance = absFloat64(report.Comparison.Delta) <= meta.VarianceThreshold
		}
		if report.Comparison.WithinVariance {
			report.Comparison.Message = "within variance threshold"
		} else {
			report.Comparison.Message = "outside variance threshold"
		}
	} else {
		report.Comparison.Message = "baseline missing"
		report.Comparison.WithinVariance = true
	}
	return report, nil
}

func benchmarkReportWorkspace(report *SuiteReport) string {
	if report == nil {
		return ""
	}
	for _, cr := range report.Cases {
		if strings.TrimSpace(cr.Workspace) != "" {
			return cr.Workspace
		}
	}
	return ""
}

func scoreBenchmarkCase(family string, weights map[string]float64, artifacts map[string]struct{}, cr CaseReport) BenchmarkCaseScore {
	score := BenchmarkCaseScore{
		RunClass:       "benchmark",
		Name:           cr.Name,
		Model:          cr.Model,
		Provider:       cr.Provider,
		Endpoint:       cr.Endpoint,
		ScoreFamily:    family,
		ArtifactChecks: map[string]bool{},
		WithinVariance: true,
		Message:        "scored benchmark case",
	}
	if cr.Skipped {
		score.OverallScore = 0
		score.Message = "case skipped"
		return score
	}
	if len(artifacts) > 0 {
		score.ArtifactPaths = make([]string, 0, len(artifacts))
	}
	for name := range artifacts {
		present := benchmarkArtifactExists(cr.ArtifactsDir, name)
		score.ArtifactChecks[name] = present
		if present {
			score.ArtifactPaths = append(score.ArtifactPaths, filepath.Join(cr.ArtifactsDir, artifactFileName(name)))
		}
	}
	sort.Strings(score.ArtifactPaths)
	dimensionScores := benchmarkCaseDimensions(family, cr, score.ArtifactChecks)
	weighted, totalWeight := 0.0, 0.0
	for _, name := range benchmarkDimensionOrder(weights) {
		dimScore := dimensionScores[name]
		weight := weights[name]
		score.DimensionScores = append(score.DimensionScores, BenchmarkCaseDimensionScore{
			Name:   name,
			Weight: weight,
			Score:  dimScore,
			Detail: benchmarkDimensionDetail(name, cr, score.ArtifactChecks),
		})
		weighted += dimScore * weight
		totalWeight += weight
	}
	if totalWeight > 0 {
		score.OverallScore = (weighted / totalWeight) * 100
	}
	score.BaselinePath = BenchmarkCaseBaselineFilePath(cr.ArtifactsDir, cr.Name, cr.Model)
	if baseline, err := LoadBenchmarkBaseline(score.BaselinePath); err == nil && baseline != nil {
		score.BaselineFound = true
		score.BaselineScore = baseline.OverallScore
		score.ScoreDelta = score.OverallScore - baseline.OverallScore
		if baseline.VarianceThreshold > 0 {
			score.WithinVariance = absFloat64(score.ScoreDelta) <= baseline.VarianceThreshold
		}
		if score.WithinVariance {
			score.Message = "within case variance"
		} else {
			score.Message = "outside case variance"
		}
	}
	return score
}

func (c BenchmarkCaseScore) comparison() BenchmarkComparisonReport {
	return BenchmarkComparisonReport{
		RunClass:       c.RunClass,
		BaselinePath:   c.BaselinePath,
		BaselineFound:  c.BaselineFound,
		BaselineScore:  c.BaselineScore,
		ActualScore:    c.OverallScore,
		Delta:          c.ScoreDelta,
		WithinVariance: c.WithinVariance,
		Message:        c.Message,
	}
}

func benchmarkCaseDimensions(family string, cr CaseReport, artifactChecks map[string]bool) map[string]float64 {
	scores := map[string]float64{
		"completion":         benchmarkCompletionScore(cr),
		"artifact_integrity": benchmarkArtifactIntegrityScore(artifactChecks),
		"stability":          benchmarkStabilityScore(cr),
		"recovery":           benchmarkRecoveryScore(cr),
		"context_pressure":   benchmarkContextPressureScore(cr),
		"provider_stability": benchmarkProviderStabilityScore(cr),
	}
	_ = family
	return scores
}

func benchmarkCompletionScore(cr CaseReport) float64 {
	if cr.Skipped {
		return 0
	}
	if cr.Success {
		return 1
	}
	return 0
}

func benchmarkArtifactIntegrityScore(artifactChecks map[string]bool) float64 {
	if len(artifactChecks) == 0 {
		return 0
	}
	var present int
	for _, ok := range artifactChecks {
		if ok {
			present++
		}
	}
	return float64(present) / float64(len(artifactChecks))
}

func benchmarkStabilityScore(cr CaseReport) float64 {
	if !cr.Success || cr.FailureKind == "infra" {
		return 0
	}
	score := 1.0
	if len(cr.PerformanceWarnings) > 0 {
		score -= float64(len(cr.PerformanceWarnings)) * 0.1
	}
	if score < 0 {
		return 0
	}
	return score
}

func benchmarkRecoveryScore(cr CaseReport) float64 {
	if cr.Skipped {
		return 0
	}
	if !cr.Success {
		if cr.RetryCount > 0 {
			return 0.4
		}
		return 0
	}
	if cr.RetryCount > 0 {
		return 1
	}
	return 0.8
}

func benchmarkContextPressureScore(cr CaseReport) float64 {
	if !cr.Success {
		return 0
	}
	if len(cr.PerformanceWarnings) == 0 {
		return 1
	}
	score := 1.0
	for _, warning := range cr.PerformanceWarnings {
		if strings.Contains(warning.Metric, "context_budget") || strings.Contains(warning.Metric, "progressive_file") {
			score -= 0.2
		}
	}
	if score < 0 {
		return 0
	}
	return score
}

func benchmarkProviderStabilityScore(cr CaseReport) float64 {
	if strings.TrimSpace(cr.Model) == "" || strings.TrimSpace(cr.Endpoint) == "" {
		return 0.5
	}
	if cr.FailureKind == "infra" {
		return 0
	}
	return 1
}

func benchmarkDimensionDetail(name string, cr CaseReport, artifactChecks map[string]bool) string {
	switch name {
	case "completion":
		return fmt.Sprintf("success=%t skipped=%t", cr.Success, cr.Skipped)
	case "artifact_integrity":
		return fmt.Sprintf("%d/%d artifacts present", benchmarkCountArtifactsPresent(artifactChecks), len(artifactChecks))
	case "stability":
		return fmt.Sprintf("warnings=%d failure_kind=%s", len(cr.PerformanceWarnings), cr.FailureKind)
	case "recovery":
		return fmt.Sprintf("attempts=%d retries=%d", cr.Attempts, cr.RetryCount)
	case "context_pressure":
		return fmt.Sprintf("warnings=%d duration_ms=%d", len(cr.PerformanceWarnings), cr.DurationMS)
	case "provider_stability":
		return fmt.Sprintf("model=%s endpoint=%s", cr.Model, cr.Endpoint)
	default:
		return ""
	}
}

func benchmarkCountArtifactsPresent(artifactChecks map[string]bool) int {
	count := 0
	for _, ok := range artifactChecks {
		if ok {
			count++
		}
	}
	return count
}

func benchmarkArtifactNames() map[string]struct{} {
	return map[string]struct{}{
		"tape.jsonl":                    {},
		"interaction.tape.jsonl":        {},
		"model.provenance.json":         {},
		"model_profile.provenance.json": {},
		"tool_transcript.json":          {},
		"framework_perf.json":           {},
		"phase_metrics.json":            {},
		"changed_files.json":            {},
	}
}

func artifactFileName(name string) string {
	return name
}

func benchmarkArtifactExists(artifactsDir, name string) bool {
	if strings.TrimSpace(artifactsDir) == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(artifactsDir, artifactFileName(name)))
	return err == nil
}

func benchmarkWeightsForFamily(family string, dimensions []string) map[string]float64 {
	base := benchmarkDimensionWeights[strings.ToLower(strings.TrimSpace(family))]
	if len(base) == 0 {
		base = benchmarkDimensionWeights["capability"]
	}
	weights := make(map[string]float64, len(dimensions))
	for _, dimension := range dimensions {
		if weight, ok := base[dimension]; ok {
			weights[dimension] = weight
			continue
		}
		weights[dimension] = 1
	}
	return weights
}

func benchmarkDefaultDimensions(family string) []string {
	if weights := benchmarkDimensionWeights[strings.ToLower(strings.TrimSpace(family))]; len(weights) > 0 {
		dims := make([]string, 0, len(weights))
		for dimension := range weights {
			dims = append(dims, dimension)
		}
		sort.Strings(dims)
		return dims
	}
	return []string{"completion"}
}

func benchmarkDimensionOrder(weights map[string]float64) []string {
	if len(weights) == 0 {
		return nil
	}
	out := make([]string, 0, len(weights))
	for dimension := range weights {
		out = append(out, dimension)
	}
	sort.Strings(out)
	return out
}

func normalizedBenchmarkDimensions(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	sort.Strings(out)
	return out
}

func benchmarkFamilyForSuite(suite *Suite) string {
	if suite == nil {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(suite.Metadata.Classification)) {
	case "benchmark":
		if strings.Contains(strings.ToLower(suite.Metadata.Name), "context") {
			return "context-pressure"
		}
		if strings.Contains(strings.ToLower(suite.Metadata.Name), "baseline") {
			return "capability"
		}
		return "journey"
	default:
		return ""
	}
}

func BenchmarkCaseBaselineFilePath(artifactsDir, caseName, modelName string) string {
	if strings.TrimSpace(artifactsDir) == "" {
		return ""
	}
	return filepath.Join(artifactsDir, GoldenBaselineFilename(caseName, modelName)+".benchmark.json")
}

func LoadBenchmarkBaseline(path string) (*BenchmarkBaseline, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var baseline BenchmarkBaseline
	if err := json.Unmarshal(data, &baseline); err != nil {
		return nil, err
	}
	return &baseline, nil
}

func WriteBenchmarkBaseline(path string, baseline *BenchmarkBaseline) error {
	if baseline == nil {
		return fmt.Errorf("benchmark baseline required")
	}
	data, err := json.MarshalIndent(baseline, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func BuildBenchmarkBaseline(report *BenchmarkReport) *BenchmarkBaseline {
	if report == nil {
		return nil
	}
	return &BenchmarkBaseline{
		SuiteName:         report.SuiteName,
		RecordedAt:        time.Now().UTC().Format(time.RFC3339),
		ScoreFamily:       report.ScoreFamily,
		ScoreDimensions:   append([]string(nil), report.ScoreDimensions...),
		ComparisonWindow:  report.ComparisonWindow,
		VarianceThreshold: report.VarianceThreshold,
		OverallScore:      report.Summary.OverallScore,
		DimensionScores:   copyFloatMap(report.Summary.DimensionScores),
	}
}

func absFloat64(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func copyFloatMap(values map[string]float64) map[string]float64 {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]float64, len(values))
	for k, v := range values {
		out[k] = v
	}
	return out
}

func sortedSetKeys(set map[string]struct{}) []string {
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for key := range set {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}
