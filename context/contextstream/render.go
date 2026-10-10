package contextstream

import (
	"errors"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/context/contextdata"
)

// StreamedSectionVersion is the version stamped into every rendered section.
// It is the contract anchor tests and downstream parsers assert on; a format
// change bumps it.
const StreamedSectionVersion = 1

// tokenAccountingTolerance is how far the sum of per-chunk token estimates may
// diverge from the compiler's own FinalTokens accounting before the slice is
// considered internally inconsistent (D-10).
const tokenAccountingTolerance = 0.10

var (
	// ErrSliceBodyMissing reports a slice whose chunk list carries an empty
	// body — a reference without content is an internal inconsistency and
	// fails the calling node rather than degrading the prompt (D-10).
	ErrSliceBodyMissing = errors.New("contextstream: streamed slice chunk without body")
	// ErrSliceTokenMismatch reports a slice whose per-chunk token estimates
	// diverge from the compiler's FinalTokens accounting beyond tolerance.
	ErrSliceTokenMismatch = errors.New("contextstream: streamed slice token accounting mismatch")
)

// RenderStats summarizes one render for telemetry and budget assertions.
type RenderStats struct {
	Chunks   int
	Tokens   int // sum of TokenEstimate — must equal Slice.FinalTokens within 10%
	Bytes    int
	CacheHit bool
	Epoch    uint64
}

// RenderStreamedSection renders the envelope's current StreamedSlice as the
// canonical prompt section. Pure: no store access, no I/O, deterministic
// ordering, byte-identical output for identical (graph state, request). It is
// the ONLY code in the repository permitted to format chunk bodies for a
// prompt. An empty or absent slice renders "" — zero prompt bytes (D-3).
func RenderStreamedSection(env *contextdata.Envelope) (string, RenderStats, error) {
	slice := env.StreamedSliceSnapshot()
	if slice == nil || len(slice.Chunks) == 0 {
		return "", RenderStats{}, nil
	}

	stats := RenderStats{CacheHit: slice.CacheHit, Epoch: slice.Epoch}
	var total int
	var b strings.Builder
	fmt.Fprintf(&b, "<streamed-context v=%d request=%q epoch=%d budget=%d tokens=%d chunks=%d cache=%q>\n",
		StreamedSectionVersion, slice.RequestID, slice.Epoch, slice.BudgetTokens, slice.FinalTokens, len(slice.Chunks), cacheLabel(slice.CacheHit))
	for i, chunk := range slice.Chunks {
		if strings.TrimSpace(chunk.Body) == "" {
			return "", RenderStats{}, fmt.Errorf("%w: chunk %q at rank %d", ErrSliceBodyMissing, chunk.ChunkID, i+1)
		}
		body := chunk.Body
		if !strings.HasSuffix(body, "\n") {
			body += "\n"
		}
		if chunk.IsSummary {
			fmt.Fprintf(&b, "#%d (summary of %s, original elided under budget) (%d tok)\n", i+1, chunk.ChunkID, chunk.TokenEstimate)
		} else {
			trust := chunk.TrustClass
			if trust == "" {
				trust = "unset"
			}
			fmt.Fprintf(&b, "#%d [trust:%s] %s (%d tok)\n", i+1, trust, chunk.ChunkID, chunk.TokenEstimate)
		}
		b.WriteString("<chunk-body>\n")
		b.WriteString(body)
		b.WriteString("</chunk-body>\n")
		total += chunk.TokenEstimate
	}
	for _, gap := range slice.GapMessages {
		if strings.TrimSpace(gap) == "" {
			continue
		}
		fmt.Fprintf(&b, "(gap: chunk %s skipped stale)\n", gap)
	}
	b.WriteString("</streamed-context>\n")

	if slice.FinalTokens > 0 {
		divergence := total - slice.FinalTokens
		if divergence < 0 {
			divergence = -divergence
		}
		if float64(divergence) > tokenAccountingTolerance*float64(slice.FinalTokens) {
			return "", RenderStats{}, fmt.Errorf("%w: chunks sum to %d tokens, compiler accounting says %d", ErrSliceTokenMismatch, total, slice.FinalTokens)
		}
	}

	stats.Chunks = len(slice.Chunks)
	stats.Tokens = total
	stats.Bytes = b.Len()
	return b.String(), stats, nil
}

func cacheLabel(cacheHit bool) string {
	if cacheHit {
		return "hit"
	}
	return "miss"
}
