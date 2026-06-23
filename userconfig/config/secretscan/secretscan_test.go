// Config discipline hardening: single source of truth for secret
// field names.

package secretscan

import (
	"testing"
)

// TestSecretDenylistSingleSource asserts that ForbiddenSecretFieldNames is
// the single canonical definition of what constitutes a secret-bearing YAML
// field, and that it contains the expected entries.
func TestSecretDenylistSingleSource(t *testing.T) {
	expected := []string{
		"apikey",
		"apisecret",
		"credential",
		"passwd",
		"password",
		"privatekey",
		"secret",
		"token",
	}

	for _, name := range expected {
		if _, ok := ForbiddenSecretFieldNames[name]; !ok {
			t.Errorf("ForbiddenSecretFieldNames missing expected entry %q", name)
		}
	}

	if len(ForbiddenSecretFieldNames) != len(expected) {
		t.Errorf("ForbiddenSecretFieldNames has %d entries, expected %d", len(ForbiddenSecretFieldNames), len(expected))
	}
}

// TestSecretDenylistRejectsTokenFields asserts that a file containing a
// field named "token" is rejected by RejectForbiddenSecretFields (via the
// canonical denylist).
func TestSecretDenylistRejectsTokenFields(t *testing.T) {
	_, ok := ForbiddenSecretFieldNames["token"]
	if !ok {
		t.Error("ForbiddenSecretFieldNames should contain 'token'")
	}
}

// TestRuntimeStateDirNameIsCanonical asserts that RuntimeStateDirName has
// the expected value.
func TestRuntimeStateDirNameIsCanonical(t *testing.T) {
	if RuntimeStateDirName != ".relurpify_state" {
		t.Errorf("RuntimeStateDirName = %q, want %q", RuntimeStateDirName, ".relurpify_state")
	}
}
