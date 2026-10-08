package agenttest

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"codeburg.org/lexbit/relurpify/platform/fs"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
	"codeburg.org/lexbit/relurpify/telemetry/perfstats"
)

// writeCaseTelemetryArtifacts materialises the telemetry-derived artifacts the
// benchmark scorer and post-hoc analysis consume: the paired tool transcript,
// the phase metrics, the changed-file list, and the framework perf snapshot.
// Failures are non-fatal — the report is still written (NFR-3).
func writeCaseTelemetryArtifacts(desc *PreparedRunDescriptor, report *CaseReport, events []telemetry.Event) {
	dir := caseArtifactsDir(desc)
	if dir == "" {
		return
	}
	if transcript := BuildToolTranscript(events); transcript != nil {
		writeJSONArtifact(filepath.Join(dir, "tool_transcript.json"), transcript)
	}
	if report != nil && len(report.PhaseMetrics) > 0 {
		writeJSONArtifact(filepath.Join(dir, "phase_metrics.json"), report.PhaseMetrics)
	}
	if report != nil && len(report.ChangedFiles) > 0 {
		writeJSONArtifact(filepath.Join(dir, "changed_files.json"), report.ChangedFiles)
	}
	writeJSONArtifact(filepath.Join(dir, "framework_perf.json"), perfstats.Get())
}

// caseArtifactsDir resolves the directory telemetry artifacts are written to.
func caseArtifactsDir(desc *PreparedRunDescriptor) string {
	if desc == nil {
		return ""
	}
	return firstNonEmpty(desc.ExecutionArtifactsDir, desc.ExecutionDir)
}

// caseReportPath is the canonical on-disk path for a prepared run's case
// report. The verification suite loads the same path, so the executor and the
// verifier agree on where the telemetry-derived report lives.
func caseReportPath(desc *PreparedRunDescriptor) string {
	if desc == nil {
		return ""
	}
	if dir := strings.TrimSpace(desc.ExecutionDir); dir != "" {
		return filepath.Join(dir, "report.json")
	}
	if dir := strings.TrimSpace(desc.ExecutionArtifactsDir); dir != "" {
		return filepath.Join(dir, "report.json")
	}
	return ""
}

func writeJSONArtifact(path string, value any) {
	if strings.TrimSpace(path) == "" {
		return
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return
	}
	if err := fs.MkdirAllSecure(filepath.Dir(path)); err != nil {
		return
	}
	_ = fs.WriteFileSecure(path, data)
}
