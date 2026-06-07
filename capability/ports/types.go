package ports

import (
	"context"
	"errors"
	"time"

	"codeburg.org/lexbit/relurpify/governance/permissions"
	"codeburg.org/lexbit/relurpify/model"
)

// Ensure ports types match the canonical model types.
type LLMToolSpec = model.LLMToolSpec

// ToolParameterType enumerates the allowed parameter types.
type ToolParameterType string

const (
	ToolParamString  ToolParameterType = "string"
	ToolParamInteger ToolParameterType = "integer"
	ToolParamNumber  ToolParameterType = "number"
	ToolParamBoolean ToolParameterType = "boolean"
	ToolParamArray   ToolParameterType = "array"
	ToolParamObject  ToolParameterType = "object"
)

// Flag expansion styles for tool manifest flags.
const (
	FlagStyleEquals   = "equals"
	FlagStyleSeparate = "separate"
)

// ToolTags are well-known capability tags.
const (
	TagReadOnly    = "read-only"
	TagExecute     = "execute"
	TagDestructive = "destructive"
	TagNetwork     = "network"
)

// Tool is the primary capability interface. Every tool, whether native,
// subprocess, or composite, implements this interface.
type Tool interface {
	Name() string
	Description() string
	Category() string
	Parameters() []ToolParameter
	Execute(ctx context.Context, args map[string]interface{}) (*ToolResult, error)
	IsAvailable(ctx context.Context) bool
	Permissions() ToolPermissions
	Tags() []string
}

// StreamingTool extends Tool with streaming execution.
type StreamingTool interface {
	Tool
	ExecuteStream(ctx context.Context, args map[string]interface{}) (<-chan ToolChunk, error)
}

// RevertibleTool extends Tool with rollback support.
type RevertibleTool interface {
	Tool
	Rollback(ctx context.Context, token RollbackToken) error
}

// RollbackToken captures state needed to undo a tool invocation.
type RollbackToken struct {
	InvocationID string
	ToolName     string
	Args         map[string]interface{}
	Result       *ToolResult
}

// ToolParameter describes a single parameter accepted by a tool.
type ToolParameter struct {
	Name        string            `json:"name" yaml:"name"`
	Type        ToolParameterType `json:"type" yaml:"type"`
	Description string            `json:"description,omitempty" yaml:"description,omitempty"`
	Required    bool              `json:"required,omitempty" yaml:"required,omitempty"`
	Default     interface{}       `json:"default,omitempty" yaml:"default,omitempty"`
}

// ToolResult is the result of executing a tool.
type ToolResult struct {
	Success  bool                   `json:"success"`
	Data     map[string]interface{} `json:"data,omitempty"`
	Error    string                 `json:"error,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// ToolChunk is a streaming chunk from a streaming tool.
type ToolChunk struct {
	Data   map[string]interface{} `json:"data,omitempty"`
	Error  string                 `json:"error,omitempty"`
	Done   bool                   `json:"done"`
	SeqNum int                    `json:"seq_num"`
}

// ToolPermissions describes the permissions a tool requires.
type ToolPermissions struct {
	Permissions *permissions.PermissionSet `json:"permissions,omitempty"`
}

// Validate ensures tool permission manifests are well-formed.
func (t ToolPermissions) Validate() error {
	if t.Permissions == nil {
		return errors.New("tool permissions missing")
	}
	return t.Permissions.Validate()
}

// ParamKeysProvider is an optional extension for tools that declare
// which parameter keys they read.
type ParamKeysProvider interface {
	ParamKeys() []string
}

// CapabilityExecutionResult is an alias for backwards compatibility.
type CapabilityExecutionResult = ToolResult

// CommandRequest is a request to run a command in a sandboxed environment.
type CommandRequest struct {
	Workdir       string        `json:"workdir,omitempty"`
	Args          []string      `json:"args,omitempty"`
	Env           []string      `json:"env,omitempty"`
	Input         string        `json:"input,omitempty"`
	Timeout       time.Duration `json:"timeout,omitempty"`
	UsePTY        bool          `json:"use_pty,omitempty"`
	MemoryBytes   int64         `json:"memory_bytes,omitempty"`
	PidsLimit     int64         `json:"pids_limit,omitempty"`
	CPUs          float64       `json:"cpus,omitempty"`
	OutputCeiling int64         `json:"output_ceiling,omitempty"`
	GracePeriod   time.Duration `json:"grace_period,omitempty"`
}

// CommandResult is the result of executing a command.
type CommandResult struct {
	Stdout      string        `json:"stdout"`
	Stderr      string        `json:"stderr"`
	ExitCode    int           `json:"exit_code"`
	Signaled    bool          `json:"signaled,omitempty"`
	TimedOut    bool          `json:"timed_out,omitempty"`
	TornDown    bool          `json:"torn_down,omitempty"`
	OOMKilled   bool          `json:"oom_killed,omitempty"`
	Duration    time.Duration `json:"duration,omitempty"`
	StdoutBytes int64         `json:"stdout_bytes,omitempty"`
	StderrBytes int64         `json:"stderr_bytes,omitempty"`
	StdoutRef   string        `json:"stdout_ref,omitempty"`
	StderrRef   string        `json:"stderr_ref,omitempty"`
}

// CommandRunner executes a command request and returns a result.
type CommandRunner interface {
	Run(ctx context.Context, req CommandRequest) (*CommandResult, error)
}

// Schema is a JSON Schema used for tool parameters.
type Schema struct {
	Type        string             `json:"type,omitempty" yaml:"type,omitempty"`
	Properties  map[string]*Schema `json:"properties,omitempty" yaml:"properties,omitempty"`
	Items       *Schema            `json:"items,omitempty" yaml:"items,omitempty"`
	Required    []string           `json:"required,omitempty" yaml:"required,omitempty"`
	Default     interface{}        `json:"default,omitempty" yaml:"default,omitempty"`
	Enum        []interface{}      `json:"enum,omitempty" yaml:"enum,omitempty"`
	Title       string             `json:"title,omitempty" yaml:"title,omitempty"`
	Description string             `json:"description,omitempty" yaml:"description,omitempty"`
	Format      string             `json:"format,omitempty" yaml:"format,omitempty"`
}

// ToolCall represents a tool invocation requested by an LLM.
type ToolCall = model.ToolCall

// ToolRegistry is a read-only interface for looking up tool manifests.
type ToolRegistry interface {
	LookupTool(name string) (ToolManifest, bool)
	ListTools() []ToolManifest
}

// NativeToolConstructor creates a Tool from a base workspace path.
type NativeToolConstructor func(basePath string) Tool
