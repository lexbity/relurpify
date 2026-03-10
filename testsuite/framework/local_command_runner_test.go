package framework_test

import (
	"context"
	"github.com/lexcodex/relurpify/framework/sandbox"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalCommandRunnerRejectsOutsideWorkspace(t *testing.T) {
	t.Helper()
	ws := t.TempDir()
	runner := sandbox.NewLocalCommandRunner(ws, nil)
	_, _, err := runner.Run(context.Background(), sandbox.CommandRequest{
		Workdir: filepath.Join(ws, ".."),
		Args:    []string{"sh", "-c", "echo hi"},
	})
	if err == nil {
		t.Fatal("expected error for outside workspace workdir")
	}
}

func TestLocalCommandRunnerRunsCommand(t *testing.T) {
	t.Helper()
	ws := t.TempDir()
	runner := sandbox.NewLocalCommandRunner(ws, nil)
	stdout, _, err := runner.Run(context.Background(), sandbox.CommandRequest{
		Workdir: ws,
		Args:    []string{"sh", "-c", "echo hello"},
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout, "hello") {
		t.Fatalf("unexpected stdout: %q", stdout)
	}
}
