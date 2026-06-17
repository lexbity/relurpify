package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/platform/fs"
	"codeburg.org/lexbit/relurpify/testsuite/agenttest"
)

const (
	testRunName       = "run"
	testPromoteName   = "promote"
	testReportName    = "report"
	testRerecordName  = "rerecord"
	testSuiteFlagName = "--suite"
	testHeaderKind    = "kind"
	smokeCaseName     = "smoke"
	freshCaseName     = "fresh"
	staleCaseName     = "stale"
	missingCaseName   = "missing"
	modelName         = "model-a"
)

func TestAgentTestPromoteCommandPromotesRun(t *testing.T) {
	prevWorkspace := workspace
	workspace = ""
	t.Cleanup(func() { workspace = prevWorkspace })

	ws := t.TempDir()
	suitePath := writeTapeSuiteFixture(t, ws, "smoke-suite", "capability", []string{smokeCaseName}, []string{"gemma4:12b"})
	runDir := filepath.Join(ws, "runs", "run-1")
	artifactsDir := filepath.Join(runDir, "artifacts", sanitizeName("smoke")+"__"+sanitizeName("gemma4:12b"))
	if err := fs.MkdirAllSecure(artifactsDir); err != nil {
		t.Fatal(err)
	}
	recordedAt := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	writeTapeJSONL(t, filepath.Join(artifactsDir, "tape.jsonl"), "smoke-suite", "smoke", "gemma4:12b", recordedAt)
	if err := os.WriteFile(filepath.Join(artifactsDir, "interaction.tape.jsonl"), []byte("interaction"), fs.PublicFileMode); err != nil {
		t.Fatal(err)
	}
	writeSuiteReportJSON(t, filepath.Join(runDir, "report.json"), agenttest.SuiteReport{
		Cases: []agenttest.CaseReport{{
			Name:         smokeCaseName,
			Model:        "gemma4:12b",
			Provider:     "ollama",
			Success:      true,
			FinishedAt:   recordedAt,
			ArtifactsDir: artifactsDir,
		}},
	})

	var stdout bytes.Buffer
	cmd := NewRootCmd()
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)
	cmd.SetArgs([]string{workspaceFlagName, ws, rootAgentTestName, testPromoteName, testSuiteFlagName, suitePath, "--run", runDir, "--case", smokeCaseName})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "promoted ") || !strings.Contains(out, "wrote lineage") {
		t.Fatalf("unexpected promote output:\n%s", out)
	}
	if _, err := os.Stat(agenttest.GoldenTapePath(suitePath, "smoke-suite", smokeCaseName, "gemma4:12b")); err != nil {
		t.Fatalf("promoted tape missing: %v", err)
	}
}

func TestAgentTestReportCommandPrintsCoverage(t *testing.T) {
	prevWorkspace := workspace
	workspace = ""
	t.Cleanup(func() { workspace = prevWorkspace })

	ws := t.TempDir()
	suitePath := writeTapeSuiteFixture(t, ws, "coverage-suite", "", []string{freshCaseName, staleCaseName, missingCaseName}, []string{modelName})
	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	freshAt := now.Add(-24 * time.Hour)
	staleAt := now.Add(-45 * 24 * time.Hour)
	writeTapeJSONL(t, agenttest.GoldenTapePath(suitePath, "coverage-suite", freshCaseName, modelName), "coverage-suite", freshCaseName, modelName, freshAt)
	writeTapeJSONL(t, agenttest.GoldenTapePath(suitePath, "coverage-suite", staleCaseName, modelName), "coverage-suite", staleCaseName, modelName, staleAt)
	writeBaselineFixture(t, agenttest.BaselineFilePath(ws, "coverage-suite", freshCaseName, modelName), freshAt)
	writeBaselineFixture(t, agenttest.BaselineFilePath(ws, "coverage-suite", staleCaseName, modelName), staleAt)

	var stdout bytes.Buffer
	cmd := NewRootCmd()
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)
	cmd.SetArgs([]string{workspaceFlagName, ws, rootAgentTestName, testReportName, testSuiteFlagName, suitePath})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "Totals: 1 suites, 3 cases, 2 present, 1 missing, 1 stale") {
		t.Fatalf("unexpected report output:\n%s", out)
	}
	if !strings.Contains(out, "Suite: coverage-suite") || !strings.Contains(out, "fresh / model-a") || !strings.Contains(out, "missing / model-a") {
		t.Fatalf("unexpected report output:\n%s", out)
	}
}

func TestAgentTestRerecordCommandPrintsPlan(t *testing.T) {
	prevWorkspace := workspace
	workspace = ""
	t.Cleanup(func() { workspace = prevWorkspace })

	ws := t.TempDir()
	suitePath := writeTapeSuiteFixture(t, ws, "coverage-suite", "", []string{freshCaseName, staleCaseName, missingCaseName}, []string{modelName})
	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	freshAt := now.Add(-24 * time.Hour)
	staleAt := now.Add(-45 * 24 * time.Hour)
	writeTapeJSONL(t, agenttest.GoldenTapePath(suitePath, "coverage-suite", freshCaseName, modelName), "coverage-suite", freshCaseName, modelName, freshAt)
	writeTapeJSONL(t, agenttest.GoldenTapePath(suitePath, "coverage-suite", staleCaseName, modelName), "coverage-suite", staleCaseName, modelName, staleAt)
	writeBaselineFixture(t, agenttest.BaselineFilePath(ws, "coverage-suite", freshCaseName, modelName), freshAt)
	writeBaselineFixture(t, agenttest.BaselineFilePath(ws, "coverage-suite", staleCaseName, modelName), staleAt)

	var stdout bytes.Buffer
	cmd := NewRootCmd()
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)
	cmd.SetArgs([]string{workspaceFlagName, ws, rootAgentTestName, testRerecordName, testSuiteFlagName, suitePath})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "Rerecord targets: 2") || !strings.Contains(out, "missing golden tape") || !strings.Contains(out, "stale golden tape") {
		t.Fatalf("unexpected rerecord output:\n%s", out)
	}
	if !strings.Contains(out, "agenttest rerecord --suite coverage-suite --case missing --model model-a") {
		t.Fatalf("expected rerecord command in output:\n%s", out)
	}
}

func writeTapeSuiteFixture(t *testing.T, workspace, suiteName, classification string, cases []string, models []string) string {
	t.Helper()
	path := filepath.Join(workspace, suiteName+".testsuite.yaml")
	var caseLines strings.Builder
	for _, c := range cases {
		caseLines.WriteString("    - name: ")
		caseLines.WriteString(c)
		caseLines.WriteString("\n      prompt: hello\n")
	}
	var modelLines strings.Builder
	for _, m := range models {
		modelLines.WriteString("    - name: ")
		modelLines.WriteString(m)
		modelLines.WriteString("\n")
	}
	data := strings.TrimSpace(`
apiVersion: relurpify.codeburg.org/v1
kind: AgentTestSuite
metadata:
  name: ` + suiteName + `
`)
	if strings.TrimSpace(classification) != "" {
		data += "\n  classification: " + classification + "\n"
	}
	data += `
spec:
  agent_name: euclo
  manifest: relurpify_cfg/agents/euclo.yaml
  models:
`
	data += modelLines.String()
	data += "  cases:\n"
	data += caseLines.String()
	if err := os.WriteFile(path, []byte(data), fs.PublicFileMode); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeTapeJSONL(t *testing.T, path, suiteName, caseName, modelName string, recordedAt time.Time) {
	t.Helper()
	if err := fs.MkdirAllSecure(filepath.Dir(path)); err != nil {
		t.Fatal(err)
	}
	header := map[string]any{
		"timestamp":   recordedAt.UTC().Format(time.RFC3339),
		"kind":        "_header",
		"fingerprint": "",
		"request": map[string]any{
			"header": map[string]any{
				testHeaderKind:      "tape",
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
		"kind":        "chat",
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

func writeSuiteReportJSON(t *testing.T, path string, report agenttest.SuiteReport) {
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
