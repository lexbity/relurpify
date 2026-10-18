package summarization

import (
	"context"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/context/knowledge"
)

func TestHeadingSummarizerKind(t *testing.T) {
	if got := NewHeadingSummarizer().Kind(); got != SummarizerKindHeading {
		t.Errorf("Kind() = %q, want %q", got, SummarizerKindHeading)
	}
}

func TestNewHeadingSummarizerDefaultsSentencesPerSection(t *testing.T) {
	if s := NewHeadingSummarizer(); s.SentencesPerSection != 2 {
		t.Errorf("SentencesPerSection = %d, want 2", s.SentencesPerSection)
	}
}

func TestHeadingSummarizerCanSummarize(t *testing.T) {
	structured := []string{"markdown", "text", "html", "rst", "asciidoc"}
	for _, ct := range structured {
		chunks := []knowledge.KnowledgeChunk{chunkWithBody("c1", ct, "anything")}
		if !NewHeadingSummarizer().CanSummarize(chunks) {
			t.Errorf("CanSummarize(%s) = false, want true", ct)
		}
	}

	cases := []struct {
		name   string
		chunks []knowledge.KnowledgeChunk
		want   bool
	}{
		{"empty chunk list", nil, false},
		{"plain prose without headings", []knowledge.KnowledgeChunk{{ID: "c1", Body: knowledge.ChunkBody{Raw: "just sentences. no structure."}}}, false},
		{"undetected via markdown headings in body", []knowledge.KnowledgeChunk{{ID: "c1", Body: knowledge.ChunkBody{Raw: "Intro.\n\n# Heading\nBody."}}}, true},
		{"any chunk qualifies", []knowledge.KnowledgeChunk{
			{ID: "c1", Body: knowledge.ChunkBody{Raw: "no headings here."}},
			chunkWithBody("c2", "markdown", "# H"),
		}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := NewHeadingSummarizer().CanSummarize(tc.chunks); got != tc.want {
				t.Errorf("CanSummarize = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestHeadingSummarizerSummarizeEmptyChunksErrors(t *testing.T) {
	_, err := NewHeadingSummarizer().Summarize(context.Background(), SummarizationRequest{})
	if err == nil || err.Error() != "no chunks to summarize" {
		t.Fatalf("err = %v, want 'no chunks to summarize'", err)
	}
}

func TestHeadingSummarizerSummarizeDefaultsSentencesPerSectionOnZeroValue(t *testing.T) {
	s := &HeadingSummarizer{} // zero value: SentencesPerSection <= 0
	chunks := []knowledge.KnowledgeChunk{chunkWithBody("c1", "text", "# Title\nBody.")}
	res, err := s.Summarize(context.Background(), SummarizationRequest{Chunks: chunks})
	if err != nil {
		t.Fatalf("Summarize: %v", err)
	}
	if s.SentencesPerSection != 2 {
		t.Errorf("SentencesPerSection = %d, want defaulted to 2", s.SentencesPerSection)
	}
	if want := "heading_summarizer:2:" + res.CoverageHash[:8]; res.DerivationMethod != want {
		t.Errorf("DerivationMethod = %q, want %q", res.DerivationMethod, want)
	}
	if res.UsedModel {
		t.Error("heading summarizer must never use a model")
	}
}

func TestHeadingSummarizerSummarizeMetadata(t *testing.T) {
	s := NewHeadingSummarizer()
	s.SentencesPerSection = 3
	chunks := []knowledge.KnowledgeChunk{
		chunkWithBody("c1", "markdown", "# A\nAlpha."),
		chunkWithBody("c2", "markdown", "# B\nBeta."),
	}
	res, err := s.Summarize(context.Background(), SummarizationRequest{Chunks: chunks})
	if err != nil {
		t.Fatalf("Summarize: %v", err)
	}
	wantHash := ComputeCoverageHash(chunks)
	if res.CoverageHash != wantHash {
		t.Errorf("CoverageHash = %q, want %q", res.CoverageHash, wantHash)
	}
	if want := "heading_summarizer:3:" + wantHash[:8]; res.DerivationMethod != want {
		t.Errorf("DerivationMethod = %q, want %q", res.DerivationMethod, want)
	}
	if res.TokenEstimate != CountTokens(res.Summary) {
		t.Errorf("TokenEstimate = %d, want %d", res.TokenEstimate, CountTokens(res.Summary))
	}
	if len(res.SourceCoverage) != 2 || res.SourceCoverage[0] != "c1" || res.SourceCoverage[1] != "c2" {
		t.Errorf("SourceCoverage = %v, want [c1 c2]", res.SourceCoverage)
	}
	if !strings.Contains(res.Summary, "# A") || !strings.Contains(res.Summary, "# B") {
		t.Errorf("summary must keep every chunk's headings, got %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "\n\n") {
		t.Errorf("chunk summaries must be joined with a blank line, got %q", res.Summary)
	}
}

func TestHeadingSummarizerSummarizeWithHeadingsKeepsStructureAndTruncates(t *testing.T) {
	doc := strings.Join([]string{
		"# Title",
		"",
		"Alpha one. Alpha two. Alpha three.",
		"",
		"- item one",
		"- item two",
		"",
		"## Sub",
		"",
		"Beta one. Beta two.",
	}, "\n")

	out := NewHeadingSummarizer().summarizeWithHeadings(doc)

	for _, want := range []string{"# Title", "## Sub", "- item one", "- item two", "Alpha one. Alpha two.", "Beta one. Beta two."} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Alpha three") {
		t.Errorf("third sentence must be truncated at 2 per section:\n%s", out)
	}
}

func TestHeadingSummarizerIsHeading(t *testing.T) {
	cases := []struct {
		line string
		want bool
	}{
		{"# Heading", true},
		{"====", true},
		{"-----", true},
		{"==", false},  // too short for underline style
		{"---", false}, // exactly 3 chars: not an underline
		{"-", false},   // list marker, not a heading
		{"text", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := NewHeadingSummarizer().isHeading(tc.line); got != tc.want {
			t.Errorf("isHeading(%q) = %v, want %v", tc.line, got, tc.want)
		}
	}
}

func TestHeadingSummarizerHasMarkdownHeadings(t *testing.T) {
	s := NewHeadingSummarizer()
	if !s.hasMarkdownHeadings("intro\n\n# Real heading\n") {
		t.Error("content with a markdown heading must be detected")
	}
	if !s.hasMarkdownHeadings("intro\n\n~~~~\nunderline style\n====\n") {
		t.Error("underline-style heading must be detected")
	}
	if s.hasMarkdownHeadings("no headings, only - lists") {
		t.Error("plain prose must not be reported as headed")
	}
}

func TestHeadingSummarizerExtractSentences(t *testing.T) {
	s := NewHeadingSummarizer()
	cases := []struct {
		text string
		want []string
	}{
		{"", nil},
		{"One. Two! Three?", []string{"One.", "Two!", "Three?"}},
		{"No terminator", []string{"No terminator"}},
		{"3.14 stays whole", []string{"3.14 stays whole"}},
		{"End.", []string{"End."}},
		{"A.  B.", []string{"A.", "B."}},
	}
	for _, tc := range cases {
		got := s.extractSentences(tc.text)
		if len(got) != len(tc.want) {
			t.Errorf("extractSentences(%q) = %#v, want %#v", tc.text, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("extractSentences(%q)[%d] = %q, want %q", tc.text, i, got[i], tc.want[i])
			}
		}
	}
}

func TestHeadingSummarizerSummarizeSectionPreservesListsAndLeads(t *testing.T) {
	s := NewHeadingSummarizer() // SentencesPerSection = 2
	section := []string{"- first", "- second", "Lead one. Lead two. Lead three."}

	out := s.summarizeSection(section)

	for _, want := range []string{"- first", "- second", "Lead one. Lead two."} {
		if !strings.Contains(out, want) {
			t.Errorf("section output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Lead three") {
		t.Errorf("section must truncate to 2 sentences:\n%s", out)
	}
}

func TestHeadingSummarizerGetContentTypeReadsChunkFields(t *testing.T) {
	s := NewHeadingSummarizer()
	if got := s.getContentType(chunkWithBody("c1", "markdown", "")); got != "markdown" {
		t.Errorf("getContentType = %q, want markdown", got)
	}
	if got := s.getContentType(knowledge.KnowledgeChunk{ID: "c2"}); got != "" {
		t.Errorf("getContentType without fields = %q, want empty", got)
	}
}
