package clinix

import (
	"context"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/runtime"
	"github.com/stretchr/testify/assert"
	"os"
	"path/filepath"
	"testing"
)

type stubRunner struct {
	last runtime.CommandRequest
}

func (s *stubRunner) Run(ctx context.Context, req runtime.CommandRequest) (string, string, error) {
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
