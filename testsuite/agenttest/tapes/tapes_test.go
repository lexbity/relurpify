package tapes

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/platform/fs"
	"codeburg.org/lexbit/relurpify/testsuite/agenttest"
)

const (
	jsonKindKey     = "kind"
	smokeCaseName   = "smoke"
	freshCaseName   = "fresh"
	staleCaseName   = "stale"
	missingCaseName = "missing"
)

func TestPromoteRunCopiesArtifactsAndWritesLineage(t *testing.T) {
	workspace := t.TempDir()
	suitePath := writeSuiteFixture(t, workspace, "smoke-suite", "capability")
	runDir := filepath.Join(workspace, "runs", "run-1")
	artifactsDir := filepath.Join(runDir, "artifacts", caseKey(smokeCaseName, "gemma4:12b"))
	if err := fs.MkdirAllSecure(artifactsDir); err != nil {
		t.Fatal(err)
	}
	recordedAt := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	writeTapeFixture(t, filepath.Join(artifactsDir, "tape.jsonl"), "smoke-suite", smokeCaseName, "gemma4:12b", recordedAt)
	if err := os.WriteFile(filepath.Join(artifactsDir, "interaction.tape.jsonl"), []byte("interaction"), fs.PublicFileMode); err != nil {
		t.Fatal(err)
	}
	report := agenttest.SuiteReport{
		Cases: []agenttest.CaseReport{{
			Name:         smokeCaseName,
			Model:        "gemma4:12b",
			Provider:     "ollama",
			Success:      true,
			FinishedAt:   recordedAt,
			ArtifactsDir: artifactsDir,
		}},
	}
	writeSuiteReport(t, filepath.Join(runDir, "report.json"), report)

	promoted, err := PromoteRun(workspace, suitePath, runDir, "smoke", false)
	if err != nil {
		t.Fatal(err)
	}
	if promoted == nil || len(promoted.Cases) != 1 {
		t.Fatalf("unexpected promote report: %+v", promoted)
	}
	caseReport := promoted.Cases[0]
	wantTape := agenttest.GoldenTapePath(suitePath, "smoke-suite", smokeCaseName, "gemma4:12b")
	if caseReport.DestinationTapePath != wantTape {
		t.Fatalf("destination tape path = %q, want %q", caseReport.DestinationTapePath, wantTape)
	}
	if _, err := os.Stat(wantTape); err != nil {
		t.Fatalf("promoted tape missing: %v", err)
	}
	wantLineage := filepath.Join(filepath.Dir(wantTape), PromotionLineageFilename("smoke", "gemma4:12b"))
	if caseReport.LineagePath != wantLineage {
		t.Fatalf("lineage path = %q, want %q", caseReport.LineagePath, wantLineage)
	}
	if _, err := os.Stat(wantLineage); err != nil {
		t.Fatalf("lineage file missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(wantTape), agenttest.GoldenBaselineFilename(smokeCaseName, "gemma4:12b"))); err != nil {
		t.Fatalf("baseline missing: %v", err)
	}
	if caseReport.DestinationInteractionTapePath == "" {
		t.Fatal("interaction tape destination missing")
	}
	if _, err := os.Stat(caseReport.DestinationInteractionTapePath); err != nil {
		t.Fatalf("interaction tape missing: %v", err)
	}
}

func TestPromoteRunCopiesBenchmarkArtifacts(t *testing.T) {
	workspace := t.TempDir()
	suitePath := writeSuiteFixture(t, workspace, "benchmark-suite", "benchmark")
	runDir := filepath.Join(workspace, "runs", "run-1")
	artifactsDir := filepath.Join(runDir, "artifacts", caseKey(smokeCaseName, "gemma4:12b"))
	if err := fs.MkdirAllSecure(artifactsDir); err != nil {
		t.Fatal(err)
	}
	recordedAt := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	writeTapeFixture(t, filepath.Join(artifactsDir, "tape.jsonl"), "benchmark-suite", smokeCaseName, "gemma4:12b", recordedAt)
	for _, name := range []string{benchmarkReportFilename, benchmarkScoreFilename, benchmarkComparisonFilename} {
		if err := os.WriteFile(filepath.Join(artifactsDir, name), []byte(name), fs.PublicFileMode); err != nil {
			t.Fatal(err)
		}
	}
	writeSuiteReport(t, filepath.Join(runDir, "report.json"), agenttest.SuiteReport{
		Cases: []agenttest.CaseReport{{
			Name:         smokeCaseName,
			Model:        "gemma4:12b",
			Provider:     "ollama",
			Success:      true,
			FinishedAt:   recordedAt,
			ArtifactsDir: artifactsDir,
		}},
	})

	promoted, err := PromoteRun(workspace, suitePath, runDir, "smoke", false)
	if err != nil {
		t.Fatal(err)
	}
	goldenDir := filepath.Dir(agenttest.GoldenTapePath(suitePath, "benchmark-suite", "smoke", "gemma4:12b"))
	for _, name := range []string{benchmarkReportFilename, benchmarkScoreFilename, benchmarkComparisonFilename} {
		if _, err := os.Stat(filepath.Join(goldenDir, name)); err != nil {
			t.Fatalf("%s missing: %v", name, err)
		}
	}
	if len(promoted.Cases) != 1 {
		t.Fatalf("unexpected promote result: %+v", promoted)
	}
}

func TestBuildCoverageAndRerecordPlan(t *testing.T) {
	workspace := t.TempDir()
	suitePath := writeSuiteWithCoverageCases(t, workspace)
	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	freshAt := now.Add(-24 * time.Hour)
	staleAt := now.Add(-45 * 24 * time.Hour)

	freshTape := agenttest.GoldenTapePath(suitePath, "coverage-suite", freshCaseName, "model-a")
	staleTape := agenttest.GoldenTapePath(suitePath, "coverage-suite", staleCaseName, "model-a")
	writeTapeFixture(t, freshTape, "coverage-suite", freshCaseName, "model-a", freshAt)
	writeTapeFixture(t, staleTape, "coverage-suite", staleCaseName, "model-a", staleAt)
	writeBaselineFixture(t, agenttest.BaselineFilePath(workspace, "coverage-suite", freshCaseName, "model-a"), freshAt)
	writeBaselineFixture(t, agenttest.BaselineFilePath(workspace, "coverage-suite", staleCaseName, "model-a"), staleAt)

	coverage, err := BuildCoverageReport(workspace, []string{suitePath}, now)
	if err != nil {
		t.Fatal(err)
	}
	if coverage.Totals.Suites != 1 {
		t.Fatalf("totals suites = %d", coverage.Totals.Suites)
	}
	if coverage.Totals.Cases != 3 {
		t.Fatalf("totals cases = %d", coverage.Totals.Cases)
	}
	if coverage.Totals.Present != 2 || coverage.Totals.Missing != 1 || coverage.Totals.Stale != 1 {
		t.Fatalf("unexpected totals: %+v", coverage.Totals)
	}
	if len(coverage.Missing) != 1 || coverage.Missing[0].CaseName != missingCaseName {
		t.Fatalf("unexpected missing coverage: %+v", coverage.Missing)
	}
	if len(coverage.Stale) != 1 || coverage.Stale[0].CaseName != staleCaseName {
		t.Fatalf("unexpected stale coverage: %+v", coverage.Stale)
	}
	freshEntry := findCoverageEntry(t, coverage, freshCaseName)
	if freshEntry.BaselineStatus != "baseline fresh" {
		t.Fatalf("fresh baseline status = %q", freshEntry.BaselineStatus)
	}
	staleEntry := findCoverageEntry(t, coverage, staleCaseName)
	if staleEntry.Status != tapeEntryStaleStatus {
		t.Fatalf("stale entry status = %q", staleEntry.Status)
	}

	plan, err := BuildRerecordPlan(workspace, []string{suitePath}, now)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Totals.Entries != 2 || plan.Totals.Missing != 1 || plan.Totals.Stale != 1 {
		t.Fatalf("unexpected rerecord totals: %+v", plan.Totals)
	}
	if len(plan.Entries) != 2 {
		t.Fatalf("unexpected rerecord entries: %+v", plan.Entries)
	}
	if !strings.Contains(plan.Entries[0].SuggestedCommand, "agenttest rerecord") {
		t.Fatalf("unexpected rerecord command: %q", plan.Entries[0].SuggestedCommand)
	}
}

func writeSuiteFixture(t *testing.T, workspace, suiteName, classification string) string {
	t.Helper()
	path := filepath.Join(workspace, "suite.yaml")
	data := fmt.Sprintf(`apiVersion: relurpify.codeburg.org/v1
kind: AgentTestSuite
metadata:
  name: %s
  classification: %s
spec:
  agent_name: euclo
  manifest: relurpify_cfg/agents/euclo.yaml
  models:
    - name: gemma4:12b
  cases:
    - name: smoke
      prompt: hello
`, suiteName, classification)
	if err := os.WriteFile(path, []byte(data), fs.PublicFileMode); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeSuiteWithCoverageCases(t *testing.T, workspace string) string {
	t.Helper()
	path := filepath.Join(workspace, "coverage-suite.yaml")
	data := fmt.Sprintf(`apiVersion: relurpify.codeburg.org/v1
kind: AgentTestSuite
metadata:
  name: coverage-suite
spec:
  agent_name: euclo
  manifest: relurpify_cfg/agents/euclo.yaml
  models:
    - name: model-a
  cases:
    - name: %s
      prompt: hello
    - name: %s
      prompt: hello
    - name: %s
      prompt: hello
`, freshCaseName, staleCaseName, missingCaseName)
	if err := os.WriteFile(path, []byte(data), fs.PublicFileMode); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeTapeFixture(t *testing.T, path, suiteName, caseName, modelName string, recordedAt time.Time) {
	t.Helper()
	if err := fs.MkdirAllSecure(filepath.Dir(path)); err != nil {
		t.Fatal(err)
	}
	header := map[string]any{
		"timestamp":   recordedAt.UTC().Format(time.RFC3339),
		jsonKindKey:   "_header",
		"fingerprint": "",
		"request": map[string]any{
			"header": map[string]any{
				jsonKindKey:         "tape",
				"model_name":        modelName,
				"recorded_at":       recordedAt.UTC().Format(time.RFC3339),
				"suite_name":        suiteName,
				"case_name":         caseName,
				"framework_version": "test",
			},
		},
	}
	entry := map[string]any{
		"timestamp":   recordedAt.UTC().Format(time.RFC3339),
		jsonKindKey:   "chat",
		"fingerprint": "abc123",
		"request": map[string]any{
			"prompt": "hello",
		},
	}
	data1, err := json.Marshal(header)
	if err != nil {
		t.Fatal(err)
	}
	data2, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	content := append(append(data1, '\n'), append(data2, '\n')...)
	if err := os.WriteFile(path, content, fs.PublicFileMode); err != nil {
		t.Fatal(err)
	}
}

func writeBaselineFixture(t *testing.T, path string, recordedAt time.Time) {
	t.Helper()
	if err := fs.MkdirAllSecure(filepath.Dir(path)); err != nil {
		t.Fatal(err)
	}
	if err := agenttest.WritePerformanceBaseline(path, &agenttest.PerformanceBaseline{
		Model:       "model-a",
		RecordedAt:  recordedAt.UTC().Format("2006-01-02"),
		LLMCalls:    1,
		TotalTokens: 1,
		DurationMS:  1,
	}); err != nil {
		t.Fatal(err)
	}
}

func writeSuiteReport(t *testing.T, path string, report agenttest.SuiteReport) {
	t.Helper()
	if err := fs.MkdirAllSecure(filepath.Dir(path)); err != nil {
		t.Fatal(err)
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, fs.PublicFileMode); err != nil {
		t.Fatal(err)
	}
}

func caseKey(caseName, modelName string) string {
	return agenttest.SanitizeName(caseName) + "__" + agenttest.SanitizeName(modelName)
}

func findCoverageEntry(t *testing.T, report *CoverageReport, caseName string) CoverageEntry {
	t.Helper()
	for _, suite := range report.Suites {
		for _, entry := range suite.Entries {
			if entry.CaseName == caseName {
				return entry
			}
		}
	}
	t.Fatalf("coverage entry %q not found", caseName)
	return CoverageEntry{}
}
