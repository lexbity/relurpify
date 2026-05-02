package prompt

import (
	"strings"
	"testing"
)

// ---- expression evaluator tests --------------------------------------------

func TestExpressionEvaluate(t *testing.T) {
	tests := []struct {
		expr  string
		state map[string]any
		want  bool
		err   bool
	}{
		// Truthiness.
		{"connected", map[string]any{"connected": true}, true, false},
		{"connected", map[string]any{"connected": false}, false, false},
		{"connected", map[string]any{}, false, false},
		{"name", map[string]any{"name": "alice"}, true, false},
		{"name", map[string]any{"name": ""}, false, false},

		// Exists.
		{"x exists", map[string]any{"x": false}, true, false},
		{"x exists", map[string]any{}, false, false},
		{"x exists", map[string]any{"x": 0}, true, false},

		// Equality.
		{`phase == "verify"`, map[string]any{"phase": "verify"}, true, false},
		{`phase == "verify"`, map[string]any{"phase": "plan"}, false, false},
		{`count != 0`, map[string]any{"count": 1}, true, false},
		{`count != 0`, map[string]any{"count": 0}, false, false},

		// Numeric ordering.
		{"score > 5", map[string]any{"score": 6}, true, false},
		{"score > 5", map[string]any{"score": 4}, false, false},
		{"score >= 5", map[string]any{"score": 5}, true, false},
		{"score < 5", map[string]any{"score": 3}, true, false},
		{"score <= 5", map[string]any{"score": 5}, true, false},

		// Logical.
		{"a && b", map[string]any{"a": true, "b": true}, true, false},
		{"a && b", map[string]any{"a": true, "b": false}, false, false},
		{"a || b", map[string]any{"a": false, "b": true}, true, false},
		{"a || b", map[string]any{"a": false, "b": false}, false, false},

		// Nested path.
		{"react.phase exists", map[string]any{"react": map[string]any{"phase": "verify"}}, true, false},
		{`react.phase == "verify"`, map[string]any{"react": map[string]any{"phase": "verify"}}, true, false},

		// Boolean literals.
		{"true", nil, true, false},
		{"false", nil, false, false},

		// Grouping.
		{"(a || b) && c", map[string]any{"a": true, "b": false, "c": true}, true, false},
		{"(a || b) && c", map[string]any{"a": false, "b": false, "c": true}, false, false},

		// Ordering operator on non-numeric → error → false.
		{`name > "alice"`, map[string]any{"name": "bob"}, false, true},
	}

	for _, tt := range tests {
		expr, err := compileExpression(tt.expr)
		if err != nil {
			if !tt.err {
				t.Errorf("compileExpression(%q): unexpected parse error: %v", tt.expr, err)
			}
			continue
		}
		got, evalErr := expr.Evaluate(tt.state)
		if evalErr != nil {
			if !tt.err {
				t.Errorf("Evaluate(%q): unexpected error: %v", tt.expr, evalErr)
			}
			continue
		}
		if tt.err {
			t.Errorf("Evaluate(%q): expected error but got none", tt.expr)
			continue
		}
		if got != tt.want {
			t.Errorf("Evaluate(%q) = %v, want %v (state=%v)", tt.expr, got, tt.want, tt.state)
		}
	}
}

func TestCompileExpressionErrors(t *testing.T) {
	bad := []string{
		"",
		"(",
		"a &&",
		"a ||",
		`"unterminated`,
	}
	for _, expr := range bad {
		_, err := compileExpression(expr)
		if err == nil {
			t.Errorf("compileExpression(%q): expected error, got nil", expr)
		}
	}
}

// ---- parser tests ----------------------------------------------------------

var samplePrompt = `---
apiVersion: framework.prompt/v1
id: test.sample
name: Sample Prompt
description: A test prompt
extends:
tags:
  paradigm: [react]
  agent: [framework]
  domain: [test]
  kind: task
  stability: stable
variables:
  lang:
    default: "Go"
  project:
    default: "(unknown)"
---

# Task
~ kind: task
~ order: 10

Write code in {lang} for {project}.

# Constraints
~ kind: constraint
~ order: 90
~ locked: true

Never delete files without confirmation.
`

func TestParseBytes_Valid(t *testing.T) {
	result, err := ParseBytes([]byte(samplePrompt), "test.prompt")
	if err != nil {
		t.Fatalf("ParseBytes: %v", err)
	}
	cfg := result.Config
	if cfg.ID != "test.sample" {
		t.Errorf("ID = %q, want %q", cfg.ID, "test.sample")
	}
	if cfg.Name != "Sample Prompt" {
		t.Errorf("Name = %q, want %q", cfg.Name, "Sample Prompt")
	}
	if len(cfg.Tags.Paradigm) != 1 || cfg.Tags.Paradigm[0] != "react" {
		t.Errorf("Tags.Paradigm = %v, want [react]", cfg.Tags.Paradigm)
	}
	if cfg.Tags.Stability != "stable" {
		t.Errorf("Tags.Stability = %q, want stable", cfg.Tags.Stability)
	}
	if cfg.Variables["lang"].Default != "Go" {
		t.Errorf("Variables[lang].Default = %q, want Go", cfg.Variables["lang"].Default)
	}
	if len(cfg.Blocks) != 2 {
		t.Fatalf("len(Blocks) = %d, want 2", len(cfg.Blocks))
	}
	taskBlock := cfg.Blocks[0]
	if taskBlock.ID != "task" {
		t.Errorf("Blocks[0].ID = %q, want task", taskBlock.ID)
	}
	if taskBlock.Order != 10 {
		t.Errorf("Blocks[0].Order = %d, want 10", taskBlock.Order)
	}
	if taskBlock.Kind != "task" {
		t.Errorf("Blocks[0].Kind = %q, want task", taskBlock.Kind)
	}
	if !strings.Contains(taskBlock.Content, "{lang}") {
		t.Errorf("Blocks[0].Content should contain {lang}")
	}
	constraintBlock := cfg.Blocks[1]
	if !constraintBlock.Locked {
		t.Errorf("Blocks[1].Locked = false, want true")
	}
	if constraintBlock.Order != 90 {
		t.Errorf("Blocks[1].Order = %d, want 90", constraintBlock.Order)
	}
}

func TestParseBytes_MissingOpenFence(t *testing.T) {
	_, err := ParseBytes([]byte("no front matter\n"), "t.prompt")
	if err == nil {
		t.Fatal("expected error for missing ---")
	}
}

func TestParseBytes_MissingCloseFence(t *testing.T) {
	_, err := ParseBytes([]byte("---\nid: foo\n"), "t.prompt")
	if err == nil {
		t.Fatal("expected error for missing closing ---")
	}
}

func TestParseBytes_DefaultStability(t *testing.T) {
	src := "---\napiVersion: framework.prompt/v1\nid: x\nname: X\n---\n"
	result, err := ParseBytes([]byte(src), "t.prompt")
	if err != nil {
		t.Fatal(err)
	}
	if result.Config.Tags.Stability != "stable" {
		t.Errorf("default stability = %q, want stable", result.Config.Tags.Stability)
	}
}

var providerBlockPrompt = `---
apiVersion: framework.prompt/v1
id: test.provider
name: Provider Test
requires_providers:
  - react.tools
---

# Tools
~ from: provider
~ provider: react.tools
`

func TestParseBytes_ProviderBlock(t *testing.T) {
	result, err := ParseBytes([]byte(providerBlockPrompt), "t.prompt")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Config.Blocks) != 1 {
		t.Fatalf("len(Blocks) = %d, want 1", len(result.Config.Blocks))
	}
	b := result.Config.Blocks[0]
	if b.From != SourceProvider {
		t.Errorf("From = %v, want SourceProvider", b.From)
	}
	if b.Provider != "react.tools" {
		t.Errorf("Provider = %q, want react.tools", b.Provider)
	}
}

// ---- registry tests --------------------------------------------------------

func TestRegistry_LoadEmptyDir(t *testing.T) {
	r := NewRegistry()
	tmpDir := t.TempDir()
	if err := r.LoadDir(tmpDir); err != nil {
		t.Fatalf("LoadDir(empty): %v", err)
	}
	if r.Count() != 0 {
		t.Errorf("Count = %d, want 0", r.Count())
	}
}

func TestRegistry_LoadNonexistentDir(t *testing.T) {
	r := NewRegistry()
	if err := r.LoadDir("/nonexistent/path/that/does/not/exist"); err != nil {
		t.Fatalf("LoadDir(nonexistent): should be nil, got %v", err)
	}
}

func TestRegistry_RegisterAndResolve(t *testing.T) {
	r := NewRegistry()

	// Load a prompt from bytes.
	result, err := ParseBytes([]byte(samplePrompt), "sample.prompt")
	if err != nil {
		t.Fatal(err)
	}
	dr := r.(*defaultRegistry)
	dr.indexOne(result.Config, result.Warnings)

	ctx := RuntimeContext{
		Variables: map[string]string{"lang": "Python"},
		State:     map[string]any{},
	}
	assembled, err := r.Resolve("test.sample", ctx)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !strings.Contains(assembled, "Python") {
		t.Errorf("resolved prompt should contain 'Python', got: %s", assembled)
	}
	if !strings.Contains(assembled, "(unknown)") {
		t.Errorf("resolved prompt should contain '(unknown)' for missing {project}")
	}
}

func TestRegistry_NotFound(t *testing.T) {
	r := NewRegistry()
	_, err := r.Resolve("nonexistent.id", RuntimeContext{})
	var nfe *NotFoundError
	if !isAs(err, &nfe) {
		t.Errorf("expected NotFoundError, got %T: %v", err, err)
	}
}

func TestRegistry_DuplicateID(t *testing.T) {
	r := NewRegistry()
	dr := r.(*defaultRegistry)
	result, _ := ParseBytes([]byte(samplePrompt), "a.prompt")
	dr.indexOne(result.Config, nil)
	dr.indexOne(result.Config, nil) // duplicate

	if r.Count() != 1 {
		t.Errorf("Count = %d, want 1 (duplicate rejected)", r.Count())
	}
}

func TestRegistry_RegisterProvider_Duplicate(t *testing.T) {
	r := NewRegistry()
	p := funcProvider(func(_ RuntimeContext) ContextChunk { return ContextChunk{Content: "x"} })
	if err := r.RegisterProvider("my.provider", p); err != nil {
		t.Fatalf("first RegisterProvider: %v", err)
	}
	err := r.RegisterProvider("my.provider", p)
	if err == nil {
		t.Fatal("second RegisterProvider: expected error")
	}
	if !IsAlreadyRegistered(err) {
		t.Errorf("expected IsAlreadyRegistered=true, got false for %v", err)
	}
}

func TestRegistry_ValidateProviders(t *testing.T) {
	r := NewRegistry()
	dr := r.(*defaultRegistry)
	result, _ := ParseBytes([]byte(providerBlockPrompt), "p.prompt")
	dr.indexOne(result.Config, nil)

	issues := r.ValidateProviders()
	if len(issues) == 0 {
		t.Fatal("expected validation issue for missing required provider")
	}

	// Register the provider, re-validate.
	p := funcProvider(func(_ RuntimeContext) ContextChunk { return ContextChunk{Content: "tools"} })
	if err := r.RegisterProvider("react.tools", p); err != nil {
		t.Fatal(err)
	}
	issues = r.ValidateProviders()
	for _, iss := range issues {
		if iss.Severity == SeverityError {
			t.Errorf("unexpected error after provider registered: %v", iss)
		}
	}
}

func TestRegistry_Filter(t *testing.T) {
	r := NewRegistry()
	dr := r.(*defaultRegistry)
	result, _ := ParseBytes([]byte(samplePrompt), "s.prompt")
	dr.indexOne(result.Config, nil)

	got := r.Filter(FilterOptions{Paradigm: "react"})
	if len(got) != 1 {
		t.Errorf("Filter(paradigm=react) = %d, want 1", len(got))
	}
	got = r.Filter(FilterOptions{Paradigm: "pipeline"})
	if len(got) != 0 {
		t.Errorf("Filter(paradigm=pipeline) = %d, want 0", len(got))
	}
}

// ---- resolver tests --------------------------------------------------------

func TestResolve_ProviderBlock(t *testing.T) {
	r := NewRegistry()
	dr := r.(*defaultRegistry)
	result, _ := ParseBytes([]byte(providerBlockPrompt), "p.prompt")
	dr.indexOne(result.Config, nil)

	p := funcProvider(func(_ RuntimeContext) ContextChunk {
		return ContextChunk{Content: "- tool1\n- tool2"}
	})
	if err := r.RegisterProvider("react.tools", p); err != nil {
		t.Fatal(err)
	}

	assembled, err := r.Resolve("test.provider", RuntimeContext{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(assembled, "tool1") {
		t.Errorf("expected tool1 in resolved output: %s", assembled)
	}
}

func TestResolve_WhenExpression(t *testing.T) {
	src := `---
apiVersion: framework.prompt/v1
id: test.when
name: When Test
---

# Always

Always included.

# Conditional
~ when: show_extra

Extra content.
`
	r := NewRegistry()
	dr := r.(*defaultRegistry)
	result, _ := ParseBytes([]byte(src), "w.prompt")
	dr.indexOne(result.Config, nil)

	// With show_extra=false.
	assembled, err := r.Resolve("test.when", RuntimeContext{
		State: map[string]any{"show_extra": false},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(assembled, "Extra") {
		t.Errorf("expected no 'Extra' block when show_extra=false, got: %s", assembled)
	}
	if !strings.Contains(assembled, "Always") {
		t.Errorf("expected 'Always' block, got: %s", assembled)
	}

	// With show_extra=true.
	assembled, err = r.Resolve("test.when", RuntimeContext{
		State: map[string]any{"show_extra": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(assembled, "Extra") {
		t.Errorf("expected 'Extra' block when show_extra=true, got: %s", assembled)
	}
}

func TestResolve_OrderSort(t *testing.T) {
	src := `---
apiVersion: framework.prompt/v1
id: test.order
name: Order Test
---

# Late
~ order: late

Late content.

# Early
~ order: early

Early content.

# Middle

Middle content.
`
	r := NewRegistry()
	dr := r.(*defaultRegistry)
	result, _ := ParseBytes([]byte(src), "o.prompt")
	dr.indexOne(result.Config, nil)

	assembled, err := r.Resolve("test.order", RuntimeContext{})
	if err != nil {
		t.Fatal(err)
	}
	earlyIdx := strings.Index(assembled, "Early")
	middleIdx := strings.Index(assembled, "Middle")
	lateIdx := strings.Index(assembled, "Late")
	if earlyIdx > middleIdx || middleIdx > lateIdx {
		t.Errorf("blocks not sorted: early=%d middle=%d late=%d in: %q",
			earlyIdx, middleIdx, lateIdx, assembled)
	}
}

func TestResolve_Interpolation(t *testing.T) {
	src := `---
apiVersion: framework.prompt/v1
id: test.interp
name: Interp Test
variables:
  lang:
    default: "Go"
---

# Task

Use {lang} to solve this.
`
	r := NewRegistry()
	dr := r.(*defaultRegistry)
	result, _ := ParseBytes([]byte(src), "i.prompt")
	dr.indexOne(result.Config, nil)

	// Default used.
	assembled, _ := r.Resolve("test.interp", RuntimeContext{})
	if !strings.Contains(assembled, "Go") {
		t.Errorf("expected 'Go' (default), got: %s", assembled)
	}

	// Runtime override.
	assembled, _ = r.Resolve("test.interp", RuntimeContext{
		Variables: map[string]string{"lang": "Rust"},
	})
	if !strings.Contains(assembled, "Rust") {
		t.Errorf("expected 'Rust' (runtime override), got: %s", assembled)
	}
}

// ---- inheritance tests -----------------------------------------------------

func TestResolve_Inheritance(t *testing.T) {
	parent := `---
apiVersion: framework.prompt/v1
id: test.parent
name: Parent
---

# Base

Base content.

# Override Me

Override this.
`
	child := `---
apiVersion: framework.prompt/v1
id: test.child
name: Child
extends: test.parent
---

# Override Me

Child content.

# New Block

New block from child.
`
	r := NewRegistry()
	dr := r.(*defaultRegistry)
	pr, _ := ParseBytes([]byte(parent), "parent.prompt")
	cr, _ := ParseBytes([]byte(child), "child.prompt")
	dr.indexOne(pr.Config, nil)
	dr.indexOne(cr.Config, nil)
	if err := dr.pass2(); err != nil {
		t.Fatal(err)
	}

	assembled, err := r.Resolve("test.child", RuntimeContext{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(assembled, "Base content") {
		t.Errorf("expected inherited 'Base content', got: %s", assembled)
	}
	if strings.Contains(assembled, "Override this") {
		t.Errorf("child should have overridden parent block, but parent content persists: %s", assembled)
	}
	if !strings.Contains(assembled, "Child content") {
		t.Errorf("expected child's override content, got: %s", assembled)
	}
	if !strings.Contains(assembled, "New block from child") {
		t.Errorf("expected child's new block, got: %s", assembled)
	}
}

func TestResolve_LockedBlockViolation(t *testing.T) {
	parent := `---
apiVersion: framework.prompt/v1
id: test.locked.parent
name: Locked Parent
---

# Immutable
~ locked: true

You cannot change this.
`
	child := `---
apiVersion: framework.prompt/v1
id: test.locked.child
name: Locked Child
extends: test.locked.parent
---

# Immutable

Trying to override.
`
	r := NewRegistry()
	dr := r.(*defaultRegistry)
	pr, _ := ParseBytes([]byte(parent), "parent.prompt")
	cr, _ := ParseBytes([]byte(child), "child.prompt")
	dr.indexOne(pr.Config, nil)
	dr.indexOne(cr.Config, nil)
	dr.pass2()

	assembled, err := r.Resolve("test.locked.child", RuntimeContext{})
	if err != nil {
		t.Fatal(err)
	}
	// Locked block keeps parent content.
	if !strings.Contains(assembled, "You cannot change this") {
		t.Errorf("locked parent block should be preserved, got: %s", assembled)
	}
}

// ---- DryRunResult ----------------------------------------------------------

func TestResolveDryRun(t *testing.T) {
	r := NewRegistry()
	dr := r.(*defaultRegistry)
	result, _ := ParseBytes([]byte(samplePrompt), "s.prompt")
	dr.indexOne(result.Config, nil)

	dry, err := r.ResolveDryRun("test.sample", RuntimeContext{
		Variables: map[string]string{"lang": "Rust"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(dry.Final, "Rust") {
		t.Errorf("dry-run Final should contain 'Rust', got: %s", dry.Final)
	}
	if dry.Variables["lang"] != "Rust" {
		t.Errorf("dry-run Variables[lang] = %q, want Rust", dry.Variables["lang"])
	}
}

// ---- interpolation unit tests ----------------------------------------------

func TestInterpolate(t *testing.T) {
	tests := []struct {
		input    string
		rtVars   map[string]string
		defaults map[string]string
		want     string
	}{
		{"hello {name}", map[string]string{"name": "world"}, nil, "hello world"},
		{"hello {name}", nil, map[string]string{"name": "default"}, "hello default"},
		{"hello {name}", nil, nil, "hello "},
		{"no vars here", nil, nil, "no vars here"},
		{`escaped \{brace\}`, nil, nil, "escaped {brace}"},
		{"nested {a.b}", nil, nil, "nested "},
	}
	for _, tt := range tests {
		got := interpolate(tt.input, tt.rtVars, tt.defaults)
		if got != tt.want {
			t.Errorf("interpolate(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ---- helpers ---------------------------------------------------------------

// funcProvider wraps a function as a ContextProvider.
type funcProvider func(RuntimeContext) ContextChunk

func (f funcProvider) Provide(ctx RuntimeContext) ContextChunk { return f(ctx) }

// isAs is a thin helper to avoid importing errors in the test file.
func isAs(err error, target **NotFoundError) bool {
	if err == nil {
		return false
	}
	nfe, ok := err.(*NotFoundError)
	if ok {
		*target = nfe
	}
	return ok
}
