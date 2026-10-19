package thoughtrecipe

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// buildFlat5Source returns a synthetic .erpe source with 5 sibling execution
// items in a single run block (depth 1). All items are DirectiveClause leaves.
func buildFlat5Source() string {
	var b strings.Builder
	b.WriteString("thoughtrecipe bench_flat5\n")
	b.WriteString("\"Benchmark flat-5 recipe.\"\n\n")
	b.WriteString("trigger as capability:\n")
	b.WriteString("  may read workspace\n\n")
	b.WriteString("agent default uses react\n\n")
	b.WriteString("run default:\n")
	for i := 1; i <= 5; i++ {
		fmt.Fprintf(&b, "  goal \"Step %d.\"\n", i)
	}
	return b.String()
}

// buildNested50Source returns a synthetic .erpe source with 50 leaf execution
// items nested inside a pipeline stage (depth 3: pipeline -> stage -> run ->
// leaves). All leaves are GoalClause.
func buildNested50Source() string {
	var b strings.Builder
	b.WriteString("thoughtrecipe bench_nested50\n")
	b.WriteString("\"Benchmark nested-50 recipe.\"\n\n")
	b.WriteString("trigger as capability:\n")
	b.WriteString("  may read workspace\n\n")
	b.WriteString("agent default uses react\n\n")
	b.WriteString("pipeline:\n")
	b.WriteString("  stage nested:\n")
	b.WriteString("    run default:\n")
	for i := 1; i <= 50; i++ {
		fmt.Fprintf(&b, "      goal \"Step %02d.\"\n", i)
	}
	return b.String()
}

// BenchmarkErpeCompileFlat5_Parse measures lexer + parser cost for the flat-5
// source. The source string is built once outside the timer; the timed loop
// calls ParseSource repeatedly.
func BenchmarkErpeCompileFlat5_Parse(b *testing.B) {
	src := buildFlat5Source()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ParseSource("flat5.euclo", src)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkErpeCompileFlat5_Typecheck measures type-validation cost for the
// flat-5 source. The document is parsed once outside the timer; the timed loop
// calls NewTypeSystem(doc).Validate() on the same AST.
func BenchmarkErpeCompileFlat5_Typecheck(b *testing.B) {
	src := buildFlat5Source()
	doc, err := ParseSource("flat5.euclo", src)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := NewTypeSystem(doc).Validate(); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkErpeCompileFlat5_Semantic measures name-resolution + contract-check
// cost for the flat-5 source. The document is parsed and typechecked once
// outside the timer; the timed loop calls NewSymbolTable(doc).Resolve() on the
// same AST.
func BenchmarkErpeCompileFlat5_Semantic(b *testing.B) {
	src := buildFlat5Source()
	doc, err := ParseSource("flat5.euclo", src)
	if err != nil {
		b.Fatal(err)
	}
	if err := NewTypeSystem(doc).Validate(); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := NewSymbolTable(doc).Resolve(); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkErpeCompileFlat5_Full measures lowering + plan-assembly cost for
// the flat-5 source. The document is parsed, typechecked, and semantically
// resolved once outside the timer; the timed loop calls LowerDocument(doc)
// (the nil-lookup convenience wrapper) on the same AST.
func BenchmarkErpeCompileFlat5_Full(b *testing.B) {
	src := buildFlat5Source()
	doc, err := ParseSource("flat5.euclo", src)
	if err != nil {
		b.Fatal(err)
	}
	if err := NewTypeSystem(doc).Validate(); err != nil {
		b.Fatal(err)
	}
	if err := NewSymbolTable(doc).Resolve(); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := LowerDocument(doc)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkErpeCompileNested50_Parse measures lexer + parser cost for the
// nested-50 source. The source string is built once outside the timer; the
// timed loop calls ParseSource repeatedly.
func BenchmarkErpeCompileNested50_Parse(b *testing.B) {
	src := buildNested50Source()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ParseSource("nested50.euclo", src)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkErpeCompileNested50_Typecheck measures type-validation cost for the
// nested-50 source. The document is parsed once outside the timer; the timed
// loop calls NewTypeSystem(doc).Validate() on the same AST.
func BenchmarkErpeCompileNested50_Typecheck(b *testing.B) {
	src := buildNested50Source()
	doc, err := ParseSource("nested50.euclo", src)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := NewTypeSystem(doc).Validate(); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkErpeCompileNested50_Semantic measures name-resolution + contract-check
// cost for the nested-50 source. The document is parsed and typechecked once
// outside the timer; the timed loop calls NewSymbolTable(doc).Resolve() on the
// same AST.
func BenchmarkErpeCompileNested50_Semantic(b *testing.B) {
	src := buildNested50Source()
	doc, err := ParseSource("nested50.euclo", src)
	if err != nil {
		b.Fatal(err)
	}
	if err := NewTypeSystem(doc).Validate(); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := NewSymbolTable(doc).Resolve(); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkErpeCompileNested50_Full measures lowering + plan-assembly cost for
// the nested-50 source. The document is parsed, typechecked, and semantically
// resolved once outside the timer; the timed loop calls LowerDocument(doc)
// (the nil-lookup convenience wrapper) on the same AST.
func BenchmarkErpeCompileNested50_Full(b *testing.B) {
	src := buildNested50Source()
	doc, err := ParseSource("nested50.euclo", src)
	if err != nil {
		b.Fatal(err)
	}
	if err := NewTypeSystem(doc).Validate(); err != nil {
		b.Fatal(err)
	}
	if err := NewSymbolTable(doc).Resolve(); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := LowerDocument(doc)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkErpeLoadWorkspace measures the end-to-end cost of loading a
// workspace with both synthetic .erpe sources. The temp directory and source
// files are created once outside the timer; the timed loop calls
// loader.LoadWorkspace repeatedly, which re-reads both files from disk,
// parses, typechecks, semantically resolves, lowers, and registers each recipe.
func BenchmarkErpeLoadWorkspace(b *testing.B) {
	root := b.TempDir()
	sourceRoot := filepath.Join(root, ThoughtRecipeSourceRoot)
	if err := os.MkdirAll(sourceRoot, 0o700); err != nil {
		b.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceRoot, "flat5.euclo"), []byte(buildFlat5Source()), 0o600); err != nil {
		b.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceRoot, "nested50.euclo"), []byte(buildNested50Source()), 0o600); err != nil {
		b.Fatal(err)
	}
	loader := NewLoader()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, err := loader.LoadWorkspace(root)
		if err != nil {
			b.Fatal(err)
		}
		if result == nil {
			b.Fatal("LoadWorkspace returned nil result")
		}
	}
}

// TestBuildSourcesParse verifies that both synthetic benchmark sources parse
// without error and produce the expected AST structure.
func TestBuildSourcesParse(t *testing.T) {
	t.Run("Flat5", func(t *testing.T) {
		doc, err := ParseSource("flat5.euclo", buildFlat5Source())
		if err != nil {
			t.Fatalf("ParseSource(flat5) failed: %v", err)
		}
		if doc.Name != "bench_flat5" {
			t.Fatalf("document name = %q, want %q", doc.Name, "bench_flat5")
		}
		if len(doc.Declarations) != 3 {
			t.Fatalf("declaration count = %d, want 3", len(doc.Declarations))
		}
		if _, ok := doc.Declarations[0].(*TriggerDecl); !ok {
			t.Fatalf("declaration 0 is %T, want *TriggerDecl", doc.Declarations[0])
		}
		if _, ok := doc.Declarations[1].(*AgentDecl); !ok {
			t.Fatalf("declaration 1 is %T, want *AgentDecl", doc.Declarations[1])
		}
		runDecl, ok := doc.Declarations[2].(*RunDecl)
		if !ok {
			t.Fatalf("declaration 2 is %T, want *RunDecl", doc.Declarations[2])
		}
		if len(runDecl.Items) != 5 {
			t.Fatalf("run items = %d, want 5", len(runDecl.Items))
		}
		for i, item := range runDecl.Items {
			if _, ok := item.(*GoalClause); !ok {
				t.Fatalf("run item %d is %T, want *GoalClause", i, item)
			}
		}
	})

	t.Run("Nested50", func(t *testing.T) {
		doc, err := ParseSource("nested50.euclo", buildNested50Source())
		if err != nil {
			t.Fatalf("ParseSource(nested50) failed: %v", err)
		}
		if doc.Name != "bench_nested50" {
			t.Fatalf("document name = %q, want %q", doc.Name, "bench_nested50")
		}
		if len(doc.Declarations) != 3 {
			t.Fatalf("declaration count = %d, want 3", len(doc.Declarations))
		}
		if _, ok := doc.Declarations[0].(*TriggerDecl); !ok {
			t.Fatalf("declaration 0 is %T, want *TriggerDecl", doc.Declarations[0])
		}
		if _, ok := doc.Declarations[1].(*AgentDecl); !ok {
			t.Fatalf("declaration 1 is %T, want *AgentDecl", doc.Declarations[1])
		}
		pipelineDecl, ok := doc.Declarations[2].(*PipelineDecl)
		if !ok {
			t.Fatalf("declaration 2 is %T, want *PipelineDecl", doc.Declarations[2])
		}
		if len(pipelineDecl.Stages) != 1 {
			t.Fatalf("pipeline stages = %d, want 1", len(pipelineDecl.Stages))
		}
		stage := pipelineDecl.Stages[0]
		if len(stage.Body) != 1 {
			t.Fatalf("stage body = %d, want 1", len(stage.Body))
		}
		runDecl, ok := stage.Body[0].(*RunDecl)
		if !ok {
			t.Fatalf("stage body 0 is %T, want *RunDecl", stage.Body[0])
		}
		if len(runDecl.Items) != 50 {
			t.Fatalf("run items = %d, want 50", len(runDecl.Items))
		}
		for i, item := range runDecl.Items {
			if _, ok := item.(*GoalClause); !ok {
				t.Fatalf("leaf %d is %T, want *GoalClause", i, item)
			}
		}
	})
}
