package main

import (
	"io"
	"os"
	"strings"
	"testing"
)

func TestV2DemoFlagCannotEnterLiveDoctorOrStatus(t *testing.T) {
	original := cfg
	defer func() { cfg = original }()
	for _, subcommand := range []string{"doctor", "status"} {
		workspace := t.TempDir()
		command := newRootCmd()
		command.SetOut(io.Discard)
		command.SetErr(io.Discard)
		command.SetArgs([]string{subcommand, "--demo", "quiet", "--workspace", workspace})
		err := command.ExecuteContext(t.Context())
		if err == nil || !strings.Contains(err.Error(), "--demo is supported only") {
			t.Fatalf("%s entered a live bootstrap path with --demo: %v", subcommand, err)
		}
		entries, err := os.ReadDir(workspace)
		if err != nil || len(entries) != 0 {
			t.Fatal("rejected demo flag caused workspace side effects")
		}
	}
}
