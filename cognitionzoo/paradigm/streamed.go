package paradigm

import (
	"context"

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
		return "", err
	}
	if section == "" {
		contextstream.EmitInjectSkipped(ctx, "empty_slice", paradigmName)
		return "", nil
	}
	contextstream.EmitInjected(ctx, paradigmName, env.NodeIDSnapshot(), stats)
	return section, nil
}
