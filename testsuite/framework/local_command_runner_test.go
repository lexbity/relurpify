package framework_test

import (
	"context"
	"github.com/lexcodex/relurpify/framework/runtime"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalCommandRunnerRejectsOutsideWorkspace(t *testing.T) {
	t.Helper()
	ws := t.TempDir()
	runner := runtime.NewLocalCommandRunner(ws, nil)
	_, _, err := runner.Run(context.Background(), runtime.CommandRequest{
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
	runner := runtime.NewLocalCommandRunner(ws, nil)
	stdout, _, err := runner.Run(context.Background(), runtime.CommandRequest{
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
