package registry

import (
	"context"
	"errors"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/capability/ports"
)

// TestRollbackTokenTTLExpiry: a token past its retention TTL is gone and the
// rollback fails with the typed ErrRollbackExpired error.
func TestRollbackTokenTTLExpiry(t *testing.T) {
	r := NewRegistry()
	now := time.Now()
	r.rollbackTokens.SetClock(func() time.Time { return now })
	r.rollbackTokens.Put("rbk-ttl", ports.RollbackToken{InvocationID: "rbk-ttl", ToolName: "x"})
	now = now.Add(2 * rollbackTokenTTL)

	if _, ok := r.rollbackTokens.Get("rbk-ttl"); ok {
		t.Fatal("token survived its TTL")
	}
	if err := r.RollbackCapability(context.Background(), "rbk-ttl"); !errors.Is(err, ErrRollbackExpired) {
		t.Fatalf("expected ErrRollbackExpired, got %v", err)
	}
}

// TestRollbackTokenCapEviction: the retention cap evicts the oldest tokens;
// rolling back an evicted token returns ErrRollbackExpired.
func TestRollbackTokenCapEviction(t *testing.T) {
	r := NewRegistry()
	for i := 0; i < rollbackTokenCap+5; i++ {
		r.rollbackTokens.Put(rollbackID(i), ports.RollbackToken{InvocationID: rollbackID(i), ToolName: "x"})
	}
	if r.rollbackTokens.Len() != rollbackTokenCap {
		t.Fatalf("Len() = %d, want %d", r.rollbackTokens.Len(), rollbackTokenCap)
	}
	if r.rollbackTokens.Evicted() != 5 {
		t.Fatalf("Evicted() = %d, want 5", r.rollbackTokens.Evicted())
	}
	if err := r.RollbackCapability(context.Background(), rollbackID(0)); !errors.Is(err, ErrRollbackExpired) {
		t.Fatalf("expected ErrRollbackExpired for evicted token, got %v", err)
	}
}

// TestRollbackTokenRedactedAtRest: stored tokens carry RedactArgs output —
// a secret-looking argument never rests in the registry in raw form.
func TestRollbackTokenRedactedAtRest(t *testing.T) {
	r := NewRegistry()
	args := map[string]any{
		"api_key": "super-secret-value",
		"path":    "/tmp/file",
	}
	redacted := ports.RedactArgs(cloneArgs(args), nil)
	r.storeRollbackTokenLocked("tool", nil, args, &ports.ToolResult{Success: true})

	var stored ports.RollbackToken
	found := false
	r.rollbackTokens.Range(func(_ string, tok ports.RollbackToken) bool {
		stored = tok
		found = true
		return false
	})
	if !found {
		t.Fatal("rollback token not stored")
	}
	if got, _ := stored.Args["api_key"]; got != redacted["api_key"] || got == "super-secret-value" {
		t.Fatalf("secret argument stored raw: %v", stored.Args)
	}
	if stored.Args["path"] != "/tmp/file" {
		t.Fatalf("non-secret argument altered: %v", stored.Args)
	}
}

func rollbackID(i int) string {
	if i == 0 {
		return "rbk-0"
	}
	digits := []byte{}
	for i > 0 {
		digits = append([]byte{byte('0' + i%10)}, digits...)
		i /= 10
	}
	return "rbk-" + string(digits)
}
