package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	configmanifest "codeburg.org/lexbit/relurpify/platform/configmanifest"
	"codeburg.org/lexbit/relurpify/userconfig/config/model"
	"codeburg.org/lexbit/relurpify/userconfig/config/security"
)

// FlagSet represents a subset of command-line flags.
type FlagSet interface {
	GetString(name string) (string, error)
	GetBool(name string) (bool, error)
	GetInt(name string) (int, error)
}

// AppConfig consolidated configuration representation.
type AppConfig struct {
	Workspace   WorkspaceConfig
	Security    security.Bundle
	Model       ModelConfig
	Tools       *ToolRegistry
	Editor      string
	SharedRoot  string
	Fingerprint string
}

// ModelConfig groups LLM providers and profiles configuration.
type ModelConfig struct {
	Providers []*model.ResolvedProvider
	Profiles  []*model.ModelProfileConfig
}

// ConfigDiagnostic captures a section-level load issue for doctor/reporting use.
type ConfigDiagnostic struct {
	Path     string
	Section  string
	Severity string
	Message  string
}

// PartialBundle captures the best-effort result of a diagnostic load.
type PartialBundle struct {
	Config  *AppConfig
	Secrets *Secrets
}

// LoadOptions controls the configuration loader inputs.
type LoadOptions struct {
	WorkspaceRoot         string
	EnvOverrides          []string
	CLIFlags              FlagSet
	SubprocessToolFactory func(configmanifest.ToolManifest) any
}

// Load executes the consolidated configuration loading boundary.
func Load(opts LoadOptions) (*AppConfig, *Secrets, error) {
	bundle, diags, err := loadBundle(opts, true)
	if err != nil {
		return nil, nil, err
	}
	for _, diag := range diags {
		if strings.EqualFold(diag.Severity, "warning") {
			log.Printf("WARN config: section=%s path=%s %s", diag.Section, diag.Path, diag.Message)
		}
	}
	return bundle.Config, bundle.Secrets, nil
}

// LoadDiagnostic loads the workspace configuration and preserves partial
// results together with per-section diagnostics.
func LoadDiagnostic(opts LoadOptions) (PartialBundle, []ConfigDiagnostic, error) {
	return loadBundle(opts, false)
}

func loadBundle(opts LoadOptions, strict bool) (PartialBundle, []ConfigDiagnostic, error) {
	overrides, err := LoadEnvOverrides(opts.EnvOverrides)
	if err != nil {
		return PartialBundle{}, nil, fmt.Errorf("load env overrides: %w", err)
	}
	secrets := LoadSecrets(opts.EnvOverrides)
	if strings.TrimSpace(secrets.LLMAPIKey) == "" {
		log.Printf("WARN config: RELURPIFY_LLM_API_KEY is not set; provider auth may fail")
	}

	wsRoot := opts.WorkspaceRoot
	if wsRoot == "" {
		wsRoot = overrides.WorkspaceRoot
	}
	if wsRoot == "" {
		return PartialBundle{}, nil, fmt.Errorf("workspace root required")
	}
	absWorkspace, err := filepath.Abs(wsRoot)
	if err != nil {
		return PartialBundle{}, nil, fmt.Errorf("resolve workspace: %w", err)
	}

	workspaceCfg, workspaceDiags := loadWorkspaceConfigSection(absWorkspace, strict)
	var diags []ConfigDiagnostic
	diags = append(diags, workspaceDiags...)
	if workspaceCfg == nil {
		workspaceCfg = defaultWorkspaceConfig(absWorkspace)
	}
	if overrides.ModelProvider != "" {
		workspaceCfg.Model.Provider = overrides.ModelProvider
	}
	if overrides.ModelName != "" {
		workspaceCfg.Model.Name = overrides.ModelName
	}
	if overrides.SandboxBackend != "" {
		workspaceCfg.Sandbox.Backend = &overrides.SandboxBackend
	}
	if overrides.LogLevel != "" {
		workspaceCfg.Logging.Level = &overrides.LogLevel
	}

	if opts.CLIFlags != nil {
		if val, err := opts.CLIFlags.GetString("model-provider"); err == nil && val != "" {
			workspaceCfg.Model.Provider = val
		}
		if val, err := opts.CLIFlags.GetString("model-name"); err == nil && val != "" {
			workspaceCfg.Model.Name = val
		}
		if val, err := opts.CLIFlags.GetString("sandbox-backend"); err == nil && val != "" {
			workspaceCfg.Sandbox.Backend = &val
		}
		if val, err := opts.CLIFlags.GetString("log-level"); err == nil && val != "" {
			workspaceCfg.Logging.Level = &val
		}
	}

	securityBundle, securityDiags, err := loadSecurityBundle(absWorkspace)
	if err != nil {
		return PartialBundle{}, nil, err
	}
	diags = append(diags, securityDiags...)

	providers, providerDiags, err := model.LoadProviderDirDetailed(filepath.Join(absWorkspace, "relurpify_cfg", "model", "provider"), StrictDecode)
	if err != nil {
		return PartialBundle{}, nil, err
	}
	diags = append(diags, toConfigDiagnostics("provider", providerDiags)...)

	profiles, profileDiags, err := model.LoadProfileDirDetailed(filepath.Join(absWorkspace, "relurpify_cfg", "model", "profiles"), StrictDecode)
	if err != nil {
		return PartialBundle{}, nil, err
	}
	diags = append(diags, toConfigDiagnostics("profile", profileDiags)...)

	if err := workspaceCfg.ValidateModelRef(providers); err != nil {
		diag := ConfigDiagnostic{Path: DefaultWorkspaceConfigPath(absWorkspace), Section: "workspace", Severity: "blocking", Message: err.Error()}
		diags = append(diags, diag)
		if strict {
			return PartialBundle{}, diags, fmt.Errorf("load workspace config: %w", err)
		}
	}
	resolvedWorkspaceModel, err := model.ResolveModelRef(workspaceCfg.Model, model.ModelRef{}, providers)
	if err != nil {
		diag := ConfigDiagnostic{Path: DefaultWorkspaceConfigPath(absWorkspace), Section: "workspace", Severity: "blocking", Message: err.Error()}
		diags = append(diags, diag)
		if strict {
			return PartialBundle{}, diags, fmt.Errorf("workspace model: %w", err)
		}
	}
	if resolvedWorkspaceModel != nil && resolvedWorkspaceModel.Provider != nil && model.ProviderRequiresAuth(resolvedWorkspaceModel.Provider.Kind) && strings.TrimSpace(secrets.LLMAPIKey) == "" {
		diag := ConfigDiagnostic{Path: DefaultWorkspaceConfigPath(absWorkspace), Section: "workspace", Severity: "blocking", Message: fmt.Sprintf("provider %q requires RELURPIFY_LLM_API_KEY", resolvedWorkspaceModel.Provider.Name)}
		diags = append(diags, diag)
		if strict {
			return PartialBundle{}, diags, errors.New(diag.Message)
		}
	}

	toolDir := DefaultToolManifestDir(absWorkspace)
	toolManifests, err := LoadToolManifests(toolDir)
	if err != nil {
		diag := ConfigDiagnostic{Path: toolDir, Section: "tool", Severity: "blocking", Message: err.Error()}
		diags = append(diags, diag)
		if strict {
			return PartialBundle{}, diags, fmt.Errorf("load tools: %w", err)
		}
	}
	var toolRegistry *ToolRegistry
	if err == nil {
		toolRegistry, err = BuildRegistry(toolManifests, securityBundle.LocalTool, nil, opts.SubprocessToolFactory)
		if err != nil {
			diag := ConfigDiagnostic{Path: toolDir, Section: "tool", Severity: "blocking", Message: err.Error()}
			diags = append(diags, diag)
			if strict {
				return PartialBundle{}, diags, fmt.Errorf("build tool registry: %w", err)
			}
		}
	}

	editor := overrides.Editor
	if strings.TrimSpace(editor) == "" {
		editor = "vi"
	}
	sharedRoot := ResolveSharedRoot(overrides.XDGDataHome)

	appConfig := &AppConfig{
		Workspace:  *workspaceCfg,
		Security:   *securityBundle,
		Model:      ModelConfig{Providers: providers, Profiles: profiles},
		Tools:      toolRegistry,
		Editor:     editor,
		SharedRoot: sharedRoot,
	}
	if toolRegistry != nil {
		fingerprint, err := fingerprintConfig(appConfig)
		if err != nil {
			return PartialBundle{}, diags, fmt.Errorf("fingerprint config: %w", err)
		}
		appConfig.Fingerprint = fingerprint
		log.Printf("INFO config loaded workspace_root=%s model_provider=%s model_name=%s sandbox_backend=%s tools_loaded=%d strict_mode=%t config_fingerprint=%s", absWorkspace, appConfig.Workspace.Model.Provider, appConfig.Workspace.Model.Name, stringValue(appConfig.Workspace.Sandbox.Backend), len(toolRegistry.ListTools()), strict, appConfig.Fingerprint)
	}

	if strict && hasBlockingConfigDiagnostics(diags) {
		return PartialBundle{}, diags, diagnosticsError("config", diags)
	}

	return PartialBundle{Config: appConfig, Secrets: &secrets}, diags, nil
}

func loadWorkspaceConfigSection(absWorkspace string, strictMode bool) (*WorkspaceConfig, []ConfigDiagnostic) {
	path := DefaultWorkspaceConfigPath(absWorkspace)
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			cfg := defaultWorkspaceConfig(absWorkspace)
			return cfg, nil
		}
		diag := ConfigDiagnostic{Path: path, Section: "workspace", Severity: "blocking", Message: fmt.Sprintf("stat workspace config: %v", err)}
		return defaultWorkspaceConfig(absWorkspace), []ConfigDiagnostic{diag}
	}
	cfg, err := LoadWorkspaceConfig(path, absWorkspace, WorkspaceLoadOptions{Strict: strictMode})
	if err != nil {
		diag := ConfigDiagnostic{Path: path, Section: "workspace", Severity: "blocking", Message: err.Error()}
		return defaultWorkspaceConfig(absWorkspace), []ConfigDiagnostic{diag}
	}
	for _, usage := range cfg.DefaultsUsed {
		log.Printf("WARN config: using default value  file=%s  key=%s  default=%v", path, usage.Key, usage.Value)
	}
	return cfg, nil
}

func loadSecurityBundle(absWorkspace string) (*security.Bundle, []ConfigDiagnostic, error) {
	bundle := &security.Bundle{}
	var diags []ConfigDiagnostic
	if sandboxPolicy, err := security.LoadSandboxPolicy("", absWorkspace, StrictDecode); err != nil {
		diags = append(diags, ConfigDiagnostic{Path: security.SandboxPolicyPath(absWorkspace), Section: "security", Severity: "blocking", Message: err.Error()})
	} else {
		bundle.Sandbox = sandboxPolicy
	}
	if shellPolicy, err := security.LoadShellPolicy("", absWorkspace, StrictDecode); err != nil {
		diags = append(diags, ConfigDiagnostic{Path: security.ShellPolicyPath(absWorkspace), Section: "security", Severity: "blocking", Message: err.Error()})
	} else {
		bundle.Shell = shellPolicy
	}
	if localToolPolicy, err := security.LoadLocalToolPolicy("", absWorkspace, StrictDecode); err != nil {
		diags = append(diags, ConfigDiagnostic{Path: security.LocalToolPolicyPath(absWorkspace), Section: "security", Severity: "blocking", Message: err.Error()})
	} else {
		bundle.LocalTool = localToolPolicy
	}
	if ingestionRules, err := security.LoadWorkspaceIngestionPolicy("", absWorkspace, StrictDecode); err != nil {
		diags = append(diags, ConfigDiagnostic{Path: security.WorkspaceIngestionPolicyPath(absWorkspace), Section: "security", Severity: "blocking", Message: err.Error()})
	} else {
		bundle.Ingestion = ingestionRules
	}
	return bundle, diags, nil
}

func defaultWorkspaceConfig(absWorkspace string) *WorkspaceConfig {
	cfg := &WorkspaceConfig{
		Workspace:    absWorkspace,
		WorkspaceAbs: absWorkspace,
	}
	_ = cfg.applyDefaults(false)
	cfg.StateDirAbs = filepath.Join(absWorkspace, cfg.stateDirValue())
	return cfg
}

func toConfigDiagnostics(section string, diags []model.LoadDiagnostic) []ConfigDiagnostic {
	if len(diags) == 0 {
		return nil
	}
	out := make([]ConfigDiagnostic, 0, len(diags))
	for _, diag := range diags {
		out = append(out, ConfigDiagnostic{
			Path:     diag.Path,
			Section:  section,
			Severity: diag.Severity,
			Message:  diag.Message,
		})
	}
	return out
}

func hasBlockingConfigDiagnostics(diags []ConfigDiagnostic) bool {
	for _, diag := range diags {
		if strings.EqualFold(strings.TrimSpace(diag.Severity), "blocking") {
			return true
		}
	}
	return false
}

func diagnosticsError(section string, diags []ConfigDiagnostic) error {
	var errs []error
	for _, diag := range diags {
		errs = append(errs, fmt.Errorf("%s [%s]: %s", diag.Path, diag.Section, diag.Message))
	}
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("load %s: %w", section, errors.Join(errs...))
}

// fingerprintConfig computes a deterministic fingerprint for the resolved
// configuration, excluding secrets.
func fingerprintConfig(cfg *AppConfig) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("app config required")
	}
	payload := struct {
		Workspace  WorkspaceConfig               `json:"workspace"`
		Security   security.Bundle               `json:"security"`
		Model      ModelConfig                   `json:"model"`
		Tools      []configmanifest.ToolManifest `json:"tools"`
		Editor     string                        `json:"editor"`
		SharedRoot string                        `json:"shared_root"`
	}{
		Workspace:  cfg.Workspace,
		Security:   cfg.Security,
		Model:      cfg.Model,
		Tools:      nil,
		Editor:     cfg.Editor,
		SharedRoot: cfg.SharedRoot,
	}
	if cfg.Tools != nil {
		payload.Tools = cfg.Tools.ListTools()
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}
