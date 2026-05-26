package cfgload

import (
	"sort"

	"codeburg.org/lexbit/relurpify/framework/cfgload/model"
)

// AgentRegistry tracks all resolved agent configurations.
type AgentRegistry struct {
	Agents map[string]*AgentConfig
}

// Names returns the agent names in deterministic order.
func (r *AgentRegistry) Names() []string {
	if r == nil || len(r.Agents) == 0 {
		return nil
	}
	names := make([]string, 0, len(r.Agents))
	for name := range r.Agents {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// AgentConfig models relurpify_cfg/agents/*.yaml after merge and resolution.
type AgentConfig struct {
	Name         string                  `yaml:"name"`
	Kind         string                  `yaml:"kind"`
	Sandbox      AgentSandboxConfig      `yaml:"sandbox"`
	Model        model.ModelRef          `yaml:"model"`
	Filesystem   []AgentFilesystemPerm   `yaml:"filesystem"`
	Capabilities AgentCapabilitiesConfig `yaml:"capabilities"`
	Execution    AgentExecutionConfig    `yaml:"execution"`
	Audit        AgentAuditConfig        `yaml:"audit"`
	Network      AgentNetworkConfig      `yaml:"network"`

	SourcePath    string                  `yaml:"-"`
	ResolvedModel *model.ResolvedModelRef `yaml:"-"`
}

type AgentSandboxConfig struct {
	Runtime  *string            `yaml:"runtime"`
	Image    *string            `yaml:"image"`
	Limits   AgentSandboxLimits `yaml:"limits"`
	Security AgentSandboxSec    `yaml:"security"`
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
