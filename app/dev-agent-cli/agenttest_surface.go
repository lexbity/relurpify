package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/platform/llm"
	"codeburg.org/lexbit/relurpify/testsuite/agenttest"
)

type agentTestSurfaceAdapter interface {
	Name() string
	ResolveStartMode(requested string, spec *core.AgentRuntimeSpec) string
	BuildStartTaskContext(mode, workspace string) map[string]any
	GoldenTapeFilename(caseName, modelName string) string
	TapePath(workspace, suiteName, caseName, modelName string) string
	FormatTapeStatus(inspection *llm.TapeInspection, expectedModel string, now time.Time) string
	FormatBaselineStatus(workspace, suiteName, caseName, modelName string, now time.Time) string
	PromoteAllowed(classification string) bool
	PromotedArtifacts(classification string, cr agenttest.CaseReport) []string
	PromotionLineageFilename(caseName, modelName string) string
	SuiteModelsForCase(suite *agenttest.Suite, c agenttest.CaseSpec) []agenttest.ModelSpec
	RunRoot(report *agenttest.SuiteReport) string
}

type eucloAgentTestSurfaceAdapter struct{}

var agentTestSurface agentTestSurfaceAdapter = eucloAgentTestSurfaceAdapter{}

func (eucloAgentTestSurfaceAdapter) Name() string { return "euclo" }

func (eucloAgentTestSurfaceAdapter) ResolveStartMode(requested string, spec *core.AgentRuntimeSpec) string {
	if trimmed := strings.TrimSpace(requested); trimmed != "" {
		return trimmed
	}
	if spec != nil && spec.Mode != "" {
		return string(spec.Mode)
	}
	return "default"
}

func (eucloAgentTestSurfaceAdapter) BuildStartTaskContext(mode, workspace string) map[string]any {
	return map[string]any{
		"mode":      strings.TrimSpace(mode),
		"workspace": workspace,
	}
}

func (eucloAgentTestSurfaceAdapter) GoldenTapeFilename(caseName, modelName string) string {
	return sanitizeAgentTestTapeName(caseName) + "__" + sanitizeAgentTestTapeName(modelName) + ".tape.jsonl"
}

func (a eucloAgentTestSurfaceAdapter) TapePath(workspace, suiteName, caseName, modelName string) string {
	return filepath.Join(workspace, "testsuite", "agenttests", "tapes", suiteName, a.GoldenTapeFilename(caseName, modelName))
}

func (eucloAgentTestSurfaceAdapter) FormatTapeStatus(inspection *llm.TapeInspection, expectedModel string, now time.Time) string {
	if inspection == nil || inspection.Header == nil {
		return "missing header"
	}
	if model := strings.TrimSpace(inspection.Header.ModelName); model != "" && model != strings.TrimSpace(expectedModel) {
		return "x model mismatch (" + model + ")"
	}
	if !inspection.FirstRecordedAt.IsZero() {
		if age := now.Sub(inspection.FirstRecordedAt); age > 30*24*time.Hour {
			return fmt.Sprintf("! %d days old", int(age.Round(24*time.Hour)/(24*time.Hour)))
		}
	}
	return "ok model match"
}

func (eucloAgentTestSurfaceAdapter) FormatBaselineStatus(workspace, suiteName, caseName, modelName string, now time.Time) string {
	baseline := filepath.Join(workspace, "testsuite", "agenttests", "tapes", suiteName, agenttest.GoldenBaselineFilename(caseName, modelName))
	info, err := os.Stat(baseline)
	if errors.Is(err, os.ErrNotExist) {
		return ""
	}
	if err != nil {
		return "baseline error: " + err.Error()
	}
	age := now.Sub(info.ModTime())
	if age > 30*24*time.Hour {
		return modelName + " baseline stale"
	}
	return modelName + " baseline present"
}

func (eucloAgentTestSurfaceAdapter) PromoteAllowed(classification string) bool {
	switch strings.ToLower(strings.TrimSpace(classification)) {
	case "", "capability", "journey", "benchmark":
		return true
	default:
		return false
	}
}

func (eucloAgentTestSurfaceAdapter) PromotedArtifacts(classification string, cr agenttest.CaseReport) []string {
	switch strings.ToLower(strings.TrimSpace(classification)) {
	case "benchmark":
		return []string{"benchmark_report.json", "benchmark_score.json", "benchmark_comparison.json"}
	case "journey":
		out := []string{"tape.jsonl"}
		if _, err := os.Stat(filepath.Join(cr.ArtifactsDir, "interaction.tape.jsonl")); err == nil {
			out = append(out, "interaction.tape.jsonl")
		}
		return out
	default:
		return []string{"tape.jsonl", "interaction.tape.jsonl", "baseline.json"}
	}
}

func (eucloAgentTestSurfaceAdapter) PromotionLineageFilename(caseName, modelName string) string {
	return sanitizeAgentTestTapeName(caseName) + "__" + sanitizeAgentTestTapeName(modelName) + ".promotion.json"
}

func (eucloAgentTestSurfaceAdapter) SuiteModelsForCase(suite *agenttest.Suite, c agenttest.CaseSpec) []agenttest.ModelSpec {
	if suite == nil {
		return nil
	}
	if c.Overrides.Model != nil {
		return []agenttest.ModelSpec{*c.Overrides.Model}
	}
	return append([]agenttest.ModelSpec(nil), suite.Spec.Models...)
}

func (eucloAgentTestSurfaceAdapter) RunRoot(report *agenttest.SuiteReport) string {
	if report == nil || len(report.Cases) == 0 {
		return ""
	}
	return filepath.Dir(report.Cases[0].ArtifactsDir)
}

func sanitizeAgentTestTapeName(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "unnamed"
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "unnamed"
	}
	return out
}
