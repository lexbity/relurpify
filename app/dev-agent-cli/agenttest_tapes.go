package main

import (
	"errors"
	"fmt"
	"github.com/lexcodex/relurpify/platform/llm"
	"github.com/lexcodex/relurpify/testsuite/agenttest"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func reportAgentTestTapes(workspace string, suitePaths []string, stdout io.Writer, now time.Time) error {
	for idx, suitePath := range suitePaths {
		suite, err := agenttest.LoadSuite(suitePath)
		if err != nil {
			return err
		}
		if idx > 0 {
			fmt.Fprintln(stdout)
		}
		fmt.Fprintf(stdout, "Suite: %s\n", suite.Metadata.Name)
		for _, c := range suite.Spec.Cases {
			fmt.Fprintf(stdout, "  %s:\n", c.Name)
			models := suiteModelsForCase(suite, c)
			if len(models) == 0 {
				fmt.Fprintln(stdout, "    (no golden tape)")
				continue
			}
			found := false
			for _, model := range models {
				tapePath := filepath.Join(workspace, "testsuite", "agenttests", "tapes", suite.Metadata.Name, goldenTapeFilename(c.Name, model.Name))
				inspection, err := llm.InspectTape(tapePath)
				if errors.Is(err, os.ErrNotExist) {
					if baselineLine := reportGoldenBaselineStatus(workspace, suite.Metadata.Name, c.Name, model.Name, now); baselineLine != "" {
						fmt.Fprintf(stdout, "    %s  %s\n", model.Name, baselineLine)
						found = true
					}
					continue
				}
				if err != nil {
					return err
				}
				found = true
				fmt.Fprintf(stdout, "    %s  %s  %s\n", model.Name, formatRecordedAt(inspection.FirstRecordedAt), formatTapeStatus(inspection, model.Name, now))
				if drift := formatGoldenDriftStatus(workspace, suite.Metadata.Name, c.Name, model.Name, now); drift != "" {
					fmt.Fprintf(stdout, "      %s\n", drift)
				}
			}
			if !found {
				if drift := reportGoldenBaselineStatus(workspace, suite.Metadata.Name, c.Name, "", now); drift != "" {
					fmt.Fprintf(stdout, "    %s\n", drift)
				} else {
					fmt.Fprintln(stdout, "    (no golden tape)")
				}
			}
		}
	}
	return nil
}

func reportGoldenBaselineStatus(workspace, suiteName, caseName, modelName string, now time.Time) string {
	baseline := filepath.Join(workspace, "testsuite", "agenttests", "tapes", suiteName, agenttest.GoldenBaselineFilename(caseName, modelName))
	info, err := os.Stat(baseline)
	if errors.Is(err, os.ErrNotExist) {
		return ""
	}
	if err != nil {
		return fmt.Sprintf("baseline error: %v", err)
	}
	age := now.Sub(info.ModTime())
	if age > 30*24*time.Hour {
		return fmt.Sprintf("%s baseline stale (%d days old)", modelName, int(age.Round(24*time.Hour)/(24*time.Hour)))
	}
	return fmt.Sprintf("%s baseline present", modelName)
}

func formatGoldenDriftStatus(workspace, suiteName, caseName, modelName string, now time.Time) string {
	baseline := filepath.Join(workspace, "testsuite", "agenttests", "tapes", suiteName, agenttest.GoldenBaselineFilename(caseName, modelName))
	info, err := os.Stat(baseline)
	if errors.Is(err, os.ErrNotExist) {
		return "baseline missing"
	}
	if err != nil {
		return fmt.Sprintf("baseline error: %v", err)
	}
	age := now.Sub(info.ModTime())
	if age > 30*24*time.Hour {
		return fmt.Sprintf("baseline stale (%d days old)", int(age.Round(24*time.Hour)/(24*time.Hour)))
	}
	return ""
}

func suiteModelsForCase(suite *agenttest.Suite, c agenttest.CaseSpec) []agenttest.ModelSpec {
	if suite == nil {
		return nil
	}
	if c.Overrides.Model != nil {
		return []agenttest.ModelSpec{*c.Overrides.Model}
	}
	return append([]agenttest.ModelSpec(nil), suite.Spec.Models...)
}

func formatRecordedAt(recordedAt time.Time) string {
	if recordedAt.IsZero() {
		return "recorded unknown"
	}
	return "recorded " + recordedAt.UTC().Format("2006-01-02")
}

func formatTapeStatus(inspection *llm.TapeInspection, expectedModel string, now time.Time) string {
	if inspection == nil || inspection.Header == nil {
		return "legacy tape"
	}
	if model := strings.TrimSpace(inspection.Header.ModelName); model != "" && model != strings.TrimSpace(expectedModel) {
		return fmt.Sprintf("x model mismatch (%s)", model)
	}
	if !inspection.FirstRecordedAt.IsZero() {
		if age := now.Sub(inspection.FirstRecordedAt); age > 30*24*time.Hour {
			return fmt.Sprintf("! %d days old", int(age.Round(24*time.Hour)/(24*time.Hour)))
		}
	}
	return "ok model match"
}
