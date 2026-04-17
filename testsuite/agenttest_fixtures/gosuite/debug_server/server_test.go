package debug_server

import "testing"

func TestGetUser(t *testing.T) {
	if got := GetUser("missing"); got != "" {
		t.Fatalf("GetUser(missing) = %q, want empty string", got)
	}
}
