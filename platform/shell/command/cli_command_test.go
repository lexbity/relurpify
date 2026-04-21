package command

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/sandbox"
	"github.com/stretchr/testify/assert"
)

type stubRunner struct {
	last sandbox.CommandRequest
}

func (s *stubRunner) Run(ctx context.Context, req sandbox.CommandRequest) (string, string, error) {
	s.last = req
	return "", "", nil
}

func TestCommandToolAddsCargoManifestPathForNestedCrate(t *testing.T) {
	base := t.TempDir()
	crateDir := filepath.Join(base, "nested")
	assert.NoError(t, os.MkdirAll(crateDir, 0o755))
	assert.NoError(t, os.WriteFile(filepath.Join(crateDir, "Cargo.toml"), []byte("[package]\nname = \"demo\"\nversion = \"0.1.0\"\nedition = \"2021\"\n"), 0o644))

	runner := &stubRunner{}
	tool := NewCommandTool(base, CommandToolConfig{
		Name:        "cli_cargo",
		Command:     "cargo",
		Description: "Run cargo",
	})
	tool.SetCommandRunner(runner)

	_, err := tool.Execute(context.Background(), core.NewContext(), map[string]interface{}{
		"args":              []interface{}{"test"},
		"working_directory": "nested",
	})
	assert.NoError(t, err)
	assert.Equal(t, []string{"cargo", "test", "--manifest-path", filepath.Join(crateDir, "Cargo.toml")}, runner.last.Args)
	assert.Equal(t, crateDir, runner.last.Workdir)
}

func TestCommandToolIsolatesNestedCargoRunWhenParentManifestExists(t *testing.T) {
	base := t.TempDir()
	assert.NoError(t, os.WriteFile(filepath.Join(base, "Cargo.toml"), []byte("[package]\nname = \"root\"\nversion = \"0.1.0\"\n"), 0o644))

	crateDir := filepath.Join(base, "nested")
	assert.NoError(t, os.MkdirAll(filepath.Join(crateDir, "src"), 0o755))
	assert.NoError(t, os.WriteFile(filepath.Join(crateDir, "Cargo.toml"), []byte("[package]\nname = \"demo\"\nversion = \"0.1.0\"\nedition = \"2021\"\n"), 0o644))
	assert.NoError(t, os.WriteFile(filepath.Join(crateDir, "src", "lib.rs"), []byte("pub fn add(a:i32,b:i32)->i32{a+b}\n"), 0o644))

	tool := NewCommandTool(base, CommandToolConfig{
		Name:        "cli_cargo",
		Command:     "cargo",
		Description: "Run cargo",
	})

	workdir, args, cleanup, err := tool.prepareExecution(crateDir, []string{"test"})
	defer cleanup()

	assert.NoError(t, err)
	assert.Equal(t, base, workdir)
	assert.Len(t, args, 3)
	assert.Equal(t, "test", args[0])
	assert.Equal(t, "--manifest-path", args[1])
	assert.NotContains(t, args[2], base)
	_, statErr := os.Stat(args[2])
	assert.NoError(t, statErr)
	_, rootErr := os.Stat(filepath.Join(filepath.Dir(args[2]), "..", "Cargo.toml"))
	assert.Error(t, rootErr)
}

func TestCommandToolNonCargoDoesNotIsolate(t *testing.T) {
	base := t.TempDir()
	runner := &stubRunner{}
	tool := NewCommandTool(base, CommandToolConfig{
		Name:        "cli_echo",
		Command:     "echo",
		Description: "echo test",
	})
	tool.SetCommandRunner(runner)

	_, err := tool.Execute(context.Background(), core.NewContext(), map[string]interface{}{
		"args": []interface{}{"hello"},
	})
	assert.NoError(t, err)
	assert.Equal(t, []string{"echo", "hello"}, runner.last.Args)
	assert.Equal(t, base, runner.last.Workdir)
}

func TestCommandToolWorkingDirectoryRelative(t *testing.T) {
	base := t.TempDir()
	sub := filepath.Join(base, "sub")
	assert.NoError(t, os.MkdirAll(sub, 0o755))
	runner := &stubRunner{}
	tool := NewCommandTool(base, CommandToolConfig{
		Name:        "cli_pwd",
		Command:     "pwd",
		Description: "pwd",
	})
	tool.SetCommandRunner(runner)

	_, err := tool.Execute(context.Background(), core.NewContext(), map[string]interface{}{
		"working_directory": "sub",
	})
	assert.NoError(t, err)
	assert.Equal(t, sub, runner.last.Workdir)
}

func TestCommandToolWorkingDirectoryAbsolute(t *testing.T) {
	base := t.TempDir()
	// absolute path within base
	abs := filepath.Join(base, "abs")
	assert.NoError(t, os.MkdirAll(abs, 0o755))
	runner := &stubRunner{}
	tool := NewCommandTool(base, CommandToolConfig{
		Name:        "cli_ls",
		Command:     "ls",
		Description: "ls",
	})
	tool.SetCommandRunner(runner)

	_, err := tool.Execute(context.Background(), core.NewContext(), map[string]interface{}{
		"working_directory": abs,
	})
	assert.NoError(t, err)
	assert.Equal(t, abs, runner.last.Workdir)
}

func TestCommandToolWithStdin(t *testing.T) {
	base := t.TempDir()
	runner := &stubRunner{}
	tool := NewCommandTool(base, CommandToolConfig{
		Name:        "cli_cat",
		Command:     "cat",
		Description: "cat",
	})
	tool.SetCommandRunner(runner)

	_, err := tool.Execute(context.Background(), core.NewContext(), map[string]interface{}{
		"stdin": "hello world",
	})
	assert.NoError(t, err)
	assert.Equal(t, "hello world", runner.last.Input)
}

func TestCommandToolManifestPathNotDuplicated(t *testing.T) {
	base := t.TempDir()
	crateDir := filepath.Join(base, "nested")
	assert.NoError(t, os.MkdirAll(crateDir, 0o755))
	assert.NoError(t, os.WriteFile(filepath.Join(crateDir, "Cargo.toml"), []byte("[package]\nname = \"demo\"\nversion = \"0.1.0\"\nedition = \"2021\"\n"), 0o644))

	runner := &stubRunner{}
	tool := NewCommandTool(base, CommandToolConfig{
		Name:        "cli_cargo",
		Command:     "cargo",
		Description: "Run cargo",
	})
	tool.SetCommandRunner(runner)

	manifestPath := filepath.Join(crateDir, "Cargo.toml")
	_, err := tool.Execute(context.Background(), core.NewContext(), map[string]interface{}{
		"args":              []interface{}{"test", "--manifest-path", manifestPath},
		"working_directory": "nested",
	})
	assert.NoError(t, err)
	// Ensure --manifest-path appears exactly once
	args := runner.last.Args
	manifestCount := 0
	for i, arg := range args {
		if arg == "--manifest-path" {
			manifestCount++
			// next argument should be the manifest path
			if i+1 < len(args) && args[i+1] == manifestPath {
				// good
			}
		}
	}
	assert.Equal(t, 1, manifestCount, "expected exactly one --manifest-path flag, got %d", manifestCount)
	// The workdir should be crateDir (no isolation because no parent manifest)
	assert.Equal(t, crateDir, runner.last.Workdir)
}

func TestCommandToolDefaultArgs(t *testing.T) {
	base := t.TempDir()
	runner := &stubRunner{}
	tool := NewCommandTool(base, CommandToolConfig{
		Name:        "cli_mkdir",
		Command:     "mkdir",
		Description: "mkdir",
		DefaultArgs: []string{"-p", "-v"},
	})
	tool.SetCommandRunner(runner)

	_, err := tool.Execute(context.Background(), core.NewContext(), map[string]interface{}{
		"args": []interface{}{"newdir"},
	})
	assert.NoError(t, err)
	assert.Equal(t, []string{"mkdir", "-p", "-v", "newdir"}, runner.last.Args)
}
