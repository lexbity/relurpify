package thoughtrecipe

import (
	"fmt"
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
// items nested inside two layers of directive blocks (depth 3: run -> outer ->
// middle -> leaves). All leaves are DirectiveClause.
func buildNested50Source() string {
	var b strings.Builder
	b.WriteString("thoughtrecipe bench_nested50\n")
	b.WriteString("\"Benchmark nested-50 recipe.\"\n\n")
	b.WriteString("trigger as capability:\n")
	b.WriteString("  may read workspace\n\n")
	b.WriteString("agent default uses react\n\n")
	b.WriteString("run default:\n")
	b.WriteString("  outer:\n")
	b.WriteString("    middle:\n")
	for i := 1; i <= 50; i++ {
		fmt.Fprintf(&b, "      goal \"Step %02d.\"\n", i)
	}
	return b.String()
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
		runDecl, ok := doc.Declarations[2].(*RunDecl)
		if !ok {
			t.Fatalf("declaration 2 is %T, want *RunDecl", doc.Declarations[2])
		}
		if len(runDecl.Items) != 1 {
			t.Fatalf("run items = %d, want 1", len(runDecl.Items))
		}
		outer, ok := runDecl.Items[0].(*DirectiveBlock)
		if !ok {
			t.Fatalf("run item 0 is %T, want *DirectiveBlock", runDecl.Items[0])
		}
		if len(outer.Body) != 1 {
			t.Fatalf("outer body = %d, want 1", len(outer.Body))
		}
		middle, ok := outer.Body[0].(*DirectiveBlock)
		if !ok {
			t.Fatalf("outer body 0 is %T, want *DirectiveBlock", outer.Body[0])
		}
		if len(middle.Body) != 50 {
			t.Fatalf("middle body = %d, want 50", len(middle.Body))
		}
		for i, item := range middle.Body {
			if _, ok := item.(*GoalClause); !ok {
				t.Fatalf("leaf %d is %T, want *GoalClause", i, item)
			}
		}
	})
}
