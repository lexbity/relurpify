package thoughtrecipes

import (
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
)

func TestRenderPromptNoVars(t *testing.T) {
	tmpl := "This is a plain prompt with no variables."
	ctx := TemplateContext{}

	result, warnings := RenderPrompt(tmpl, ctx)

	if result != tmpl {
		t.Errorf("expected unchanged prompt, got %q", result)
	}
	if len(warnings) != 0 {
		t.Errorf("expected no warnings, got %v", warnings)
	}
}

func TestRenderPromptTaskInstruction(t *testing.T) {
	tmpl := "Please analyze: ${task.instruction}"
	ctx := TemplateContext{
		Task: TemplateTaskView{
			Instruction: "fix the bug in main.go",
		},
	}

	result, warnings := RenderPrompt(tmpl, ctx)

	expected := "Please analyze: fix the bug in main.go"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
	if len(warnings) != 0 {
		t.Errorf("expected no warnings, got %v", warnings)
	}
}

func TestRenderPromptTaskType(t *testing.T) {
	tmpl := "Task type is: ${task.type}"
	ctx := TemplateContext{
		Task: TemplateTaskView{
			Type: "code_modification",
		},
	}

	result, warnings := RenderPrompt(tmpl, ctx)

	expected := "Task type is: code_modification"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
	if len(warnings) != 0 {
		t.Errorf("expected no warnings, got %v", warnings)
	}
}

func TestRenderPromptTaskWorkspace(t *testing.T) {
	tmpl := "Working in: ${task.workspace}"
	ctx := TemplateContext{
		Task: TemplateTaskView{
			Workspace: "/home/user/project",
		},
	}

	result, warnings := RenderPrompt(tmpl, ctx)

	expected := "Working in: /home/user/project"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
	if len(warnings) != 0 {
		t.Errorf("expected no warnings, got %v", warnings)
	}
}

func TestRenderPromptContextAlias(t *testing.T) {
	tmpl := "Previous findings: ${context.explore_findings}"
	ctx := TemplateContext{
		Context: map[string]string{
			"explore_findings": "Found 3 matching files",
		},
	}

	result, warnings := RenderPrompt(tmpl, ctx)

	expected := "Previous findings: Found 3 matching files"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
	if len(warnings) != 0 {
		t.Errorf("expected no warnings, got %v", warnings)
	}
}

func TestRenderPromptContextMissing(t *testing.T) {
	tmpl := "Missing value: '${context.unknown_key}'"
	ctx := TemplateContext{
		Context: map[string]string{},
	}

	result, warnings := RenderPrompt(tmpl, ctx)

	expected := "Missing value: ''"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
	if len(warnings) != 1 {
		t.Errorf("expected 1 warning, got %d: %v", len(warnings), warnings)
	}
	if len(warnings) > 0 && warnings[0] != "unresolved template variable: context.unknown_key" {
		t.Errorf("unexpected warning: %q", warnings[0])
	}
}

func TestRenderPromptEnrichmentAST(t *testing.T) {
	tmpl := "AST summary: ${enrichment.ast}"
	ctx := TemplateContext{
		Enrichment: TemplateEnrichmentView{
			AST: "Package main with 5 functions",
		},
	}

	result, warnings := RenderPrompt(tmpl, ctx)

	expected := "AST summary: Package main with 5 functions"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
	if len(warnings) != 0 {
		t.Errorf("expected no warnings, got %v", warnings)
	}
}

func TestRenderPromptEnrichmentDisabled(t *testing.T) {
	tmpl := "BKC: '${enrichment.bkc}'"
	ctx := TemplateContext{
		Enrichment: TemplateEnrichmentView{
			BKC: "", // disabled - empty string
		},
	}

	result, warnings := RenderPrompt(tmpl, ctx)

	expected := "BKC: ''"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
	// Empty enrichment is valid (renders as empty), not a warning
	if len(warnings) != 0 {
		t.Errorf("expected no warnings for empty enrichment, got %v", warnings)
	}
}

func TestRenderPromptEnrichmentArchaeology(t *testing.T) {
	tmpl := "Archaeology: ${enrichment.archaeology}"
	ctx := TemplateContext{
		Enrichment: TemplateEnrichmentView{
			Archaeology: "3 commits affecting this file",
		},
	}

	result, _ := RenderPrompt(tmpl, ctx)

	expected := "Archaeology: 3 commits affecting this file"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestRenderPromptMultipleVars(t *testing.T) {
	tmpl := "Task: ${task.instruction} (${task.type}) in ${task.workspace}"
	ctx := TemplateContext{
		Task: TemplateTaskView{
			Instruction: "refactor auth",
			Type:        "code_modification",
			Workspace:   "/code",
		},
	}

	result, warnings := RenderPrompt(tmpl, ctx)

	expected := "Task: refactor auth (code_modification) in /code"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
	if len(warnings) != 0 {
		t.Errorf("expected no warnings, got %v", warnings)
	}
}

func TestRenderPromptMalformed(t *testing.T) {
	// Malformed patterns should be passed through unchanged
	tests := []struct {
		name     string
		tmpl     string
		expected string
	}{
		{"unclosed", "Start ${task.instruction", "Start ${task.instruction"},
		{"no braces", "No $braces here", "No $braces here"},
		{"empty braces", "Empty ${} here", "Empty ${} here"},
		// Nested braces are edge case - outer ${task.} gets parsed with inner ${type} as content
		{"nested braces", "Nested ${task.${type}} here", "Nested ${task.} here"},
	}

	ctx := TemplateContext{
		Task: TemplateTaskView{
			Instruction: "test",
			Type:        "analysis",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, warnings := RenderPrompt(tt.tmpl, ctx)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
			// Some malformed patterns may generate warnings - that's acceptable
			_ = warnings // suppress unused warning
		})
	}
}

func TestRenderPromptEmpty(t *testing.T) {
	result, warnings := RenderPrompt("", TemplateContext{})

	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
	if warnings != nil {
		t.Errorf("expected nil warnings, got %v", warnings)
	}
}

func TestBuildTemplateContextNilTask(t *testing.T) {
	ctx := BuildTemplateContext(nil, nil, EnrichmentBundle{}, nil)

	// Should produce zero-value Task view without panic
	if ctx.Task.Instruction != "" || ctx.Task.Type != "" || ctx.Task.Workspace != "" {
		t.Error("expected zero-value Task view for nil task")
	}
}

func TestBuildTemplateContextNilState(t *testing.T) {
	task := &core.Task{
		Instruction: "test instruction",
		Type:        core.TaskTypeAnalysis,
	}

	ctx := BuildTemplateContext(task, nil, EnrichmentBundle{}, nil)

	// Should produce empty context map without panic
	if ctx.Task.Instruction != "test instruction" {
		t.Errorf("expected instruction %q, got %q", "test instruction", ctx.Task.Instruction)
	}
	if ctx.Context == nil {
		t.Error("expected non-nil Context map")
	}
	if len(ctx.Context) != 0 {
		t.Errorf("expected empty Context map, got %v", ctx.Context)
	}
}

func TestBuildTemplateContextWithCaptures(t *testing.T) {
	// Create a context with some captured state
	state := core.NewContext()
	state.Set("pipeline.explore", "Found 5 files")
	state.Set("pipeline.analyze", "Analysis complete")

	resolver := NewAliasResolver(nil)

	ctx := BuildTemplateContext(nil, state, EnrichmentBundle{}, resolver)

	// Check that standard aliases were resolved from state
	if ctx.Context["explore_findings"] != "Found 5 files" {
		t.Errorf("expected explore_findings = %q, got %q", "Found 5 files", ctx.Context["explore_findings"])
	}
	if ctx.Context["analysis_result"] != "Analysis complete" {
		t.Errorf("expected analysis_result = %q, got %q", "Analysis complete", ctx.Context["analysis_result"])
	}
}

func TestBuildTemplateContextWithWorkspace(t *testing.T) {
	task := &core.Task{
		Instruction: "test",
		Type:        core.TaskTypeAnalysis,
		Context: map[string]any{
			"workspace": "/my/workspace",
		},
	}

	ctx := BuildTemplateContext(task, nil, EnrichmentBundle{}, nil)

	if ctx.Task.Workspace != "/my/workspace" {
		t.Errorf("expected workspace %q, got %q", "/my/workspace", ctx.Task.Workspace)
	}
}

func TestBuildEnrichmentBundle(t *testing.T) {
	bundle := BuildEnrichmentBundle(
		"AST data",
		"Archaeology data",
		"BKC data",
	)

	if bundle.AST != "AST data" {
		t.Errorf("expected AST %q, got %q", "AST data", bundle.AST)
	}
	if bundle.Archaeology != "Archaeology data" {
		t.Errorf("expected Archaeology %q, got %q", "Archaeology data", bundle.Archaeology)
	}
	if bundle.BKC != "BKC data" {
		t.Errorf("expected BKC %q, got %q", "BKC data", bundle.BKC)
	}
}

func TestRenderPromptUnknownNamespace(t *testing.T) {
	tmpl := "Unknown: ${unknown.var}"
	ctx := TemplateContext{}

	result, warnings := RenderPrompt(tmpl, ctx)

	expected := "Unknown: "
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
	if len(warnings) != 1 {
		t.Errorf("expected 1 warning, got %d: %v", len(warnings), warnings)
	}
}

func TestRenderPromptInvalidTaskSubpath(t *testing.T) {
	tmpl := "Invalid: ${task.invalid}"
	ctx := TemplateContext{
		Task: TemplateTaskView{
			Instruction: "test",
		},
	}

	result, warnings := RenderPrompt(tmpl, ctx)

	expected := "Invalid: "
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
	if len(warnings) != 1 {
		t.Errorf("expected 1 warning, got %d: %v", len(warnings), warnings)
	}
}

func TestRenderPromptInvalidEnrichmentSubpath(t *testing.T) {
	tmpl := "Invalid: ${enrichment.invalid}"
	ctx := TemplateContext{
		Enrichment: TemplateEnrichmentView{
			AST: "data",
		},
	}

	result, warnings := RenderPrompt(tmpl, ctx)

	expected := "Invalid: "
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
	if len(warnings) != 1 {
		t.Errorf("expected 1 warning, got %d: %v", len(warnings), warnings)
	}
}

func TestRenderPromptMultipleUnresolved(t *testing.T) {
	tmpl := "${task.missing1} and ${context.missing2} and ${enrichment.missing3}"
	ctx := TemplateContext{}

	result, warnings := RenderPrompt(tmpl, ctx)

	expected := " and  and "
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
	if len(warnings) != 3 {
		t.Errorf("expected 3 warnings, got %d: %v", len(warnings), warnings)
	}
}

func TestBuildContextMapWithResolver(t *testing.T) {
	state := core.NewContext()
	state.Set("pipeline.explore", "exploration data")
	state.Set("euclo.root_cause", "root cause data")

	// Test with standard resolver
	resolver := NewAliasResolver(nil)
	ctx := buildContextMap(state, resolver)

	if ctx["explore_findings"] != "exploration data" {
		t.Errorf("expected explore_findings, got %q", ctx["explore_findings"])
	}
	if ctx["root_cause"] != "root cause data" {
		t.Errorf("expected root_cause, got %q", ctx["root_cause"])
	}
}

func TestBuildContextMapNilState(t *testing.T) {
	resolver := NewAliasResolver(nil)
	ctx := buildContextMap(nil, resolver)

	if ctx == nil {
		t.Error("expected non-nil map")
	}
	if len(ctx) != 0 {
		t.Errorf("expected empty map, got %v", ctx)
	}
}

func TestBuildContextMapNilResolver(t *testing.T) {
	state := core.NewContext()
	state.Set("pipeline.explore", "data")

	ctx := buildContextMap(state, nil)

	if ctx == nil {
		t.Error("expected non-nil map")
	}
	if len(ctx) != 0 {
		t.Errorf("expected empty map with nil resolver, got %v", ctx)
	}
}

func TestRenderPromptPreservesNonTemplate(t *testing.T) {
	tmpl := "Normal text with $dollar and {braces} and $100 prices"
	ctx := TemplateContext{
		Task: TemplateTaskView{
			Instruction: "test",
		},
	}

	result, warnings := RenderPrompt(tmpl, ctx)

	// Non-template patterns should be preserved
	expected := "Normal text with $dollar and {braces} and $100 prices"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
	if len(warnings) != 0 {
		t.Errorf("expected no warnings, got %v", warnings)
	}
}

func BenchmarkRenderPrompt(b *testing.B) {
	tmpl := "Please analyze: ${task.instruction}\nContext: ${context.explore_findings}\nAST: ${enrichment.ast}"
	ctx := TemplateContext{
		Task: TemplateTaskView{
			Instruction: "fix the bug in main.go that causes the crash when loading large files",
			Type:        "code_modification",
			Workspace:   "/home/user/project",
		},
		Context: map[string]string{
			"explore_findings": "Found 3 relevant files: main.go, loader.go, utils.go",
		},
		Enrichment: TemplateEnrichmentView{
			AST: "Package main contains 5 functions, 12 methods, and imports from 8 packages",
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, _ := RenderPrompt(tmpl, ctx)
		_ = result
	}
}
