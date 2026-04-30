package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"codeburg.org/lexbit/relurpify/platform/llm"
	"codeburg.org/lexbit/relurpify/testsuite/agenttest"
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
			models := agentTestSurface.SuiteModelsForCase(suite, c)
			if len(models) == 0 {
				fmt.Fprintln(stdout, "    (no golden tape)")
				continue
			}
			found := false
			for _, model := range models {
				tapePath := agentTestSurface.TapePath(workspace, suite.Metadata.Name, c.Name, model.Name)
				inspection, err := llm.InspectTape(tapePath)
				if errors.Is(err, os.ErrNotExist) {
					if baselineLine := agentTestSurface.FormatBaselineStatus(workspace, suite.Metadata.Name, c.Name, model.Name, now); baselineLine != "" {
						fmt.Fprintf(stdout, "    %s  %s\n", model.Name, baselineLine)
						found = true
					}
					continue
				}
				if err != nil {
					return err
				}
				found = true
				fmt.Fprintf(stdout, "    %s  %s  %s\n", model.Name, formatRecordedAt(inspection.FirstRecordedAt), agentTestSurface.FormatTapeStatus(inspection, model.Name, now))
				if drift := agentTestSurface.FormatBaselineStatus(workspace, suite.Metadata.Name, c.Name, model.Name, now); drift != "" {
					fmt.Fprintf(stdout, "      %s\n", drift)
				}
			}
			if !found {
				if drift := agentTestSurface.FormatBaselineStatus(workspace, suite.Metadata.Name, c.Name, "", now); drift != "" {
					fmt.Fprintf(stdout, "    %s\n", drift)
				} else {
					fmt.Fprintln(stdout, "    (no golden tape)")
				}
			}
		}
	}
	return nil
}

func formatRecordedAt(recordedAt time.Time) string {
	if recordedAt.IsZero() {
		return "recorded unknown"
	}
	return "recorded " + recordedAt.UTC().Format("2006-01-02")
}
