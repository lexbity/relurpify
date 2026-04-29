package agentgraph

import "codeburg.org/lexbit/relurpify/framework/core"

type Result = core.Result
type TaskType = core.TaskType
type TaskContext = core.TaskContext
type Telemetry = core.Telemetry
type Event = core.Event
type LLMOptions = core.LLMOptions
type LanguageModel = core.LanguageModel
type LLMResponse = core.LLMResponse
type Message = core.Message
type ToolCall = core.ToolCall
type Tool = core.Tool

const (
	EventGraphStart  = core.EventGraphStart
	EventGraphFinish = core.EventGraphFinish
	EventNodeStart   = core.EventNodeStart
	EventNodeFinish  = core.EventNodeFinish
	EventNodeError   = core.EventNodeError
)

var WithTaskContext = core.WithTaskContext
