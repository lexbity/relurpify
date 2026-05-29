// Package llmconfig adapts framework configuration (YAML model profiles and
// provider definitions loaded by framework/cfgload) into the domain types owned
// by platform/llm. It lives in the framework layer so that platform/llm depends
// only on platform-level types and never imports framework/cfgload. Dependency
// direction: framework/llmconfig -> {framework/cfgload, platform/llm}.
package llmconfig

import (
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/cfgload"
	cfgmodel "codeburg.org/lexbit/relurpify/framework/cfgload/model"
	"codeburg.org/lexbit/relurpify/platform/llm"
)

// LoadProfileRegistry loads model profile configs from the canonical directory
// and builds a platform/llm profile registry. A missing directory returns an
// empty registry using built-in defaults.
func LoadProfileRegistry(configDir string) (*llm.ProfileRegistry, error) {
	if strings.TrimSpace(configDir) == "" {
		return llm.NewProfileRegistryFromProfiles(nil), nil
	}
	loaded, err := cfgmodel.LoadProfileDir(configDir, cfgload.StrictDecode)
	if err != nil {
		lower := strings.ToLower(err.Error())
		if strings.Contains(lower, "no such file") || strings.Contains(lower, "does not exist") {
			return llm.NewProfileRegistryFromProfiles(nil), nil
		}
		return nil, fmt.Errorf("read model profiles dir: %w", err)
	}
	return ProfileRegistryFromConfigs(loaded)
}

// ProfileRegistryFromConfigs builds a registry from already-loaded profile
// configs (e.g. cfgload.AppConfig.Model.Profiles).
func ProfileRegistryFromConfigs(configs []*cfgmodel.ModelProfileConfig) (*llm.ProfileRegistry, error) {
	profiles := make([]*llm.ModelProfile, 0, len(configs))
	for _, cfg := range configs {
		if cfg == nil {
			continue
		}
		profiles = append(profiles, convertModelProfileConfig(cfg))
	}
	return llm.NewProfileRegistryFromProfiles(profiles), nil
}

// convertModelProfileConfig maps the framework YAML DTO into the platform/llm
// domain profile. It reads only exported contract fields, so it can live above
// platform/llm without breaking the layer boundary.
func convertModelProfileConfig(cfg *cfgmodel.ModelProfileConfig) *llm.ModelProfile {
	if cfg == nil {
		return nil
	}
	profile := &llm.ModelProfile{
		Pattern:    cfg.Pattern,
		SourcePath: cfg.SourcePath,
	}
	switch strings.ToLower(strings.TrimSpace(cfg.ToolCalling.Intent)) {
	case "native":
		profile.ToolCalling.NativeAPI = true
	case "prompt_based":
		profile.ToolCalling.NativeAPI = false
	case "auto":
		profile.ToolCalling.NativeAPI = false
	}
	profile.ToolCalling.DoubleEncodedArgs = cfg.ToolCalling.DoubleEncodeArgs
	profile.ToolCalling.MaxToolsPerCall = cfg.ToolCalling.MaxConcurrentTools
	if cfg.Context.MaxTokens > 0 {
		profile.ContextSize = cfg.Context.MaxTokens
	}
	profile.Normalize()
	return profile
}
