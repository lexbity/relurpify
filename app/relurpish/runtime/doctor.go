package runtime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/ayenitd"
	"github.com/lexcodex/relurpify/framework/config"
	"github.com/lexcodex/relurpify/framework/manifest"
	"github.com/lexcodex/relurpify/framework/templates"
	"github.com/lexcodex/relurpify/platform/llm"
)

// DependencyStatus captures one local dependency check.
type DependencyStatus struct {
	Name      string
	Required  bool
	Available bool
	Blocking  bool
	Details   string
}

// DoctorReport summarizes workspace readiness and local dependency state.
type DoctorReport struct {
	Workspace        string
	ConfigRoot       string
	WorkspacePresent bool
	ConfigExists     bool
	ManifestExists   bool
	ConfigError      string
	ManifestError    string
	Inference        InferenceBackendReport
	Dependencies     []DependencyStatus
	CheckedAt        time.Time
}

func (r DoctorReport) HasBlockingIssues() bool {
	if !r.ConfigExists || !r.ManifestExists {
		return true
	}
	if r.ConfigError != "" || r.ManifestError != "" {
		return true
	}
	for _, dep := range r.Dependencies {
		if dep.Blocking {
			return true
		}
	}
	return false
}

func (r DoctorReport) NeedsInitialization() bool {
	return !r.WorkspacePresent || !r.ConfigExists || !r.ManifestExists
}

// BuildDoctorReport checks workspace state and local runtime dependencies
// without requiring the runtime to start successfully.
func BuildDoctorReport(ctx context.Context, cfg Config) DoctorReport {
	paths := config.New(cfg.Workspace)
	report := DoctorReport{
		Workspace:  cfg.Workspace,
		ConfigRoot: paths.ConfigRoot(),
		CheckedAt:  time.Now().UTC(),
	}
	if info, err := os.Stat(paths.ConfigRoot()); err == nil && info.IsDir() {
		report.WorkspacePresent = true
	}
	if _, err := os.Stat(cfg.ConfigPath); err == nil {
		report.ConfigExists = true
		if _, err := LoadWorkspaceConfig(cfg.ConfigPath); err != nil {
			report.ConfigError = err.Error()
		}
	}
	if _, err := os.Stat(cfg.ManifestPath); err == nil {
		report.ManifestExists = true
		if _, err := manifest.LoadAgentManifest(cfg.ManifestPath); err != nil {
			report.ManifestError = err.Error()
		}
	}

	var env EnvironmentReport
	backend, err := llm.New(llm.ProviderConfigFromRuntimeConfig(cfg))
	if err != nil {
		env = ProbeEnvironment(ctx, cfg, nil)
		if env.Inference.Error == "" {
			env.Inference.Error = err.Error()
		}
		env.Inference.State = llm.BackendHealthUnhealthy
	} else {
		defer backend.Close()
		env = ProbeEnvironment(ctx, cfg, backend)
	}
	report.Inference = env.Inference
	// Convert ayenitd probe results
	// Map available Config fields to ayenitd.WorkspaceConfig.
	// Some fields may be missing in Config; use zero values.
	ayenitdCfg := ayenitd.WorkspaceConfig{
		Workspace:                  cfg.Workspace,
		ManifestPath:               cfg.ManifestPath,
		InferenceProvider:          cfg.InferenceProvider,
		InferenceEndpoint:          cfg.InferenceEndpoint,
		InferenceModel:             cfg.InferenceModel,
		InferenceAPIKey:            cfg.InferenceAPIKey,
		InferenceNativeToolCalling: cfg.InferenceNativeToolCalling,
		ConfigPath:                 cfg.ConfigPath,
		AgentsDir:                  cfg.AgentsDir,
		AgentName:                  cfg.AgentName,
		LogPath:                    cfg.LogPath,
		TelemetryPath:              cfg.TelemetryPath,
		EventsPath:                 cfg.EventsPath,
		MemoryPath:                 cfg.MemoryPath,
		HITLTimeout:                cfg.HITLTimeout,
		AuditLimit:                 cfg.AuditLimit,
		Sandbox:                    cfg.Sandbox,
	}
	ayenitdResults := ayenitd.ProbeWorkspace(ayenitdCfg, nil)
	var deps []DependencyStatus
	for _, r := range ayenitdResults {
		deps = append(deps, DependencyStatus{
			Name:      r.Name,
			Required:  r.Required,
			Available: r.OK,
			Blocking:  r.Required && !r.OK,
			Details:   r.Message,
		})
	}
	// Keep existing sandbox and chromium checks
	deps = append(deps, DependencyStatus{
		Name:      "runsc",
		Required:  false,
		Available: env.Sandbox.Runsc.Error == "",
		Blocking:  false,
		Details:   formatSandboxDetail(firstNonEmpty(env.Sandbox.Runsc.Version, env.Sandbox.Runsc.Error)),
	})
	deps = append(deps, DependencyStatus{
		Name:      "docker",
		Required:  false,
		Available: env.Sandbox.Docker.Error == "",
		Blocking:  false,
		Details:   formatSandboxDetail(firstNonEmpty(env.Sandbox.Docker.Version, env.Sandbox.Docker.Error)),
	})
	// Inference backend check is already covered by ayenitd, but keep for compatibility.
	deps = append(deps, DependencyStatus{
		Name:      "inference",
		Required:  true,
		Available: env.Inference.State == llm.BackendHealthReady || env.Inference.State == llm.BackendHealthDegraded,
		Blocking:  env.Inference.State == llm.BackendHealthUnhealthy,
		Details:   firstNonEmpty(env.Inference.SelectedModel, env.Inference.Error),
	})
	deps = append(deps, detectChromiumStatus(ctx))
	report.Dependencies = deps
	return report
}

// InitializeWorkspaceFromTemplates materializes starter workspace config under
// relurpify_cfg using the shared template resolver.
func InitializeWorkspaceFromTemplates(cfg Config, overwrite bool) error {
	if cfg.Workspace == "" {
		return fmt.Errorf("workspace path required")
	}
	paths := config.New(cfg.Workspace)
	if err := os.MkdirAll(paths.ConfigRoot(), 0o755); err != nil {
		return err
	}
	resolver := templates.NewResolver()
	configTemplate, err := resolver.ResolveWorkspaceConfigTemplate()
	if err != nil {
		return fmt.Errorf("resolve workspace config template: %w", err)
	}
	manifestTemplate, err := resolver.ResolveWorkspaceManifestTemplate()
	if err != nil {
		return fmt.Errorf("resolve workspace manifest template: %w", err)
	}
	if err := copyTemplateFile(configTemplate, cfg.ConfigPath, cfg.Workspace, overwrite); err != nil {
		return err
	}
	if err := copyTemplateFile(manifestTemplate, cfg.ManifestPath, cfg.Workspace, overwrite); err != nil {
		return err
	}
	for _, dir := range []string{
		paths.AgentsDir(),
		paths.SkillsDir(),
		paths.LogsDir(),
		paths.TelemetryDir(),
		paths.MemoryDir(),
		paths.SessionsDir(),
		paths.TestRunsDir(),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func copyTemplateFile(src, dst, workspace string, overwrite bool) error {
	if !overwrite {
		if _, err := os.Stat(dst); err == nil {
			return nil
		}
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	rendered := strings.ReplaceAll(string(data), "${workspace}", filepath.ToSlash(workspace))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, []byte(rendered), 0o644)
}

func detectChromiumStatus(ctx context.Context) DependencyStatus {
	binaries := []string{"chromium", "chromium-browser", "google-chrome", "google-chrome-stable"}
	for _, name := range binaries {
		path, err := execLookPath(name)
		if err != nil {
			continue
		}
		version, _ := runCommand(ctx, path, "--version")
		return DependencyStatus{
			Name:      "chromium",
			Required:  false,
			Available: true,
			Blocking:  false,
			Details:   strings.TrimSpace(firstNonEmpty(version, path)),
		}
	}
	return DependencyStatus{
		Name:      "chromium",
		Required:  false,
		Available: false,
		Blocking:  false,
		Details:   "not found",
	}
}

func formatSandboxDetail(detail string) string {
	if detail == "" {
		return "sandbox unavailable — tool sandboxing disabled"
	}
	// If it's an error message, append the note
	if strings.Contains(detail, "error") || strings.Contains(detail, "not found") {
		return detail + " — sandbox unavailable — tool sandboxing disabled"
	}
	// If it's a version string, we're good
	return detail
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

var execLookPath = func(file string) (string, error) {
	return execLookPathImpl(file)
}
