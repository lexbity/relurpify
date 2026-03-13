package core

import "fmt"

type MemoryMode string

const (
	MemoryModeFresh  MemoryMode = "fresh"
	MemoryModeShared MemoryMode = "shared"
	MemoryModeCloned MemoryMode = "cloned"
)

type StateMode string

const (
	StateModeFresh  StateMode = "fresh"
	StateModeShared StateMode = "shared"
	StateModeCloned StateMode = "cloned"
	StateModeForked StateMode = "forked"
)

type ToolScopePolicy string

const (
	ToolScopeInherits ToolScopePolicy = "inherits"
	ToolScopeScoped   ToolScopePolicy = "scoped"
	ToolScopeCustom   ToolScopePolicy = "custom"
)

type AgentInvocationPolicy struct {
	MemoryMode MemoryMode      `yaml:"memory_mode,omitempty" json:"memory_mode,omitempty"`
	StateMode  StateMode       `yaml:"state_mode,omitempty" json:"state_mode,omitempty"`
	ToolScope  ToolScopePolicy `yaml:"tool_scope,omitempty" json:"tool_scope,omitempty"`
}

func (p AgentInvocationPolicy) Validate() error {
	switch p.MemoryMode {
	case "", MemoryModeFresh, MemoryModeShared, MemoryModeCloned:
	default:
		return fmt.Errorf("memory_mode %q invalid", p.MemoryMode)
	}
	switch p.StateMode {
	case "", StateModeFresh, StateModeShared, StateModeCloned, StateModeForked:
	default:
		return fmt.Errorf("state_mode %q invalid", p.StateMode)
	}
	switch p.ToolScope {
	case "", ToolScopeInherits, ToolScopeScoped, ToolScopeCustom:
	default:
		return fmt.Errorf("tool_scope %q invalid", p.ToolScope)
	}
	return nil
}

type AgentCompositionSpec struct {
	Type    string                 `yaml:"type,omitempty" json:"type,omitempty"`
	Handler string                 `yaml:"handler,omitempty" json:"handler,omitempty"`
	Policy  *AgentInvocationPolicy `yaml:"policy,omitempty" json:"policy,omitempty"`
}

func (s *AgentCompositionSpec) Validate() error {
	if s == nil {
		return nil
	}
	if s.Type == "" {
		return fmt.Errorf("composition type required")
	}
	if s.Type == "custom" && s.Handler == "" {
		return fmt.Errorf("composition handler required for custom type")
	}
	if s.Policy != nil {
		if err := s.Policy.Validate(); err != nil {
			return fmt.Errorf("composition policy invalid: %w", err)
		}
	}
	return nil
}
