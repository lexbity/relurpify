package thoughtrecipe

import "testing"

func TestThoughtRecipeDocumentPreservesDeclarationOrderAndSpans(t *testing.T) {
	doc := &ThoughtRecipeDocument{
		SourcePath: "relurpify_cfg/euclo/code_review.euclo",
		Name:       "code_review",
		Header: ThoughtRecipeHeader{
			positioned:  positioned{Span: NewSpan("relurpify_cfg/euclo/code_review.euclo", 1, 1, 2, 1)},
			Name:        Identifier{positioned: positioned{Span: NewSpan("relurpify_cfg/euclo/code_review.euclo", 1, 8, 1, 19)}, Value: "code_review"},
			Description: &StringLiteral{positioned: positioned{Span: NewSpan("relurpify_cfg/euclo/code_review.euclo", 2, 1, 2, 16)}, Raw: `"Review."`, Value: "Review."},
		},
		Declarations: []Declaration{
			&TriggerDecl{positioned: positioned{Span: NewSpan("relurpify_cfg/euclo/code_review.euclo", 4, 1, 6, 1)}},
			&InputDecl{positioned: positioned{Span: NewSpan("relurpify_cfg/euclo/code_review.euclo", 8, 1, 8, 24)}},
			&AgentDecl{positioned: positioned{Span: NewSpan("relurpify_cfg/euclo/code_review.euclo", 10, 1, 10, 28)}},
		},
	}

	if doc.SourcePath != "relurpify_cfg/euclo/code_review.euclo" {
		t.Fatalf("SourcePath = %q, want %q", doc.SourcePath, "relurpify_cfg/euclo/code_review.euclo")
	}
	if doc.Name != "code_review" {
		t.Fatalf("Name = %q, want %q", doc.Name, "code_review")
	}
	if got := doc.Header.Name.Value; got != "code_review" {
		t.Fatalf("Header.Name = %q, want %q", got, "code_review")
	}
	if got := doc.Header.Description.Value; got != "Review." {
		t.Fatalf("Header.Description = %q, want %q", got, "Review.")
	}

	wantTypes := []string{"*thoughtrecipe.TriggerDecl", "*thoughtrecipe.InputDecl", "*thoughtrecipe.AgentDecl"}
	for i, decl := range doc.Declarations {
		if got := typeName(decl); got != wantTypes[i] {
			t.Fatalf("declaration %d type = %s, want %s", i, got, wantTypes[i])
		}
	}

	if got := doc.Header.GetSpan(); got.Start.Line != 1 || got.Start.Column != 1 || got.End.Line != 2 || got.End.Column != 1 {
		t.Fatalf("header span = %+v, want 1:1-2:1", got)
	}
	if got := doc.Declarations[0].GetSpan(); got.Start.Line != 4 || got.End.Line != 6 {
		t.Fatalf("trigger span = %+v, want 4:1-6:1", got)
	}
	if got := doc.Declarations[1].GetSpan(); got.Start.Line != 8 || got.End.Line != 8 {
		t.Fatalf("input span = %+v, want 8:1-8:24", got)
	}
}

func TestAstPreservesRawLiteralsAndNestedBlocks(t *testing.T) {
	doc := &ThoughtRecipeDocument{
		SourcePath: "relurpify_cfg/euclo/code_review.euclo",
		Name:       "code_review",
		Declarations: []Declaration{
			&RunDecl{
				positioned: positioned{Span: NewSpan("relurpify_cfg/euclo/code_review.euclo", 12, 1, 17, 1)},
				Agent:      Identifier{positioned: positioned{Span: NewSpan("relurpify_cfg/euclo/code_review.euclo", 12, 5, 12, 12)}, Value: "reviewer"},
				Items: []ExecutionItem{
					&FromClause{
						positioned: positioned{Span: NewSpan("relurpify_cfg/euclo/code_review.euclo", 13, 3, 13, 22)},
						Source:     &PathExpr{positioned: positioned{Span: NewSpan("relurpify_cfg/euclo/code_review.euclo", 13, 8, 13, 22)}, Raw: "input.workspace", Parts: []Identifier{{Value: "input"}, {Value: "workspace"}}},
					},
					&GoalClause{
						positioned: positioned{Span: NewSpan("relurpify_cfg/euclo/code_review.euclo", 14, 3, 14, 53)},
						Text:       StringLiteral{positioned: positioned{Span: NewSpan("relurpify_cfg/euclo/code_review.euclo", 14, 8, 14, 53)}, Raw: `"Review the code."`, Value: "Review the code."},
					},
					&CapabilityInvocation{
						positioned: positioned{Span: NewSpan("relurpify_cfg/euclo/code_review.euclo", 15, 3, 15, 30)},
						Namespace:  Identifier{Value: "relurpic"},
						Capability: Identifier{Value: "code_review"},
					},
					&CaptureBlock{
						positioned: positioned{Span: NewSpan("relurpify_cfg/euclo/code_review.euclo", 16, 3, 17, 1)},
						Bindings: []CaptureBinding{
							{
								positioned:  positioned{Span: NewSpan("relurpify_cfg/euclo/code_review.euclo", 17, 5, 17, 38)},
								Source:      Identifier{Value: "findings"},
								Annotation:  &NamedTypeExpr{positioned: positioned{Span: NewSpan("relurpify_cfg/euclo/code_review.euclo", 17, 15, 17, 28)}, Name: PathExpr{Raw: "ReviewFindings"}},
								Destination: PathExpr{Raw: "state.findings", Parts: []Identifier{{Value: "state"}, {Value: "findings"}}},
							},
						},
					},
				},
			},
			&AskDecl{
				positioned: positioned{Span: NewSpan("relurpify_cfg/euclo/code_review.euclo", 20, 1, 24, 1)},
				Items: []AskItem{
					&QuestionClause{
						positioned: positioned{Span: NewSpan("relurpify_cfg/euclo/code_review.euclo", 21, 3, 21, 50)},
						Text:       StringLiteral{Raw: `"Review, refactor, or explain?"`, Value: "Review, refactor, or explain?"},
					},
					&ChoicesListClause{
						positioned: positioned{Span: NewSpan("relurpify_cfg/euclo/code_review.euclo", 22, 3, 22, 40)},
						Raw:        `["review", "refactor", "explain"]`,
						Items:      []ValueExpr{StringLiteral{Raw: `"review"`, Value: "review"}, StringLiteral{Raw: `"refactor"`, Value: "refactor"}, StringLiteral{Raw: `"explain"`, Value: "explain"}},
					},
				},
			},
		},
	}

	run := doc.Declarations[0].(*RunDecl)
	if got := run.Items[0].(*FromClause).Source.(*PathExpr).Raw; got != "input.workspace" {
		t.Fatalf("from source raw = %q, want %q", got, "input.workspace")
	}
	if got := run.Items[1].(*GoalClause).Text.Raw; got != `"Review the code."` {
		t.Fatalf("goal raw = %q, want %q", got, `"Review the code."`)
	}
	if got := run.Items[2].(*CapabilityInvocation).Namespace.Value; got != "relurpic" {
		t.Fatalf("namespace = %q, want %q", got, "relurpic")
	}
	binding := run.Items[3].(*CaptureBlock).Bindings[0]
	if got := binding.Annotation.(*NamedTypeExpr).Name.Raw; got != "ReviewFindings" {
		t.Fatalf("annotation raw = %q, want %q", got, "ReviewFindings")
	}
	if got := binding.Destination.Raw; got != "state.findings" {
		t.Fatalf("destination raw = %q, want %q", got, "state.findings")
	}
	if binding.Forwarding {
		t.Fatal("binding marked as forwarding unexpectedly")
	}

	ask := doc.Declarations[1].(*AskDecl)
	if got := ask.Items[0].(*QuestionClause).Text.Value; got != "Review, refactor, or explain?" {
		t.Fatalf("question value = %q, want %q", got, "Review, refactor, or explain?")
	}
	if got := ask.Items[1].(*ChoicesListClause).Raw; got != `["review", "refactor", "explain"]` {
		t.Fatalf("choices raw = %q, want %q", got, `["review", "refactor", "explain"]`)
	}
}

func typeName(v any) string {
	switch v.(type) {
	case *TriggerDecl:
		return "*thoughtrecipe.TriggerDecl"
	case *InputDecl:
		return "*thoughtrecipe.InputDecl"
	case *TypeDecl:
		return "*thoughtrecipe.TypeDecl"
	case *AgentDecl:
		return "*thoughtrecipe.AgentDecl"
	case *RunDecl:
		return "*thoughtrecipe.RunDecl"
	case *RouteDecl:
		return "*thoughtrecipe.RouteDecl"
	case *DelegateDecl:
		return "*thoughtrecipe.DelegateDecl"
	case *AskDecl:
		return "*thoughtrecipe.AskDecl"
	case *PipelineDecl:
		return "*thoughtrecipe.PipelineDecl"
	default:
		return "unknown"
	}
}
