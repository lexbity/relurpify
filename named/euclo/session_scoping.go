package euclo

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/lexcodex/relurpify/framework/core"
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
	existing := state.GetString("euclo.session_id")
	if existing == "" {
		state.Set("euclo.session_id", sessionID)
		return nil
	}
	if existing == sessionID {
		return nil
	}
	return fmt.Errorf("euclo session scoping violation: state belongs to session %s, current session is %s", existing, sessionID)
}
