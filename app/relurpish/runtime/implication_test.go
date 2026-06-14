package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	capabilityports "codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/capability/sandbox"
	governanceports "codeburg.org/lexbit/relurpify/governance/ports"
	"codeburg.org/lexbit/relurpify/platform/fs"
	"codeburg.org/lexbit/relurpify/testsuite/testhelper"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

// TestDoctorReadyImpliesRuntimeNewOk verifies that on a good workspace W,
// doctor.Ready == true implies runtime.New succeeds (AC-9, Q7).
func TestDoctorReadyImpliesRuntimeNewOk(t *testing.T) {
	workspace := t.TempDir()
	testhelper.WriteCleanWorkspace(t, workspace, testhelper.WorkspaceOpts{
		Provider: "offline",
		SeedFiles: map[string]string{
			"notes.txt": "content\n",
		},
	})
	testhelper.InitGitRepo(t, workspace)

	cfg := ConfigForWorkspace(DefaultConfig(), workspace)
	cfg.InferenceProvider = "offline"
	cfg.InferenceModel = "offline-synthetic"
	cfg.SharedRoot = filepath.Join(workspace, ".local", "share", "relurpify")
	cfg.SecurityRunner = &implicationRunner{}
	cfg.SandboxBackendFactory = func(context.Context, string, governanceports.SandboxConfig, string, string) (governanceports.SandboxRuntime, error) {
		return &implicationSandbox{}, nil
	}

	writeSharedTemplates(t, cfg.SharedRoot)
	mustWriteStateConfig(t, workspace)

	secrets := config.Secrets{}
	report := BuildDoctorReport(context.Background(), cfg, secrets)
	if !report.Ready() {
		t.Fatalf("expected doctor report Ready() on good workspace, blocking issues: %v", blockingErrors(report))
	}

	rt, err := New(context.Background(), cfg, secrets)
	if err != nil {
		t.Log("doctor blocking issues:", blockingErrors(report))
		t.Fatal("runtime.New should succeed when doctor is Ready, but got error:", err)
	}
	defer func() { _ = rt.Close(context.Background()) }()
}

// TestDoctorBlocksOnBrokenDefaultProfile verifies that a broken default model
// profile causes both doctor to report blocking issues and runtime.New to fail
// (AC-9, Q8).
func TestDoctorBlocksOnBrokenDefaultProfile(t *testing.T) {
	workspace := t.TempDir()
	testhelper.WriteCleanWorkspace(t, workspace, testhelper.WorkspaceOpts{
		Provider: "offline",
		SeedFiles: map[string]string{
			"notes.txt": "content\n",
		},
	})

	modelDir := filepath.Join(workspace, "relurpify_cfg", "model", "profiles")
	if err := os.MkdirAll(modelDir, fs.PublicDirMode); err != nil {
		t.Fatalf("mkdir model profiles: %v", err)
	}
	mustWriteFile(t, filepath.Join(modelDir, "default.llm.yaml"), `schema: relurpify/model/profile/v1
pattern: "*"
tool_calling:
  intent: native
context:
  max_tokens: 0
generation:
  temperature: 0.2
  top_p: 0.9
`)

	cfg := ConfigForWorkspace(DefaultConfig(), workspace)
	cfg.InferenceProvider = "offline"
	cfg.InferenceModel = "offline-synthetic"
	cfg.SecurityRunner = &implicationRunner{}
	cfg.SandboxBackendFactory = func(context.Context, string, governanceports.SandboxConfig, string, string) (governanceports.SandboxRuntime, error) {
		return &implicationSandbox{}, nil
	}

	secrets := config.Secrets{}
	report := BuildDoctorReport(context.Background(), cfg, secrets)
	if report.Ready() {
		t.Fatal("expected doctor report to have blocking issues on broken default profile")
	}

	_, err := New(context.Background(), cfg, secrets)
	if err == nil {
		t.Fatal("expected runtime.New to fail on broken default profile")
	}
}

type implicationRunner struct{}

func (f *implicationRunner) Run(_ context.Context, _ capabilityports.CommandRequest) (*capabilityports.CommandResult, error) {
	return &capabilityports.CommandResult{Stdout: "fake", ExitCode: 0}, nil
}

type implicationSandbox struct{}

func (f *implicationSandbox) Verify(context.Context) error                        { return nil }
func (f *implicationSandbox) ValidatePolicy(governanceports.SandboxPolicy) error   { return nil }
func (f *implicationSandbox) ApplyPolicy(context.Context, governanceports.SandboxPolicy) error {
	return nil
}
func (f *implicationSandbox) Policy() governanceports.SandboxPolicy               { return governanceports.SandboxPolicy{} }
func (f *implicationSandbox) RunConfig() governanceports.SandboxConfig             { return governanceports.SandboxConfig{} }
func (f *implicationSandbox) Name() string                                        { return "fake" }
func (f *implicationSandbox) NewCommandRunner(*sandbox.CommandRunnerConfig) (capabilityports.CommandRunner, error) {
	return &implicationRunner{}, nil
}

func writeSharedTemplates(t *testing.T, sharedRoot string) {
	t.Helper()
	wsDir := filepath.Join(sharedRoot, "templates", "workspace")
	securityDir := filepath.Join(wsDir, "security")
	for _, dir := range []string{wsDir, securityDir} {
		if err := os.MkdirAll(dir, fs.PublicDirMode); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	mustWriteFile(t, filepath.Join(wsDir, "workspace.yaml"), `schema: relurpify/workspace/v1
paths:
  state_dir: .relurpify_state
model:
  provider: offline
  name: gemma4:e4b
sandbox:
  backend: gvisor
logging:
  level: info
  format: json
audit:
  retention_days: 7
telemetry:
  enabled: false
`)
	mustWriteFile(t, filepath.Join(wsDir, "agent.yaml"), `schema: relurpify/agent/v1
kind: AgentManifest
metadata:
  name: euclo
spec:
  agent:
    model:
      provider: offline
      name: offline-synthetic
`)
	for name, content := range map[string]string{
		"sandbox.policy.yaml":           "schema: relurpify/policy/sandbox/v1\nread_only_root: false\nno_new_privileges: false\n",
		"shell.policy.yaml":             "schema: relurpify/policy/shell/v1\nrules: []\n",
		"localtool.policy.yaml":         "schema: relurpify/policy/localtool/v1\ntools:\n  cli_git:\n    execute: allow\n",
		"workspaceingestion.policy.yaml": "schema: relurpify/policy/workspaceingestion/v1\nrules: []\n",
	} {
		mustWriteFile(t, filepath.Join(securityDir, name), content)
	}
}

func mustWriteStateConfig(t *testing.T, workspace string) {
	t.Helper()
	stateDir := filepath.Join(workspace, ".relurpify_state")
	if err := os.MkdirAll(stateDir, fs.PublicDirMode); err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}
	mustWriteFile(t, filepath.Join(stateDir, "workspace.yaml"), `provider: offline
model: offline-synthetic
sandbox_backend: gvisor
execution_mode: staged
agents: ["euclo"]
last_updated: 0
`)
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Clean(path), []byte(content), fs.PublicFileMode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func blockingErrors(report DoctorReport) []string {
	var errs []string
	if !report.ConfigExists {
		errs = append(errs, "config: config file not found (ConfigExists=false)")
	}
	if report.ConfigError != "" {
		errs = append(errs, "config: "+report.ConfigError)
	}
	if report.ManifestError != "" {
		errs = append(errs, "manifest: "+report.ManifestError)
	}
	if report.ModelProfilesError != "" {
		errs = append(errs, "model_profiles: "+report.ModelProfilesError)
	}
	if report.StarterTemplatesError != "" {
		errs = append(errs, "starter_templates: "+report.StarterTemplatesError)
	}
	for _, dep := range report.Dependencies {
		if dep.Blocking {
			errs = append(errs, dep.Name+": "+dep.Details)
		}
	}
	return errs
}
