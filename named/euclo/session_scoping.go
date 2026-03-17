package euclo

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/lexcodex/relurpify/framework/core"
)

// generateSessionID produces a random session identifier for Euclo execution
// scoping. The ID is used to detect and prevent recursive Euclo invocations
// within the same execution.
func generateSessionID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Fallback: should never happen with crypto/rand.
		return fmt.Sprintf("euclo-session-%x", b[:4])
	}
	return "euclo-" + hex.EncodeToString(b)
}

// enforceSessionScoping checks whether the given state already belongs to a
// different Euclo session. If euclo.session_id is already set and differs
// from the provided sessionID, the call is considered recursive and rejected.
// If the key is unset or matches, the session is allowed.
func enforceSessionScoping(state *core.Context, sessionID string) error {
	if state == nil || sessionID == "" {
		return nil
	}
	existing := state.GetString("euclo.session_id")
	if existing == "" {
		// Fresh state — claim it.
		state.Set("euclo.session_id", sessionID)
		return nil
	}
	if existing == sessionID {
		return nil
	}
	return fmt.Errorf("euclo session scoping violation: state belongs to session %s, current session is %s", existing, sessionID)
}
