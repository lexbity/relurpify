package agenttest

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// trustDescriptor builds a descriptor that satisfies Validate, pointed at
// dir; individual tests override the fields they probe.
func trustDescriptor(dir, runID string) *PreparedRunDescriptor {
	return &PreparedRunDescriptor{
		RunID:                runID,
		SuitePath:            filepath.Join(dir, "suite.yaml"),
		SuiteName:            "trust",
		CaseName:             "case",
		AgentName:            "euclo",
		Instruction:          "do it",
		WorkspaceRoot:        dir,
		DerivedWorkspaceRoot: dir,
		RunRoot:              filepath.Join(dir, "run"),
		ConfigPath:           filepath.Join(dir, "cfg", "config.yaml"),
		SetupDir:             filepath.Join(dir, "setup"),
		ExecutionDir:         dir,
		VerificationDir:      filepath.Join(dir, "verification"),
		BackendSelection:     PreparedRunSelectionSingle,
		BackendProvider:      "ollama",
		BackendFamily:        "ollama",
		BackendEndpoint:      "http://localhost:11434",
		MaxRetries:           0,
	}
}

// TestMissingReportFails proves the first link of the false-pass chain is
// broken (R-7): a crashed executor that wrote no report is a typed infra
// failure, never Success:true.
func TestMissingReportFails(t *testing.T) {
	desc := &PreparedRunDescriptor{ExecutionDir: filepath.Join(t.TempDir(), "execution")}
	_, err := loadExecutorCaseReport(desc)
	require.ErrorIs(t, err, errReportMissing)
}

// TestUnparsableReportFails proves the second link: garbage bytes are a typed
// report_unparsable failure, never a pass.
func TestUnparsableReportFails(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "report.json"), []byte("{not json"), 0o600))
	desc := &PreparedRunDescriptor{ExecutionDir: dir}
	_, err := loadExecutorCaseReport(desc)
	require.ErrorIs(t, err, errReportUnparsable)
}

// TestZeroByteReportFailsVerification proves the third link: a zero-byte
// report.json fails VerifyPreparedRun with the artifact and reason named.
func TestZeroByteReportFailsVerification(t *testing.T) {
	dir := t.TempDir()
	execDir := filepath.Join(dir, "execution")
	require.NoError(t, os.MkdirAll(execDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(execDir, "report.json"), nil, 0o600))
	desc := trustDescriptor(dir, "run-zero")
	desc.ExecutionDir = execDir
	prepared := &PreparedRun{Descriptor: desc}
	report, err := VerifyPreparedRun(context.Background(), prepared, CaseReport{}, &Suite{}, CaseSpec{}, nil)
	require.NoError(t, err)
	require.False(t, report.Success, "a zero-byte report must fail verification")
	found := false
	for _, check := range report.Checks {
		if strings.Contains(check.AssertionID, "execution report") && !check.Passed {
			found = true
			require.Contains(t, check.Message, "empty report")
		}
	}
	require.True(t, found, "the failing check names the artifact and reason")
}

// TestUnparsableReportFailsVerification proves the parse check in
// verification: garbage bytes in report.json fail with the reason spelled out.
func TestUnparsableReportFailsVerification(t *testing.T) {
	dir := t.TempDir()
	execDir := filepath.Join(dir, "execution")
	require.NoError(t, os.MkdirAll(execDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(execDir, "report.json"), []byte("{not json"), 0o600))
	desc := trustDescriptor(dir, "run-garbage")
	desc.ExecutionDir = execDir
	prepared := &PreparedRun{Descriptor: desc}
	report, err := VerifyPreparedRun(context.Background(), prepared, CaseReport{}, &Suite{}, CaseSpec{}, nil)
	require.NoError(t, err)
	require.False(t, report.Success)
	for _, check := range report.Checks {
		if strings.Contains(check.AssertionID, "execution report") {
			require.Contains(t, check.Message, "unparsable report")
		}
	}
}

// TestWriteFailureFailsCase proves the write contract (FR-24): a case report
// that cannot be written fails the case — the executor returns the error
// instead of green-passing with no evidence on disk.
func TestWriteFailureFailsCase(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("skipping: root bypasses file permissions")
	}
	dir := t.TempDir()
	blocker := filepath.Join(dir, "report.json")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o600))
	require.NoError(t, os.Chmod(blocker, 0o400)) // read-only: the write must fail

	desc := trustDescriptor(dir, "run-write-fail")
	err := writeCaseReportOrFail(desc, CaseReport{Name: "write_fail", Success: true})
	require.Error(t, err, "an unwritable report must fail the case")
	require.Contains(t, err.Error(), "write case report")
}
