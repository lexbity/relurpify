package summarization

import (
	"context"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/context/knowledge"
)

func TestASTSummarizerKind(t *testing.T) {
	if got := NewASTSummarizer().Kind(); got != SummarizerKindAST {
		t.Errorf("Kind() = %q, want %q", got, SummarizerKindAST)
	}
}

func TestASTSummarizerCanSummarize(t *testing.T) {
	codeLanguages := []string{"go", "python", "rust", "javascript", "typescript", "java", "c", "cpp", "csharp"}
	for _, lang := range codeLanguages {
		chunks := []knowledge.KnowledgeChunk{chunkWithBody("c1", lang, "whatever")}
		if !NewASTSummarizer().CanSummarize(chunks) {
			t.Errorf("CanSummarize(%s) = false, want true", lang)
		}
	}

	cases := []struct {
		name   string
		chunks []knowledge.KnowledgeChunk
		want   bool
	}{
		{"empty chunk list", nil, false},
		{"markdown is not code", []knowledge.KnowledgeChunk{chunkWithBody("c1", "markdown", "# H")}, false},
		{"missing content type", []knowledge.KnowledgeChunk{{ID: "c1", Body: knowledge.ChunkBody{Raw: "func X() {}"}}}, false},
		{"non-string content type", []knowledge.KnowledgeChunk{{ID: "c1", Body: knowledge.ChunkBody{Fields: map[string]any{"content_type": 42}}}}, false},
		{"any code chunk in the set qualifies", []knowledge.KnowledgeChunk{
			chunkWithBody("c1", "markdown", "# H"),
			chunkWithBody("c2", "python", "def f(): pass"),
		}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := NewASTSummarizer().CanSummarize(tc.chunks); got != tc.want {
				t.Errorf("CanSummarize = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestASTSummarizerSummarizeEmptyChunksErrors(t *testing.T) {
	_, err := NewASTSummarizer().Summarize(context.Background(), SummarizationRequest{})
	if err == nil || err.Error() != "no chunks to summarize" {
		t.Fatalf("err = %v, want 'no chunks to summarize'", err)
	}
}

func TestASTSummarizerSummarizeResultMetadata(t *testing.T) {
	s := NewASTSummarizer()
	chunks := []knowledge.KnowledgeChunk{
		chunkWithBody("c1", "go", "package p\n\nfunc A() int {\n\treturn 1\n}\n"),
		chunkWithBody("c2", "cobol", "HELLO WORLD."),
	}
	res, err := s.Summarize(context.Background(), SummarizationRequest{Chunks: chunks})
	if err != nil {
		t.Fatalf("Summarize: %v", err)
	}

	wantHash := ComputeCoverageHash(chunks)
	if res.CoverageHash != wantHash {
		t.Errorf("CoverageHash = %q, want %q", res.CoverageHash, wantHash)
	}
	if want := "ast_summarizer:" + wantHash[:8]; res.DerivationMethod != want {
		t.Errorf("DerivationMethod = %q, want %q", res.DerivationMethod, want)
	}
	if res.TokenEstimate != CountTokens(res.Summary) {
		t.Errorf("TokenEstimate = %d, want %d", res.TokenEstimate, CountTokens(res.Summary))
	}
	if res.UsedModel {
		t.Error("AST summarizer must never use a model")
	}
	wantCoverage := []knowledge.ChunkID{"c1", "c2"}
	if len(res.SourceCoverage) != len(wantCoverage) {
		t.Fatalf("SourceCoverage = %v, want %v", res.SourceCoverage, wantCoverage)
	}
	for i, id := range wantCoverage {
		if res.SourceCoverage[i] != id {
			t.Errorf("SourceCoverage[%d] = %q, want %q", i, res.SourceCoverage[i], id)
		}
	}
	// Both chunks must contribute, joined by a blank line.
	if !strings.Contains(res.Summary, "\n\n") {
		t.Errorf("multi-chunk summary must join with a blank line, got %q", res.Summary)
	}
}

func TestASTSummarizerSummarizeGoElidesFunctionBodies(t *testing.T) {
	src := strings.Join([]string{
		"package pkg",
		"",
		"// Doc for A.",
		"func A(a int) int {",
		"\tif a > 0 {",
		"\t\treturn a",
		"\t}",
		"\treturn 0",
		"}",
		"",
		"type T struct {",
		"\tX int",
		"}",
		"",
		"var V = 1",
		"const C = 2",
	}, "\n")

	out := NewASTSummarizer().summarizeGo(src)

	for _, want := range []string{"package pkg", "// Doc for A.", "func A(a int) int {", "\t// ...", "type T struct {", "\tX int", "var V = 1", "const C = 2"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	for _, banned := range []string{"return a", "return 0", "if a > 0"} {
		if strings.Contains(out, banned) {
			t.Errorf("output must elide body line %q:\n%s", banned, out)
		}
	}
}

func TestASTSummarizerSummarizeGoKeepsOneLineFunctions(t *testing.T) {
	out := NewASTSummarizer().summarizeGo("func Add(a, b int) int { return a + b }")
	if out != "func Add(a, b int) int { return a + b }" {
		t.Errorf("one-line function = %q, want preserved verbatim", out)
	}
}

func TestASTSummarizerSummarizeGoJoinsMultiLineSignatures(t *testing.T) {
	src := strings.Join([]string{
		"func B(",
		"\tx int,",
		"\ty int,",
		") int {",
		"\treturn x + y",
		"}",
	}, "\n")

	out := NewASTSummarizer().summarizeGo(src)

	if !strings.Contains(out, "func B( x int, y int, ) int {") {
		t.Errorf("joined signature missing:\n%s", out)
	}
	if !strings.Contains(out, "\t// ...") {
		t.Errorf("elision marker missing:\n%s", out)
	}
	if strings.Contains(out, "return x + y") {
		t.Errorf("body must be elided:\n%s", out)
	}
}

func TestASTSummarizerSummarizeGoHandlesUnterminatedFunction(t *testing.T) {
	out := NewASTSummarizer().summarizeGo("func D() {")
	for _, want := range []string{"func D() {", "\t// ...", "}"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestASTSummarizerSummarizeGoDropsVarWithBrace(t *testing.T) {
	out := NewASTSummarizer().summarizeGo("var M = map[string]int{\n\t\"k\": 1,\n}")
	if strings.Contains(out, "map[string]int") {
		t.Errorf("composite var literal must be dropped, got:\n%s", out)
	}
}

func TestASTSummarizerSummarizePython(t *testing.T) {
	src := strings.Join([]string{
		"\"\"\"Module docstring.\"\"\"",
		"import os",
		"",
		"# Module comment.",
		"MAX = 10",
		"",
		"def add(a, b):",
		"    return a + b",
		"",
		"class Greeter:",
		"    def greet(self):",
		"        pass",
	}, "\n")

	out := NewASTSummarizer().summarizePython(src)

	for _, want := range []string{"\"\"\"Module docstring.\"\"\"", "import os", "# Module comment.", "MAX = 10", "def add(a, b):", "class Greeter:", "    # ..."} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	for _, banned := range []string{"return a + b", "def greet", "pass"} {
		if strings.Contains(out, banned) {
			t.Errorf("output must elide %q:\n%s", banned, out)
		}
	}
}

func TestASTSummarizerSummarizeRust(t *testing.T) {
	src := strings.Join([]string{
		"use std::fmt;",
		"mod helpers;",
		"",
		"#[derive(Debug)]",
		"struct Point {",
		"    x: i32,",
		"}",
		"",
		"impl Point {}",
		"",
		"fn show(p: &Point) -> i32 {",
		"    p.x + 1",
		"}",
		"",
		"fn one() -> i32 { 1 }",
		"",
		"fn partial() -> i32 {",
	}, "\n")

	out := NewASTSummarizer().summarizeRust(src)

	for _, want := range []string{"use std::fmt;", "mod helpers;", "#[derive(Debug)]", "struct Point {", "impl Point {}", "fn show(p: &Point) -> i32 {", "    // ...", "fn one() -> i32 { 1 }", "fn partial() -> i32 {"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	for _, banned := range []string{"p.x + 1", "x: i32"} {
		if strings.Contains(out, banned) {
			t.Errorf("output must elide %q:\n%s", banned, out)
		}
	}
}

func TestASTSummarizerSummarizeJS(t *testing.T) {
	src := strings.Join([]string{
		"import { x } from 'mod';",
		"// Header comment.",
		"function f(a) {",
		"  return a * 2;",
		"}",
		"const g = (x) => {",
		"  return x;",
		"};",
		"export const h = () => {",
		"  go();",
		"}",
		"class Widget {",
		"  render() {}",
		"}",
		"interface Shape { area(): number; }",
		"type Alias = string;",
		"export default Widget;",
	}, "\n")

	out := NewASTSummarizer().summarizeJS(src)

	for _, want := range []string{
		"import { x } from 'mod';",
		"// Header comment.",
		"function f(a) {",
		"  // ...",
		"const g = (x) => {",
		"export const h = () => {",
		"class Widget {",
		"interface Shape { area(): number; }",
		"type Alias = string;",
		"export default Widget;",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	for _, banned := range []string{"return a * 2", "return x", "go();", "render()"} {
		if strings.Contains(out, banned) {
			t.Errorf("output must elide %q:\n%s", banned, out)
		}
	}
}

func TestASTSummarizerSummarizeGeneric(t *testing.T) {
	src := strings.Join([]string{
		"#!/bin/sh",
		"# shell comment",
		"/* block intro",
		"* continued",
		"def process(data):",
		"class Handler:",
		"x = 1",
	}, "\n")

	out := NewASTSummarizer().summarizeGeneric(src)

	for _, want := range []string{"# shell comment", "/* block intro", "* continued", "def process(data):", "  // ...", "class Handler:"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "x = 1") {
		t.Errorf("unrecognized statements must be dropped, got:\n%s", out)
	}
}

func TestASTSummarizerSummarizeDispatchesByContentType(t *testing.T) {
	s := NewASTSummarizer()
	goOut := s.summarizeCode("func A() int {\n\treturn 1\n}\n", "go")
	if !strings.Contains(goOut, "func A() int {") {
		t.Errorf("go dispatch failed:\n%s", goOut)
	}
	pyOut := s.summarizeCode("def f():\n    pass\n", "python")
	if !strings.Contains(pyOut, "def f():") || strings.Contains(pyOut, "pass") {
		t.Errorf("python dispatch failed:\n%s", pyOut)
	}
	tsOut := s.summarizeCode("const x = () => {\n  work();\n};\n", "typescript")
	if strings.Contains(tsOut, "work();") {
		t.Errorf("typescript dispatch must elide bodies:\n%s", tsOut)
	}
	genericOut := s.summarizeCode("func generic(a): # keep\nunknown line\n", "lisp")
	if !strings.Contains(genericOut, "func generic(a):") {
		t.Errorf("unknown content type must fall back to generic:\n%s", genericOut)
	}
	if strings.Contains(genericOut, "unknown line") {
		t.Errorf("generic summarizer must drop unmatched lines:\n%s", genericOut)
	}
}
