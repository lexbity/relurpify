package stages

import (
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/pipeline"
	"codeburg.org/lexbit/relurpify/framework/retrieval"
)

func TestExploreStageDecodeValidateApply(t *testing.T) {
	stage := &ExploreStage{Task: &core.Task{Instruction: "find files"}}
	resp := &core.LLMResponse{Text: `{"relevant_files":[{"path":"main.rs","reason":"contains bug"}],"tool_suggestions":["file_read"],"summary":"Focus on main.rs"}`}

	out, err := pipeline.DecodeStageOutput(stage, resp)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if err := pipeline.ValidateStageOutput(stage, out); err != nil {
		t.Fatalf("validate failed: %v", err)
	}
	ctx := core.NewContext()
	if err := pipeline.ApplyStageOutput(stage, ctx, out); err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	if got := ctx.GetString("pipeline.explore.files"); got == "" {
		t.Fatalf("expected derived file list")
	}
}

func TestExploreStageDecodeAcceptsStructuredToolSuggestions(t *testing.T) {
	stage := &ExploreStage{Task: &core.Task{Instruction: "find files"}}
	resp := &core.LLMResponse{Text: `{"relevant_files":[{"path":"main.rs","reason":"contains bug"}],"tool_suggestions":[{"name":"file_read","reason":"inspect file"},{"tool":"search_grep"}],"summary":"Focus on main.rs"}`}

	out, err := pipeline.DecodeStageOutput(stage, resp)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	selection, ok := out.(FileSelection)
	if !ok {
		t.Fatalf("expected FileSelection, got %T", out)
	}
	if len(selection.ToolSuggestions) != 2 || selection.ToolSuggestions[0] != "file_read" || selection.ToolSuggestions[1] != "search_grep" {
		t.Fatalf("unexpected tool suggestions: %#v", selection.ToolSuggestions)
	}
}

func TestAnalyzeStageAcceptsSummaryWithoutIssues(t *testing.T) {
	stage := &AnalyzeStage{Task: &core.Task{Instruction: "analyze"}}
	resp := &core.LLMResponse{Text: `{"issues":[],"summary":"nothing"}`}

	out, err := pipeline.DecodeStageOutput(stage, resp)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if err := pipeline.ValidateStageOutput(stage, out); err != nil {
		t.Fatalf("expected validation success, got %v", err)
	}
}

func TestAnalyzeStageRejectsMalformedJSON(t *testing.T) {
	stage := &AnalyzeStage{Task: &core.Task{Instruction: "analyze"}}
	resp := &core.LLMResponse{Text: `{"issues":[`}

	_, err := pipeline.DecodeStageOutput(stage, resp)
	if err == nil {
		t.Fatalf("expected decode failure")
	}
}

func TestPlanStageDecodeValidateApply(t *testing.T) {
	stage := &PlanStage{Task: &core.Task{Instruction: "plan"}}
	resp := &core.LLMResponse{Text: `{"strategy":"minimal patch","steps":[{"id":"s1","title":"Fix add","description":"Update implementation","files":["lib.rs"]}],"risks":["regression"]}`}

	out, err := pipeline.DecodeStageOutput(stage, resp)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if err := pipeline.ValidateStageOutput(stage, out); err != nil {
		t.Fatalf("validate failed: %v", err)
	}
	ctx := core.NewContext()
	if err := pipeline.ApplyStageOutput(stage, ctx, out); err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	if ctx.GetString("pipeline.plan") == "" {
		t.Fatalf("expected plan stored")
	}
}

func TestCodeStageRejectsEditsWithoutPath(t *testing.T) {
	stage := &CodeStage{Task: &core.Task{Instruction: "code"}}
	resp := &core.LLMResponse{Text: `{"edits":[{"path":"","action":"update","content":"x","summary":"bad"}],"summary":"edit"}`}

	out, err := pipeline.DecodeStageOutput(stage, resp)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if err := pipeline.ValidateStageOutput(stage, out); err == nil {
		t.Fatalf("expected validation failure")
	}
}

func TestVerifyStageDecodeValidateApply(t *testing.T) {
	stage := &VerifyStage{Task: &core.Task{Instruction: "verify"}}
	resp := &core.LLMResponse{Text: `{"status":"needs_manual_verification","summary":"Tests still need to run","checks":[{"name":"cargo test","status":"skipped","details":"not executed"}],"remaining_issues":["run tests"]}`}

	out, err := pipeline.DecodeStageOutput(stage, resp)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if err := pipeline.ValidateStageOutput(stage, out); err != nil {
		t.Fatalf("validate failed: %v", err)
	}
	ctx := core.NewContext()
	if err := pipeline.ApplyStageOutput(stage, ctx, out); err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	if ctx.GetString("pipeline.verify") == "" {
		t.Fatalf("expected verification report stored")
	}
}

func TestCodeStageApplyStoresEditIntentWithoutMutatingWorkspace(t *testing.T) {
	stage := &CodeStage{Task: &core.Task{Instruction: "apply edits"}}
	ctx := core.NewContext()
	out := EditPlan{
		Edits: []FileEdit{{
			Path:    "src/lib.rs",
			Action:  "create",
			Content: "pub fn add(a: i32, b: i32) -> i32 { a + b }\n",
			Summary: "create file",
		}},
		Summary: "one edit",
	}

	if err := pipeline.ApplyStageOutput(stage, ctx, out); err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	if got := ctx.GetString("pipeline.code"); got == "" {
		t.Fatalf("expected edit intent stored")
	}
	raw, ok := ctx.Get("pipeline.code.intent_only")
	if !ok || raw != true {
		t.Fatalf("expected intent-only marker, got %#v", raw)
	}
}

func TestCodeStageRejectsUnknownEditAction(t *testing.T) {
	stage := &CodeStage{Task: &core.Task{Instruction: "code"}}
	resp := &core.LLMResponse{Text: `{"edits":[{"path":"src/lib.rs","action":"list_files","content":"x","summary":"bad"}],"summary":"edit"}`}

	out, err := pipeline.DecodeStageOutput(stage, resp)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if err := pipeline.ValidateStageOutput(stage, out); err == nil {
		t.Fatalf("expected validation failure")
	}
}

func TestCodingStageFactoryReturnsFiveStagesForCodeModification(t *testing.T) {
	factory := CodingStageFactory{}
	stages, err := factory.StagesForTask(&core.Task{Instruction: "fix code", Type: core.TaskTypeCodeModification})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stages) != 5 {
		t.Fatalf("expected 5 stages, got %d", len(stages))
	}
}

func TestCodingStageFactoryReturnsTwoStagesForAnalysis(t *testing.T) {
	factory := CodingStageFactory{}
	stages, err := factory.StagesForTask(&core.Task{Instruction: "verify code", Type: core.TaskTypeAnalysis})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stages) != 2 {
		t.Fatalf("expected 2 stages, got %d", len(stages))
	}
}

func TestExploreStageBuildPromptIncludesContextAndTools(t *testing.T) {
	stage := &ExploreStage{Task: &core.Task{
		Instruction: "find the failing Rust module",
		Context: map[string]any{
			"context_file_contents": []core.ContextFileContent{
				{Path: "src/lib.rs", Content: "pub fn add(a: i32, b: i32) -> i32 { a - b }\n"},
			},
		},
	}}

	prompt, err := stage.BuildPrompt(core.NewContext())
	if err != nil {
		t.Fatalf("build prompt failed: %v", err)
	}
	if !strings.Contains(prompt, "Context files:") || !strings.Contains(prompt, "src/lib.rs") {
		t.Fatalf("expected prompt to include contextual files, got %q", prompt)
	}
	if !strings.Contains(prompt, "file_read") || !strings.Contains(prompt, "query_ast") {
		t.Fatalf("expected prompt to include explicit tool names, got %q", prompt)
	}
}

func TestExploreStageBuildPromptRendersReferenceOnlyContextFiles(t *testing.T) {
	stage := &ExploreStage{Task: &core.Task{
		Instruction: "inspect referenced files",
		Context: map[string]any{
			"context_file_contents": []core.ContextFileContent{
				{
					Path:    "src/lib.rs",
					Summary: "Rust library entrypoint",
					Reference: &core.ContextReference{
						Kind:   core.ContextReferenceFile,
						ID:     "src/lib.rs",
						URI:    "src/lib.rs",
						Detail: "summary",
					},
				},
			},
		},
	}}

	prompt, err := stage.BuildPrompt(core.NewContext())
	if err != nil {
		t.Fatalf("build prompt failed: %v", err)
	}
	if !strings.Contains(prompt, "src/lib.rs [detail=summary]") {
		t.Fatalf("expected reference detail in prompt, got %q", prompt)
	}
	if !strings.Contains(prompt, "Rust library entrypoint") {
		t.Fatalf("expected summary-backed context file rendering, got %q", prompt)
	}
}

func TestWorkflowRetrievalContextFormatsEvidenceAndCitations(t *testing.T) {
	state := core.NewContext()
	state.Set("pipeline.workflow_retrieval", map[string]any{
		"query":      "find workflow evidence",
		"scope":      "workflow:wf-1",
		"cache_tier": "l2_hot",
		"results": []map[string]any{
			{
				"text": "retrieved workflow evidence that matters",
				"citations": []retrieval.PackedCitation{{
					ChunkID:      "chunk:1",
					CanonicalURI: "memory://workflow/1",
				}},
			},
		},
	})

	rendered := workflowRetrievalContext(state)
	if !strings.Contains(rendered, "Query: find workflow evidence") {
		t.Fatalf("expected query in workflow retrieval context, got %q", rendered)
	}
	if !strings.Contains(rendered, "Evidence:") {
		t.Fatalf("expected evidence section, got %q", rendered)
	}
	if !strings.Contains(rendered, "Sources: memory://workflow/1") {
		t.Fatalf("expected citation source, got %q", rendered)
	}
}

func TestWorkflowRetrievalContextFormatsReferenceOnlyEvidence(t *testing.T) {
	state := core.NewContext()
	state.Set("pipeline.workflow_retrieval", map[string]any{
		"query": "find workflow evidence",
		"results": []map[string]any{
			{
				"summary": "remembered workflow fact",
				"reference": map[string]any{
					"kind":   string(core.ContextReferenceRetrievalEvidence),
					"uri":    "memory://workflow/2",
					"detail": "packed",
				},
			},
		},
	})

	rendered := workflowRetrievalContext(state)
	if !strings.Contains(rendered, "remembered workflow fact") {
		t.Fatalf("expected summary-backed retrieval evidence, got %q", rendered)
	}
	if !strings.Contains(rendered, "Reference: memory://workflow/2") {
		t.Fatalf("expected retrieval reference, got %q", rendered)
	}
}

func TestPlanStageBuildPromptExcludesToolSectionWhenNoToolsAvailable(t *testing.T) {
	stage := &PlanStage{Task: &core.Task{Instruction: "plan a fix"}}
	ctx := core.NewContext()
	ctx.Set("pipeline.analyze", IssueList{
		Issues:  []Issue{{ID: "i1", Severity: "high", Title: "bug", Description: "desc"}},
		Summary: "one issue",
	})

	prompt, err := stage.BuildPrompt(ctx)
	if err != nil {
		t.Fatalf("build prompt failed: %v", err)
	}
	if !strings.Contains(prompt, "Available tools for this stage: none") {
		t.Fatalf("expected no-tool marker in plan prompt, got %q", prompt)
	}
	if strings.Contains(prompt, "file_write") {
		t.Fatalf("did not expect write tools in plan prompt, got %q", prompt)
	}
}
