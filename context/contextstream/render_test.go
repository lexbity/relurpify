package contextstream

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/context/contextdata"
)

func seedSlice(env *contextdata.Envelope, chunks []contextdata.StreamedChunk, finalTokens int) {
	env.SetStreamedSlice(&contextdata.StreamedSlice{
		RequestID:    "ctxstream.req-0173",
		Epoch:        7,
		BudgetTokens: 8192,
		FinalTokens:  finalTokens,
		Chunks:       chunks,
		CacheHit:     true,
	})
}

func TestRenderStreamedSectionFormat(t *testing.T) {
	env := contextdata.NewEnvelope("t", "s")
	seedSlice(env, []contextdata.StreamedChunk{
		{ChunkID: "chunk-9f31c2", ContentHash: "h1", Body: "grounded knowledge body\n", TokenEstimate: 412, TrustClass: "workspace"},
		{ChunkID: "chunk-77a0", ContentHash: "h2", Body: "summary body", TokenEstimate: 204, IsSummary: true},
	}, 616)

	section, stats, err := RenderStreamedSection(env)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(section, fmt.Sprintf(`<streamed-context v=%d request="ctxstream.req-0173" epoch=7 budget=8192 tokens=616 chunks=2 cache="hit">`, StreamedSectionVersion)) {
		t.Fatalf("header missing or wrong: %q", section)
	}
	if !strings.Contains(section, "#1 [trust:workspace] chunk-9f31c2 (412 tok)") {
		t.Fatalf("rank-1 header missing: %q", section)
	}
	if !strings.Contains(section, "#2 (summary of chunk-77a0, original elided under budget) (204 tok)") {
		t.Fatalf("summary header missing: %q", section)
	}
	if !strings.Contains(section, "<chunk-body>\ngrounded knowledge body\n</chunk-body>") {
		t.Fatalf("verbatim body missing: %q", section)
	}
	if !strings.HasPrefix(section, "<streamed-context") || !strings.HasSuffix(section, "</streamed-context>\n") {
		t.Fatalf("section not delimited: %q", section)
	}
	if stats.Chunks != 2 || stats.Tokens != 616 || stats.CacheHit != true || stats.Epoch != 7 {
		t.Fatalf("stats = %+v", stats)
	}
	if stats.Bytes == 0 {
		t.Fatal("stats.Bytes not counted")
	}
}

func TestRenderStreamedSectionEmptyRendersZeroBytes(t *testing.T) {
	empty := contextdata.NewEnvelope("t", "s")
	section, stats, err := RenderStreamedSection(empty)
	if err != nil || section != "" || stats.Chunks != 0 {
		t.Fatalf("absent slice render = (%q, %+v, %v)", section, stats, err)
	}
	empty.SetStreamedSlice(&contextdata.StreamedSlice{Epoch: 3})
	section, _, err = RenderStreamedSection(empty)
	if err != nil || section != "" {
		t.Fatalf("zero-chunk slice render = (%q, %v)", section, err)
	}
}

func TestRenderStreamedSectionGapsAndTrustFallback(t *testing.T) {
	env := contextdata.NewEnvelope("t", "s")
	env.SetStreamedSlice(&contextdata.StreamedSlice{
		RequestID:   "r",
		Epoch:       1,
		FinalTokens: 10,
		Chunks: []contextdata.StreamedChunk{
			{ChunkID: "chunk-a", Body: "body a", TokenEstimate: 10},
		},
		GapMessages: []string{"chunk-stale"},
	})
	section, _, err := RenderStreamedSection(env)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(section, "(gap: chunk chunk-stale skipped stale)") {
		t.Fatalf("gap line missing: %q", section)
	}
	if !strings.Contains(section, "[trust:unset]") {
		t.Fatalf("unset trust marker missing: %q", section)
	}
}

func TestRenderStreamedSectionErrors(t *testing.T) {
	t.Run("body_missing", func(t *testing.T) {
		env := contextdata.NewEnvelope("t", "s")
		env.SetStreamedSlice(&contextdata.StreamedSlice{
			Epoch: 1, FinalTokens: 5,
			Chunks: []contextdata.StreamedChunk{{ChunkID: "chunk-x", TokenEstimate: 5}},
		})
		_, _, err := RenderStreamedSection(env)
		if !errors.Is(err, ErrSliceBodyMissing) {
			t.Fatalf("err = %v, want ErrSliceBodyMissing", err)
		}
	})
	t.Run("token_mismatch", func(t *testing.T) {
		env := contextdata.NewEnvelope("t", "s")
		env.SetStreamedSlice(&contextdata.StreamedSlice{
			Epoch: 1, FinalTokens: 1000,
			Chunks: []contextdata.StreamedChunk{{ChunkID: "chunk-x", Body: "b", TokenEstimate: 100}},
		})
		_, _, err := RenderStreamedSection(env)
		if !errors.Is(err, ErrSliceTokenMismatch) {
			t.Fatalf("err = %v, want ErrSliceTokenMismatch", err)
		}
	})
	t.Run("within_tolerance", func(t *testing.T) {
		env := contextdata.NewEnvelope("t", "s")
		env.SetStreamedSlice(&contextdata.StreamedSlice{
			Epoch: 1, FinalTokens: 100,
			Chunks: []contextdata.StreamedChunk{{ChunkID: "chunk-x", Body: "b", TokenEstimate: 95}},
		})
		if _, _, err := RenderStreamedSection(env); err != nil {
			t.Fatalf("10%% tolerance exceeded by 5%% divergence: %v", err)
		}
	})
}

// TestRenderStreamedSectionDeterministic is NFR-3: identical slice ⇒
// byte-identical section, across repeated renders (the compiled slice carries
// no timestamps in the section).
func TestRenderStreamedSectionDeterministic(t *testing.T) {
	build := func() *contextdata.Envelope {
		env := contextdata.NewEnvelope("t", "s")
		seedSlice(env, []contextdata.StreamedChunk{
			{ChunkID: "chunk-a", Body: "alpha\n", TokenEstimate: 30, TrustClass: "workspace"},
			{ChunkID: "chunk-b", Body: "beta", TokenEstimate: 20, IsSummary: true},
		}, 50)
		return env
	}
	first, _, err := RenderStreamedSection(build())
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	for i := 0; i < 100; i++ {
		again, _, err := RenderStreamedSection(build())
		if err != nil {
			t.Fatalf("render %d: %v", i, err)
		}
		if again != first {
			t.Fatalf("render %d diverges from the first (NFR-3)", i)
		}
	}
}

// BenchmarkRenderStreamedSection pins NFR-1: an 8192-token slice (~32KB of
// body text) renders in string-concatenation time; the CI budget with a 10x
// safety factor is 50ms per op.
func BenchmarkRenderStreamedSection(b *testing.B) {
	env := contextdata.NewEnvelope("t", "s")
	body := strings.Repeat("knowledge payload line\n", 170) // ~4KB per chunk
	chunks := make([]contextdata.StreamedChunk, 8)
	for i := range chunks {
		chunks[i] = contextdata.StreamedChunk{
			ChunkID:       contextdata.ChunkID(fmt.Sprintf("chunk-%d", i)),
			Body:          body,
			TokenEstimate: 1024,
			TrustClass:    "workspace",
		}
	}
	env.SetStreamedSlice(&contextdata.StreamedSlice{
		RequestID: "bench", Epoch: 1, BudgetTokens: 8192, FinalTokens: 8192, Chunks: chunks,
	})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := RenderStreamedSection(env); err != nil {
			b.Fatal(err)
		}
	}
}
