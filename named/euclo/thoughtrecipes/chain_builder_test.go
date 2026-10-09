package thoughtrecipe

import (
	"strings"
	"testing"
)

// TestBuildChainerChainPinsLinkLoweringAndRequiredParts: each `link:` block
// becomes one Chain step with name, from-key inputs, prompt, and capture
// destination; a link missing its prompt or capture destination is an authoring
// error, never a silently skipped link.
func TestBuildChainerChainPinsLinkLoweringAndRequiredParts(t *testing.T) {
	directives := []TypedDirective{
		{
			Name:     "link",
			TextArgs: []string{"summarize"},
			Body: []TypedDirective{
				{Name: "from", TextArgs: []string{"state.input_a"}},
				{Name: "prompt", TextArgs: []string{`"Summarize A."`}},
				{Name: "capture", TextArgs: []string{"result -> state.out"}},
			},
		},
	}
	chain, err := buildChainerChain(directives)
	if err != nil {
		t.Fatalf("buildChainerChain: %v", err)
	}
	if len(chain.Links) != 1 {
		t.Fatalf("chain links = %d, want 1", len(chain.Links))
	}
	link := chain.Links[0]
	if link.Name != "summarize" {
		t.Fatalf("link name = %q, want summarize", link.Name)
	}
	if len(link.InputKeys) != 1 || link.InputKeys[0] != "state.input_a" {
		t.Fatalf("link input keys = %#v, want [state.input_a]", link.InputKeys)
	}
	if link.SystemPrompt != "Summarize A." {
		t.Fatalf("link system prompt = %q, want unquoted text", link.SystemPrompt)
	}
	if link.OutputKey != "state.out" {
		t.Fatalf("link output key = %q, want state.out", link.OutputKey)
	}
}

func TestBuildChainerChainRejectsMissingPrompt(t *testing.T) {
	directives := []TypedDirective{{
		Name:     "link",
		TextArgs: []string{"l1"},
		Body: []TypedDirective{
			{Name: "capture", TextArgs: []string{"result -> state.out"}},
		},
	}}
	_, err := buildChainerChain(directives)
	if err == nil || !strings.Contains(err.Error(), "requires a `prompt` clause") {
		t.Fatalf("expected missing-prompt error, got %v", err)
	}
}

func TestBuildChainerChainRejectsMissingCapture(t *testing.T) {
	directives := []TypedDirective{{
		Name:     "link",
		TextArgs: []string{"l1"},
		Body: []TypedDirective{
			{Name: "prompt", TextArgs: []string{`"p"`}},
		},
	}}
	_, err := buildChainerChain(directives)
	if err == nil || !strings.Contains(err.Error(), "requires a `capture` destination") {
		t.Fatalf("expected missing-capture error, got %v", err)
	}
}

func TestBuildChainerChainRejectsEmptyChain(t *testing.T) {
	_, err := buildChainerChain(nil)
	if err == nil || !strings.Contains(err.Error(), "at least one `link` block") {
		t.Fatalf("expected empty-chain error, got %v", err)
	}
}

func TestUntilIterationCap(t *testing.T) {
	cap, err := untilIterationCap([]TypedDirective{{Name: "until", TextArgs: []string{"2"}}})
	if err != nil || cap != 2 {
		t.Fatalf("untilIterationCap([until 2]) = %d, %v; want 2, nil", cap, err)
	}
	if cap, err := untilIterationCap(nil); err != nil || cap != 0 {
		t.Fatalf("untilIterationCap(nil) = %d, %v; want 0, nil", cap, err)
	}
	if _, err := untilIterationCap([]TypedDirective{{Name: "until", TextArgs: []string{"bogus"}}}); err == nil {
		t.Fatal("expected malformed until to error")
	}
	if _, err := untilIterationCap([]TypedDirective{{Name: "until", TextArgs: []string{"0"}}}); err == nil {
		t.Fatal("expected non-positive until to error")
	}
}

// TestChainerBuiltChainValidates ensures the built chain passes the chainer
// structural validator (output keys set, no self-references).
func TestChainerBuiltChainValidates(t *testing.T) {
	directives := []TypedDirective{
		{
			Name:     "link",
			TextArgs: []string{"a"},
			Body: []TypedDirective{
				{Name: "prompt", TextArgs: []string{`"A"`}},
				{Name: "capture", TextArgs: []string{"result -> state.out_a"}},
			},
		},
		{
			Name:     "link",
			TextArgs: []string{"b"},
			Body: []TypedDirective{
				{Name: "from", TextArgs: []string{"state.out_a"}},
				{Name: "prompt", TextArgs: []string{`"B"`}},
				{Name: "capture", TextArgs: []string{"result -> state.out_b"}},
			},
		},
	}
	chain, err := buildChainerChain(directives)
	if err != nil {
		t.Fatalf("buildChainerChain: %v", err)
	}
	if err := chain.Validate(); err != nil {
		t.Fatalf("chain.Validate: %v", err)
	}
	if len(chain.Links) != 2 {
		t.Fatalf("chain links = %d, want 2", len(chain.Links))
	}
}
