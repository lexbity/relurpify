package agenttest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type resolvedCaseExecution struct {
	Model         string
	ModelSource   string
	ManifestModel string
	Endpoint      string
	RecordingMode string
	TapePath      string
}

func resolveCaseExecution(suite *Suite, c CaseSpec, model ModelSpec, manifestModel string, opts RunOptions, layout runCaseLayout, targetWorkspace, workspace string) (resolvedCaseExecution, error) {
	recording := suite.Spec.Recording
	if c.Overrides.Recording != nil {
		recording = *c.Overrides.Recording
	}
	exec := resolvedCaseExecution{
		ManifestModel: strings.TrimSpace(manifestModel),
		Endpoint:      firstNonEmpty(opts.EndpointOverride, model.Endpoint, "http://localhost:11434"),
		RecordingMode: firstNonEmpty(recording.Mode, "off"),
	}
	switch {
	case strings.TrimSpace(opts.ModelOverride) != "":
		exec.Model = strings.TrimSpace(opts.ModelOverride)
		exec.ModelSource = "cli_override"
	case strings.TrimSpace(model.Name) != "":
		exec.Model = strings.TrimSpace(model.Name)
		exec.ModelSource = "suite_or_case"
	case strings.TrimSpace(manifestModel) != "":
		exec.Model = strings.TrimSpace(manifestModel)
		exec.ModelSource = "manifest"
	default:
		return resolvedCaseExecution{}, fmt.Errorf("no model resolved for case %q: set --model, spec.models, case override model, or manifest spec.agent.model.name", c.Name)
	}

	mode, tapePath, err := resolveRecordingPlan(suite, c, recording, exec, layout, targetWorkspace, workspace)
	if err != nil {
		return resolvedCaseExecution{}, err
	}
	exec.RecordingMode = mode
	exec.TapePath = tapePath
	if exec.RecordingMode == "replay" {
		if exec.TapePath == "" {
			return resolvedCaseExecution{}, fmt.Errorf("replay tape unavailable for suite %q case %q model %q", suite.Metadata.Name, c.Name, exec.Model)
		}
		if _, err := os.Stat(exec.TapePath); err != nil {
			return resolvedCaseExecution{}, fmt.Errorf("replay tape unavailable at %s: %w", exec.TapePath, err)
		}
	}
	return exec, nil
}

func resolveRecordingPlan(suite *Suite, c CaseSpec, recording RecordingSpec, exec resolvedCaseExecution, layout runCaseLayout, targetWorkspace, workspace string) (string, string, error) {
	mode := strings.TrimSpace(recording.Mode)
	if mode != "" && mode != "off" {
		tapePath, err := resolveExplicitOrDefaultTapePath(suite, recording, layout, targetWorkspace, workspace)
		return mode, tapePath, err
	}
	switch strings.TrimSpace(recording.Strategy) {
	case "replay-if-golden":
		if golden := resolveGoldenTapePath(suite.SourcePath, suite.Metadata.Name, c.Name, exec.Model); golden != "" {
			return "replay", golden, nil
		}
		return "off", "", nil
	case "replay-only":
		if golden := resolveGoldenTapePath(suite.SourcePath, suite.Metadata.Name, c.Name, exec.Model); golden != "" {
			return "replay", golden, nil
		}
		return "replay", "", nil
	default:
		tapePath, err := resolveExplicitOrDefaultTapePath(suite, recording, layout, targetWorkspace, workspace)
		return firstNonEmpty(mode, "off"), tapePath, err
	}
}

func resolveExplicitOrDefaultTapePath(suite *Suite, recording RecordingSpec, layout runCaseLayout, targetWorkspace, workspace string) (string, error) {
	mode := strings.TrimSpace(recording.Mode)
	if mode != "record" && mode != "replay" {
		return "", nil
	}
	if strings.TrimSpace(recording.Tape) != "" {
		resolved := suite.ResolvePath(recording.Tape)
		tapePath := resolveAgainstWorkspace(targetWorkspace, resolved, recording.Tape)
		tapePath = mapTargetPathToWorkspace(tapePath, targetWorkspace, workspace)
		checked, err := ensurePathWithin(workspace, tapePath)
		if err != nil {
			return "", err
		}
		return checked, nil
	}
	return layout.TapePath, nil
}

func resolveGoldenTapePath(suitePath, suiteName, caseName, modelName string) string {
	suitePath = strings.TrimSpace(suitePath)
	if suitePath == "" {
		return ""
	}
	suiteKey := strings.TrimSpace(suiteName)
	if suiteKey == "" {
		suiteKey = strings.TrimSuffix(filepathBase(suitePath), ".testsuite.yaml")
	}
	goldenDir := filepathJoin(filepathDir(suitePath), "tapes", suiteKey)
	goldenPath := filepathJoin(goldenDir, sanitizeName(caseName)+"__"+sanitizeName(modelName)+".tape.jsonl")
	if _, err := os.Stat(goldenPath); err == nil {
		return goldenPath
	}
	return ""
}

var (
	filepathJoin = func(elem ...string) string { return filepath.Join(elem...) }
	filepathDir  = filepath.Dir
	filepathBase = filepath.Base
)
