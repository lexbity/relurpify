package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	runtimesvc "codeburg.org/lexbit/relurpify/app/relurpish/runtime"
	"codeburg.org/lexbit/relurpify/capability/ports"
	fsandbox "codeburg.org/lexbit/relurpify/capability/sandbox"
	governanceports "codeburg.org/lexbit/relurpify/governance/ports"
	govsandbox "codeburg.org/lexbit/relurpify/governance/sandbox"
	"codeburg.org/lexbit/relurpify/platform/llm"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

// fakeCommandRunner is the sandbox runner stand-in: every command succeeds
// without touching the host.
type fakeCommandRunner struct{}

func (r *fakeCommandRunner) Run(context.Context, fsandbox.CommandRequest) (*ports.CommandResult, error) {
	return &ports.CommandResult{ExitCode: 0}, nil
}

// fakeSandboxRuntime satisfies governanceports.SandboxRuntime without a real
// backend, mirroring the e2e harness double.
type fakeSandboxRuntime struct {
	policy govsandbox.SandboxPolicy
}

func (f *fakeSandboxRuntime) Verify(context.Context) error                  { return nil }
func (f *fakeSandboxRuntime) ValidatePolicy(govsandbox.SandboxPolicy) error { return nil }
func (f *fakeSandboxRuntime) ApplyPolicy(_ context.Context, p govsandbox.SandboxPolicy) error {
	f.policy = p
	return nil
}
func (f *fakeSandboxRuntime) Policy() govsandbox.SandboxPolicy { return f.policy }
func (f *fakeSandboxRuntime) RunConfig() govsandbox.SandboxConfig {
	return govsandbox.SandboxConfig{}
}
func (f *fakeSandboxRuntime) Name() string { return "fake" }
func (f *fakeSandboxRuntime) NewCommandRunner(*fsandbox.CommandRunnerConfig) (fsandbox.CommandRunner, error) {
	return &fakeCommandRunner{}, nil
}

// offlineTestConfig builds a normalized config over a fresh temp workspace
// with the offline inference provider and fake sandbox legs, so the CLI
// command paths boot hermetically.
func offlineTestConfig(t *testing.T) runtimesvc.Config {
	t.Helper()
	workspace := t.TempDir()
	cfg := runtimesvc.DefaultConfig()
	cfg.Workspace = workspace
	require.NoError(t, cfg.Normalize())
	cfg.InferenceProvider = "offline"
	cfg.InferenceModel = "offline-synthetic"
	cfg.SecurityRunner = &fakeCommandRunner{}
	cfg.SandboxBackendFactory = func(context.Context, string, govsandbox.SandboxConfig, string, string) (governanceports.SandboxRuntime, error) {
		return &fakeSandboxRuntime{}, nil
	}
	return cfg
}

// useConfig swaps the package-global boot config for the duration of the test.
func useConfig(t *testing.T, c runtimesvc.Config) {
	t.Helper()
	origCfg, origSecrets := cfg, secrets
	t.Cleanup(func() { cfg, secrets = origCfg, origSecrets })
	cfg = c
	secrets = config.Secrets{}
}

// executeRoot runs the CLI with the given args and captured stdout/stderr/stdin.
func executeRoot(t *testing.T, args []string, stdin string) (string, error) {
	t.Helper()
	root := newRootCmd()
	var out strings.Builder
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	if stdin != "" {
		root.SetIn(strings.NewReader(stdin))
	}
	err := root.Execute()
	return out.String(), err
}

// doctorOutcomeToleratesEnvironment accepts the degraded outcomes the
// doctor legitimately reports on hosts without a sandbox backend, while
// rejecting anything else.
func doctorOutcomeToleratesEnvironment(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		return
	}
	msg := err.Error()
	require.True(t,
		strings.Contains(msg, "doctor found blocking issues") ||
			strings.Contains(msg, "doctor: workspace degraded") ||
			strings.Contains(msg, "workspace not ready"),
		"unexpected doctor error: %v", err)
}

func TestYesNoRendersBothStates(t *testing.T) {
	assert.Equal(t, "yes", yesNo(true))
	assert.Equal(t, "no", yesNo(false))
}

func TestUniqueStringsDeduplicatesPreservingOrder(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  []string
	}{
		{"empty", nil, nil},
		{"single", []string{"a"}, []string{"a"}},
		{"duplicates", []string{"a", "b", "a", "c", "b"}, []string{"a", "b", "c"}},
		{"already unique", []string{"a", "b"}, []string{"a", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, uniqueStrings(tt.input))
		})
	}
}

func TestDoctorPromptMatchesReportState(t *testing.T) {
	var report runtimesvc.DoctorReport // zero: NeedsInitialization() == true
	assert.Equal(t, "Initialize relurpify_cfg/ from starter templates? [y/N]: ", doctorPrompt(report, false))
	// Initialization dominates: even with --fix, an uninitialized workspace is
	// asked the initialize question.
	assert.Equal(t, "Initialize relurpify_cfg/ from starter templates? [y/N]: ", doctorPrompt(report, true))

	initialized := runtimesvc.DoctorReport{ConfigExists: true, WorkspacePresent: true}
	assert.Equal(t, "Apply doctor fixes? [y/N]: ", doctorPrompt(initialized, false))
	assert.Equal(t, "Overwrite current starter config and manifest from templates? [y/N]: ", doctorPrompt(initialized, true))
}

func TestConfirmDoctorActionAcceptsOnlyAffirmatives(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"y", "y\n", true},
		{"yes", "YES\n", true},
		{"surrounded whitespace", "  y  \n", true},
		{"n", "n\n", false},
		{"empty line", "\n", false},
		{"eof", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out strings.Builder
			assert.Equal(t, tt.want, confirmDoctorAction(strings.NewReader(tt.input), &out, "Proceed? [y/N]: "))
			assert.Equal(t, "Proceed? [y/N]: ", out.String(), "prompt must be written before reading")
		})
	}

	var out strings.Builder
	assert.False(t, confirmDoctorAction(strings.NewReader(""), &out, ""), "empty prompt must fail closed without reading")
	assert.Empty(t, out.String())
}

func TestRenderDoctorReportReadyWorkspace(t *testing.T) {
	report := runtimesvc.DoctorReport{
		Workspace:             "/tmp/ws",
		ConfigRoot:            "/tmp/ws/relurpify_cfg",
		WorkspacePresent:      true,
		ConfigExists:          true,
		ManifestFingerprint:   "abcdef123456",
		ManifestPolicySummary: "deny-by-default",
		ProtectedPaths:        []string{"relurpify_cfg", "relurpify_cfg", ".git"},
		Inference: runtimesvc.InferenceBackendReport{
			Provider:        "ollama",
			Endpoint:        "http://localhost:11434",
			State:           llm.BackendHealthReady,
			ToolCallingMode: "native",
			Models:          []string{"gemma4:e4b", "offline-synthetic"},
			SelectedModel:   "gemma4:e4b",
			SelectedProfile: "default",
			ProfileReason:   "pattern match",
			ProfileSource:   "default.llm.yaml",
		},
		Providers: []runtimesvc.ProviderHealth{
			{Name: "ollama", Selected: true, State: "healthy", Endpoint: "http://localhost:11434", Models: []string{"gemma4:e4b"}},
			{Name: "lmstudio", Selected: false, State: "unreachable", Endpoint: "http://localhost:1234", SetupHint: "start lmstudio", Error: "connection refused"},
		},
		Dependencies: []runtimesvc.DependencyStatus{
			{Name: "workspace", Available: true, Required: true},
			{Name: "disk", Degraded: true, Details: "low space"},
			{Name: "runner", Required: false},
		},
		SandboxReady: true,
		ModelReady:   true,
	}

	var out strings.Builder
	renderDoctorReport(&out, report)
	rendered := out.String()

	assert.Contains(t, rendered, "Workspace: /tmp/ws")
	assert.Contains(t, rendered, "Workspace present: yes")
	assert.Contains(t, rendered, "Config file: yes")
	assert.Contains(t, rendered, "Manifest fingerprint: abcdef123456")
	assert.Contains(t, rendered, "Manifest policy: deny-by-default")
	assert.Contains(t, rendered, "Sandbox roots: relurpify_cfg, .git", "protected paths must be deduplicated")
	assert.Contains(t, rendered, "provider: ollama")
	assert.Contains(t, rendered, "models: gemma4:e4b, offline-synthetic")
	assert.Contains(t, rendered, "selected: gemma4:e4b")
	assert.Contains(t, rendered, "profile: default")
	assert.Contains(t, rendered, "* ollama [healthy]")
	assert.Contains(t, rendered, "  lmstudio [unreachable]")
	assert.Contains(t, rendered, "hint: start lmstudio")
	assert.Contains(t, rendered, "error: connection refused")
	assert.Contains(t, rendered, "- workspace: ok [required]")
	assert.Contains(t, rendered, "- disk: degraded [warning] (low space)")
	assert.Contains(t, rendered, "- runner: missing [warning]")
	assert.Contains(t, rendered, "Result: ready")
}

func TestRenderDoctorReportDegradedAndBlockingWorkspaces(t *testing.T) {
	degraded := runtimesvc.DoctorReport{
		Workspace:        "/tmp/ws",
		WorkspacePresent: true,
		ConfigExists:     true,
		Inference: runtimesvc.InferenceBackendReport{
			State: llm.BackendHealthUnhealthy,
			Error: "no backend",
		},
		SandboxReady: false,
		ModelReady:   false,
	}

	var out strings.Builder
	renderDoctorReport(&out, degraded)
	rendered := out.String()
	assert.Contains(t, rendered, "provider: unknown")
	assert.Contains(t, rendered, "endpoint: -")
	assert.Contains(t, rendered, "state: unhealthy")
	assert.Contains(t, rendered, "tool_calling_mode: unknown")
	assert.Contains(t, rendered, "models: -")
	assert.Contains(t, rendered, "error: no backend")
	assert.Contains(t, rendered, "Result: degraded — sandbox, inference backend not ready")

	// Only the model leg degraded.
	degraded.SandboxReady = true
	out.Reset()
	renderDoctorReport(&out, degraded)
	assert.Contains(t, out.String(), "Result: degraded — inference backend not ready")

	blocking := runtimesvc.DoctorReport{ConfigError: "bad yaml"}
	out.Reset()
	renderDoctorReport(&out, blocking)
	assert.Contains(t, out.String(), "Config file: no (bad yaml)")
	assert.Contains(t, out.String(), "Result: blocking issues detected")
}

func TestStatusCommandRendersJSONSnapshot(t *testing.T) {
	useConfig(t, offlineTestConfig(t))

	out, err := executeRoot(t, []string{"status", "--format", "json"}, "")
	doctorOutcomeToleratesEnvironment(t, err)
	require.NotEmpty(t, out, "status --format json must render output")

	// The runtime may interleave log lines; the JSON document must be present
	// and parse as an object. Use a Decoder so trailing non-JSON output
	// (e.g. cobra's "Error: workspace not ready" on stderr) is ignored.
	idx := strings.Index(out, "{")
	require.GreaterOrEqual(t, idx, 0, "expected JSON output, got: %s", out)
	var snapshot map[string]any
	dec := json.NewDecoder(strings.NewReader(out[idx:]))
	require.NoError(t, dec.Decode(&snapshot),
		"rendered status JSON must parse, got: %s", out)
}

func TestStatusCommandRendersTextSnapshot(t *testing.T) {
	useConfig(t, offlineTestConfig(t))

	out, err := executeRoot(t, []string{"status"}, "")
	doctorOutcomeToleratesEnvironment(t, err)
	assert.Contains(t, out, "workspace:", "text status must render the workspace header")
	assert.Contains(t, out, "ready:", "text status must render the readiness verdict")
}

func TestStatusCommandRejectsUnknownFormat(t *testing.T) {
	useConfig(t, offlineTestConfig(t))

	_, err := executeRoot(t, []string{"status", "--format", "yaml"}, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown --format "yaml"`)
}

func TestStatusCommandOfflineFlagNormalizesProvider(t *testing.T) {
	c := offlineTestConfig(t)
	c.InferenceProvider = "ollama"
	useConfig(t, c)

	// --offline forces the offline provider before normalization; the bogus
	// format keeps the command hermetic (it errors before any backend dial).
	_, err := executeRoot(t, []string{"status", "--offline", "--format", "yaml"}, "")
	require.Error(t, err)
	assert.Equal(t, "offline", cfg.InferenceProvider)
}

func TestDoctorCommandBootstrapsUninitializedWorkspace(t *testing.T) {
	useConfig(t, offlineTestConfig(t))

	out, err := executeRoot(t, []string{"doctor"}, "y\n")
	doctorOutcomeToleratesEnvironment(t, err)

	// The startup bootstrap materializes the starter tree before the doctor
	// prompt ever renders, so the report shows an existing config.
	_, statErr := os.Stat(filepath.Join(cfg.Workspace, "relurpify_cfg", "workspace.yaml"))
	assert.NoError(t, statErr, "doctor on an empty workspace must materialize the starter workspace")
	assert.Contains(t, out, "Config file: yes")
}

func TestDoctorFixDeclinedDoesNotRewriteConfig(t *testing.T) {
	c := offlineTestConfig(t)
	useConfig(t, c)
	require.NoError(t, runtimesvc.InitializeWorkspaceFromTemplates(c, false))
	configPath := filepath.Join(c.Workspace, "relurpify_cfg", "workspace.yaml")
	before, err := os.ReadFile(configPath)
	require.NoError(t, err)

	out, runErr := executeRoot(t, []string{"doctor", "--fix"}, "n\n")
	doctorOutcomeToleratesEnvironment(t, runErr)

	assert.Contains(t, out, "Overwrite current starter config and manifest from templates? [y/N]:")
	after, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after), "declined --fix must leave the config byte-identical")
}

func TestDoctorFixConfirmedRewritesConfig(t *testing.T) {
	c := offlineTestConfig(t)
	useConfig(t, c)
	require.NoError(t, runtimesvc.InitializeWorkspaceFromTemplates(c, false))

	out, runErr := executeRoot(t, []string{"doctor", "--fix"}, "y\n")
	doctorOutcomeToleratesEnvironment(t, runErr)

	assert.Contains(t, out, "Workspace starter configuration written to relurpify_cfg/")
}

func TestDoctorSetProviderSwitchesProviderAndRerenders(t *testing.T) {
	c := offlineTestConfig(t)
	useConfig(t, c)
	require.NoError(t, runtimesvc.InitializeWorkspaceFromTemplates(c, false))

	out, err := executeRoot(t, []string{"doctor", "--set-provider", "ollama"}, "")
	doctorOutcomeToleratesEnvironment(t, err)
	assert.Contains(t, out, `Switched to provider "ollama"`)

	providerPath := config.New(c.Workspace).RuntimeProvidersFile()
	data, readErr := os.ReadFile(providerPath)
	require.NoError(t, readErr, "runtime providers file must be written")
	assert.Equal(t, "provider: ollama\n", string(data))
	assert.Equal(t, "ollama", cfg.InferenceProvider)
}

func TestDoctorSetProviderRejectsUnknownProvider(t *testing.T) {
	c := offlineTestConfig(t)
	useConfig(t, c)
	require.NoError(t, runtimesvc.InitializeWorkspaceFromTemplates(c, false))

	_, err := executeRoot(t, []string{"doctor", "--set-provider", "does-not-exist"}, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `provider "does-not-exist" not found in catalog`)
}

func TestDoctorSetProviderRejectsBlankProvider(t *testing.T) {
	useConfig(t, offlineTestConfig(t))

	_, err := executeRoot(t, []string{"doctor", "--set-provider", "   "}, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider name required")
}

func TestDoctorSetProviderRejectsProviderMissingFromEmptyCatalog(t *testing.T) {
	useConfig(t, offlineTestConfig(t)) // fresh temp workspace: no relurpify_cfg, so no providers

	_, err := executeRoot(t, []string{"doctor", "--set-provider", "ollama"}, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `provider "ollama" not found in catalog`,
		"an uninitialized workspace has an empty provider catalog")
}
