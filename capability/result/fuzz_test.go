package capresult

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentspec "codeburg.org/lexbit/relurpify/capability/agentspec"
)

// FuzzSummarizeCapabilityResultEnvelope drives the summarize path with
// arbitrary untrusted trust classes, dispositions, and summary payloads.
// Invariants: the summarize pipeline is total (never panics, never returns a
// nil envelope for a non-nil source), the resulting disposition is always
// summarized with a non-empty insertion action drawn from the decision
// vocabulary, and DefaultInsertionDecision is total over arbitrary inputs.
func FuzzSummarizeCapabilityResultEnvelope(f *testing.F) {
	f.Add("builtin-trusted", "raw", "hello world")
	f.Add("remote-declared-untrusted", "metadata-only", "")
	f.Add("weird-trust", "made-up-disposition", "\x00\xff\xfe binary-ish summary")
	f.Add("", "", "   ")

	f.Fuzz(func(t *testing.T, trustClass, disposition, summary string) {
		desc := testDescriptor("cap-1", agentspec.TrustClass(trustClass))

		defaultDecision := DefaultInsertionDecision(desc, ContentDisposition(disposition))
		assert.NotEmpty(t, defaultDecision.Action,
			"DefaultInsertionDecision must always produce an action, got trust=%q disposition=%q", trustClass, disposition)

		source := NewCapabilityResultEnvelope(desc, nil, ContentDisposition(disposition), nil, nil)
		require.NotNil(t, source)

		summarized := SummarizeCapabilityResultEnvelope(source, summary)
		require.NotNil(t, summarized)
		assert.Equal(t, ContentDispositionSummarized, summarized.Disposition)
		assert.NotEmpty(t, summarized.Insertion.Action)
		assert.Contains(t, []agentspec.InsertionAction{
			agentspec.InsertionActionDirect,
			agentspec.InsertionActionSummarized,
			agentspec.InsertionActionMetadataOnly,
			agentspec.InsertionActionHITLRequired,
			agentspec.InsertionActionDenied,
		}, summarized.Insertion.Action, "summarized envelope must carry a vocabulary action")

		// Summarizing an already-summarized envelope must stay total.
		again := SummarizeCapabilityResultEnvelope(summarized, summary+" again")
		require.NotNil(t, again)
		assert.Equal(t, ContentDispositionSummarized, again.Disposition)
	})
}
