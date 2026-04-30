package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/platform/llm"
	"codeburg.org/lexbit/relurpify/testsuite/agenttest"
)

func promoteAgentTestRun(workspace, suitePath, runDir, caseName string, all bool, stdout io.Writer) error {
	suite, err := agenttest.LoadSuite(suitePath)
	if err != nil {
		return err
	}
	report, err := loadSuiteReport(filepath.Join(runDir, "report.json"))
	if err != nil {
		return err
	}
	targetCases := selectPromotableCases(report, caseName, all)
	if len(targetCases) == 0 {
		return fmt.Errorf("no promotable cases found in run %s", runDir)
	}
	for _, cr := range targetCases {
		if cr.Skipped || !cr.Success {
			return fmt.Errorf("case %q did not pass in run %s", cr.Name, runDir)
		}
		if !agentTestSurface.PromoteAllowed(suite.Metadata.Classification) {
			return fmt.Errorf("case %q is not promotable for suite classification %q", cr.Name, suite.Metadata.Classification)
		}
		srcTape := filepath.Join(cr.ArtifactsDir, "tape.jsonl")
		header, err := readTapeHeader(srcTape)
		if err != nil {
			return fmt.Errorf("case %q tape invalid: %w", cr.Name, err)
		}
		if header == nil {
			return fmt.Errorf("case %q tape has no header", cr.Name)
		}
		if strings.TrimSpace(header.ModelName) != "" && strings.TrimSpace(cr.Model) != "" && strings.TrimSpace(header.ModelName) != strings.TrimSpace(cr.Model) {
			return fmt.Errorf("case %q tape header model %q does not match report model %q", cr.Name, header.ModelName, cr.Model)
		}
		destTape := agentTestSurface.TapePath(workspace, suite.Metadata.Name, cr.Name, cr.Model)
		if err := os.MkdirAll(filepath.Dir(destTape), 0o755); err != nil {
			return err
		}
		if err := copyFile(srcTape, destTape); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "promoted %s -> %s\n", srcTape, destTape)
		if err := promoteSuiteLayerArtifacts(suite, cr, runDir, destTape, stdout); err != nil {
			return err
		}
		srcInteractionTape := filepath.Join(cr.ArtifactsDir, "interaction.tape.jsonl")
		if _, err := os.Stat(srcInteractionTape); err == nil {
			destInteractionTape := strings.TrimSuffix(destTape, ".tape.jsonl") + ".interaction.tape.jsonl"
			if err := copyFile(srcInteractionTape, destInteractionTape); err != nil {
				return err
			}
			fmt.Fprintf(stdout, "promoted %s -> %s\n", srcInteractionTape, destInteractionTape)
		}
		if err := writePromotionLineage(filepath.Dir(destTape), suite, cr, destTape); err != nil {
			return err
		}
	}
	return nil
}

func promoteSuiteLayerArtifacts(suite *agenttest.Suite, cr agenttest.CaseReport, runDir, destTape string, stdout io.Writer) error {
	switch strings.ToLower(strings.TrimSpace(suite.Metadata.Classification)) {
	case "benchmark":
		destBaseline := filepath.Join(filepath.Dir(destTape), agenttest.GoldenBaselineFilename(cr.Name, cr.Model))
		if baseline := agenttest.BuildPerformanceBaseline(cr, cr.FinishedAt); baseline != nil {
			if err := agenttest.WritePerformanceBaseline(destBaseline, baseline); err != nil {
				return err
			}
			fmt.Fprintf(stdout, "promoted baseline %s\n", destBaseline)
		}
		runBenchmarkReport := filepath.Join(runDir, "benchmark_report.json")
		if _, err := os.Stat(runBenchmarkReport); err == nil {
			destBenchmarkReport := filepath.Join(filepath.Dir(destTape), "benchmark_report.json")
			if err := copyFile(runBenchmarkReport, destBenchmarkReport); err != nil {
				return err
			}
			fmt.Fprintf(stdout, "promoted %s -> %s\n", runBenchmarkReport, destBenchmarkReport)
		}
		for _, artifact := range []string{"benchmark_score.json", "benchmark_comparison.json"} {
			src := filepath.Join(cr.ArtifactsDir, artifact)
			if _, err := os.Stat(src); err == nil {
				dst := filepath.Join(filepath.Dir(destTape), artifact)
				if err := copyFile(src, dst); err != nil {
					return err
				}
				fmt.Fprintf(stdout, "promoted %s -> %s\n", src, dst)
			}
		}
	case "journey":
		// Journey promotion is tape-lineage only; interaction tapes are already copied above.
		return nil
	default:
		destBaseline := filepath.Join(filepath.Dir(destTape), agenttest.GoldenBaselineFilename(cr.Name, cr.Model))
		if baseline := agenttest.BuildPerformanceBaseline(cr, cr.FinishedAt); baseline != nil {
			if err := agenttest.WritePerformanceBaseline(destBaseline, baseline); err != nil {
				return err
			}
			fmt.Fprintf(stdout, "promoted baseline %s\n", destBaseline)
		}
	}
	return nil
}

type promotionLineageRecord struct {
	SuiteName         string    `json:"suite_name"`
	SuitePath         string    `json:"suite_path"`
	Classification    string    `json:"classification"`
	CaseName          string    `json:"case_name"`
	Model             string    `json:"model"`
	Provider          string    `json:"provider,omitempty"`
	Layer             string    `json:"layer"`
	PromotedArtifacts []string  `json:"promoted_artifacts"`
	SourceRunDir      string    `json:"source_run_dir"`
	SourceArtifacts   string    `json:"source_artifacts"`
	DestinationTape   string    `json:"destination_tape,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
}

func writePromotionLineage(destDir string, suite *agenttest.Suite, cr agenttest.CaseReport, destTape string) error {
	if suite == nil {
		return nil
	}
	record := promotionLineageRecord{
		SuiteName:         suite.Metadata.Name,
		SuitePath:         suite.SourcePath,
		Classification:    suite.Metadata.Classification,
		CaseName:          cr.Name,
		Model:             cr.Model,
		Provider:          cr.Provider,
		Layer:             strings.TrimSpace(suite.Metadata.Classification),
		PromotedArtifacts: agentTestSurface.PromotedArtifacts(suite.Metadata.Classification, cr),
		SourceRunDir:      filepath.Dir(cr.ArtifactsDir),
		SourceArtifacts:   cr.ArtifactsDir,
		DestinationTape:   destTape,
		CreatedAt:         time.Now().UTC(),
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(destDir, agentTestSurface.PromotionLineageFilename(cr.Name, cr.Model)), data, 0o644)
}

func loadSuiteReport(path string) (*agenttest.SuiteReport, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var report agenttest.SuiteReport
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, err
	}
	return &report, nil
}

func selectPromotableCases(report *agenttest.SuiteReport, caseName string, all bool) []agenttest.CaseReport {
	if report == nil {
		return nil
	}
	if all {
		out := append([]agenttest.CaseReport(nil), report.Cases...)
		sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
		return out
	}
	for _, c := range report.Cases {
		if c.Name == caseName {
			return []agenttest.CaseReport{c}
		}
	}
	return nil
}

func readTapeHeader(path string) (*llm.TapeHeader, error) {
	inspection, err := llm.InspectTape(path)
	if err != nil {
		return nil, err
	}
	return inspection.Header, nil
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}
