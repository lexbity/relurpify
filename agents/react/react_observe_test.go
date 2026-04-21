package react

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
)

// --- helpers ---

func observeNode(t *testing.T, task *core.Task) (*reactObserveNode, *ReActAgent) {
	t.Helper()
	agent := &ReActAgent{
		Tools:         capability.NewRegistry(),
		maxIterations: 10,
	}
	node := &reactObserveNode{id: "react_observe", agent: agent, task: task}
	return node, agent
}

func editTask() *core.Task {
	return &core.Task{Instruction: "fix the login bug"}
}

func readOnlyTask() *core.Task {
	return &core.Task{Instruction: "summarize the config file"}
}

func inspectTask() *core.Task {
	return &core.Task{Instruction: "inspect the config file"}
}

func addObservations(state *core.Context, obs ...ToolObservation) {
	existing := getToolObservations(state)
	state.Set("react.tool_observations", append(existing, obs...))
}

func writeObs(success bool) ToolObservation {
	return ToolObservation{Tool: "file_write", Phase: "edit", Success: success, Summary: "wrote file"}
}

func testToolObs(success bool, stdout, stderr string) ToolObservation {
	return ToolObservation{
		Tool:    "go_test",
		Phase:   "verify",
		Success: success,
		Summary: "ran tests",
		Data:    map[string]interface{}{"stdout": stdout, "stderr": stderr},
	}
}

func fileReadObs(path, snippet string) ToolObservation {
	return ToolObservation{
		Tool:    "file_read",
		Phase:   "explore",
		Success: true,
		Summary: snippet,
		Args:    map[string]interface{}{"path": path},
		Data:    map[string]interface{}{"snippet": snippet},
	}
}

func buildFailObs(stderr string) ToolObservation {
	return ToolObservation{
		Tool:    "go_test",
		Phase:   "verify",
		Success: false,
		Summary: "tests failed",
		Data:    map[string]interface{}{"stderr": stderr, "stdout": ""},
	}
}

func readFailObs(message string) ToolObservation {
	return ToolObservation{
		Tool:    "file_read",
		Phase:   "explore",
		Success: false,
		Summary: "read failed",
		Data:    map[string]interface{}{"error": message},
	}
}

// requireCompleted asserts state indicates a complete, successful observe result.
func requireCompleted(t *testing.T, result *core.Result, state *core.Context) {
	t.Helper()
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.Success {
		t.Fatalf("expected result.Success=true, got false (error: %v)", result.Error)
	}
	if complete, _ := result.Data["complete"].(bool); !complete {
		t.Fatalf("expected result.Data[complete]=true")
	}
	done, _ := state.Get("react.done")
	if done != true {
		t.Fatalf("expected react.done=true, got %v", done)
	}
	if summary := strings.TrimSpace(state.GetString("react.synthetic_summary")); summary == "" {
		t.Fatal("expected react.synthetic_summary to be set")
	}
	if reason := strings.TrimSpace(state.GetString("react.incomplete_reason")); reason != "" {
		t.Fatalf("expected react.incomplete_reason to be empty, got %q", reason)
	}
	raw, ok := state.Get("react.final_output")
	if !ok {
		t.Fatal("expected react.final_output to be set")
	}
	payload, _ := raw.(map[string]interface{})
	if payload["summary"] == nil || strings.TrimSpace(payload["summary"].(string)) == "" {
		t.Fatalf("expected react.final_output[summary] to be set, got %#v", payload)
	}
}

// requireIncomplete asserts state indicates the loop should continue.
func requireIncomplete(t *testing.T, result *core.Result, state *core.Context) {
	t.Helper()
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.Success {
		t.Fatalf("expected result.Success=true (incomplete is not an error), got false")
	}
	if complete, _ := result.Data["complete"].(bool); complete {
		t.Fatalf("expected result.Data[complete]=false")
	}
	done, _ := state.Get("react.done")
	if done == true {
		t.Fatalf("expected react.done != true, got %v", done)
	}
}

// --- verification latched summary ---

func TestObserveNode_VerificationLatchedSummary_ShortCircuits(t *testing.T) {
	node, _ := observeNode(t, editTask())
	state := core.NewContext()
	state.Set("react.verification_latched_summary", "tests passed")
	state.Set("react.last_tool_result", map[string]interface{}{})

	result, err := node.Execute(context.Background(), state)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("expected successful result, got %+v", result)
	}
	done, _ := state.Get("react.done")
	if done != true {
		t.Fatalf("expected react.done=true, got %v", done)
	}
	finalRaw, ok := state.Get("react.final_output")
	if !ok {
		t.Fatal("expected react.final_output to be set")
	}
	payload, _ := finalRaw.(map[string]interface{})
	if got := payload["summary"]; !strings.Contains(strings.ToLower(fmt.Sprint(got)), "tests passed") {
		t.Fatalf("expected final_output summary to reflect latched value, got %#v", payload)
	}
}

// --- verificationSummaryFromSuccess early-return branch ---

func TestObserveNode_VerificationSummaryFromSuccess_EarlyReturn(t *testing.T) {
	// task that needs editing, has an edit observation, then a passing verification tool
	node, _ := observeNode(t, editTask())
	state := core.NewContext()
	state.Set("react.last_tool_result", map[string]interface{}{"success": true})
	addObservations(state,
		writeObs(true),
		testToolObs(true, "ok", ""),
	)
	state.Set("react.decision", decisionPayload{Complete: false})

	result, err := node.Execute(context.Background(), state)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	requireCompleted(t, result, state)
	if !strings.Contains(state.GetString("react.synthetic_summary"), "go_test") {
		t.Fatalf("expected synthetic_summary to reference the verification tool, got %q", state.GetString("react.synthetic_summary"))
	}
}

// --- editSummaryFromSuccess early-return branch ---

func TestObserveNode_EditSummaryFromSuccess_EarlyReturn(t *testing.T) {
	// task needs editing, no verification required, last successful obs is a write
	node, _ := observeNode(t, editTask())
	state := core.NewContext()
	state.Set("react.last_tool_result", map[string]interface{}{"success": true})
	addObservations(state, writeObs(true))
	state.Set("react.decision", decisionPayload{Complete: false})

	result, err := node.Execute(context.Background(), state)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	requireCompleted(t, result, state)
	if !strings.Contains(state.GetString("react.synthetic_summary"), "applied") {
		t.Fatalf("expected edit summary, got %q", state.GetString("react.synthetic_summary"))
	}
}

// --- readOnlySummaryFromState early-return branch ---

func TestObserveNode_ReadOnlySummaryFromState_EarlyReturn(t *testing.T) {
	node, _ := observeNode(t, readOnlyTask())
	state := core.NewContext()
	state.Set("react.last_tool_result", map[string]interface{}{"success": true})
	addObservations(state, fileReadObs("/app/config.yaml", "service: api"))
	state.Set("react.decision", decisionPayload{Complete: false})

	result, err := node.Execute(context.Background(), state)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	requireCompleted(t, result, state)
	summary := state.GetString("react.synthetic_summary")
	if !strings.Contains(summary, "config.yaml") && !strings.Contains(summary, "service: api") {
		t.Fatalf("expected read-only summary to reference the file, got %q", summary)
	}
}

// --- analysisSummaryFromFailure early-return branch ---

func TestObserveNode_AnalysisSummaryFromFailure_EarlyReturn(t *testing.T) {
	// non-editing task, build tool failed with a meaningful error
	node, _ := observeNode(t, readOnlyTask())
	failedObs := buildFailObs("FAIL: TestLogin undefined method")
	state := core.NewContext()
	state.Set("react.last_tool_result", map[string]interface{}{"success": false, "error": "tests failed"})
	addObservations(state, failedObs)
	state.Set("react.decision", decisionPayload{Complete: false})

	result, err := node.Execute(context.Background(), state)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	requireCompleted(t, result, state)
	summary := state.GetString("react.synthetic_summary")
	if !strings.Contains(summary, "go_test") {
		t.Fatalf("expected analysis summary to name the failing tool, got %q", summary)
	}
}

// --- scheduleRecoveryProbe: loop continues ---

func TestObserveNode_ScheduleRecoveryProbe_ContinuesLoop(t *testing.T) {
	// read-only task, failure in lastMap, but recovery probe tool available
	task := &core.Task{
		Instruction: "inspect the config file",
		Context: map[string]any{
			"agent_spec": &core.AgentRuntimeSpec{
				SkillConfig: core.AgentSkillConfig{
					Recovery: core.AgentRecoveryPolicy{
						FailureProbeTools: []string{"file_read"},
					},
				},
			},
		},
	}
	node, agent := observeNode(t, task)

	fileReadTool := stubTool{
		name:   "file_read",
		params: []core.ToolParameter{{Name: "path", Required: true}},
	}
	if err := agent.Tools.Register(fileReadTool); err != nil {
		t.Fatalf("register tool: %v", err)
	}

	state := core.NewContext()
	// Failure in lastMap triggers recovery probe scheduling
	state.Set("react.last_tool_result", map[string]interface{}{"success": false, "error": "file not found"})
	state.Set("react.failure_path", "/src/app.go")
	// A non-empty observation history so scheduleRecoveryProbe can find a signature
	addObservations(state, readFailObs("FAIL: file not found"))
	state.Set("react.decision", decisionPayload{Complete: false})

	result, err := node.Execute(context.Background(), state)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	requireIncomplete(t, result, state)
	// recovery probe should have queued a tool call
	raw, ok := state.Get("react.tool_calls")
	if !ok {
		t.Fatal("expected react.tool_calls to be set after recovery probe")
	}
	calls, _ := raw.([]core.ToolCall)
	if len(calls) == 0 {
		t.Fatal("expected at least one queued tool call from recovery probe")
	}
	if calls[0].Name != "file_read" {
		t.Fatalf("expected recovery probe to queue file_read, got %q", calls[0].Name)
	}
}

// --- applyCompletionSummary helper ---

func TestApplyCompletionSummary_SetsAllStateKeys(t *testing.T) {
	node, _ := observeNode(t, editTask())
	state := core.NewContext()
	lastMap := map[string]interface{}{"success": true, "data": "ok"}
	var diag strings.Builder
	diag.WriteString("Iteration 1 observation.\n")

	result := node.applyCompletionSummary(state, "all tests passed", lastMap, &diag)

	if result == nil || !result.Success {
		t.Fatalf("expected successful result, got %+v", result)
	}
	if complete, _ := result.Data["complete"].(bool); !complete {
		t.Fatal("expected result.Data[complete]=true")
	}
	if done, _ := state.Get("react.done"); done != true {
		t.Fatalf("expected react.done=true, got %v", done)
	}
	if got := state.GetString("react.synthetic_summary"); got != "all tests passed" {
		t.Fatalf("expected react.synthetic_summary=%q, got %q", "all tests passed", got)
	}
	if got := state.GetString("react.incomplete_reason"); got != "" {
		t.Fatalf("expected react.incomplete_reason to be cleared, got %q", got)
	}
	raw, ok := state.Get("react.final_output")
	if !ok {
		t.Fatal("expected react.final_output to be set")
	}
	payload := raw.(map[string]interface{})
	if payload["summary"] != "all tests passed" {
		t.Fatalf("unexpected final_output summary: %#v", payload)
	}
	if payload["result"] == nil {
		t.Fatal("expected final_output to carry the last tool result")
	}
	diagnostic := result.Data["diagnostic"].(string)
	if !strings.Contains(diagnostic, "Conclusion: all tests passed") {
		t.Fatalf("expected diagnostic to contain conclusion, got %q", diagnostic)
	}
}

// --- setSynthesizedConclusion helper ---

func TestSetSynthesizedConclusion_UpdatesStateWithoutReturning(t *testing.T) {
	state := core.NewContext()
	state.Set("react.incomplete_reason", "stuck in loop")
	var diag strings.Builder

	setSynthesizedConclusion(state, "analysis complete", &diag)

	if got := state.GetString("react.synthetic_summary"); got != "analysis complete" {
		t.Fatalf("expected react.synthetic_summary=%q, got %q", "analysis complete", got)
	}
	if got := state.GetString("react.incomplete_reason"); got != "" {
		t.Fatalf("expected react.incomplete_reason to be cleared, got %q", got)
	}
	if !strings.Contains(diag.String(), "Conclusion: analysis complete") {
		t.Fatalf("expected diagnostic to contain conclusion, got %q", diag.String())
	}
	// react.done must NOT be set — that's the caller's responsibility
	if _, ok := state.Get("react.done"); ok {
		t.Fatal("setSynthesizedConclusion must not set react.done")
	}
}

// --- repeated tool loop: summary available ---

func TestObserveNode_RepeatedToolLoop_WithSummary_Completes(t *testing.T) {
	node, _ := observeNode(t, editTask())
	state := core.NewContext()
	// 3 identical write observations → repeated loop
	obs := writeObs(true)
	addObservations(state, obs, obs, obs)
	state.Set("react.last_tool_result", map[string]interface{}{"success": true})
	state.Set("react.decision", decisionPayload{Complete: false})

	result, err := node.Execute(context.Background(), state)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	requireCompleted(t, result, state)
}

// --- repeated tool loop: no summary, falls back to incomplete_reason ---

func TestObserveNode_RepeatedToolLoop_NoSummary_SetsIncompleteReason(t *testing.T) {
	// editing task + pending tool calls that never wrote anything → no summary available
	node, _ := observeNode(t, editTask())
	state := core.NewContext()
	// 3 identical non-write observations that would trigger repeated detection
	// but don't satisfy any completion check
	badObs := ToolObservation{
		Tool:    "file_read",
		Phase:   "explore",
		Success: false,
		Summary: "error reading",
		Args:    map[string]interface{}{"path": "/src/x.go"},
		Data:    map[string]interface{}{"error": "failed"},
	}
	addObservations(state, badObs, badObs, badObs)
	state.Set("react.last_tool_result", map[string]interface{}{"error": "failed"})
	state.Set("react.decision", decisionPayload{Complete: false})

	result, err := node.Execute(context.Background(), state)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("expected successful result even on incomplete, got %+v", result)
	}
	done, _ := state.Get("react.done")
	if done != true {
		t.Fatalf("expected react.done=true after repeated loop, got %v", done)
	}
	reason := state.GetString("react.incomplete_reason")
	if !strings.Contains(reason, "stuck") && !strings.Contains(reason, "repeating") {
		t.Fatalf("expected incomplete_reason to mention stall, got %q", reason)
	}
}

// --- iteration budget exhausted: summary available ---

func TestObserveNode_IterationExhausted_WithSummary_Completes(t *testing.T) {
	node, agent := observeNode(t, editTask())
	agent.maxIterations = 1
	state := core.NewContext()
	state.Set("react.iteration", 1) // will become 2 → exceeds max of 1
	addObservations(state, writeObs(true))
	state.Set("react.last_tool_result", map[string]interface{}{"success": true})
	state.Set("react.decision", decisionPayload{Complete: false})

	result, err := node.Execute(context.Background(), state)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	requireCompleted(t, result, state)
	if summary := state.GetString("react.synthetic_summary"); !strings.Contains(summary, "applied") {
		t.Fatalf("expected edit summary from exhaustion path, got %q", summary)
	}
}

// --- iteration budget exhausted: no summary, sets incomplete_reason ---

func TestObserveNode_IterationExhausted_NoSummary_SetsIncompleteReason(t *testing.T) {
	// editing task with no write observation → can't produce summary
	node, agent := observeNode(t, editTask())
	agent.maxIterations = 1
	state := core.NewContext()
	state.Set("react.iteration", 1)
	// No write observation, so no edit summary is possible
	addObservations(state, fileReadObs("/src/main.go", "package main"))
	state.Set("react.last_tool_result", map[string]interface{}{"success": true})
	state.Set("react.decision", decisionPayload{Complete: false})

	result, err := node.Execute(context.Background(), state)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	done, _ := state.Get("react.done")
	if done != true {
		t.Fatalf("expected react.done=true on exhaustion, got %v", done)
	}
	reason := state.GetString("react.incomplete_reason")
	if !strings.Contains(reason, "iteration budget exhausted") {
		t.Fatalf("expected exhaustion reason, got %q", reason)
	}
}

// --- iteration counter increments each call ---

func TestObserveNode_IterationCounter_Increments(t *testing.T) {
	node, _ := observeNode(t, editTask())
	state := core.NewContext()
	state.Set("react.iteration", 3)
	state.Set("react.last_tool_result", map[string]interface{}{})
	state.Set("react.decision", decisionPayload{Complete: true})

	_, err := node.Execute(context.Background(), state)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	iterVal, _ := state.Get("react.iteration")
	iter, _ := iterVal.(int)
	if iter != 4 {
		t.Fatalf("expected iteration counter=4, got %d", iter)
	}
}

// --- decision.Complete propagates when no other path fires ---

func TestObserveNode_DecisionComplete_Propagates(t *testing.T) {
	node, _ := observeNode(t, readOnlyTask())
	state := core.NewContext()
	state.Set("react.last_tool_result", map[string]interface{}{})
	state.Set("react.decision", decisionPayload{Complete: true})

	result, err := node.Execute(context.Background(), state)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("expected successful result, got %+v", result)
	}
	done, _ := state.Get("react.done")
	if done != true {
		t.Fatalf("expected react.done=true when decision.Complete=true, got %v", done)
	}
}

// --- pending tool calls override decision.Complete ---

func TestObserveNode_PendingToolCalls_OverrideDecisionComplete(t *testing.T) {
	node, _ := observeNode(t, readOnlyTask())
	state := core.NewContext()
	state.Set("react.last_tool_result", map[string]interface{}{})
	state.Set("react.decision", decisionPayload{Complete: true})
	// Pending tool calls should force another iteration
	state.Set("react.tool_calls", []core.ToolCall{{Name: "file_read", Args: map[string]interface{}{"path": "/src/a.go"}}})

	result, err := node.Execute(context.Background(), state)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	requireIncomplete(t, result, state)
}
