package summarization

import (
	"context"
	"errors"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/context/knowledge"
)

func TestRouteEmptyChunksErrors(t *testing.T) {
	_, err := Route(context.Background(), nil, 100, nil, nil)
	if err == nil || err.Error() != "no chunks to summarize" {
		t.Fatalf("Route with no chunks: err = %v, want 'no chunks to summarize'", err)
	}
}

func TestRouteEnforcesGenerationCap(t *testing.T) {
	chunk := chunkWithBody("c1", "go", "package p")
	chunk.DerivationGeneration = DefaultDerivationGenerationCap // at cap -> rejected

	_, err := Route(context.Background(), []knowledge.KnowledgeChunk{chunk}, 100, nil, nil)
	if err == nil {
		t.Fatal("Route must reject chunks at the generation cap")
	}
	var capErr *GenerationCapError
	if !errors.As(err, &capErr) {
		t.Fatalf("err = %v, want *GenerationCapError", err)
	}
	if capErr.Generation != DefaultDerivationGenerationCap || capErr.Cap != DefaultDerivationGenerationCap {
		t.Errorf("capErr = %+v, want generation %d >= cap %d", capErr, DefaultDerivationGenerationCap, DefaultDerivationGenerationCap)
	}
}

func TestRouteAcceptsChunksBelowGenerationCap(t *testing.T) {
	chunk := chunkWithBody("c1", "go", "package p")
	chunk.DerivationGeneration = DefaultDerivationGenerationCap - 1

	res, err := Route(context.Background(), []knowledge.KnowledgeChunk{chunk}, 100, nil, nil)
	if err != nil {
		t.Fatalf("Route below cap: %v", err)
	}
	if res == nil || res.Summary == "" {
		t.Fatalf("Route below cap returned %+v, want a summary", res)
	}
}

func TestRouteSelectsFirstCapableSummarizer(t *testing.T) {
	chunks := []knowledge.KnowledgeChunk{chunkWithBody("c1", "go", "package p")}
	first := &fakeSummarizer{kind: "first", can: false}
	second := &fakeSummarizer{kind: "second", can: true}

	res, err := Route(context.Background(), chunks, 250, []Summarizer{first, second}, nil)
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if res.Summary != "fake summary" {
		t.Errorf("summary = %q, want scripted 'fake summary'", res.Summary)
	}
	if second.calls != 1 {
		t.Errorf("second summarizer calls = %d, want 1", second.calls)
	}
	if first.calls != 0 {
		t.Errorf("first summarizer must not be invoked when it cannot summarize, got %d calls", first.calls)
	}
	if len(second.canGot) != 1 || len(second.canGot[0]) != 1 {
		t.Errorf("CanSummarize received %v, want the chunk slice once", second.canGot)
	}
	if second.gotReq.Kind != "second" {
		t.Errorf("request kind = %q, want 'second'", second.gotReq.Kind)
	}
	if second.gotReq.TargetTokenBudget != 250 {
		t.Errorf("request budget = %d, want 250", second.gotReq.TargetTokenBudget)
	}
	if second.gotReq.SourceOrigin != knowledge.SourceOriginFile {
		t.Errorf("request source origin = %q, want %q", second.gotReq.SourceOrigin, knowledge.SourceOriginFile)
	}
	if len(second.gotReq.Chunks) != 1 || second.gotReq.Chunks[0].ID != "c1" {
		t.Errorf("request chunks = %+v, want the input chunks", second.gotReq.Chunks)
	}
}

func TestRouteBuildsDefaultSummarizersWhenNoneProvided(t *testing.T) {
	cases := []struct {
		name            string
		chunk           knowledge.KnowledgeChunk
		wantDerivation  string
	}{
		{"code routes to ast", chunkWithBody("c1", "go", "package p"), "ast_summarizer:"},
		{"markdown routes to heading", chunkWithBody("c1", "markdown", "# Title\nText."), "heading_summarizer:"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := Route(context.Background(), []knowledge.KnowledgeChunk{tc.chunk}, 100, nil, nil)
			if err != nil {
				t.Fatalf("Route: %v", err)
			}
			if len(res.DerivationMethod) < len(tc.wantDerivation) || res.DerivationMethod[:len(tc.wantDerivation)] != tc.wantDerivation {
				t.Errorf("DerivationMethod = %q, want prefix %q", res.DerivationMethod, tc.wantDerivation)
			}
			if res.UsedModel {
				t.Error("default summarizers must not use a model")
			}
		})
	}
}

func TestRouteErrorsWhenNoSummarizerCanHandleChunks(t *testing.T) {
	chunks := []knowledge.KnowledgeChunk{chunkWithBody("c1", "json", `{"k": 1}`)}
	unable := &fakeSummarizer{kind: "unable", can: false}

	_, err := Route(context.Background(), chunks, 100, []Summarizer{unable}, nil)
	if err == nil || err.Error() != "no suitable summarizer found for chunks" {
		t.Fatalf("err = %v, want 'no suitable summarizer found for chunks'", err)
	}
	if unable.calls != 0 {
		t.Errorf("Summarize must not run when nothing can handle the chunks, got %d calls", unable.calls)
	}
}

func TestRouteWrapsSummarizerError(t *testing.T) {
	chunks := []knowledge.KnowledgeChunk{chunkWithBody("c1", "go", "package p")}
	boom := &fakeSummarizer{kind: "boom", can: true, err: fakeErr()}

	_, err := Route(context.Background(), chunks, 100, []Summarizer{boom}, nil)
	if err == nil {
		t.Fatal("Route must surface summarizer failures")
	}
	if !errors.Is(err, fakeErr()) {
		t.Errorf("err = %v, want wrapped scripted failure", err)
	}
	if len(err.Error()) == 0 || !strings.Contains(err.Error(), "summarization failed:") {
		t.Errorf("err = %v, want 'summarization failed:' context", err)
	}
}

func TestRouteWithKindEmptyChunksErrors(t *testing.T) {
	_, err := RouteWithKind(context.Background(), nil, SummarizerKindAST, 100, nil, nil)
	if err == nil || err.Error() != "no chunks to summarize" {
		t.Fatalf("err = %v, want 'no chunks to summarize'", err)
	}
}

func TestRouteWithKindEnforcesGenerationCap(t *testing.T) {
	chunk := chunkWithBody("c1", "go", "package p")
	chunk.DerivationGeneration = DefaultDerivationGenerationCap + 1

	_, err := RouteWithKind(context.Background(), []knowledge.KnowledgeChunk{chunk}, SummarizerKindAST, 100, nil, nil)
	var capErr *GenerationCapError
	if !errors.As(err, &capErr) {
		t.Fatalf("err = %v, want *GenerationCapError", err)
	}
	if capErr.Generation != DefaultDerivationGenerationCap+1 {
		t.Errorf("capErr.Generation = %d, want %d", capErr.Generation, DefaultDerivationGenerationCap+1)
	}
}

func TestRouteWithKindSelectsRequestedKind(t *testing.T) {
	chunks := []knowledge.KnowledgeChunk{chunkWithBody("c1", "go", "package p")}
	astFake := &fakeSummarizer{kind: SummarizerKindAST, can: true}
	headingFake := &fakeSummarizer{kind: SummarizerKindHeading, can: true}

	res, err := RouteWithKind(context.Background(), chunks, SummarizerKindHeading, 10, []Summarizer{astFake, headingFake}, nil)
	if err != nil {
		t.Fatalf("RouteWithKind: %v", err)
	}
	if headingFake.calls != 1 {
		t.Errorf("heading summarizer calls = %d, want 1", headingFake.calls)
	}
	if astFake.calls != 0 {
		t.Errorf("ast summarizer must be skipped when a different kind is requested, got %d calls", astFake.calls)
	}
	if res.Summary != "fake summary" {
		t.Errorf("summary = %q, want scripted summary", res.Summary)
	}
}

func TestRouteWithKindCreatesMissingBuiltinSummarizer(t *testing.T) {
	cases := []struct {
		kind           SummarizerKind
		chunk          knowledge.KnowledgeChunk
		wantDerivation string
	}{
		{SummarizerKindAST, chunkWithBody("c1", "rust", "fn main() {}"), "ast_summarizer:"},
		{SummarizerKindHeading, chunkWithBody("c1", "text", "# Title\nBody."), "heading_summarizer:"},
	}
	for _, tc := range cases {
		t.Run(string(tc.kind), func(t *testing.T) {
			res, err := RouteWithKind(context.Background(), []knowledge.KnowledgeChunk{tc.chunk}, tc.kind, 100, nil, nil)
			if err != nil {
				t.Fatalf("RouteWithKind: %v", err)
			}
			if len(res.DerivationMethod) < len(tc.wantDerivation) || res.DerivationMethod[:len(tc.wantDerivation)] != tc.wantDerivation {
				t.Errorf("DerivationMethod = %q, want prefix %q", res.DerivationMethod, tc.wantDerivation)
			}
		})
	}
}

func TestRouteWithKindProseWithoutModelErrors(t *testing.T) {
	chunks := []knowledge.KnowledgeChunk{chunkWithBody("c1", "text", "hello")}
	_, err := RouteWithKind(context.Background(), chunks, SummarizerKindProse, 100, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "prose summarizer requires a language model") {
		t.Fatalf("err = %v, want prose-requires-model error", err)
	}
}

func TestRouteWithKindUnknownKindErrors(t *testing.T) {
	chunks := []knowledge.KnowledgeChunk{chunkWithBody("c1", "text", "hello")}
	_, err := RouteWithKind(context.Background(), chunks, SummarizerKind("bogus"), 100, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "unknown summarizer kind: bogus") {
		t.Fatalf("err = %v, want unknown-kind error", err)
	}
}

func TestRouteWithKindWrapsSummarizerError(t *testing.T) {
	chunks := []knowledge.KnowledgeChunk{chunkWithBody("c1", "go", "package p")}
	boom := &fakeSummarizer{kind: SummarizerKindAST, can: true, err: fakeErr()}

	_, err := RouteWithKind(context.Background(), chunks, SummarizerKindAST, 100, []Summarizer{boom}, nil)
	if !errors.Is(err, fakeErr()) {
		t.Fatalf("err = %v, want wrapped scripted failure", err)
	}
	if !strings.Contains(err.Error(), "summarization failed:") {
		t.Errorf("err = %v, want 'summarization failed:' context", err)
	}
}

func TestSelectSummarizerErrorsWithoutCandidates(t *testing.T) {
	_, err := SelectSummarizer([]knowledge.KnowledgeChunk{chunkWithBody("c1", "go", "p")}, nil)
	if err == nil || err.Error() != "no summarizers available" {
		t.Fatalf("err = %v, want 'no summarizers available'", err)
	}
}

func TestSelectSummarizerReturnsFirstCapable(t *testing.T) {
	chunks := []knowledge.KnowledgeChunk{chunkWithBody("c1", "markdown", "# H")}
	first := &fakeSummarizer{kind: "first", can: true}
	second := &fakeSummarizer{kind: "second", can: true}

	got, err := SelectSummarizer(chunks, []Summarizer{first, second})
	if err != nil {
		t.Fatalf("SelectSummarizer: %v", err)
	}
	if got != Summarizer(first) {
		t.Fatalf("selected %v, want the first capable summarizer", got.Kind())
	}
	if second.canGot != nil {
		t.Error("selection must stop at the first capable summarizer")
	}
}

func TestSelectSummarizerErrorsWhenNoneCapable(t *testing.T) {
	chunks := []knowledge.KnowledgeChunk{chunkWithBody("c1", "json", "{}")}
	unable := &fakeSummarizer{kind: "unable", can: false}

	_, err := SelectSummarizer(chunks, []Summarizer{unable})
	if err == nil || err.Error() != "no suitable summarizer found" {
		t.Fatalf("err = %v, want 'no suitable summarizer found'", err)
	}
}

func TestCreateDefaultSummarizersReturnsBuiltinSet(t *testing.T) {
	all := CreateDefaultSummarizers()
	if len(all) != 2 {
		t.Fatalf("len = %d, want 2", len(all))
	}
	if all[0].Kind() != SummarizerKindAST {
		t.Errorf("first kind = %q, want ast", all[0].Kind())
	}
	if all[1].Kind() != SummarizerKindHeading {
		t.Errorf("second kind = %q, want heading", all[1].Kind())
	}
	// The default set must be usable by Route for both content families.
	code := []knowledge.KnowledgeChunk{chunkWithBody("c1", "go", "package p")}
	res, err := Route(context.Background(), code, 100, all, nil)
	if err != nil {
		t.Fatalf("Route with defaults: %v", err)
	}
	if res.DerivationMethod[:14] != "ast_summarizer" {
		t.Errorf("code chunk routed to %q", res.DerivationMethod)
	}
}
