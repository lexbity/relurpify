package agents

import frameworkconfig "github.com/lexcodex/relurpify/framework/config"

// Deprecated: use framework/config.New(workspace).ConfigRoot().
// ConfigDir returns the workspace-local configuration directory.
func ConfigDir(workspace string) string {
	return frameworkconfig.New(workspace).ConfigRoot()
}

// Deprecated: use framework/config.GlobalConfig.
// GlobalConfig matches relurpify_cfg/config.yaml inside the workspace.
type GlobalConfig = frameworkconfig.GlobalConfig

// Deprecated: use framework/config.ModelRef.
// ModelRef enumerates available models.
type ModelRef = frameworkconfig.ModelRef

// Deprecated: use framework/config.FeatureFlags.
// FeatureFlags toggles runtime capabilities.
type FeatureFlags = frameworkconfig.FeatureFlags

// Deprecated: use framework/config.ContextConfig.
// ContextConfig controls shared context.
type ContextConfig = frameworkconfig.ContextConfig

// Deprecated: use framework/config.LoggingConfig.
// LoggingConfig describes log output.
type LoggingConfig = frameworkconfig.LoggingConfig

// Deprecated: use framework/config.DefaultConfigPath.
// DefaultConfigPath returns relurpify_cfg/config.yaml within the workspace.
func DefaultConfigPath(workspace string) string {
	return frameworkconfig.DefaultConfigPath(workspace)
}

// Deprecated: use framework/config.DefaultAgentPaths.
// DefaultAgentPaths returns the canonical search paths rooted in relurpify_cfg.
func DefaultAgentPaths(workspace string) []string {
	return frameworkconfig.DefaultAgentPaths(workspace)
}

// Deprecated: use framework/config.LoadGlobalConfig.
// LoadGlobalConfig loads the config or returns defaults when missing.
func LoadGlobalConfig(path, workspace string) (*GlobalConfig, error) {
	return frameworkconfig.LoadGlobalConfig(path, workspace)
}

// Deprecated: use framework/config.SaveGlobalConfig.
// SaveGlobalConfig writes the config to disk.
func SaveGlobalConfig(path string, cfg *GlobalConfig) error {
	return frameworkconfig.SaveGlobalConfig(path, cfg)
}

func expandPath(path, workspace string) string {
	return frameworkconfig.ExpandPath(path, workspace)
}
