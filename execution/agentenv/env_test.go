package agentenv

import (
	"strings"
	"testing"
)

func TestSnapshotProcessEnvCopiesCurrentEnvironment(t *testing.T) {
	t.Setenv("RELURPIFY_AGENTENV_SNAPSHOT_TEST", "present")

	env := SnapshotProcessEnv()
	env[0] = "mutated=true"

	next := SnapshotProcessEnv()
	for _, entry := range next {
		if strings.HasPrefix(entry, "RELURPIFY_AGENTENV_SNAPSHOT_TEST=") {
			if entry != "RELURPIFY_AGENTENV_SNAPSHOT_TEST=present" {
				t.Fatalf("snapshot entry = %q, want test env value", entry)
			}
			return
		}
	}
	t.Fatal("expected test environment entry in snapshot")
}
