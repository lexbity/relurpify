package agents

import frameworkconfig "codeburg.org/lexbit/relurpify/framework/manifest"

// Deprecated: use framework/manifest.New(workspace).ConfigRoot().
// ConfigDir returns the workspace-local configuration directory.
func ConfigDir(workspace string) string {
	return frameworkmanifest.New(workspace).ConfigRoot()
}

// Deprecated: use framework/manifest.GlobalConfig.
// GlobalConfig matches relurpify_cfg/manifest.yaml inside the workspace.
type GlobalConfig = frameworkmanifest.GlobalConfig

// Deprecated: use framework/manifest.ModelRef.
// ModelRef enumerates available models.
type ModelRef = frameworkmanifest.ModelRef

// Deprecated: use framework/manifest.FeatureFlags.
// FeatureFlags toggles runtime capabilities.
type FeatureFlags = frameworkmanifest.FeatureFlags

// Deprecated: use framework/manifest.ContextConfig.
// ContextConfig controls shared context.
type ContextConfig = frameworkmanifest.ContextConfig

// Deprecated: use framework/manifest.LoggingConfig.
// LoggingConfig describes log output.
type LoggingConfig = frameworkmanifest.LoggingConfig

// Deprecated: use framework/manifest.DefaultConfigPath.
// DefaultConfigPath returns relurpify_cfg/manifest.yaml within the workspace.
func DefaultConfigPath(workspace string) string {
	return frameworkmanifest.DefaultConfigPath(workspace)
}

// Deprecated: use framework/manifest.DefaultAgentPaths.
// DefaultAgentPaths returns the canonical search paths rooted in relurpify_cfg.
func DefaultAgentPaths(workspace string) []string {
	return frameworkmanifest.DefaultAgentPaths(workspace)
}

// Deprecated: use framework/manifest.LoadGlobalConfig.
// LoadGlobalConfig loads the config or returns defaults when missing.
func LoadGlobalConfig(path, workspace string) (*GlobalConfig, error) {
	return frameworkmanifest.LoadGlobalConfig(path, workspace)
}

// Deprecated: use framework/manifest.SaveGlobalConfig.
// SaveGlobalConfig writes the config to disk.
func SaveGlobalConfig(path string, cfg *GlobalConfig) error {
	return frameworkmanifest.SaveGlobalConfig(path, cfg)
}

func expandPath(path, workspace string) string {
	return frameworkmanifest.ExpandPath(path, workspace)
}
