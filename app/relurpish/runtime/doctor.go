package runtime

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/ayenitd"
	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/sandbox"
	"codeburg.org/lexbit/relurpify/governance/permissions"
	"codeburg.org/lexbit/relurpify/platform/fs"
	"codeburg.org/lexbit/relurpify/platform/llm"
	"codeburg.org/lexbit/relurpify/userconfig/config"
	"codeburg.org/lexbit/relurpify/userconfig/modelselect"
	"codeburg.org/lexbit/relurpify/userconfig/templates"
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
	Workspace             string
	ConfigRoot            string
	WorkspacePresent      bool
	ConfigExists          bool
	ManifestExists        bool
	ModelProfilesExists   bool
	StarterTemplatesReady bool
	ConfigError           string
	ManifestError         string
	ModelProfilesError    string
	StarterTemplatesError string
	ManifestWarnings      []string
	DeprecationNotices    []string
	ProtectedPaths        []string
	ManifestFingerprint   string
	ManifestPolicySummary string
	Inference             InferenceBackendReport
	Dependencies          []DependencyStatus
	CheckedAt             time.Time
}

func (r DoctorReport) HasBlockingIssues() bool {
	if !r.ConfigExists {
		return true
	}
	if r.ConfigError != "" || r.ManifestError != "" || r.ModelProfilesError != "" || r.StarterTemplatesError != "" {
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
	return !r.WorkspacePresent || !r.ConfigExists
}

// Ready reports whether the workspace can start without landing in the Doctor tab.
func (r DoctorReport) Ready() bool {
	return !r.HasBlockingIssues()
}

// BuildDoctorReport checks workspace state and local runtime dependencies
// without requiring the runtime to start successfully.
func BuildDoctorReport(ctx context.Context, cfg Config, secrets config.Secrets) DoctorReport {
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
		if loaded, err := config.LoadRuntimeWorkspaceConfig(cfg.ConfigPath); err != nil {
			report.ConfigError = err.Error()
		} else if loaded.SandboxBackend != "" && cfg.SandboxBackend == "" {
			cfg.SandboxBackend = loaded.SandboxBackend
			if loaded.Provider != "" && cfg.InferenceProvider == "" {
				cfg.InferenceProvider = loaded.Provider
			}
			if loaded.Model != "" && cfg.InferenceModel == "" {
				cfg.InferenceModel = loaded.Model
			}
		}
	}
	if _, err := os.Stat(cfg.ManifestPath); err == nil {
		report.ManifestExists = true
		if snapshot, err := config.LoadDocument(cfg.ManifestPath); err != nil {
			report.ManifestError = err.Error()
		} else {
			report.ManifestFingerprint = hex.EncodeToString(snapshot.Fingerprint[:])
			report.ManifestPolicySummary = summarizeManifestPolicy(snapshot.Document)
			if len(snapshot.Warnings) > 0 {
				report.ManifestWarnings = append(report.ManifestWarnings, snapshot.Warnings...)
				report.DeprecationNotices = append(report.DeprecationNotices, snapshot.Warnings...)
			}
		}
	}
	report.ProtectedPaths = config.New(cfg.Workspace).GovernanceRoots(cfg.ManifestPath, cfg.ConfigPath, config.DefaultWorkspaceConfigPath(cfg.Workspace))

	resolver := templates.NewResolver(cfg.SharedRoot)
	if starterConfig, err := resolver.ResolveWorkspaceConfigTemplate(); err == nil {
		if starterManifest, err := resolver.ResolveWorkspaceAgentTemplate(); err == nil {
			sandboxTemplate, sandboxErr := resolver.ResolveWorkspaceSecurityTemplate("sandbox")
			shellTemplate, shellErr := resolver.ResolveWorkspaceSecurityTemplate("shell")
			localToolTemplate, localToolErr := resolver.ResolveWorkspaceSecurityTemplate("localtool")
			ingestionTemplate, ingestionErr := resolver.ResolveWorkspaceSecurityTemplate("workspaceingestion")
			if sandboxErr == nil && shellErr == nil && localToolErr == nil && ingestionErr == nil {
				report.StarterTemplatesReady = true
				_ = starterConfig
				_ = starterManifest
				_ = sandboxTemplate
				_ = shellTemplate
				_ = localToolTemplate
				_ = ingestionTemplate
			} else {
				report.StarterTemplatesError = firstNonEmpty(errorString(sandboxErr), errorString(shellErr), errorString(localToolErr), errorString(ingestionErr))
			}
		} else {
			report.StarterTemplatesError = err.Error()
		}
	} else {
		report.StarterTemplatesError = err.Error()
	}

	var env EnvironmentReport
	backend, err := llm.New(llm.ProviderConfigFromRuntimeConfig(cfg), llm.ProviderSecrets{
		APIKey: secrets.LLMAPIKey,
	})
	if err != nil {
		env = ProbeEnvironment(ctx, cfg, secrets, nil)
		if env.Inference.Error == "" {
			env.Inference.Error = err.Error()
		}
		env.Inference.State = llm.BackendHealthUnhealthy
	} else {
		defer func() { _ = backend.Close() }()
		env = ProbeEnvironment(ctx, cfg, secrets, backend)
	}
	report.Inference = env.Inference
	if reg, err := modelselect.LoadProfileRegistry(config.New(cfg.Workspace).ModelProfilesDir()); err == nil {
		resolution := reg.Resolve(cfg.InferenceProvider, report.Inference.SelectedModel)
		if resolution.SourcePath != "" {
			report.ModelProfilesExists = true
		} else {
			report.ModelProfilesError = "no workspace model profile matched the selected model"
		}
	} else {
		report.ModelProfilesError = err.Error()
	}
	// Convert ayenitd probe results
	// Map available Config fields to ayenitd.WorkspaceConfig.
	// Some fields may be missing in Config; use zero values.
	ayenitdCfg := ayenitd.WorkspaceConfig{
		Workspace:                  cfg.Workspace,
		ManifestPath:               cfg.ManifestPath,
		InferenceProvider:          cfg.InferenceProvider,
		InferenceEndpoint:          cfg.InferenceEndpoint,
		InferenceModel:             cfg.InferenceModel,
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
		SandboxBackend:             cfg.SandboxBackend,
		Sandbox:                    cfg.Sandbox,
	}
	ayenitdResults := ayenitd.ProbeWorkspace(ctx, ayenitdCfg, llm.ProviderSecrets{APIKey: secrets.LLMAPIKey}, nil)
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
	deps = append(deps, DependencyStatus{
		Name:      "starter-templates",
		Required:  true,
		Available: report.StarterTemplatesReady,
		Blocking:  report.StarterTemplatesError != "",
		Details:   firstNonEmpty(report.StarterTemplatesError, "workspace template archive available"),
	})
	deps = append(deps, DependencyStatus{
		Name:      "model-profile",
		Required:  true,
		Available: report.ModelProfilesExists,
		Blocking:  report.ModelProfilesError != "",
		Details:   firstNonEmpty(report.ModelProfilesError, report.Inference.SelectedProfile, "workspace profile available"),
	})
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
	deps = append(deps, DependencyStatus{
		Name:      "inference",
		Required:  true,
		Available: env.Inference.State == llm.BackendHealthReady || env.Inference.State == llm.BackendHealthDegraded,
		Blocking:  env.Inference.State == llm.BackendHealthUnhealthy,
		Details:   firstNonEmpty(env.Inference.SelectedModel, env.Inference.Error),
	})
	deps = append(deps, detectChromiumStatus(ctx, cfg.CommandPolicy))
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
	if err := os.MkdirAll(paths.ConfigRoot(), fs.PublicDirMode); err != nil { // public: config root
		return err
	}
	resolver := templates.NewResolver(cfg.SharedRoot)
	configTemplate, err := resolver.ResolveWorkspaceConfigTemplate()
	if err != nil {
		return fmt.Errorf("resolve workspace config template: %w", err)
	}
	sandboxTemplate, err := resolver.ResolveWorkspaceSecurityTemplate("sandbox")
	if err != nil {
		return fmt.Errorf("resolve sandbox security template: %w", err)
	}
	shellTemplate, err := resolver.ResolveWorkspaceSecurityTemplate("shell")
	if err != nil {
		return fmt.Errorf("resolve shell security template: %w", err)
	}
	localToolTemplate, err := resolver.ResolveWorkspaceSecurityTemplate("localtool")
	if err != nil {
		return fmt.Errorf("resolve localtool security template: %w", err)
	}
	ingestionTemplate, err := resolver.ResolveWorkspaceSecurityTemplate("workspaceingestion")
	if err != nil {
		return fmt.Errorf("resolve ingestion security template: %w", err)
	}
	workspaceConfigPath := cfg.ConfigPath
	if workspaceConfigPath == "" {
		workspaceConfigPath = config.DefaultWorkspaceStateConfigPath(cfg.Workspace)
	}
	if err := copyTemplateFile(configTemplate, workspaceConfigPath, cfg.Workspace, overwrite); err != nil {
		return err
	}
	securityDir := filepath.Join(paths.ConfigRoot(), "security")
	for _, dir := range []string{securityDir} {
		if err := os.MkdirAll(dir, fs.PublicDirMode); err != nil { // public: security policies dir
			return err
		}
	}
	if err := copyTemplateFile(sandboxTemplate, filepath.Join(securityDir, "sandbox.policy.yaml"), cfg.Workspace, overwrite); err != nil {
		return err
	}
	if err := copyTemplateFile(shellTemplate, filepath.Join(securityDir, "shell.policy.yaml"), cfg.Workspace, overwrite); err != nil {
		return err
	}
	if err := copyTemplateFile(localToolTemplate, filepath.Join(securityDir, "localtool.policy.yaml"), cfg.Workspace, overwrite); err != nil {
		return err
	}
	if err := copyTemplateFile(ingestionTemplate, filepath.Join(securityDir, "workspaceingestion.policy.yaml"), cfg.Workspace, overwrite); err != nil {
		return err
	}
	stateDir := config.DefaultWorkspaceStateDir(cfg.Workspace)
	for _, dir := range []string{
		paths.AgentsDir(),
		stateDir,
		filepath.Join(stateDir, "logs"),
		filepath.Join(stateDir, "telemetry"),
		filepath.Join(stateDir, "memory"),
		filepath.Join(stateDir, "sessions"),
		filepath.Join(stateDir, "test_run"),
	} {
		if err := os.MkdirAll(dir, fs.PublicDirMode); err != nil { // public: runtime state dirs
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
	data, err := os.ReadFile(filepath.Clean(src))
	if err != nil {
		return err
	}
	rendered := strings.ReplaceAll(string(data), "${workspace}", filepath.ToSlash(workspace))
	cleanDst := filepath.Clean(dst)
	if !strings.HasPrefix(cleanDst, filepath.Clean(workspace)) {
		return fmt.Errorf("path traversal: %s", dst)
	}
	if err := os.MkdirAll(filepath.Dir(cleanDst), fs.PublicDirMode); err != nil { // public: template dst dir
		return err
	}
	return os.WriteFile(filepath.Clean(cleanDst), []byte(rendered), fs.PublicFileMode) //nolint:gosec // public: rendered template
}

func detectChromiumStatus(ctx context.Context, policy sandbox.CommandPolicy) DependencyStatus {
	binaries := []string{"chromium", "chromium-browser", "google-chrome", "google-chrome-stable"}
	for _, name := range binaries {
		path, err := execLookPath(name)
		if err != nil {
			continue
		}
		version, _ := runCommand(ctx, policy, path, "--version")
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
		return "sandbox unavailable — agent runtime will FAIL TO START (no host-exec fallback)"
	}
	// If it's an error message, append the note
	if strings.Contains(detail, "error") || strings.Contains(detail, "not found") {
		return detail + " — agent runtime will FAIL TO START (no host-exec fallback)"
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

func summarizeManifestPolicy(doc *config.Document) string {
	if doc == nil {
		return ""
	}
	var parts []string
	permNode, ok := doc.Section("permissions")
	if ok {
		var permSpec permissions.PermissionSet
		if err := permNode.Decode(&permSpec); err == nil {
			permCount := len(permSpec.FileSystem) + len(permSpec.Executables) + len(permSpec.Network)
			if permCount > 0 {
				parts = append(parts, fmt.Sprintf("permissions=%d", permCount))
			}
		}
	}
	agentNode, ok := doc.Section("agent")
	if ok {
		var agentSpec agentspec.AgentRuntimeSpec
		if err := agentNode.Decode(&agentSpec); err == nil {
			parts = append(parts, fmt.Sprintf("tool-calling=%s", agentSpec.ResolveToolCallingIntent()))
		}
	}
	return strings.Join(parts, ", ")
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

var execLookPath = func(file string) (string, error) {
	return execLookPathImpl(file)
}
