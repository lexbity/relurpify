package euclo

import (
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateSessionIDFormat(t *testing.T) {
	id := generateSessionID()
	assert.NotEmpty(t, id)
	assert.Contains(t, id, "euclo-")
	// 16 bytes = 32 hex chars + "euclo-" prefix = 38 chars total.
	assert.Len(t, id, 38)
}

func TestGenerateSessionIDUnique(t *testing.T) {
	ids := make(map[string]bool, 100)
	for i := 0; i < 100; i++ {
		id := generateSessionID()
		assert.False(t, ids[id], "duplicate session ID generated: %s", id)
		ids[id] = true
	}
}

func TestEnforceSessionScopingAllowsFreshState(t *testing.T) {
	state := core.NewContext()
	sessionID := generateSessionID()

	err := enforceSessionScoping(state, sessionID)
	require.NoError(t, err)

	// Session ID should now be stored in state.
	stored := state.GetString("euclo.session_id")
	assert.Equal(t, sessionID, stored)
}

func TestEnforceSessionScopingAllowsSameSession(t *testing.T) {
	state := core.NewContext()
	sessionID := generateSessionID()
	state.Set("euclo.session_id", sessionID)

	err := enforceSessionScoping(state, sessionID)
	require.NoError(t, err)
}

func TestEnforceSessionScopingBlocksRecursion(t *testing.T) {
	state := core.NewContext()
	existingSession := generateSessionID()
	state.Set("euclo.session_id", existingSession)

	newSession := generateSessionID()
	err := enforceSessionScoping(state, newSession)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "session scoping violation")
	assert.Contains(t, err.Error(), existingSession)
	assert.Contains(t, err.Error(), newSession)
}

func TestEnforceSessionScopingNilState(t *testing.T) {
	err := enforceSessionScoping(nil, "some-session")
	require.NoError(t, err)
}

func TestEnforceSessionScopingEmptySessionID(t *testing.T) {
	state := core.NewContext()
	err := enforceSessionScoping(state, "")
	require.NoError(t, err)
}
