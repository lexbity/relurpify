package versioning

import (
	"testing"

	"github.com/lexcodex/relurpify/framework/middleware/mcp/protocol"
	"github.com/stretchr/testify/require"
)

func TestChooseRevisionPrefersSupportedRequestedRevision(t *testing.T) {
	matrix := DefaultSupportMatrix()

	revision, err := matrix.ChooseRevision([]string{protocol.Revision20251125, protocol.Revision20250618})

	require.NoError(t, err)
	require.Equal(t, protocol.Revision20250618, revision)
}

func TestChooseRevisionFallsBackToPrimary(t *testing.T) {
	matrix := DefaultSupportMatrix()

	revision, err := matrix.ChooseRevision([]string{"2099-01-01"})

	require.NoError(t, err)
	require.Equal(t, protocol.Revision20250618, revision)
}

func TestNegotiateRejectsUnsupportedPeerRevision(t *testing.T) {
	matrix := DefaultSupportMatrix()

	_, err := matrix.Negotiate("2099-01-01", nil)

	require.ErrorContains(t, err, "unsupported peer protocol version")
}

func TestNegotiateRejectsMismatchedPeerRevision(t *testing.T) {
	matrix := DefaultSupportMatrix()
	matrix.Supported[protocol.Revision20251125] = RevisionSupport{Revision: protocol.Revision20251125}

	_, err := matrix.Negotiate(protocol.Revision20251125, []string{protocol.Revision20250618})

	require.ErrorContains(t, err, "does not match requested version")
}

func TestNegotiateAcceptsPrimaryRevision(t *testing.T) {
	matrix := DefaultSupportMatrix()

	result, err := matrix.Negotiate(protocol.Revision20250618, nil)

	require.NoError(t, err)
	require.Equal(t, protocol.Revision20250618, result.Requested)
	require.Equal(t, protocol.Revision20250618, result.Negotiated)
	require.True(t, result.IsPrimary)
}
