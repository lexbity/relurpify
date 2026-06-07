package agentenv

import "os"

// SnapshotProcessEnv captures the process environment once for startup
// configuration and secret loading.
func SnapshotProcessEnv() []string {
	return append([]string(nil), os.Environ()...)
}
