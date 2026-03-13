package runner

import (
	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/graph"
	agenttestpkg "github.com/lexcodex/relurpify/testsuite/agenttest"
)

type RunOptions = agenttestpkg.RunOptions
type SuiteReport = agenttestpkg.SuiteReport
type CaseReport = agenttestpkg.CaseReport
type Runner = agenttestpkg.Runner
type Suite = agenttestpkg.Suite
type SuiteMeta = agenttestpkg.SuiteMeta
type SuiteSpec = agenttestpkg.SuiteSpec
type SuiteExecutionSpec = agenttestpkg.SuiteExecutionSpec
type WorkspaceSpec = agenttestpkg.WorkspaceSpec
type ModelSpec = agenttestpkg.ModelSpec
type RecordingSpec = agenttestpkg.RecordingSpec
type CaseSpec = agenttestpkg.CaseSpec
type BrowserFixtureSpec = agenttestpkg.BrowserFixtureSpec
type RequiresSpec = agenttestpkg.RequiresSpec
type SetupSpec = agenttestpkg.SetupSpec
type SetupFileSpec = agenttestpkg.SetupFileSpec
type ExpectSpec = agenttestpkg.ExpectSpec
type FileContentExpectation = agenttestpkg.FileContentExpectation
type CaseOverrideSpec = agenttestpkg.CaseOverrideSpec
type MemorySpec = agenttestpkg.MemorySpec
type MemoryRetrievalSpec = agenttestpkg.MemoryRetrievalSpec
type MemorySeedSpec = agenttestpkg.MemorySeedSpec
type DeclarativeMemorySeedSpec = agenttestpkg.DeclarativeMemorySeedSpec
type ProceduralMemorySeedSpec = agenttestpkg.ProceduralMemorySeedSpec
type WorkflowSeedSpec = agenttestpkg.WorkflowSeedSpec
type WorkflowRecordSeedSpec = agenttestpkg.WorkflowRecordSeedSpec
type WorkflowRunSeedSpec = agenttestpkg.WorkflowRunSeedSpec
type WorkflowKnowledgeSeedSpec = agenttestpkg.WorkflowKnowledgeSeedSpec
type WorkflowCheckpointSeedSpec = agenttestpkg.WorkflowCheckpointSeedSpec
type TokenUsageReport = agenttestpkg.TokenUsageReport
type MemoryOutcomeReport = agenttestpkg.MemoryOutcomeReport

var LoadSuite = agenttestpkg.LoadSuite
var FilterSuiteCasesByTags = agenttestpkg.FilterSuiteCasesByTags

// RegisterNamedAgent is retained as a compatibility no-op so existing
// named/testfu callers do not need to change imports during the runner merge.
func RegisterNamedAgent(_ string, _ func(string, agentenv.AgentEnvironment) graph.Agent) {}
