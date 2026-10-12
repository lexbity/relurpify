package paradigm

import (
	"context"
	"errors"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/contextstream"
)

// StreamedSection renders the envelope's current compiled slice through the
// substrate renderer for one paradigm's prompt assembly. It is the single
// integration seam every DSL-reachable paradigm calls (D-2); the renderer
// itself lives in contextstream and stays pure. Telemetry rides the context
// sink: injected for a non-empty section, inject_skipped for the zero-byte
// path, render_error for the D-10 inconsistencies, which fail the calling
// node — a silently truncated slice is indistinguishable from a silently
// wrong agent.
func StreamedSection(ctx context.Context, env *contextdata.Envelope, paradigmName string) (string, error) {
	if env == nil {
		return "", nil
	}
	section, stats, err := contextstream.RenderStreamedSection(env)
	if err != nil {
		contextstream.EmitRenderError(ctx, err)
		// The streamed slice is an advisory prompt section. Content
		// corruption (a chunk without a body) must fail the node — wrong
		// bytes in a prompt are never acceptable. An accounting mismatch,
		// though, is estimate bookkeeping drifting from the compiler's
		// counter: aborting a turn that may already have done its work over
		// an estimate delta fails the user's task to defend a rounding
		// rule. Degrade to zero prompt bytes (D-3) and let the mismatch
		// telemetry carry it.
		if errors.Is(err, contextstream.ErrSliceTokenMismatch) {
			contextstream.EmitInjectSkipped(ctx, "token_accounting_mismatch", paradigmName)
			return "", nil
		}
		return "", err
	}
	if section == "" {
		contextstream.EmitInjectSkipped(ctx, "empty_slice", paradigmName)
		return "", nil
	}
	contextstream.EmitInjected(ctx, paradigmName, env.NodeIDSnapshot(), stats)
	return section, nil
}
