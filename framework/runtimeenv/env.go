package runtimeenv

import "os"

// Capture snapshots the process environment once at startup.
func Capture() []string {
	return append([]string(nil), os.Environ()...)
}
