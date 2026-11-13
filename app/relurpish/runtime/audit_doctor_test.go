package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/governance/policy"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

// seedAuditChain writes a few committed records into the given audit dir and
// returns the chain file (to be tampered with by the caller).
func seedAuditChain(t *testing.T, auditDir string) string {
	t.Helper()
	l, err := policy.NewFileChainAuditLogger(auditDir, policy.FileChainOptions{BatchFlush: time.Millisecond})
	if err != nil {
		t.Fatalf("NewFileChainAuditLogger: %v", err)
	}
	for i := 0; i < 5; i++ {
		if err := l.Log(context.Background(), policy.AuditRecord{
			AgentID: "agent-x", Action: "exec:binary:ssh", Type: "executable", Result: "granted",
		}); err != nil {
			t.Fatalf("Log: %v", err)
		}
	}
	if err := l.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	entries, err := os.ReadDir(auditDir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "chain-") {
			return filepath.Join(auditDir, e.Name())
		}
	}
	t.Fatal("no chain file seeded")
	return ""
}

func auditChainDependency(report DoctorReport) *DependencyStatus {
	for i := range report.Dependencies {
		if report.Dependencies[i].Name == "audit_chain" {
			return &report.Dependencies[i]
		}
	}
	return nil
}

func TestDoctorReport_TamperedChainBlocksInStrictMode(t *testing.T) {
	workspace := t.TempDir()
	writeMinimalDoctorWorkspace(t, workspace, map[string]string{
		"ollama.provider.yaml": "schema: relurpify/model/provider/v1\nname: ollama\nendpoint: http://localhost:11434\nkind: ollama\n",
	})
	auditDir := filepath.Join(config.DefaultWorkspaceStateDir(workspace), "audit", "agent-x")
	chainFile := seedAuditChain(t, auditDir)

	// Flip a byte mid-file: the head anchor still matches its tail line, but a
	// full replay (doctor integrity probe) must report the break.
	data, err := os.ReadFile(chainFile)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	data[len(data)/2] ^= 0xff
	if err := os.WriteFile(chainFile, data, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg := Config{
		Workspace:         workspace,
		InferenceProvider: "ollama",
		InferenceEndpoint: "http://localhost:11434",
		AuditEnforcement:  "strict",
	}
	report := BuildDoctorReport(context.Background(), cfg, config.Secrets{})
	dep := auditChainDependency(report)
	if dep == nil {
		t.Fatal("doctor must surface an audit_chain dependency")
	}
	if !dep.Blocking {
		t.Fatalf("tampered chain must be BLOCKING in strict mode: %+v", dep)
	}
	if dep.Available {
		t.Fatalf("tampered chain must not be available: %+v", dep)
	}
}

func TestDoctorReport_TamperedChainDegradesInBestEffort(t *testing.T) {
	workspace := t.TempDir()
	writeMinimalDoctorWorkspace(t, workspace, map[string]string{
		"ollama.provider.yaml": "schema: relurpify/model/provider/v1\nname: ollama\nendpoint: http://localhost:11434\nkind: ollama\n",
	})
	auditDir := filepath.Join(config.DefaultWorkspaceStateDir(workspace), "audit", "agent-x")
	chainFile := seedAuditChain(t, auditDir)

	data, err := os.ReadFile(chainFile)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	data[len(data)/2] ^= 0xff
	_ = os.WriteFile(chainFile, data, 0o600)

	cfg := Config{
		Workspace:         workspace,
		InferenceProvider: "ollama",
		InferenceEndpoint: "http://localhost:11434",
		AuditEnforcement:  "best_effort",
	}
	report := BuildDoctorReport(context.Background(), cfg, config.Secrets{})
	dep := auditChainDependency(report)
	if dep == nil {
		t.Fatal("doctor must surface an audit_chain dependency")
	}
	if dep.Blocking {
		t.Fatalf("best_effort must downgrade chain failure to a warning, got %+v", dep)
	}
	if !dep.Degraded {
		t.Fatalf("best_effort must mark the degraded chain, got %+v", dep)
	}
}

func TestDoctorReport_FreshWorkspaceAuditDepIsNonBlocking(t *testing.T) {
	workspace := t.TempDir()
	writeMinimalDoctorWorkspace(t, workspace, map[string]string{
		"ollama.provider.yaml": "schema: relurpify/model/provider/v1\nname: ollama\nendpoint: http://localhost:11434\nkind: ollama\n",
	})
	cfg := Config{
		Workspace:         workspace,
		InferenceProvider: "ollama",
		InferenceEndpoint: "http://localhost:11434",
		AuditEnforcement:  "strict",
	}
	report := BuildDoctorReport(context.Background(), cfg, config.Secrets{})
	dep := auditChainDependency(report)
	if dep == nil {
		t.Fatal("doctor must surface an audit_chain dependency")
	}
	if dep.Blocking {
		t.Fatalf("a fresh workspace must not report a blocking audit chain: %+v", dep)
	}
	if dep.Available || !dep.Degraded || !strings.Contains(dep.Details, "checked during execution boot") {
		t.Fatalf("missing audit storage must remain explicitly unverified: %+v", dep)
	}
	if _, err := os.Stat(config.DefaultWorkspaceStateDir(workspace)); !os.IsNotExist(err) {
		t.Fatalf("read-only Doctor created workspace state: %v", err)
	}
}

func TestDoctorAuditProbeDoesNotOverwriteFiles(t *testing.T) {
	workspace := t.TempDir()
	auditDir := filepath.Join(config.DefaultWorkspaceStateDir(workspace), "audit")
	if err := os.MkdirAll(auditDir, 0o700); err != nil {
		t.Fatal(err)
	}
	// The removed write probe truncated a user's file with this name.
	path := filepath.Join(auditDir, ".doctor-write-probe")
	const content = "must survive diagnostics"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	probeAuditChain(Config{Workspace: workspace, AuditEnforcement: "strict"})
	got, err := os.ReadFile(path)
	if err != nil || string(got) != content {
		t.Fatalf("diagnosis mutated existing content: %q, %v", got, err)
	}
}

func TestDoctorAuditProbeRejectsUninspectableRoot(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(config.DefaultWorkspaceStateDir(workspace), []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, enforcement := range []string{"strict", "best_effort"} {
		dep := probeAuditChain(Config{Workspace: workspace, AuditEnforcement: enforcement})
		if dep.Available || dep.Blocking != (enforcement == "strict") || !strings.Contains(dep.Details, "cannot inspect") {
			t.Fatalf("uninspectable storage got a false pass: %+v", dep)
		}
	}
}
