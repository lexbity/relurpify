package contextmgr

import (
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/search"
)

type Context = core.Context
type ContextBudget = core.ContextBudget
type ContextItem = core.ContextItem
type ContextItemType = core.ContextItemType
type CompressionStrategy = core.CompressionStrategy
type FileSummary = core.FileSummary
type Interaction = core.Interaction
type PlanStep = core.PlanStep
type SharedContext = core.SharedContext
type SummaryLevel = core.SummaryLevel
type Summarizer = core.Summarizer
type Task = core.Task
type TaskType = core.TaskType
type Telemetry = core.Telemetry
type CodeIndex = search.CodeIndex
type SearchEngine = search.SearchEngine
type SearchQuery = search.SearchQuery
type TokenUsage = core.TokenUsage
type BudgetState = core.BudgetState

const (
	SummaryConcise = core.SummaryConcise
)

const (
	SearchHybrid = search.SearchHybrid
)

const (
	BudgetNeedsCompression = core.BudgetNeedsCompression
	BudgetCritical         = core.BudgetCritical
)

var estimateTokens = core.EstimateTokens
var estimateCodeTokens = core.EstimateCodeTokens
