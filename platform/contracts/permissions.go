package contracts

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// PermissionType enumerates the supported permission families.
type PermissionType string

const (
	PermissionTypeFilesystem PermissionType = "filesystem"
	PermissionTypeExecutable PermissionType = "executable"
	PermissionTypeNetwork    PermissionType = "network"
	PermissionTypeCapability PermissionType = "capability"
	PermissionTypeIPC        PermissionType = "ipc"
	PermissionTypeHITL       PermissionType = "hitl"
	permissionMatchAll                      = "**"
)

// AgentPermissionLevel enumerates allow/deny/ask for agent-level permission decisions.
type AgentPermissionLevel string

const (
	AgentPermissionAllow AgentPermissionLevel = "allow"
	AgentPermissionDeny  AgentPermissionLevel = "deny"
	AgentPermissionAsk   AgentPermissionLevel = "ask"
)

// FileSystemAction enumerates filesystem operations.
type FileSystemAction string

const (
	FileSystemRead    FileSystemAction = "fs:read"
	FileSystemWrite   FileSystemAction = "fs:write"
	FileSystemDelete  FileSystemAction = "fs:delete"
	FileSystemRename  FileSystemAction = "fs:rename"
	FileSystemMove    FileSystemAction = "fs:move"
	FileSystemExecute FileSystemAction = "fs:execute"
	FileSystemList    FileSystemAction = "fs:list"
)

// FileSystemPermission scopes access to a portion of the workspace.
type FileSystemPermission struct {
	Action        FileSystemAction `json:"action" yaml:"action"`
	Path          string           `json:"path" yaml:"path"`
	Justification string           `json:"justification,omitempty" yaml:"justification,omitempty"`
	HITLRequired  bool             `json:"hitl_required,omitempty" yaml:"hitl_required,omitempty"`
	ReadOnlyMount bool             `json:"read_only_mount,omitempty" yaml:"read_only_mount,omitempty"`
}

// ExecutablePermission restricts binary execution.
type ExecutablePermission struct {
	Binary        string   `json:"binary" yaml:"binary"`
	Args          []string `json:"args,omitempty" yaml:"args,omitempty"`
	Env           []string `json:"env,omitempty" yaml:"env,omitempty"`
	Checksum      string   `json:"checksum,omitempty" yaml:"checksum,omitempty"`
	HITLRequired  bool     `json:"hitl_required,omitempty" yaml:"hitl_required,omitempty"`
	ProxyRequired bool     `json:"proxy_required,omitempty" yaml:"proxy_required,omitempty"`
}

// NetworkPermission describes network access.
type NetworkPermission struct {
	Direction    string `json:"direction" yaml:"direction"` // egress or ingress
	Protocol     string `json:"protocol" yaml:"protocol"`
	Host         string `json:"host,omitempty" yaml:"host,omitempty"`
	Port         int    `json:"port,omitempty" yaml:"port,omitempty"`
	Description  string `json:"description,omitempty" yaml:"description,omitempty"`
	HITLRequired bool   `json:"hitl_required,omitempty" yaml:"hitl_required,omitempty"`
}

// CapabilityPermission enumerates Linux capability requirements.
type CapabilityPermission struct {
	Capability    string `json:"capability" yaml:"capability"`
	Justification string `json:"justification,omitempty" yaml:"justification,omitempty"`
}

// IPCPermission restricts inter-process communication.
type IPCPermission struct {
	Kind         string `json:"kind" yaml:"kind"` // pipe/socket/signal
	Target       string `json:"target" yaml:"target"`
	Description  string `json:"description,omitempty" yaml:"description,omitempty"`
	HITLRequired bool   `json:"hitl_required,omitempty" yaml:"hitl_required,omitempty"`
}

// PermissionSet aggregates the permissions declared by an agent manifest.
type PermissionSet struct {
	FileSystem   []FileSystemPermission `json:"filesystem,omitempty" yaml:"filesystem,omitempty"`
	Executables  []ExecutablePermission `json:"executables,omitempty" yaml:"executables,omitempty"`
	Network      []NetworkPermission    `json:"network,omitempty" yaml:"network,omitempty"`
	Capabilities []CapabilityPermission `json:"capabilities,omitempty" yaml:"capabilities,omitempty"`
	IPC          []IPCPermission        `json:"ipc,omitempty" yaml:"ipc,omitempty"`
	HITLRequired []string               `json:"hitl_required,omitempty" yaml:"hitl_required,omitempty"`
}

// Validate ensures the permission declaration is consistent.
func (p *PermissionSet) Validate() error {
	if p == nil {
		return errors.New("permission set missing")
	}
	if len(p.FileSystem) == 0 &&
		len(p.Executables) == 0 &&
		len(p.Network) == 0 &&
		len(p.Capabilities) == 0 &&
		len(p.IPC) == 0 {
		return errors.New("permission set must declare at least one permission scope")
	}
	for _, perm := range p.FileSystem {
		if perm.Path == "" {
			return fmt.Errorf("filesystem permission %s missing path", perm.Action)
		}
		if !strings.HasPrefix(string(perm.Action), "fs:") {
			return fmt.Errorf("invalid filesystem action %s", perm.Action)
		}
		if err := validateGlobPath(perm.Path); err != nil {
			return fmt.Errorf("invalid filesystem path %s: %w", perm.Path, err)
		}
	}
	for _, exec := range p.Executables {
		if exec.Binary == "" {
			return errors.New("executable permission missing binary")
		}
		if strings.Contains(exec.Binary, "/") {
			return fmt.Errorf("executable %s must be referenced by name", exec.Binary)
		}
	}
	for _, net := range p.Network {
		if net.Direction == "" {
			return errors.New("network permission missing direction")
		}
		if net.Protocol == "" {
			return fmt.Errorf("network permission for %s missing protocol", net.Direction)
		}
		if net.Direction == "egress" && net.Host == "" {
			return errors.New("egress network permission must declare host")
		}
	}
	for _, cap := range p.Capabilities {
		if cap.Capability == "" {
			return errors.New("capability permission missing capability")
		}
	}
	for _, ipc := range p.IPC {
		if ipc.Kind == "" || ipc.Target == "" {
			return errors.New("ipc permission missing kind or target")
		}
	}
	return nil
}

// PermissionDescriptor describes a single permission decision.
type PermissionDescriptor struct {
	Type         PermissionType
	Action       string
	Resource     string
	Metadata     map[string]string
	RequiresHITL bool
}

// PermissionDeniedError indicates a permission check failed.
type PermissionDeniedError struct {
	Descriptor PermissionDescriptor
	Message    string
}

func (e *PermissionDeniedError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("permission denied: %v - %s", e.Descriptor, e.Message)
	}
	return fmt.Sprintf("permission denied: %v", e.Descriptor)
}

// FilePermissionChecker is the interface that platform/fs uses to check file
// permissions against the agent's file matrix. This interface is implemented by
// framework/authorization.PermissionManager.
type FilePermissionChecker interface {
	// CheckFilePermission validates a file operation against the agent's file matrix.
	// It returns nil if allowed, or an error if denied/ask requiring HITL.
	CheckFilePermission(ctx context.Context, agentID, basePath, action, absPath string, matrix AgentFileMatrix) error
}

// NetworkPermissionChecker is the interface that platform/browser uses to check
// network access against the agent's declared network permissions.
// Implemented by framework/authorization.PermissionManager.
type NetworkPermissionChecker interface {
	CheckNetwork(ctx context.Context, agentID, direction, protocol, host string, port int) error
}

// CapabilityChecker is the interface for checking capability usage permissions.
// Implemented by framework/authorization.PermissionManager.
type CapabilityChecker interface {
	CheckCapability(ctx context.Context, agentID string, capability string) error
}

// NewFileSystemPermissionSet builds a permission set for the provided actions scoped to base.
func NewFileSystemPermissionSet(base string, actions ...FileSystemAction) *PermissionSet {
	scope := computeWorkspaceScope(base)
	perms := make([]FileSystemPermission, 0, len(actions))
	for _, action := range actions {
		perms = append(perms, FileSystemPermission{
			Action: action,
			Path:   scope,
		})
	}
	return &PermissionSet{
		FileSystem: perms,
	}
}

// computeWorkspaceScope normalizes the workspace path into a glob that grants
// access to every file inside the directory tree without accidentally escaping
// to parent directories.
func computeWorkspaceScope(base string) string {
	if base == "" || base == "." {
		return "**"
	}
	clean := filepath.ToSlash(filepath.Clean(base))
	if clean == "." || clean == "" {
		return "**"
	}
	clean = strings.TrimSuffix(clean, "/")
	return clean + "/**"
}

// Sort normalizes permissions for deterministic manifests.
func (p *PermissionSet) Sort() {
	sort.Slice(p.FileSystem, func(i, j int) bool {
		return p.FileSystem[i].Path < p.FileSystem[j].Path
	})
	sort.Slice(p.Executables, func(i, j int) bool {
		return p.Executables[i].Binary < p.Executables[j].Binary
	})
	sort.Slice(p.Network, func(i, j int) bool {
		return p.Network[i].Host < p.Network[j].Host
	})
	sort.Slice(p.Capabilities, func(i, j int) bool {
		return p.Capabilities[i].Capability < p.Capabilities[j].Capability
	})
	sort.Slice(p.IPC, func(i, j int) bool {
		return p.IPC[i].Target < p.IPC[j].Target
	})
}

// validateGlobPath enforces simple invariants on glob inputs to prevent agents
// from sneaking traversal patterns into the allow/deny lists.
func validateGlobPath(path string) error {
	if path == "" {
		return errors.New("glob cannot be empty")
	}
	if strings.Contains(path, "..") {
		return errors.New("glob cannot contain ..")
	}
	clean := filepath.ToSlash(filepath.Clean(path))
	if strings.HasPrefix(clean, "../") || clean == ".." {
		return errors.New("glob cannot escape workspace")
	}
	re := regexp.MustCompile(`^[\w./*\-{}$]+$`)
	if !re.MatchString(path) {
		return errors.New("glob contains unsupported characters")
	}
	return nil
}
