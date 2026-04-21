package relurpic

import "codeburg.org/lexbit/relurpify/framework/core"

type MemoryMode = core.MemoryMode
type StateMode = core.StateMode
type ToolScopePolicy = core.ToolScopePolicy
type AgentInvocationPolicy = core.AgentInvocationPolicy

const (
	MemoryModeFresh  = core.MemoryModeFresh
	MemoryModeShared = core.MemoryModeShared
	MemoryModeCloned = core.MemoryModeCloned

	StateModeFresh  = core.StateModeFresh
	StateModeShared = core.StateModeShared
	StateModeCloned = core.StateModeCloned
	StateModeForked = core.StateModeForked

	ToolScopeInherits = core.ToolScopeInherits
	ToolScopeScoped   = core.ToolScopeScoped
	ToolScopeCustom   = core.ToolScopeCustom
)

var DefaultInvocationPolicies = map[string]core.AgentInvocationPolicy{
	"react":      {MemoryMode: core.MemoryModeShared, StateMode: core.StateModeCloned, ToolScope: core.ToolScopeInherits},
	"architect":  {MemoryMode: core.MemoryModeShared, StateMode: core.StateModeCloned, ToolScope: core.ToolScopeInherits},
	"planner":    {MemoryMode: core.MemoryModeShared, StateMode: core.StateModeFresh, ToolScope: core.ToolScopeScoped},
	"pipeline":   {MemoryMode: core.MemoryModeFresh, StateMode: core.StateModeFresh, ToolScope: core.ToolScopeInherits},
	"reflection": {MemoryMode: core.MemoryModeShared, StateMode: core.StateModeCloned, ToolScope: core.ToolScopeInherits},
	"chainer":    {MemoryMode: core.MemoryModeFresh, StateMode: core.StateModeFresh, ToolScope: core.ToolScopeInherits},
	"rewoo":      {MemoryMode: core.MemoryModeFresh, StateMode: core.StateModeFresh, ToolScope: core.ToolScopeInherits},
	"htn":        {MemoryMode: core.MemoryModeShared, StateMode: core.StateModeCloned, ToolScope: core.ToolScopeInherits},
	"blackboard": {MemoryMode: core.MemoryModeShared, StateMode: core.StateModeCloned, ToolScope: core.ToolScopeInherits},
	"goalcon":    {MemoryMode: core.MemoryModeShared, StateMode: core.StateModeForked, ToolScope: core.ToolScopeInherits},
}
