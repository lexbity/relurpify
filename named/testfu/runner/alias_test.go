package runner

import (
	"reflect"
	"testing"

	agenttestpkg "codeburg.org/lexbit/relurpify/testsuite/agenttest"
)

func TestAliasParityWithAgenttest(t *testing.T) {
	assertSameType[RunOptions, agenttestpkg.RunOptions](t, "RunOptions")
	assertSameType[SuiteReport, agenttestpkg.SuiteReport](t, "SuiteReport")
	assertSameType[CaseReport, agenttestpkg.CaseReport](t, "CaseReport")
	assertSameType[Suite, agenttestpkg.Suite](t, "Suite")
	assertSameType[SuiteMeta, agenttestpkg.SuiteMeta](t, "SuiteMeta")
	assertSameType[SuiteSpec, agenttestpkg.SuiteSpec](t, "SuiteSpec")
	assertSameType[SuiteExecutionSpec, agenttestpkg.SuiteExecutionSpec](t, "SuiteExecutionSpec")
	assertSameType[WorkspaceSpec, agenttestpkg.WorkspaceSpec](t, "WorkspaceSpec")
	assertSameType[ModelSpec, agenttestpkg.ModelSpec](t, "ModelSpec")
	assertSameType[RecordingSpec, agenttestpkg.RecordingSpec](t, "RecordingSpec")
	assertSameType[CaseSpec, agenttestpkg.CaseSpec](t, "CaseSpec")
	assertSameType[BrowserFixtureSpec, agenttestpkg.BrowserFixtureSpec](t, "BrowserFixtureSpec")
	assertSameType[RequiresSpec, agenttestpkg.RequiresSpec](t, "RequiresSpec")
	assertSameType[SetupSpec, agenttestpkg.SetupSpec](t, "SetupSpec")
	assertSameType[SetupFileSpec, agenttestpkg.SetupFileSpec](t, "SetupFileSpec")
	assertSameType[ExpectSpec, agenttestpkg.ExpectSpec](t, "ExpectSpec")
	assertSameType[FileContentExpectation, agenttestpkg.FileContentExpectation](t, "FileContentExpectation")
	assertSameType[CaseOverrideSpec, agenttestpkg.CaseOverrideSpec](t, "CaseOverrideSpec")
	assertSameType[MemorySpec, agenttestpkg.MemorySpec](t, "MemorySpec")
	assertSameType[MemoryRetrievalSpec, agenttestpkg.MemoryRetrievalSpec](t, "MemoryRetrievalSpec")
	assertSameType[MemorySeedSpec, agenttestpkg.MemorySeedSpec](t, "MemorySeedSpec")
	assertSameType[DeclarativeMemorySeedSpec, agenttestpkg.DeclarativeMemorySeedSpec](t, "DeclarativeMemorySeedSpec")
	assertSameType[ProceduralMemorySeedSpec, agenttestpkg.ProceduralMemorySeedSpec](t, "ProceduralMemorySeedSpec")
	assertSameType[WorkflowSeedSpec, agenttestpkg.WorkflowSeedSpec](t, "WorkflowSeedSpec")
	assertSameType[WorkflowRecordSeedSpec, agenttestpkg.WorkflowRecordSeedSpec](t, "WorkflowRecordSeedSpec")
	assertSameType[WorkflowRunSeedSpec, agenttestpkg.WorkflowRunSeedSpec](t, "WorkflowRunSeedSpec")
	assertSameType[WorkflowKnowledgeSeedSpec, agenttestpkg.WorkflowKnowledgeSeedSpec](t, "WorkflowKnowledgeSeedSpec")
	assertSameType[WorkflowCheckpointSeedSpec, agenttestpkg.WorkflowCheckpointSeedSpec](t, "WorkflowCheckpointSeedSpec")
	assertSameType[TokenUsageReport, agenttestpkg.TokenUsageReport](t, "TokenUsageReport")
	assertSameType[MemoryOutcomeReport, agenttestpkg.MemoryOutcomeReport](t, "MemoryOutcomeReport")

	if reflect.ValueOf(LoadSuite).Pointer() != reflect.ValueOf(agenttestpkg.LoadSuite).Pointer() {
		t.Fatal("LoadSuite should forward directly to testsuite/agenttest")
	}
	if reflect.ValueOf(FilterSuiteCasesByTags).Pointer() != reflect.ValueOf(agenttestpkg.FilterSuiteCasesByTags).Pointer() {
		t.Fatal("FilterSuiteCasesByTags should forward directly to testsuite/agenttest")
	}
}

func assertSameType[A any, B any](t *testing.T, name string) {
	t.Helper()
	var a A
	var b B
	if reflect.TypeOf(a) != reflect.TypeOf(b) {
		t.Fatalf("%s alias drifted: %v != %v", name, reflect.TypeOf(a), reflect.TypeOf(b))
	}
}
