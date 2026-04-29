package relurpic

import agentspec "codeburg.org/lexbit/relurpify/framework/agentspec"

type MemoryMode = agentspec.MemoryMode
type StateMode = agentspec.StateMode
type ToolScopePolicy = agentspec.ToolScopePolicy
type AgentInvocationPolicy = agentspec.AgentInvocationPolicy

const (
	MemoryModeFresh  = agentspec.MemoryModeFresh
	MemoryModeShared = agentspec.MemoryModeShared
	MemoryModeCloned = agentspec.MemoryModeCloned

	StateModeFresh  = agentspec.StateModeFresh
	StateModeShared = agentspec.StateModeShared
	StateModeCloned = agentspec.StateModeCloned
	StateModeForked = agentspec.StateModeForked

	ToolScopeInherits = agentspec.ToolScopeInherits
	ToolScopeScoped   = agentspec.ToolScopeScoped
	ToolScopeCustom   = agentspec.ToolScopeCustom
)

var DefaultInvocationPolicies = map[string]agentspec.AgentInvocationPolicy{
	"react":      {MemoryMode: agentspec.MemoryModeShared, StateMode: agentspec.StateModeCloned, ToolScope: agentspec.ToolScopeInherits},
	"planner":    {MemoryMode: agentspec.MemoryModeShared, StateMode: agentspec.StateModeFresh, ToolScope: agentspec.ToolScopeScoped},
	"pipeline":   {MemoryMode: agentspec.MemoryModeFresh, StateMode: agentspec.StateModeFresh, ToolScope: agentspec.ToolScopeInherits},
	"reflection": {MemoryMode: agentspec.MemoryModeShared, StateMode: agentspec.StateModeCloned, ToolScope: agentspec.ToolScopeInherits},
	"chainer":    {MemoryMode: agentspec.MemoryModeFresh, StateMode: agentspec.StateModeFresh, ToolScope: agentspec.ToolScopeInherits},
	"rewoo":      {MemoryMode: agentspec.MemoryModeFresh, StateMode: agentspec.StateModeFresh, ToolScope: agentspec.ToolScopeInherits},
	"htn":        {MemoryMode: agentspec.MemoryModeShared, StateMode: agentspec.StateModeCloned, ToolScope: agentspec.ToolScopeInherits},
	"blackboard": {MemoryMode: agentspec.MemoryModeShared, StateMode: agentspec.StateModeCloned, ToolScope: agentspec.ToolScopeInherits},
	"goalcon":    {MemoryMode: agentspec.MemoryModeShared, StateMode: agentspec.StateModeForked, ToolScope: agentspec.ToolScopeInherits},
}
