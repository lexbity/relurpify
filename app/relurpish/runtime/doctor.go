package runtime

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/ayenitd"
	"codeburg.org/lexbit/relurpify/capability/sandbox"
	"codeburg.org/lexbit/relurpify/named/euclo/euclocontract"
	platformfs "codeburg.org/lexbit/relurpify/platform/fs"
	"codeburg.org/lexbit/relurpify/platform/llm"
	"codeburg.org/lexbit/relurpify/userconfig/config"
	"codeburg.org/lexbit/relurpify/userconfig/modelselect"
	te "codeburg.org/lexbit/relurpify/userconfig/templates"
	templatesembed "codeburg.org/lexbit/relurpify/userconfig/templates/embedfs"
)

// DependencyStatus captures one local dependency check.
type DependencyStatus struct {
	Name      string
	Required  bool
	Available bool
	Degraded  bool
	Blocking  bool
	Details   string
}

// ProviderHealth captures the health and metadata of a single catalog provider.
type ProviderHealth struct {
	Name      string
	Kind      string
	Endpoint  string
	Models    []string
	State     string
	SetupHint string
	Selected  bool
	Error     string
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
	ContractSource        string // "builtin+split" | "manifest"
	ManifestFingerprint   string
	ManifestPolicySummary string
	Inference             InferenceBackendReport
	Providers             []ProviderHealth
	Dependencies          []DependencyStatus
	CheckedAt             time.Time

	// SandboxReady is true when the sandbox backend is verified,
	// policy is loaded, and a filesystem scope is constructed.
	// When false, tools are hard-denied regardless of model status.
	SandboxReady bool
	// ModelReady is true when the inference backend reports healthy.
	// When false, guest chat is blocked but tools may still run
	// if SandboxReady is true (e.g. tape/offline sessions).
	ModelReady bool
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

// Ready reports whether the workspace is fully operational:
// both sandbox and model are verified, and no blocking issues exist.
func (r DoctorReport) Ready() bool {
	return r.SandboxReady && r.ModelReady && !r.HasBlockingIssues()
}

// BuildDoctorReport checks workspace state and local runtime dependencies
// without requiring the runtime to start successfully.
func BuildDoctorReport(ctx context.Context, cfg Config, secrets config.Secrets) DoctorReport {
	// Diagnostics are non-interactive: strip the HITL-gated command policy so
	// dependency/chromium probes (and ProbeEnvironment below) never block on
	// approval. See diagnosticConfig.
	cfg = diagnosticConfig(cfg)
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
		// Try V1 nested format first (relurpify/workspace/v1).
		// If V1 is unavailable, also inspect the flat workspace config.
		v1Cfg, v1Err := config.LoadRuntimeWorkspaceConfigV1(cfg.ConfigPath)
		if v1Err == nil {
			if v1Cfg.Model.Provider != "" && (strings.TrimSpace(cfg.InferenceProvider) == "" || strings.EqualFold(strings.TrimSpace(cfg.InferenceProvider), "ollama")) {
				cfg.InferenceProvider = v1Cfg.Model.Provider
			}
			if v1Cfg.Model.Name != "" {
				report.Inference.SelectedModel = v1Cfg.Model.Name
			}
			if v1Cfg.Sandbox.Backend != "" && cfg.SandboxBackend == "" {
				cfg.SandboxBackend = v1Cfg.Sandbox.Backend
			}
		}
		// Also inspect the flat loader for fields not present in V1
		// (TapePath, Agents, etc.).
		if loaded, err := config.LoadRuntimeWorkspaceConfig(cfg.ConfigPath); err == nil {
			if loaded.SandboxBackend != "" && cfg.SandboxBackend == "" {
				cfg.SandboxBackend = loaded.SandboxBackend
			}
			if loaded.Provider != "" && (strings.TrimSpace(cfg.InferenceProvider) == "" || strings.EqualFold(strings.TrimSpace(cfg.InferenceProvider), "ollama")) {
				cfg.InferenceProvider = loaded.Provider
			}
			if loaded.Model != "" && cfg.InferenceModel == "" {
				cfg.InferenceModel = loaded.Model
			}
			if loaded.TapePath != "" && cfg.InferenceTapePath == "" {
				cfg.InferenceTapePath = loaded.TapePath
			}
		} else if v1Err != nil {
			// Both loaders failed — surface the V1 error.
			report.ConfigError = v1Err.Error()
		}
	}
	if strings.EqualFold(strings.TrimSpace(cfg.InferenceProvider), "tape") && strings.TrimSpace(cfg.InferenceTapePath) == "" {
		cfg.InferenceTapePath = config.DefaultWorkspaceStateTapeFile(cfg.Workspace)
	}
	report.ContractSource = "builtin+split"
	fp := config.ContractFingerprint(euclocontract.DefaultContract(), cfg.Workspace)
	report.ManifestFingerprint = fmt.Sprintf("%x", fp)
	// Deduplicate sandbox roots (FR-8)
	rawPaths := config.New(cfg.Workspace).GovernanceRoots(config.DefaultWorkspaceConfigPath(cfg.Workspace))
	report.ProtectedPaths = uniqueStrings(rawPaths)

	resolver := te.NewResolver(cfg.SharedRoot)
	if starterConfig, err := resolver.ResolveWorkspaceConfigTemplate(); err == nil {
		sandboxTemplate, sandboxErr := resolver.ResolveWorkspaceSecurityTemplate("sandbox")
		shellTemplate, shellErr := resolver.ResolveWorkspaceSecurityTemplate("shell")
		localToolTemplate, localToolErr := resolver.ResolveWorkspaceSecurityTemplate("localtool")
		ingestionTemplate, ingestionErr := resolver.ResolveWorkspaceSecurityTemplate("workspaceingestion")
		if sandboxErr == nil && shellErr == nil && localToolErr == nil && ingestionErr == nil {
			report.StarterTemplatesReady = true
			_ = starterConfig
			_ = sandboxTemplate
			_ = shellTemplate
			_ = localToolTemplate
			_ = ingestionTemplate
		} else {
			report.StarterTemplatesError = firstNonEmpty(errorString(sandboxErr), errorString(shellErr), errorString(localToolErr), errorString(ingestionErr))
		}
	} else {
		// Check embedded templates as fallback.
		report.StarterTemplatesReady = checkEmbeddedTemplates()
		if !report.StarterTemplatesReady {
			report.StarterTemplatesError = err.Error()
		}
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
	// Build provider catalog health list.
	bundle, diags, err := config.LoadDiagnostic(config.LoadOptions{WorkspaceRoot: cfg.Workspace})
	if err == nil && bundle.Config != nil {
		reg, _ := buildProviderRegistry(bundle.Config.Model.Providers)
		if reg != nil {
			var providerHealthList []ProviderHealth
			for _, def := range bundle.Config.Model.Providers {
				ph := ProviderHealth{
					Name:      def.Name,
					Kind:      def.Kind,
					Endpoint:  def.Endpoint,
					SetupHint: def.SetupHint,
				}
				// Check if this is the currently selected provider
				if strings.EqualFold(def.Name, cfg.InferenceProvider) {
					ph.Selected = true
				}
				// Probe the provider's health
				pcfg := llm.ProviderConfig{
					Provider: def.Name,
					Kind:     def.Kind,
					Endpoint: def.Endpoint,
				}
				if pbe, pberr := llm.New(pcfg, llm.ProviderSecrets{APIKey: secrets.LLMAPIKey}); pberr == nil {
					if phState, phErr := pbe.Health(ctx); phState != nil {
						ph.State = string(phState.State)
					} else if phErr != nil {
						ph.State = "unhealthy"
						ph.Error = phErr.Error()
					}
					if models, modErr := pbe.ListModels(ctx); modErr == nil {
						for _, m := range models {
							ph.Models = append(ph.Models, m.Name)
						}
					}
					_ = pbe.Close()
				} else {
					ph.State = "unhealthy"
					ph.Error = pberr.Error()
				}
				providerHealthList = append(providerHealthList, ph)
			}
			report.Providers = providerHealthList
		}
		// Model profile check using the same bundle.
		regProfiles := modelselect.NewProfileRegistryFromProfiles(bundle.Config.Model.Profiles)
		resolution := regProfiles.Resolve(cfg.InferenceProvider, report.Inference.SelectedModel)
		if resolution.SourcePath != "" {
			report.ModelProfilesExists = true
		} else if diag := firstConfigDiagnostic(diags, "profile"); diag != nil && strings.EqualFold(strings.TrimSpace(diag.Severity), "blocking") {
			report.ModelProfilesError = diag.Message
		} else {
			report.ModelProfilesError = "no workspace model profile matched the selected model"
		}
	} else if err != nil {
		report.ModelProfilesError = err.Error()
	} else {
		report.ModelProfilesError = "workspace config bundle unavailable"
	}
	// Convert ayenitd probe results
	// Map available Config fields to ayenitd.WorkspaceConfig.
	// Some fields may be missing in Config; use zero values.
	ayenitdCfg := ayenitd.WorkspaceConfig{
		Workspace:                  cfg.Workspace,
		InferenceProvider:          cfg.InferenceProvider,
		InferenceEndpoint:          cfg.InferenceEndpoint,
		InferenceModel:             cfg.InferenceModel,
		InferenceTapePath:          cfg.InferenceTapePath,
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
		if r.Name == "inference_backend" {
			continue // shown in dedicated Inference backend block (FR-8)
		}
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
	runscOK := env.Sandbox.Runsc.Error == ""
	dockerOK := env.Sandbox.Docker.Error == ""
	dockerDegraded := env.Sandbox.Docker.Path != "" && !dockerOK
	deps = append(deps, DependencyStatus{
		Name:      "sandbox_backend",
		Required:  true,
		Available: runscOK || dockerOK,
		Blocking:  report.ConfigError == "" && (!runscOK && !dockerOK),
		Details:   formatSandboxDetail(firstNonEmpty(env.Sandbox.Runsc.Version, env.Sandbox.Docker.Version, env.Sandbox.Runsc.Error, env.Sandbox.Docker.Error)),
	})
	deps = append(deps, DependencyStatus{
		Name:      "runsc",
		Required:  false,
		Available: runscOK,
		Blocking:  false,
		Details:   formatSandboxDetail(firstNonEmpty(env.Sandbox.Runsc.Version, env.Sandbox.Runsc.Error)),
	})
	dockerDetail := formatSandboxDetail(firstNonEmpty(env.Sandbox.Docker.Version, env.Sandbox.Docker.Error))
	if dockerDegraded {
		dockerDetail = "degraded: docker installed but unreachable"
	}
	deps = append(deps, DependencyStatus{
		Name:      "docker",
		Required:  false,
		Available: dockerOK,
		Degraded:  dockerDegraded,
		Blocking:  false,
		Details:   dockerDetail,
	})
	// inference_backend is shown in the dedicated "Inference backend:" block,
	// not duplicated here as a dependency entry (FR-8).
	deps = append(deps, detectChromiumStatus(ctx, cfg.CommandPolicy))
	report.Dependencies = deps

	report.SandboxReady = computeSandboxReady(report)
	report.ModelReady = computeModelReady(report)
	return report
}

func uniqueStrings(s []string) []string {
	if len(s) < 2 {
		return s
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(s))
	for _, v := range s {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

func computeSandboxReady(r DoctorReport) bool {
	if r.ConfigError != "" || r.StarterTemplatesError != "" {
		return false
	}
	if !r.ConfigExists || !r.StarterTemplatesReady {
		return false
	}
	return true
}

func computeModelReady(r DoctorReport) bool {
	return r.Inference.State == llm.BackendHealthReady || r.Inference.State == llm.BackendHealthDegraded
}

func checkEmbeddedTemplates() bool {
	efs := templatesembed.DefaultFS()
	// Verify the key template files exist in the embed.
	for _, path := range []string{
		"workspace/workspace.yaml",
		"workspace/model/profiles/default.llm.yaml",
		"workspace/model/provider/ollama.provider.yaml",
		"workspace/security/sandbox.policy.yaml",
		"workspace/security/shell.policy.yaml",
		"workspace/security/localtool.policy.yaml",
		"workspace/security/workspaceingestion.policy.yaml",
	} {
		if _, err := efs.Open(path); err != nil {
			return false
		}
	}
	return true
}

// InitializeWorkspaceFromTemplates materializes the full default workspace
// configuration tree under <workspace>/relurpify_cfg from the embedded template
// bundle: workspace.yaml, model profiles + provider catalog, security policies,
// and tool definitions. When overwrite is false, existing files are preserved
// (idempotent re-run).
func InitializeWorkspaceFromTemplates(cfg Config, overwrite bool) error {
	if cfg.Workspace == "" {
		return fmt.Errorf("workspace path required")
	}
	configRoot := config.New(cfg.Workspace).ConfigRoot()
	if err := os.MkdirAll(configRoot, platformfs.PublicDirMode); err != nil { // public: config root
		return err
	}
	// The embedded template bundle is the canonical, distribution-safe source:
	// it is compiled into the binary, so initialization materializes the full
	// default tree (workspace.yaml, model profiles + provider catalog, security
	// policies, and tool definitions) even for an installed binary with no
	// source checkout on disk. The embedded "workspace/" subtree maps 1:1 onto
	// <workspace>/relurpify_cfg/.
	efs := templatesembed.DefaultFS()
	const templateRoot = "workspace"
	walkErr := fs.WalkDir(efs, templateRoot, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(templateRoot, filepath.FromSlash(p))
		if relErr != nil {
			return relErr
		}
		data, readErr := fs.ReadFile(efs, p)
		if readErr != nil {
			return fmt.Errorf("read embedded template %s: %w", p, readErr)
		}
		return copyTemplateContent(data, filepath.Join(configRoot, rel), cfg.Workspace, overwrite)
	})
	if walkErr != nil {
		return fmt.Errorf("materialize workspace templates: %w", walkErr)
	}
	stateDir := config.DefaultWorkspaceStateDir(cfg.Workspace)
	for _, dir := range []string{
		stateDir,
		filepath.Join(stateDir, "logs"),
		filepath.Join(stateDir, "telemetry"),
		filepath.Join(stateDir, "memory"),
		filepath.Join(stateDir, "sessions"),
		filepath.Join(stateDir, "test_run"),
	} {
		if err := os.MkdirAll(dir, platformfs.PublicDirMode); err != nil { // public: runtime state dirs
			return err
		}
	}
	return nil
}

func copyTemplateContent(data []byte, dst, workspace string, overwrite bool) error {
	if !overwrite {
		if _, err := os.Stat(dst); err == nil {
			return nil
		}
	}
	rendered := strings.ReplaceAll(string(data), "${workspace}", filepath.ToSlash(workspace))
	cleanDst := filepath.Clean(dst)
	if !strings.HasPrefix(cleanDst, filepath.Clean(workspace)) {
		return fmt.Errorf("path traversal: %s", dst)
	}
	if err := os.MkdirAll(filepath.Dir(cleanDst), platformfs.PublicDirMode); err != nil { // public: template dst dir
		return err
	}
	return os.WriteFile(filepath.Clean(cleanDst), []byte(rendered), platformfs.PublicFileMode) //nolint:gosec // workspace-scoped template output after prefix check
}

func detectChromiumStatus(ctx context.Context, policy sandbox.CommandPolicy) DependencyStatus {
	binaries := []string{"chromium", "chromium-browser", "google-chrome", "google-chrome-stable"}
	for _, name := range binaries {
		path, err := exec.LookPath(name)
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

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}


