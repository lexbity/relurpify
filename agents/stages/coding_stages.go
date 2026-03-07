package stages

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/pipeline"
)

// ExploreStage identifies the most relevant files and next tools for a task.
type ExploreStage struct {
	Task *core.Task
}

func (s *ExploreStage) Name() string { return "explore" }
func (s *ExploreStage) AllowedToolNames() []string {
	return []string{
		"file_list",
		"file_read",
		"file_search",
		"search_grep",
		"search_semantic",
		"query_ast",
		"lsp_get_definition",
		"lsp_get_references",
		"lsp_search_symbols",
		"lsp_document_symbols",
		"lsp_get_hover",
		"lsp_get_diagnostics",
	}
}
func (s *ExploreStage) Contract() pipeline.ContractDescriptor {
	return pipeline.ContractDescriptor{
		Name: "file-selection",
		Metadata: pipeline.ContractMetadata{
			InputKey:      "pipeline.input",
			OutputKey:     "pipeline.explore",
			SchemaVersion: "v1",
			AllowTools:    true,
		},
	}
}
func (s *ExploreStage) BuildPrompt(ctx *core.Context) (string, error) {
	return buildStagePrompt("explore", s.Task, ctx, "Exploration focus", map[string]any{
		"task_instruction": taskInstruction(s.Task),
	}, s.AllowedToolNames(), `{
  "relevant_files":[{"path":"...","reason":"..."}],
  "tool_suggestions":["..."],
  "summary":"..."
}`), nil
}
func (s *ExploreStage) Decode(resp *core.LLMResponse) (any, error) {
	var out FileSelection
	if err := json.Unmarshal([]byte(extractJSON(resp.Text)), &out); err != nil {
		return nil, err
	}
	return out, nil
}
func (s *ExploreStage) Validate(output any) error {
	selection, ok := output.(FileSelection)
	if !ok {
		return fmt.Errorf("expected FileSelection output")
	}
	if strings.TrimSpace(selection.Summary) == "" {
		return fmt.Errorf("summary required")
	}
	if len(selection.RelevantFiles) == 0 && len(selection.ToolSuggestions) == 0 {
		return fmt.Errorf("at least one relevant file or tool suggestion required")
	}
	return nil
}
func (s *ExploreStage) Apply(ctx *core.Context, output any) error {
	selection := output.(FileSelection)
	ctx.Set("pipeline.explore", selection)
	ctx.Set("pipeline.explore.files", filePaths(selection))
	return nil
}

// AnalyzeStage converts explored context into a structured issue list.
type AnalyzeStage struct {
	Task *core.Task
}

func (s *AnalyzeStage) Name() string { return "analyze" }
func (s *AnalyzeStage) AllowedToolNames() []string {
	return []string{
		"file_read",
		"file_search",
		"search_grep",
		"search_semantic",
		"query_ast",
		"lsp_get_diagnostics",
		"lsp_get_definition",
		"lsp_get_references",
	}
}
func (s *AnalyzeStage) Contract() pipeline.ContractDescriptor {
	return pipeline.ContractDescriptor{
		Name: "issue-list",
		Metadata: pipeline.ContractMetadata{
			InputKey:      "pipeline.explore",
			OutputKey:     "pipeline.analyze",
			SchemaVersion: "v1",
			AllowTools:    true,
			RetryPolicy: pipeline.RetryPolicy{
				MaxAttempts:            1,
				RetryOnValidationError: true,
			},
		},
	}
}
func (s *AnalyzeStage) BuildPrompt(ctx *core.Context) (string, error) {
	raw, _ := ctx.Get("pipeline.explore")
	return buildStagePrompt("analyze", s.Task, ctx, "Explore output", map[string]any{
		"explore_output": raw,
		"instructions":   "You MUST return a concrete issue summary for the failing task. If the bug is unclear, call file_read or diagnostics/search tools before returning JSON. Identify the real failing issue from the code and tests, not a hypothetical one.",
	}, s.AllowedToolNames(), `{
  "issues":[{"id":"...","severity":"low|medium|high","title":"...","description":"...","file":"...","line":0}],
  "summary":"..."
}`), nil
}
func (s *AnalyzeStage) Decode(resp *core.LLMResponse) (any, error) {
	var out IssueList
	if err := json.Unmarshal([]byte(extractJSON(resp.Text)), &out); err != nil {
		return nil, err
	}
	return out, nil
}
func (s *AnalyzeStage) Validate(output any) error {
	issues, ok := output.(IssueList)
	if !ok {
		return fmt.Errorf("expected IssueList output")
	}
	if strings.TrimSpace(issues.Summary) == "" {
		return fmt.Errorf("summary required")
	}
	return nil
}
func (s *AnalyzeStage) Apply(ctx *core.Context, output any) error {
	ctx.Set("pipeline.analyze", output)
	return nil
}

// PlanStage turns issues into an ordered fix plan.
type PlanStage struct {
	Task *core.Task
}

func (s *PlanStage) Name() string { return "plan" }
func (s *PlanStage) Contract() pipeline.ContractDescriptor {
	return pipeline.ContractDescriptor{
		Name: "fix-plan",
		Metadata: pipeline.ContractMetadata{
			InputKey:      "pipeline.analyze",
			OutputKey:     "pipeline.plan",
			SchemaVersion: "v1",
			AllowTools:    false,
		},
	}
}
func (s *PlanStage) BuildPrompt(ctx *core.Context) (string, error) {
	raw, _ := ctx.Get("pipeline.analyze")
	return buildStagePrompt("plan", s.Task, ctx, "Issue list", raw, nil, `{
  "strategy":"...",
  "steps":[{"id":"...","title":"...","description":"...","files":["..."]}],
  "risks":["..."]
}`), nil
}
func (s *PlanStage) Decode(resp *core.LLMResponse) (any, error) {
	var out FixPlan
	if err := json.Unmarshal([]byte(extractJSON(resp.Text)), &out); err != nil {
		return nil, err
	}
	return out, nil
}
func (s *PlanStage) Validate(output any) error {
	plan, ok := output.(FixPlan)
	if !ok {
		return fmt.Errorf("expected FixPlan output")
	}
	if strings.TrimSpace(plan.Strategy) == "" {
		return fmt.Errorf("strategy required")
	}
	if len(plan.Steps) == 0 {
		return fmt.Errorf("at least one plan step required")
	}
	return nil
}
func (s *PlanStage) Apply(ctx *core.Context, output any) error {
	ctx.Set("pipeline.plan", output)
	return nil
}

// CodeStage proposes concrete edits derived from the fix plan.
type CodeStage struct {
	Task *core.Task
}

func (s *CodeStage) Name() string { return "code" }
func (s *CodeStage) AllowedToolNames() []string {
	return []string{
		"file_read",
		"file_write",
		"file_create",
		"file_delete",
		"lsp_format",
	}
}
func (s *CodeStage) Contract() pipeline.ContractDescriptor {
	return pipeline.ContractDescriptor{
		Name: "edit-plan",
		Metadata: pipeline.ContractMetadata{
			InputKey:      "pipeline.plan",
			OutputKey:     "pipeline.code",
			SchemaVersion: "v1",
			AllowTools:    true,
		},
	}
}
func (s *CodeStage) BuildPrompt(ctx *core.Context) (string, error) {
	raw, _ := ctx.Get("pipeline.plan")
	return buildStagePrompt("code", s.Task, ctx, "Fix plan", map[string]any{
		"fix_plan":     raw,
		"instructions": "For every update action, content must be the complete final file contents, not a partial snippet. Use file_read first if you need the current file.",
	}, s.AllowedToolNames(), `{
  "edits":[{"path":"...","action":"create|update|delete","content":"...","summary":"..."}],
  "summary":"..."
}`), nil
}
func (s *CodeStage) Decode(resp *core.LLMResponse) (any, error) {
	var out EditPlan
	if err := json.Unmarshal([]byte(extractJSON(resp.Text)), &out); err != nil {
		return nil, err
	}
	return out, nil
}
func (s *CodeStage) Validate(output any) error {
	plan, ok := output.(EditPlan)
	if !ok {
		return fmt.Errorf("expected EditPlan output")
	}
	if len(plan.Edits) == 0 {
		return fmt.Errorf("at least one edit required")
	}
	if strings.TrimSpace(plan.Summary) == "" {
		return fmt.Errorf("summary required")
	}
	for _, edit := range plan.Edits {
		if strings.TrimSpace(edit.Path) == "" {
			return fmt.Errorf("edit path required")
		}
		if strings.TrimSpace(edit.Action) == "" {
			return fmt.Errorf("edit action required")
		}
	}
	return nil
}
func (s *CodeStage) Apply(ctx *core.Context, output any) error {
	plan := output.(EditPlan)
	root := workspaceRoot(s.Task)
	for _, edit := range plan.Edits {
		path := filepath.Clean(edit.Path)
		if root != "" && !filepath.IsAbs(path) {
			path = filepath.Join(root, path)
		}
		switch strings.TrimSpace(edit.Action) {
		case "create", "update":
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(path, []byte(edit.Content), 0o644); err != nil {
				return err
			}
		case "delete":
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return err
			}
		default:
			return fmt.Errorf("unsupported edit action %q", edit.Action)
		}
	}
	ctx.Set("pipeline.code", plan)
	return nil
}

// VerifyStage summarizes the verification outcome for the planned edits.
type VerifyStage struct {
	Task *core.Task
}

func (s *VerifyStage) Name() string { return "verify" }
func (s *VerifyStage) AllowedToolNames() []string {
	return []string{
		"file_read",
		"exec_run_tests",
		"exec_run_linter",
		"exec_run_build",
		"cli_go",
		"cli_cargo",
		"cli_python",
		"cli_node",
		"go_test",
		"go_build",
		"rust_cargo_test",
		"rust_cargo_check",
		"node_npm_test",
		"node_syntax_check",
		"python_pytest",
		"python_unittest",
		"python_compile_check",
	}
}

func (s *VerifyStage) RequiresToolExecution(task *core.Task, state *core.Context, tools []core.Tool) bool {
	if task == nil || len(tools) == 0 {
		return false
	}
	instruction := strings.ToLower(task.Instruction)
	for _, tool := range tools {
		if tool == nil {
			continue
		}
		if strings.Contains(instruction, strings.ToLower(tool.Name())) {
			return true
		}
	}
	return false
}

func (s *VerifyStage) Contract() pipeline.ContractDescriptor {
	return pipeline.ContractDescriptor{
		Name: "verification-report",
		Metadata: pipeline.ContractMetadata{
			InputKey:      "pipeline.code",
			OutputKey:     "pipeline.verify",
			SchemaVersion: "v1",
			AllowTools:    true,
		},
	}
}
func (s *VerifyStage) BuildPrompt(ctx *core.Context) (string, error) {
	raw, _ := ctx.Get("pipeline.code")
	if raw == nil {
		raw, _ = ctx.Get("pipeline.explore")
	}
	return buildStagePrompt("verify", s.Task, ctx, "Verification target", map[string]any{
		"target":       raw,
		"instructions": "If the task asks you to run a command or verify with a specific tool, you MUST call that verification tool before returning JSON.",
	}, s.AllowedToolNames(), `{
  "status":"pass|fail|needs_manual_verification",
  "summary":"...",
  "checks":[{"name":"...","command":"...","status":"pass|fail|skipped","details":"..."}],
  "remaining_issues":["..."]
}`), nil
}
func (s *VerifyStage) Decode(resp *core.LLMResponse) (any, error) {
	var out VerificationReport
	if err := json.Unmarshal([]byte(extractJSON(resp.Text)), &out); err != nil {
		return nil, err
	}
	return out, nil
}
func (s *VerifyStage) Validate(output any) error {
	report, ok := output.(VerificationReport)
	if !ok {
		return fmt.Errorf("expected VerificationReport output")
	}
	switch strings.TrimSpace(report.Status) {
	case "pass", "fail", "needs_manual_verification":
	default:
		return fmt.Errorf("invalid verification status")
	}
	if strings.TrimSpace(report.Summary) == "" {
		return fmt.Errorf("summary required")
	}
	return nil
}
func (s *VerifyStage) Apply(ctx *core.Context, output any) error {
	ctx.Set("pipeline.verify", output)
	return nil
}
