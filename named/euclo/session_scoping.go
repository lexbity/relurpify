package euclo

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/euclo/runtime/statebus"
)

func generateSessionID() string {
	b := make([]byte, 16)
	if _, err := rand.Reader.Read(b); err != nil {
		return fmt.Sprintf("euclo-session-%x", b[:4])
	}
	return "euclo-" + hex.EncodeToString(b)
}

func enforceSessionScoping(state *core.Context, sessionID string) error {
	if state == nil || sessionID == "" {
		return nil
	}
	existing := statebus.GetString(state, "euclo.session_id")
	if existing == "" {
		statebus.SetAny(state, "euclo.session_id", sessionID)
		return nil
	}
	if existing == sessionID {
		return nil
	}
	return fmt.Errorf("euclo session scoping violation: state belongs to session %s, current session is %s", existing, sessionID)
}
