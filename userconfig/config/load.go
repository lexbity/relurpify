package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"codeburg.org/lexbit/relurpify/capability/ports"
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
	Tools       ports.ToolRegistry
	Editor      string
	SharedRoot  string
	Fingerprint string
}

// ModelConfig groups LLM providers and profiles configuration.
type ModelConfig struct {
	Providers []*model.ResolvedProvider
	Profiles  []*model.ModelProfileConfig
}

// LoadOptions controls the configuration loader inputs.
type LoadOptions struct {
	WorkspaceRoot         string
	EnvOverrides          []string
	CLIFlags              FlagSet
	SubprocessToolFactory func(ports.ToolManifest) ports.Tool
}

// Load executes the consolidated configuration loading boundary.
func Load(opts LoadOptions) (*AppConfig, *Secrets, error) {
	overrides, err := LoadEnvOverrides(opts.EnvOverrides)
	if err != nil {
		return nil, nil, fmt.Errorf("load env overrides: %w", err)
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
		return nil, nil, fmt.Errorf("workspace root required")
	}
	absWorkspace, err := filepath.Abs(wsRoot)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve workspace: %w", err)
	}

	workspaceConfigPath := DefaultWorkspaceConfigPath(absWorkspace)
	strictMode := overrides.Strict
	workspaceCfg, err := LoadWorkspaceConfig(workspaceConfigPath, absWorkspace, WorkspaceLoadOptions{Strict: strictMode})
	if err != nil {
		return nil, nil, fmt.Errorf("load workspace config: %w", err)
	}
	InstallSecretFieldChecks()

	for _, usage := range workspaceCfg.DefaultsUsed {
		log.Printf("WARN config: using default value  file=%s  key=%s  default=%v", workspaceConfigPath, usage.Key, usage.Value)
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

	securityBundle, err := security.LoadBundle(absWorkspace, StrictDecode)
	if err != nil {
		return nil, nil, fmt.Errorf("load security policies: %w", err)
	}

	providerDir := filepath.Join(absWorkspace, "relurpify_cfg", "model", "provider")
	providers, err := model.LoadProviderDir(providerDir, StrictDecode)
	if err != nil {
		return nil, nil, fmt.Errorf("load LLM providers: %w", err)
	}

	if err := workspaceCfg.ValidateModelRef(providers); err != nil {
		return nil, nil, err
	}
	resolvedWorkspaceModel, err := model.ResolveModelRef(workspaceCfg.Model, model.ModelRef{}, providers)
	if err != nil {
		return nil, nil, fmt.Errorf("workspace model: %w", err)
	}
	if resolvedWorkspaceModel != nil && resolvedWorkspaceModel.Provider != nil && model.ProviderRequiresAuth(resolvedWorkspaceModel.Provider.Kind) && strings.TrimSpace(secrets.LLMAPIKey) == "" {
		return nil, nil, fmt.Errorf("provider %q requires RELURPIFY_LLM_API_KEY", resolvedWorkspaceModel.Provider.Name)
	}

	profileDir := filepath.Join(absWorkspace, "relurpify_cfg", "model", "profiles")
	profiles, err := model.LoadProfileDir(profileDir, StrictDecode)
	if err != nil {
		return nil, nil, fmt.Errorf("load model profiles: %w", err)
	}

	modelConfig := ModelConfig{
		Providers: providers,
		Profiles:  profiles,
	}

	toolDir := DefaultToolManifestDir(absWorkspace)
	toolManifests, err := LoadToolManifests(toolDir)
	if err != nil {
		return nil, nil, fmt.Errorf("load tools: %w", err)
	}
	toolRegistry, err := BuildRegistry(toolManifests, securityBundle.LocalTool, nil, opts.SubprocessToolFactory)
	if err != nil {
		return nil, nil, fmt.Errorf("build tool registry: %w", err)
	}

	editor := overrides.Editor
	if strings.TrimSpace(editor) == "" {
		editor = "vi"
	}
	sharedRoot := ResolveSharedRoot(overrides.XDGDataHome)

	appConfig := &AppConfig{
		Workspace:  *workspaceCfg,
		Security:   *securityBundle,
		Model:      modelConfig,
		Tools:      toolRegistry,
		Editor:     editor,
		SharedRoot: sharedRoot,
	}
	fingerprint, err := ConfigFingerprint(appConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("fingerprint config: %w", err)
	}
	appConfig.Fingerprint = fingerprint
	log.Printf("INFO config loaded workspace_root=%s model_provider=%s model_name=%s sandbox_backend=%s tools_loaded=%d strict_mode=%t config_fingerprint=%s", absWorkspace, appConfig.Workspace.Model.Provider, appConfig.Workspace.Model.Name, stringValue(appConfig.Workspace.Sandbox.Backend), len(toolRegistry.ListTools()), strictMode, appConfig.Fingerprint)

	return appConfig, &secrets, nil
}

// ConfigFingerprint computes a deterministic fingerprint for the resolved
// configuration, excluding secrets.
func ConfigFingerprint(cfg *AppConfig) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("app config required")
	}
	payload := struct {
		Workspace  WorkspaceConfig      `json:"workspace"`
		Security   security.Bundle      `json:"security"`
		Model      ModelConfig          `json:"model"`
		Tools      []ports.ToolManifest `json:"tools"`
		Editor     string               `json:"editor"`
		SharedRoot string               `json:"shared_root"`
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
