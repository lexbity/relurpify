package config

// PermissionSet aggregates the permissions declared by an agent manifest.
type PermissionSet struct {
	FileSystem   []FileSystemPermission `yaml:"filesystem,omitempty" json:"filesystem,omitempty"`
	Executables  []ExecutablePermission `yaml:"executables,omitempty" json:"executables,omitempty"`
	Network      []NetworkPermission    `yaml:"network,omitempty" json:"network,omitempty"`
	Capabilities []CapabilityPermission `yaml:"capabilities,omitempty" json:"capabilities,omitempty"`
	IPC          []IPCPermission        `yaml:"ipc,omitempty" json:"ipc,omitempty"`
	HITLRequired []string               `yaml:"hitl_required,omitempty" json:"hitl_required,omitempty"`
}

// FileSystemPermission scopes access to a portion of the workspace.
type FileSystemPermission struct {
	Action        string `yaml:"action" json:"action"`
	Path          string `yaml:"path" json:"path"`
	Justification string `yaml:"justification,omitempty" json:"justification,omitempty"`
	HITLRequired  bool   `yaml:"hitl_required,omitempty" json:"hitl_required,omitempty"`
	ReadOnlyMount bool   `yaml:"read_only_mount,omitempty" json:"read_only_mount,omitempty"`
}

// ExecutablePermission restricts binary execution.
type ExecutablePermission struct {
	Binary        string   `yaml:"binary" json:"binary"`
	Args          []string `yaml:"args,omitempty" json:"args,omitempty"`
	Env           []string `yaml:"env,omitempty" json:"env,omitempty"`
	Checksum      string   `yaml:"checksum,omitempty" json:"checksum,omitempty"`
	HITLRequired  bool     `yaml:"hitl_required,omitempty" json:"hitl_required,omitempty"`
	ProxyRequired bool     `yaml:"proxy_required,omitempty" json:"proxy_required,omitempty"`
}

// NetworkPermission describes network access.
type NetworkPermission struct {
	Direction    string `yaml:"direction" json:"direction"`
	Protocol     string `yaml:"protocol" json:"protocol"`
	Host         string `yaml:"host,omitempty" json:"host,omitempty"`
	Port         int    `yaml:"port,omitempty" json:"port,omitempty"`
	Description  string `yaml:"description,omitempty" json:"description,omitempty"`
	HITLRequired bool   `yaml:"hitl_required,omitempty" json:"hitl_required,omitempty"`
}

// CapabilityPermission enumerates Linux capability requirements.
type CapabilityPermission struct {
	Capability    string `yaml:"capability" json:"capability"`
	Justification string `yaml:"justification,omitempty" json:"justification,omitempty"`
}

// IPCPermission restricts inter-process communication.
type IPCPermission struct {
	Kind         string `yaml:"kind" json:"kind"`
	Target       string `yaml:"target" json:"target"`
	Description  string `yaml:"description,omitempty" json:"description,omitempty"`
	HITLRequired bool   `yaml:"hitl_required,omitempty" json:"hitl_required,omitempty"`
}
