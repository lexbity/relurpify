package cfgload

import "strings"

// MergeAgentConfig applies the named agent config on top of the base config.
// It returns a new struct and does not mutate either input.
func MergeAgentConfig(base, named *AgentConfig) *AgentConfig {
	if base == nil && named == nil {
		return &AgentConfig{}
	}
	merged := cloneAgentConfig(base)
	if named == nil {
		return &merged
	}

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

	if strings.TrimSpace(named.Model.Provider) != "" {
		merged.Model.Provider = named.Model.Provider
	}
	if strings.TrimSpace(named.Model.Name) != "" {
		merged.Model.Name = named.Model.Name
	}

	if len(named.Filesystem) > 0 {
		merged.Filesystem = cloneFilesystemPermissions(named.Filesystem)
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

	merged.ResolvedModel = nil
	return &merged
}

func cloneAgentConfig(src *AgentConfig) AgentConfig {
	if src == nil {
		return AgentConfig{}
	}
	clone := *src
	clone.Filesystem = cloneFilesystemPermissions(src.Filesystem)
	clone.Capabilities.Tools = append([]string(nil), src.Capabilities.Tools...)
	clone.Capabilities.Relurpic = append([]string(nil), src.Capabilities.Relurpic...)
	clone.Capabilities.Prompts = append([]string(nil), src.Capabilities.Prompts...)
	clone.Sandbox.Security.DropCapabilities = append([]string(nil), src.Sandbox.Security.DropCapabilities...)
	clone.Network.Allow = append([]AgentNetworkRule(nil), src.Network.Allow...)
	clone.ResolvedModel = nil
	return clone
}

func cloneFilesystemPermissions(src []AgentFilesystemPerm) []AgentFilesystemPerm {
	if len(src) == 0 {
		return nil
	}
	out := make([]AgentFilesystemPerm, len(src))
	for i, perm := range src {
		out[i] = AgentFilesystemPerm{
			Action:  append([]string(nil), perm.Action...),
			Path:    perm.Path,
			Exclude: append([]string(nil), perm.Exclude...),
		}
	}
	return out
}
