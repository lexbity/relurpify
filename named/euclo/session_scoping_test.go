package euclo

import (
	"crypto/rand"
	"errors"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
)

type failingRandReader struct{}

func (failingRandReader) Read(p []byte) (int, error) {
	return 0, errors.New("rand failure")
}

func TestGenerateSessionID(t *testing.T) {
	id := generateSessionID()
	if id == "" || !strings.HasPrefix(id, "euclo-") {
		t.Fatalf("unexpected generated session id: %q", id)
	}

	original := rand.Reader
	rand.Reader = failingRandReader{}
	t.Cleanup(func() {
		rand.Reader = original
	})

	fallback := generateSessionID()
	if fallback == "" || !strings.HasPrefix(fallback, "euclo-session-") {
		t.Fatalf("unexpected fallback session id: %q", fallback)
	}
}

func TestEnforceSessionScoping(t *testing.T) {
	if err := enforceSessionScoping(nil, "session-a"); err != nil {
		t.Fatalf("expected nil error for nil state, got %v", err)
	}

	state := core.NewContext()
	if err := enforceSessionScoping(state, ""); err != nil {
		t.Fatalf("expected nil error for empty session id, got %v", err)
	}

	if err := enforceSessionScoping(state, "session-a"); err != nil {
		t.Fatalf("expected first session to be recorded, got %v", err)
	}
	if got := state.GetString("euclo.session_id"); got != "session-a" {
		t.Fatalf("expected session id to be stored, got %q", got)
	}

	if err := enforceSessionScoping(state, "session-a"); err != nil {
		t.Fatalf("expected matching session id to pass, got %v", err)
	}

	if err := enforceSessionScoping(state, "session-b"); err == nil {
		t.Fatal("expected mismatched session id to fail")
	}
}
