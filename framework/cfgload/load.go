package cfgload

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/framework/cfgload/model"
	"codeburg.org/lexbit/relurpify/framework/cfgload/security"
	"codeburg.org/lexbit/relurpify/framework/sandbox"
	"codeburg.org/lexbit/relurpify/platform/contracts"
	"gopkg.in/yaml.v3"
)

func init() {
	agentspec.RejectForbiddenSecretFields = RejectForbiddenSecretFields
	sandbox.RejectForbiddenSecretFields = RejectForbiddenSecretFields
	model.DecodeWithSchema = func(path string, data []byte, out any) (any, error) {
		return DecodeWithSchema(path, data, NewSchemaRegistry(), out)
	}
	security.DecodeWithSchema = func(path string, data []byte, out any) (any, error) {
		return DecodeWithSchema(path, data, NewSchemaRegistry(), out)
	}
}

// FlagSet represents a subset of command-line flags.
type FlagSet interface {
	GetString(name string) (string, error)
	GetBool(name string) (bool, error)
	GetInt(name string) (int, error)
}

// AppConfig consolidated configuration representation.
type AppConfig struct {
	Workspace WorkspaceConfig
	Security  security.Bundle
	Model     ModelConfig
	Tools     contracts.ToolRegistry
	Agents    AgentRegistry
}

// ModelConfig groups LLM providers and profiles configuration.
type ModelConfig struct {
	Providers []*model.Provider
	Profiles  []*model.Profile
}

// AgentRegistry tracks all registered named agent configurations.
type AgentRegistry struct {
	Agents map[string]*AgentConfig
}

// LoadOptions controls the configuration loader inputs.
type LoadOptions struct {
	WorkspaceRoot string
	EnvOverrides  []string
	CLIFlags      FlagSet
}

type AgentConfig struct {
	Name         string                  `yaml:"name"`
	Kind         string                  `yaml:"kind"`
	Sandbox      AgentSandboxConfig      `yaml:"sandbox"`
	Model        AgentModelConfig        `yaml:"model"`
	Filesystem   []AgentFilesystemPerm   `yaml:"filesystem"`
	Capabilities AgentCapabilitiesConfig `yaml:"capabilities"`
	Execution    AgentExecutionConfig    `yaml:"execution"`
	Audit        AgentAuditConfig        `yaml:"audit"`
	Network      AgentNetworkConfig      `yaml:"network"`
	SourcePath   string                  `yaml:"-"`
}

type AgentSandboxConfig struct {
	Runtime  *string             `yaml:"runtime"`
	Image    *string             `yaml:"image"`
	Limits   AgentSandboxLimits  `yaml:"limits"`
	Security AgentSandboxSec     `yaml:"security"`
}

type AgentSandboxLimits struct {
	CPU          *string `yaml:"cpu"`
	Memory       *string `yaml:"memory"`
	DiskIO       *string `yaml:"disk_io"`
	MaxProcesses *int    `yaml:"max_processes"`
	MaxOpenFiles *int    `yaml:"max_open_files"`
}

type AgentSandboxSec struct {
	RunAsUser        *int     `yaml:"run_as_user"`
	RunAsGroup       *int     `yaml:"run_as_group"`
	NoNewPrivileges  *bool    `yaml:"no_new_privileges"`
	ReadOnlyRoot     *bool    `yaml:"read_only_root"`
	DropCapabilities []string `yaml:"drop_capabilities"`
}

type AgentModelConfig struct {
	Name        *string  `yaml:"name"`
	Temperature *float64 `yaml:"temperature"`
	MaxTokens   *int     `yaml:"max_tokens"`
}

type AgentFilesystemPerm struct {
	Action  []string `yaml:"action"`
	Path    string   `yaml:"path"`
	Exclude []string `yaml:"exclude"`
}

type AgentCapabilitiesConfig struct {
	Tools    []string `yaml:"tools"`
	Relurpic []string `yaml:"relurpic"`
	Prompts  []string `yaml:"prompts"`
}

type AgentExecutionConfig struct {
	MaxIterations      *int    `yaml:"max_iterations"`
	CheckpointPolicy   *string `yaml:"checkpoint_policy"`
	HITLTimeoutSeconds *int    `yaml:"hitl_timeout_seconds"`
}

type AgentAuditConfig struct {
	Level         *string `yaml:"level"`
	RetentionDays *int    `yaml:"retention_days"`
}

type AgentNetworkConfig struct {
	Allow []AgentNetworkRule `yaml:"allow"`
}

type AgentNetworkRule struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Protocol string `yaml:"protocol"`
}

var varRegex = regexp.MustCompile(`\$\{([A-Za-z0-9_]+)(?::-([^}]+))?\}`)

func resolveVariables(val string, workspace string, env []string, defaultModel string) (string, error) {
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

		// If name is RELURPIFY_MODEL and not in env, fallback to defaultModel
		if name == "RELURPIFY_MODEL" {
			if defaultModel != "" {
				return defaultModel
			}
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

func resolveNodeVariables(node *yaml.Node, workspace string, env []string, defaultModel string) error {
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

func mergeAgentConfig(base, named AgentConfig) AgentConfig {
	merged := base

	if named.Name != "" {
		merged.Name = named.Name
	}
	if named.Kind != "" {
		merged.Kind = named.Kind
	}
	if named.SourcePath != "" {
		merged.SourcePath = named.SourcePath
	}

	if named.Sandbox.Runtime != nil {
		merged.Sandbox.Runtime = named.Sandbox.Runtime
	}
	if named.Sandbox.Image != nil {
		merged.Sandbox.Image = named.Sandbox.Image
	}

	if named.Sandbox.Limits.CPU != nil {
		merged.Sandbox.Limits.CPU = named.Sandbox.Limits.CPU
	}
	if named.Sandbox.Limits.Memory != nil {
		merged.Sandbox.Limits.Memory = named.Sandbox.Limits.Memory
	}
	if named.Sandbox.Limits.DiskIO != nil {
		merged.Sandbox.Limits.DiskIO = named.Sandbox.Limits.DiskIO
	}
	if named.Sandbox.Limits.MaxProcesses != nil {
		merged.Sandbox.Limits.MaxProcesses = named.Sandbox.Limits.MaxProcesses
	}
	if named.Sandbox.Limits.MaxOpenFiles != nil {
		merged.Sandbox.Limits.MaxOpenFiles = named.Sandbox.Limits.MaxOpenFiles
	}

	if named.Sandbox.Security.RunAsUser != nil {
		merged.Sandbox.Security.RunAsUser = named.Sandbox.Security.RunAsUser
	}
	if named.Sandbox.Security.RunAsGroup != nil {
		merged.Sandbox.Security.RunAsGroup = named.Sandbox.Security.RunAsGroup
	}
	if named.Sandbox.Security.NoNewPrivileges != nil {
		merged.Sandbox.Security.NoNewPrivileges = named.Sandbox.Security.NoNewPrivileges
	}
	if named.Sandbox.Security.ReadOnlyRoot != nil {
		merged.Sandbox.Security.ReadOnlyRoot = named.Sandbox.Security.ReadOnlyRoot
	}
	if len(named.Sandbox.Security.DropCapabilities) > 0 {
		merged.Sandbox.Security.DropCapabilities = append([]string(nil), named.Sandbox.Security.DropCapabilities...)
	}

	if named.Model.Name != nil {
		merged.Model.Name = named.Model.Name
	}
	if named.Model.Temperature != nil {
		merged.Model.Temperature = named.Model.Temperature
	}
	if named.Model.MaxTokens != nil {
		merged.Model.MaxTokens = named.Model.MaxTokens
	}

	if len(named.Filesystem) > 0 {
		merged.Filesystem = append([]AgentFilesystemPerm(nil), named.Filesystem...)
	}

	if len(named.Capabilities.Tools) > 0 {
		merged.Capabilities.Tools = append([]string(nil), named.Capabilities.Tools...)
	}
	if len(named.Capabilities.Relurpic) > 0 {
		merged.Capabilities.Relurpic = append([]string(nil), named.Capabilities.Relurpic...)
	}
	if len(named.Capabilities.Prompts) > 0 {
		merged.Capabilities.Prompts = append([]string(nil), named.Capabilities.Prompts...)
	}

	if named.Execution.MaxIterations != nil {
		merged.Execution.MaxIterations = named.Execution.MaxIterations
	}
	if named.Execution.CheckpointPolicy != nil {
		merged.Execution.CheckpointPolicy = named.Execution.CheckpointPolicy
	}
	if named.Execution.HITLTimeoutSeconds != nil {
		merged.Execution.HITLTimeoutSeconds = named.Execution.HITLTimeoutSeconds
	}

	if named.Audit.Level != nil {
		merged.Audit.Level = named.Audit.Level
	}
	if named.Audit.RetentionDays != nil {
		merged.Audit.RetentionDays = named.Audit.RetentionDays
	}

	if len(named.Network.Allow) > 0 {
		merged.Network.Allow = append([]AgentNetworkRule(nil), named.Network.Allow...)
	}

	return merged
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

func LoadBaseAgentConfig(path string, workspace string, env []string, defaultModel string) (*AgentConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	
	if err := resolveNodeVariables(&doc, workspace, env, defaultModel); err != nil {
		return nil, err
	}
	
	resolvedData, err := yaml.Marshal(&doc)
	if err != nil {
		return nil, err
	}
	
	var baseAgent AgentConfig
	decl, err := DecodeWithSchema(path, resolvedData, NewSchemaRegistry(), &baseAgent)
	if err != nil {
		return nil, err
	}
	if decl.Kind != "agent" {
		return nil, fmt.Errorf("base agent config %s must be relurpify/agent/v1", path)
	}
	baseAgent.SourcePath = path
	return &baseAgent, nil
}

func LoadAgentRegistry(dir string, baseConfig *AgentConfig, workspace string, env []string, defaultModel string) (*AgentRegistry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return &AgentRegistry{Agents: make(map[string]*AgentConfig)}, nil
		}
		return nil, err
	}

	registry := &AgentRegistry{Agents: make(map[string]*AgentConfig)}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == "_base.agent.yaml" || (!strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml")) {
			continue
		}
		path := filepath.Join(dir, name)
		
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		
		var doc yaml.Node
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return nil, err
		}
		
		if err := resolveNodeVariables(&doc, workspace, env, defaultModel); err != nil {
			return nil, err
		}
		
		resolvedData, err := yaml.Marshal(&doc)
		if err != nil {
			return nil, err
		}
		
		var namedAgent AgentConfig
		decl, err := DecodeWithSchema(path, resolvedData, NewSchemaRegistry(), &namedAgent)
		if err != nil {
			return nil, err
		}
		if decl.Kind != "agent" {
			return nil, fmt.Errorf("agent file %s must be relurpify/agent/v1", path)
		}
		namedAgent.SourcePath = path
		if namedAgent.Name == "" {
			namedAgent.Name = strings.TrimSuffix(strings.TrimSuffix(name, ".yaml"), ".yml")
		}

		var merged AgentConfig
		if baseConfig != nil {
			merged = mergeAgentConfig(*baseConfig, namedAgent)
		} else {
			merged = namedAgent
		}

		absWorkspace, err := filepath.Abs(workspace)
		if err == nil {
			applyFilesystemSecurityInvariant(&merged, absWorkspace)
		}

		registry.Agents[merged.Name] = &merged
	}

	return registry, nil
}

// Load executes the consolidated configuration loading boundary.
func Load(opts LoadOptions) (*AppConfig, *Secrets, error) {
	overrides := LoadEnvOverrides(opts.EnvOverrides)
	secrets := LoadSecrets(opts.EnvOverrides)

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

	for _, usage := range workspaceCfg.DefaultsUsed {
		log.Printf("WARN config: using default value  file=%s  key=%s  default=%v", workspaceConfigPath, usage.Key, usage.Value)
	}

	if overrides.Model != "" {
		workspaceCfg.Model.DefaultName = &overrides.Model
	}
	if overrides.SandboxBackend != "" {
		workspaceCfg.Sandbox.Backend = &overrides.SandboxBackend
	}
	if overrides.LogLevel != "" {
		workspaceCfg.Logging.Level = &overrides.LogLevel
	}

	if opts.CLIFlags != nil {
		if val, err := opts.CLIFlags.GetString("model"); err == nil && val != "" {
			workspaceCfg.Model.DefaultName = &val
		}
		if val, err := opts.CLIFlags.GetString("sandbox-backend"); err == nil && val != "" {
			workspaceCfg.Sandbox.Backend = &val
		}
		if val, err := opts.CLIFlags.GetString("log-level"); err == nil && val != "" {
			workspaceCfg.Logging.Level = &val
		}
	}

	securityBundle, err := security.LoadBundle(absWorkspace)
	if err != nil {
		return nil, nil, fmt.Errorf("load security policies: %w", err)
	}

	providerDir := filepath.Join(absWorkspace, "relurpify_cfg", "model", "provider")
	providers, err := model.LoadProviderDir(providerDir)
	if err != nil {
		return nil, nil, fmt.Errorf("load LLM providers: %w", err)
	}

	profileDir := filepath.Join(absWorkspace, "relurpify_cfg", "model", "profiles")
	profiles, err := model.LoadProfileDir(profileDir)
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
	toolRegistry := contracts.NewStaticToolRegistry(toolManifests)

	defaultModel := ""
	if workspaceCfg.Model.DefaultName != nil {
		defaultModel = *workspaceCfg.Model.DefaultName
	}

	agentsDir := filepath.Join(absWorkspace, "relurpify_cfg", "agents")
	basePath := filepath.Join(agentsDir, "_base.agent.yaml")
	baseAgent, err := LoadBaseAgentConfig(basePath, absWorkspace, opts.EnvOverrides, defaultModel)
	if err != nil {
		return nil, nil, fmt.Errorf("load base agent config: %w", err)
	}

	agentRegistry, err := LoadAgentRegistry(agentsDir, baseAgent, absWorkspace, opts.EnvOverrides, defaultModel)
	if err != nil {
		return nil, nil, fmt.Errorf("load agent registry: %w", err)
	}

	appConfig := &AppConfig{
		Workspace: *workspaceCfg,
		Security:  *securityBundle,
		Model:     modelConfig,
		Tools:     toolRegistry,
		Agents:    *agentRegistry,
	}

	return appConfig, &secrets, nil
}
