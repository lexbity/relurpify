package agenttest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"codeburg.org/lexbit/relurpify/platform/fs"
)

// PreparedRun bundles the descriptor and its run-scoped artifact layout.
type PreparedRun struct {
	Descriptor *PreparedRunDescriptor
	Artifacts  PreparedRunArtifacts
}

// PrepareRun builds the descriptor and writes it to the setup directory.
func PrepareRun(suite *Suite, c CaseSpec, model ModelSpec, opts RunOptions, targetWorkspace, runRoot, runID string) (*PreparedRun, error) {
	desc, err := BuildPreparedRunDescriptor(suite, c, model, opts, targetWorkspace, runRoot, runID)
	if err != nil {
		return nil, err
	}
	artifacts := NewPreparedRunArtifacts(targetWorkspace, runRoot, suite.Spec.AgentName, runID)
	if err := artifacts.Ensure(); err != nil {
		return nil, err
	}
	if err := MaterializeDerivedWorkspace(
		targetWorkspace,
		artifacts.SetupWorkspaceDir,
		opts.SharedRoot,
		resolveTemplateProfile(suite, c),
		suite.Spec.Manifest,
		resolveWorkspaceExclude(suite, c),
		resolveWorkspaceFiles(suite, c),
	); err != nil {
		return nil, err
	}
	if err := desc.Write(artifacts.DescriptorPath()); err != nil {
		return nil, err
	}
	if err := touchPreparedRunArtifactFiles(desc); err != nil {
		return nil, err
	}
	return &PreparedRun{Descriptor: desc, Artifacts: artifacts}, nil
}

func preparedRunCaseID(runID, caseName, modelName string) string {
	parts := []string{strings.TrimSpace(runID), sanitizeName(caseName), sanitizeName(modelName)}
	return sanitizeName(strings.Join(parts, "__"))
}

func preparedRunReportPath(desc *PreparedRunDescriptor) string {
	if desc == nil {
		return ""
	}
	return filepath.Join(desc.ExecutionDir, "report.json")
}

func preparedRunVerificationPath(desc *PreparedRunDescriptor) string {
	if desc == nil {
		return ""
	}
	return filepath.Join(desc.VerificationDir, "verification.json")
}

func preparedRunEnsure(desc *PreparedRunDescriptor) error {
	if desc == nil {
		return fmt.Errorf("descriptor required")
	}
	return desc.Normalize()
}

func touchPreparedRunArtifactFiles(desc *PreparedRunDescriptor) error {
	if desc == nil {
		return fmt.Errorf("descriptor required")
	}
	paths := []string{
		filepath.Join(desc.SetupLogsDir, "agenttest.log"),
		filepath.Join(desc.SetupTelemetryDir, "agenttest.jsonl"),
		filepath.Join(desc.ExecutionLogsDir, "agenttest.log"),
		filepath.Join(desc.ExecutionTelemetryDir, "agenttest.jsonl"),
		filepath.Join(desc.ExecutionDir, "report.json"),
		filepath.Join(desc.VerificationDir, "verification.json"),
	}
	for _, path := range paths {
		if err := fs.MkdirAllSecure(filepath.Dir(path)); err != nil {
			return err
		}
		f, err := os.OpenFile(filepath.Clean(path), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, fs.PublicFileMode)
		if err != nil {
			return err
		}
		_ = f.Close()
	}
	return nil
}
