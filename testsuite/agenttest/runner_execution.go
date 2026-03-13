package agenttest

import (
	"fmt"
	"os"
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

	if exec.RecordingMode == "record" || exec.RecordingMode == "replay" {
		tapePath := recording.Tape
		if tapePath == "" {
			tapePath = layout.TapePath
		} else {
			if strings.TrimSpace(recording.Tape) == "" {
				return resolvedCaseExecution{}, fmt.Errorf("recording tape path required")
			}
			resolved := suite.ResolvePath(tapePath)
			tapePath = resolveAgainstWorkspace(targetWorkspace, resolved, tapePath)
			tapePath = mapTargetPathToWorkspace(tapePath, targetWorkspace, workspace)
			checked, err := ensurePathWithin(workspace, tapePath)
			if err != nil {
				return resolvedCaseExecution{}, err
			}
			tapePath = checked
		}
		exec.TapePath = tapePath
		if exec.RecordingMode == "replay" {
			if _, err := os.Stat(exec.TapePath); err != nil {
				return resolvedCaseExecution{}, fmt.Errorf("replay tape unavailable at %s: %w", exec.TapePath, err)
			}
		}
	}
	return exec, nil
}
