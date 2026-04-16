package euclo

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/runtime/statebus"
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
	existing := statebus.GetString(state, state.KeySessionID)
	if existing == "" {
		statebus.SetAny(state, state.KeySessionID, sessionID)
		return nil
	}
	if existing == sessionID {
		return nil
	}
	return fmt.Errorf("euclo session scoping violation: state belongs to session %s, current session is %s", existing, sessionID)
}
