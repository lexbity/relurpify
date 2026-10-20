// Command fakerunner is the supervisor test's child: a stand-in
// relurpify-runner that writes a valid status.json heartbeat (with its own
// PID) and sleeps until signaled — enough of the runner contract for the
// attach/spawn/degrade/stop tests.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func main() {
	// Same flags as the real runner (a subset): --state-dir is the only one
	// the supervisor contract depends on here. No process env reads.
	workspace := flag.String("workspace", "", "accepted and ignored by the fake")
	stateDir := flag.String("state-dir", "", "runtime state dir")
	flag.Parse()
	_ = workspace
	if *stateDir == "" {
		fmt.Fprintln(os.Stderr, "--state-dir required")
		os.Exit(2)
	}
	statusDir := filepath.Join(*stateDir, "jobs")
	if err := os.MkdirAll(statusDir, 0o700); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	write := func() {
		status := map[string]any{
			"runner_id":    "fake-runner",
			"pid":          os.Getpid(),
			"started_at":   time.Now().UTC(),
			"heartbeat_at": time.Now().UTC(),
			"draining":     false,
		}
		data, _ := json.Marshal(status)
		tmp := filepath.Join(statusDir, "status.json.tmp")
		if err := os.WriteFile(tmp, data, 0o600); err != nil {
			return
		}
		_ = os.Rename(tmp, filepath.Join(statusDir, "status.json"))
	}
	write()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for range ticker.C {
		write()
	}
}
