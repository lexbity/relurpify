package agenttest

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"codeburg.org/lexbit/relurpify/capability/sandbox"
	"codeburg.org/lexbit/relurpify/platform/fs"
)

// PreparedRunVerificationReport records artifact-only verification results.
type PreparedRunVerificationReport struct {
	DescriptorPath         string            `json:"descriptor_path"`
	ExecutionReportPath    string            `json:"execution_report_path,omitempty"`
	SetupLogPath           string            `json:"setup_log_path,omitempty"`
	SetupTelemetryPath     string            `json:"setup_telemetry_path,omitempty"`
	ExecutionLogPath       string            `json:"execution_log_path,omitempty"`
	ExecutionTelemetryPath string            `json:"execution_telemetry_path,omitempty"`
	Checks                 []AssertionResult `json:"checks,omitempty"`
	VerificationResults    []AssertionResult `json:"verification_results,omitempty"`
	Success                bool              `json:"success"`
}

// VerifyPreparedRun validates the run-scoped artifacts produced for a prepared run.
// A nil runner is acceptable only when there are no verification steps; when
// verification steps or scripts are present, the runner must be non-nil or
// the function returns an error. Use RequireSandbox to obtain a verified runner.
func VerifyPreparedRun(ctx context.Context, prepared *PreparedRun, caseReport CaseReport, suite *Suite, c CaseSpec, runner sandbox.CommandRunner) (*PreparedRunVerificationReport, error) {
	if prepared == nil || prepared.Descriptor == nil {
		return nil, fmt.Errorf("prepared run required")
	}
	desc := prepared.Descriptor
	if err := preparedRunEnsure(desc); err != nil {
		return nil, err
	}
	report := &PreparedRunVerificationReport{
		DescriptorPath:         prepared.Artifacts.DescriptorPath(),
		ExecutionReportPath:    preparedRunReportPath(desc),
		SetupLogPath:           filepath.Join(desc.SetupLogsDir, "agenttest.log"),
		SetupTelemetryPath:     filepath.Join(desc.SetupTelemetryDir, "agenttest.jsonl"),
		ExecutionLogPath:       filepath.Join(desc.ExecutionLogsDir, "agenttest.log"),
		ExecutionTelemetryPath: filepath.Join(desc.ExecutionTelemetryDir, "agenttest.jsonl"),
		Success:                caseReport.Success,
	}

	artifactChecks := []struct {
		path string
		name string
	}{
		{report.DescriptorPath, "descriptor"},
		{report.ExecutionReportPath, "execution report"},
		{report.SetupLogPath, "setup log"},
		{report.SetupTelemetryPath, "setup telemetry"},
		{report.ExecutionLogPath, "execution log"},
		{report.ExecutionTelemetryPath, "execution telemetry"},
	}
	for _, check := range artifactChecks {
		passed := strings.TrimSpace(check.path) != ""
		if passed {
			if _, err := os.Stat(check.path); err != nil {
				passed = false
				report.Success = false
			}
		}
		report.Checks = append(report.Checks, AssertionResult{
			AssertionID: "prepared_run.artifact[" + check.name + "]",
			Tier:        "outcome",
			Passed:      passed,
			Message:     check.path,
		})
	}

	if desc.Verification.Script != "" || len(desc.Verification.Steps) > 0 {
		if runner == nil {
			return nil, fmt.Errorf("runner required when verification steps are present; use RequireSandbox to obtain one")
		}
		verificationResults := runVerificationSteps(ctx, VerifySpec{
			Steps:  convertPreparedVerificationSteps(desc.Verification.Steps),
			Script: desc.Verification.Script,
		}, desc.DerivedWorkspaceRoot, runner)
		report.VerificationResults = append(report.VerificationResults, verificationResults...)
		for _, result := range verificationResults {
			if !result.Passed {
				report.Success = false
			}
		}
	}

	if desc.Verification.ExpectedArtifacts != nil {
		for _, rel := range desc.Verification.ExpectedArtifacts {
			abs := rel
			if !filepath.IsAbs(abs) {
				abs = filepath.Join(desc.DerivedWorkspaceRoot, rel)
			}
			passed := true
			if _, err := os.Stat(abs); err != nil {
				passed = false
				report.Success = false
			}
			report.Checks = append(report.Checks, AssertionResult{
				AssertionID: "prepared_run.expected_artifact[" + rel + "]",
				Tier:        "outcome",
				Passed:      passed,
				Message:     abs,
			})
		}
	}

	if c.Expect.Outcome != nil && len(c.Expect.Outcome.FilesContain) > 0 {
		contentResults, contentFailures := evaluateFileContentExpectations(c.Expect.Outcome.FilesContain, desc.DerivedWorkspaceRoot)
		report.Checks = append(report.Checks, contentResults...)
		for _, failure := range contentFailures {
			_ = failure
			report.Success = false
		}
	}

	if caseReport.Output != "" {
		report.Checks = append(report.Checks, AssertionResult{
			AssertionID: "prepared_run.output_present",
			Tier:        "outcome",
			Passed:      true,
			Message:     "case output captured",
		})
	}

	if err := writePreparedRunVerificationReport(preparedRunVerificationPath(desc), report); err != nil {
		return nil, err
	}
	return report, nil
}

func convertPreparedVerificationSteps(steps []PreparedVerificationStep) []VerifyStepSpec {
	if len(steps) == 0 {
		return nil
	}
	out := make([]VerifyStepSpec, 0, len(steps))
	for _, step := range steps {
		out = append(out, VerifyStepSpec(step))
	}
	return out
}

func writePreparedRunVerificationReport(path string, report *PreparedRunVerificationReport) error {
	if strings.TrimSpace(path) == "" || report == nil {
		return nil
	}
	if err := fs.MkdirAllSecure(filepath.Dir(path)); err != nil {
		return err
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return fs.WriteFileSecure(path, data)
}

func preparedRunFromSuiteCase(suite *Suite, c CaseSpec, model ModelSpec, opts RunOptions, targetWorkspace, runRoot, runID string) (*PreparedRun, error) {
	return PrepareRun(suite, c, model, opts, targetWorkspace, runRoot, runID)
}
