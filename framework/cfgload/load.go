package cfgload

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"regexp"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/cfgload/model"
	"codeburg.org/lexbit/relurpify/framework/cfgload/security"
	"codeburg.org/lexbit/relurpify/platform/contracts"
	"gopkg.in/yaml.v3"
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
	Tools       contracts.ToolRegistry
	Agents      AgentRegistry
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
	WorkspaceRoot string
	EnvOverrides  []string
	CLIFlags      FlagSet
}

var varRegex = regexp.MustCompile(`\$\{([A-Za-z0-9_]+)(?::-([^}]+))?\}`)

func resolveVariables(val string, workspace string, env []string, defaultModel model.ModelRef) (string, error) {
	var err error
	res := varRegex.ReplaceAllStringFunc(val, func(m string) string {
		if err != nil {
			return m
		}
		submatches := varRegex.FindStringSubmatch(m)
		if len(submatches) < 2 {
			return m
		}
		name := submatches[1]
		hasDefault := len(submatches) > 2 && submatches[2] != ""
		fallback := ""
		if hasDefault {
			fallback = submatches[2]
		}

		if name == "workspace" {
			return workspace
		}

		// Try to lookup in env overrides first
		envVal := lookupEnv(env, name)
		if envVal != "" {
			return envVal
		}

		if name == "RELURPIFY_MODEL_PROVIDER" && strings.TrimSpace(defaultModel.Provider) != "" {
			return defaultModel.Provider
		}
		if name == "RELURPIFY_MODEL_NAME" && strings.TrimSpace(defaultModel.Name) != "" {
			return defaultModel.Name
		}

		if envVal == "" && hasDefault {
			return fallback
		}

		if envVal == "" {
			err = fmt.Errorf("unresolved variable reference: %s", m)
			return m
		}

		return envVal
	})
	if err != nil {
		return "", err
	}
	return res, nil
}

func resolveNodeVariables(node *yaml.Node, workspace string, env []string, defaultModel model.ModelRef) error {
	if node == nil {
		return nil
	}
	switch node.Kind {
	case yaml.DocumentNode:
		for _, child := range node.Content {
			if err := resolveNodeVariables(child, workspace, env, defaultModel); err != nil {
				return err
			}
		}
	case yaml.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			valNode := node.Content[i+1]
			if err := resolveNodeVariables(valNode, workspace, env, defaultModel); err != nil {
				return err
			}
		}
	case yaml.SequenceNode:
		for _, child := range node.Content {
			if err := resolveNodeVariables(child, workspace, env, defaultModel); err != nil {
				return err
			}
		}
	case yaml.ScalarNode:
		resolved, err := resolveVariables(node.Value, workspace, env, defaultModel)
		if err != nil {
			return err
		}
		node.Value = resolved
	}
	return nil
}

func applyFilesystemSecurityInvariant(agent *AgentConfig, workspaceAbs string) {
	configExclude := filepath.Join(workspaceAbs, "relurpify_cfg") + "/**"
	configExcludeAlt := filepath.Join(workspaceAbs, "relurpify_cfg")
	for i, perm := range agent.Filesystem {
		hasExclude := false
		for _, ex := range perm.Exclude {
			if ex == configExclude || ex == configExcludeAlt {
				hasExclude = true
				break
			}
		}
		if !hasExclude {
			agent.Filesystem[i].Exclude = append(perm.Exclude, configExclude)
		}
	}
}

// Load executes the consolidated configuration loading boundary.
func Load(opts LoadOptions) (*AppConfig, *Secrets, error) {
	overrides := LoadEnvOverrides(opts.EnvOverrides)
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
	toolRegistry, err := BuildRegistry(toolManifests, securityBundle.LocalTool, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("build tool registry: %w", err)
	}

	editor := overrides.Editor
	if strings.TrimSpace(editor) == "" {
		editor = "vi"
	}
	sharedRoot := ResolveSharedRoot(overrides.XDGDataHome)

	agentsDir := filepath.Join(absWorkspace, "relurpify_cfg", "agents")
	agentRegistry, err := LoadAgentRegistry(agentsDir, absWorkspace, opts.EnvOverrides, workspaceCfg.Model, providers, toolRegistry, StrictDecode)
	if err != nil {
		return nil, nil, fmt.Errorf("load agent registry: %w", err)
	}

	appConfig := &AppConfig{
		Workspace:  *workspaceCfg,
		Security:   *securityBundle,
		Model:      modelConfig,
		Tools:      toolRegistry,
		Agents:     *agentRegistry,
		Editor:     editor,
		SharedRoot: sharedRoot,
	}
	fingerprint, err := ConfigFingerprint(appConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("fingerprint config: %w", err)
	}
	appConfig.Fingerprint = fingerprint
	log.Printf("INFO config loaded workspace_root=%s model_provider=%s model_name=%s sandbox_backend=%s agents=%v tools_loaded=%d strict_mode=%t config_fingerprint=%s", absWorkspace, appConfig.Workspace.Model.Provider, appConfig.Workspace.Model.Name, stringValue(appConfig.Workspace.Sandbox.Backend), agentRegistry.Names(), len(toolRegistry.ListTools()), strictMode, appConfig.Fingerprint)

	return appConfig, &secrets, nil
}

// ConfigFingerprint computes a deterministic fingerprint for the resolved
// configuration, excluding secrets.
func ConfigFingerprint(cfg *AppConfig) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("app config required")
	}
	payload := struct {
		Workspace  WorkspaceConfig          `json:"workspace"`
		Security   security.Bundle          `json:"security"`
		Model      ModelConfig              `json:"model"`
		Tools      []contracts.ToolManifest `json:"tools"`
		Agents     map[string]*AgentConfig  `json:"agents"`
		Editor     string                   `json:"editor"`
		SharedRoot string                   `json:"shared_root"`
	}{
		Workspace:  cfg.Workspace,
		Security:   cfg.Security,
		Model:      cfg.Model,
		Tools:      nil,
		Agents:     cfg.Agents.Agents,
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
