package shell

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/lexcodex/relurpify/framework/config"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/sandbox"
	"github.com/lexcodex/relurpify/platform/shell/archive"
	"github.com/lexcodex/relurpify/platform/shell/build"
	"github.com/lexcodex/relurpify/platform/shell/fileops"
	"github.com/lexcodex/relurpify/platform/shell/network"
	"github.com/lexcodex/relurpify/platform/shell/scheduler"
	"github.com/lexcodex/relurpify/platform/shell/system"
	"github.com/lexcodex/relurpify/platform/shell/text"
)

type recordingRunner struct {
	requests []sandbox.CommandRequest
	stdout   string
	stderr   string
	err      error
}

func (r *recordingRunner) Run(_ context.Context, req sandbox.CommandRequest) (string, string, error) {
	r.requests = append(r.requests, req)
	return r.stdout, r.stderr, r.err
}

func TestCommandQueryAndBindingTool(t *testing.T) {
	dir := t.TempDir()
	bindingsPath := filepath.Join(dir, "shell_bindings.yaml")
	if err := os.WriteFile(bindingsPath, []byte(strings.TrimSpace(`
version: v1
bindings:
  - id: hello
    name: hello
    description: greet
    command: ["echo", "hello"]
    shell: false
    args_passthrough: true
    tags: ["read-only"]
  - id: shellcmd
    name: shellcmd
    description: shell command
    command: ["echo hi"]
    shell: true
    args_passthrough: false
`)), 0o600); err != nil {
		t.Fatalf("write bindings: %v", err)
	}

	bindings, err := LoadShellBindings(bindingsPath)
	if err != nil {
		t.Fatalf("load bindings: %v", err)
	}
	if len(bindings) != 2 {
		t.Fatalf("expected 2 bindings, got %d", len(bindings))
	}

	query := NewCommandQuery([]string{"echo", "bash"}, bindings)
	req, err := query.Resolve("hello", []string{"world"})
	if err != nil {
		t.Fatalf("resolve binding: %v", err)
	}
	if !reflect.DeepEqual(req.Args, []string{"echo", "hello", "world"}) {
		t.Fatalf("unexpected command args: %#v", req.Args)
	}
	if _, err := query.ValidateRaw([]string{"echo", "ok"}); err != nil {
		t.Fatalf("validate raw: %v", err)
	}
	if _, err := query.Resolve("missing", nil); err == nil {
		t.Fatal("expected missing binding to fail")
	}

	shellQuery := NewCommandQuery([]string{"echo"}, []ShellBinding{bindings[1]})
	if _, err := shellQuery.Resolve("shellcmd", nil); err == nil {
		t.Fatal("expected shell binding to fail when bash is not allowed")
	}
	allowedShellQuery := NewCommandQuery(nil, []ShellBinding{bindings[1]})
	shellReq, err := allowedShellQuery.Resolve("shellcmd", nil)
	if err != nil {
		t.Fatalf("resolve shell binding with shell allowed: %v", err)
	}
	if !reflect.DeepEqual(shellReq.Args, []string{"bash", "-c", "echo hi"}) {
		t.Fatalf("unexpected shell-wrapped args: %#v", shellReq.Args)
	}

	runner := &recordingRunner{stdout: "ok", stderr: "warn"}
	tool := NewShellBindingTool(bindings[0], query, runner)
	if got := tool.Name(); got != "hello" {
		t.Fatalf("unexpected tool name: %q", got)
	}
	if got := tool.Category(); got != "shell-binding" {
		t.Fatalf("unexpected tool category: %q", got)
	}
	if len(tool.Parameters()) != 1 {
		t.Fatalf("expected passthrough parameter, got %#v", tool.Parameters())
	}
	if !tool.IsAvailable(context.Background(), core.NewContext()) {
		t.Fatal("expected tool to be available with a runner")
	}

	result, err := tool.Execute(context.Background(), core.NewContext(), map[string]interface{}{"extra_args": "planet"})
	if err != nil {
		t.Fatalf("execute binding: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected successful binding execution, got %#v", result)
	}
	if len(runner.requests) != 1 || !reflect.DeepEqual(runner.requests[0].Args, []string{"echo", "hello", "planet"}) {
		t.Fatalf("unexpected runner request: %#v", runner.requests)
	}

	runner.err = context.DeadlineExceeded
	result, err = tool.Execute(context.Background(), core.NewContext(), nil)
	if err != nil {
		t.Fatalf("execute with runner error should still return tool result: %v", err)
	}
	if result.Success {
		t.Fatal("expected failed tool execution when runner returns an error")
	}
	if !strings.Contains(result.Error, "command execution failed") {
		t.Fatalf("unexpected error text: %q", result.Error)
	}
}

func TestShellCommandRegistriesAndCommandLineTools(t *testing.T) {
	dir := t.TempDir()
	skillsDir := filepath.Join(config.New(dir).SkillsDir())
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatalf("create skills dir: %v", err)
	}

	runner := &recordingRunner{}
	tools := CommandLineTools(dir, runner)
	if len(tools) == 0 {
		t.Fatal("expected command line tool registry to return tools")
	}
	seen := make(map[string]struct{}, len(tools))
	for _, tool := range tools {
		if tool == nil {
			t.Fatal("unexpected nil tool in registry")
		}
		name := tool.Name()
		if name == "" {
			t.Fatal("expected non-empty tool name")
		}
		if _, ok := seen[name]; ok {
			t.Fatalf("duplicate tool name %q", name)
		}
		seen[name] = struct{}{}
		if setter, ok := tool.(interface{ SetCommandRunner(sandbox.CommandRunner) }); ok {
			setter.SetCommandRunner(runner)
		}
	}

	registryGroups := []struct {
		name  string
		tools []core.Tool
	}{
		{"text", text.Tools(dir)},
		{"fileops", fileops.Tools(dir)},
		{"system", system.Tools(dir)},
		{"build", build.Tools(dir)},
		{"archive", archive.Tools(dir)},
		{"network", network.Tools(dir)},
		{"scheduler", scheduler.Tools(dir)},
	}
	for _, group := range registryGroups {
		if len(group.tools) == 0 {
			t.Fatalf("expected %s registry to return tools", group.name)
		}
		for _, tool := range group.tools {
			if tool == nil || tool.Name() == "" {
				t.Fatalf("unexpected tool in %s registry: %#v", group.name, tool)
			}
		}
	}
}
