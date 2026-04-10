package shell

import (
	"context"
	"os"
	"path/filepath"
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
	if _, ok := seen["shell_tool_discover"]; !ok {
		t.Fatal("expected discovery query tool in registry")
	}
	if _, ok := seen["shell_tool_instantiate"]; !ok {
		t.Fatal("expected instantiation query tool in registry")
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
