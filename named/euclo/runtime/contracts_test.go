package runtime

import (
	"strings"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	euclorelurpic "github.com/lexcodex/relurpify/named/euclo/relurpicabilities"
)

func TestEnforcePreExecutionCapabilityContracts_ArchaeologyImplementRequiresPlanBinding(t *testing.T) {
	err := EnforcePreExecutionCapabilityContracts(UnitOfWork{
		PrimaryRelurpicCapabilityID: euclorelurpic.CapabilityArchaeologyImplement,
	})
	if err == nil || !strings.Contains(err.Error(), "compiled living plan") {
		t.Fatalf("expected plan binding error, got %v", err)
	}
	err = EnforcePreExecutionCapabilityContracts(UnitOfWork{
		PrimaryRelurpicCapabilityID: euclorelurpic.CapabilityArchaeologyImplement,
		PlanBinding:                 &UnitOfWorkPlanBinding{IsPlanBacked: true},
	})
	if err != nil {
		t.Fatalf("expected nil with plan-backed binding, got %v", err)
	}
}

func TestEvaluatePostExecutionCapabilityContracts_ChatAskBlocksFileMutation(t *testing.T) {
	state := core.NewContext()
	state.Set("euclo.edit_execution", EditExecutionRecord{
		Executed: []EditOperationRecord{{Path: "x.go", Status: "applied"}},
	})
	work := UnitOfWork{PrimaryRelurpicCapabilityID: euclorelurpic.CapabilityChatAsk}
	rt, err := EvaluatePostExecutionCapabilityContracts(work, state, time.Unix(1700000000, 0).UTC())
	if err == nil {
		t.Fatal("expected non-mutating contract violation")
	}
	if !rt.Blocked || rt.ViolationReason != "non_mutating_contract_violated" {
		t.Fatalf("expected blocked contract, got %#v err=%v", rt, err)
	}
}

func TestEvaluatePostExecutionCapabilityContracts_ChatImplementAllowsMutation(t *testing.T) {
	state := core.NewContext()
	state.Set("pipeline.code", map[string]any{"summary": "planned edits"})
	work := UnitOfWork{PrimaryRelurpicCapabilityID: euclorelurpic.CapabilityChatImplement}
	rt, err := EvaluatePostExecutionCapabilityContracts(work, state, time.Now().UTC())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rt.Blocked {
		t.Fatalf("implement should not block on pipeline.code, got %#v", rt)
	}
}

func TestEvaluatePostExecutionCapabilityContracts_InspectFirstRecordsMutationDiagnostic(t *testing.T) {
	state := core.NewContext()
	state.Set("euclo.edit_execution", EditExecutionRecord{
		Executed: []EditOperationRecord{{Path: "y.go", Status: "applied"}},
	})
	work := UnitOfWork{PrimaryRelurpicCapabilityID: euclorelurpic.CapabilityChatInspect}
	rt, err := EvaluatePostExecutionCapabilityContracts(work, state, time.Now().UTC())
	if err != nil {
		t.Fatalf("inspect-first allows completion with diagnostics: %v", err)
	}
	found := false
	for _, d := range rt.Diagnostics {
		if strings.Contains(d, "inspect-first") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected inspect-first diagnostic, got %#v", rt.Diagnostics)
	}
}

func TestBuildCapabilityContractDeferredIssues_CompilePlanWithoutBindingProducesIssue(t *testing.T) {
	now := time.Unix(1700000000, 123456789).UTC()
	work := UnitOfWork{
		WorkflowID:                  "wf-d1",
		RunID:                       "run-d1",
		ExecutionID:                 "exec-d1",
		PrimaryRelurpicCapabilityID: euclorelurpic.CapabilityArchaeologyCompilePlan,
		SemanticInputs: SemanticInputBundle{
			PatternRefs:           []string{"p1"},
			TensionRefs:           []string{"t1"},
			ProvenanceRefs:        []string{"pr1"},
			RequestProvenanceRefs: []string{"rq1"},
		},
	}
	issues := BuildCapabilityContractDeferredIssues(work, core.NewContext(), now)
	if len(issues) != 1 {
		t.Fatalf("expected one deferred issue, got %#v", issues)
	}
	issue := issues[0]
	if issue.Kind != DeferredIssueAmbiguity {
		t.Fatalf("expected ambiguity kind, got %q", issue.Kind)
	}
	if issue.WorkflowID != "wf-d1" || issue.RunID != "run-d1" || issue.ExecutionID != "exec-d1" {
		t.Fatalf("unexpected work ids: %#v", issue)
	}
	if !strings.Contains(issue.Title, "plan") {
		t.Fatalf("expected plan-oriented title, got %q", issue.Title)
	}
	if len(issue.Evidence.RelevantPatternRefs) != 1 || issue.Evidence.RelevantPatternRefs[0] != "p1" {
		t.Fatalf("expected pattern ref evidence, got %#v", issue.Evidence.RelevantPatternRefs)
	}
	if len(issue.Evidence.RelevantTensionRefs) != 1 || issue.Evidence.RelevantTensionRefs[0] != "t1" {
		t.Fatalf("expected tension ref evidence, got %#v", issue.Evidence.RelevantTensionRefs)
	}
}

func TestBuildCapabilityContractDeferredIssues_NonCompilePlanProducesNone(t *testing.T) {
	work := UnitOfWork{PrimaryRelurpicCapabilityID: euclorelurpic.CapabilityChatAsk}
	if got := BuildCapabilityContractDeferredIssues(work, core.NewContext(), time.Now().UTC()); len(got) != 0 {
		t.Fatalf("expected no issues, got %#v", got)
	}
}

func TestBuildCapabilityContractRuntimeState_DebugEscalationTriggeredAfterMutation(t *testing.T) {
	state := core.NewContext()
	state.Set("euclo.edit_execution", EditExecutionRecord{Executed: []EditOperationRecord{{Path: "z.go", Status: "applied"}}})
	rt := BuildCapabilityContractRuntimeState(UnitOfWork{
		PrimaryRelurpicCapabilityID: euclorelurpic.CapabilityDebugInvestigate,
	}, state, time.Now().UTC())
	if !rt.DebugEscalationTriggered || rt.DebugEscalationTarget != euclorelurpic.CapabilityChatImplement {
		t.Fatalf("expected debug escalation toward implement, got %#v", rt)
	}
}
