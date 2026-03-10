package rust

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/sandbox"
	"github.com/stretchr/testify/assert"
)

type stubCommandRunner struct {
	lastReq sandbox.CommandRequest
	stdout  string
	stderr  string
	err     error
}

func (s *stubCommandRunner) Run(ctx context.Context, req sandbox.CommandRequest) (string, string, error) {
	s.lastReq = req
	return s.stdout, s.stderr, s.err
}

func TestRustWorkspaceDetectToolFindsNearestManifest(t *testing.T) {
	base := t.TempDir()
	crateRoot := filepath.Join(base, "apps", "demo")
	targetFile := filepath.Join(crateRoot, "src", "lib.rs")
	assert.NoError(t, os.MkdirAll(filepath.Dir(targetFile), 0o755))
	assert.NoError(t, os.WriteFile(filepath.Join(base, "Cargo.toml"), []byte("[workspace]\nmembers=[\"apps/demo\"]\n"), 0o644))
	assert.NoError(t, os.WriteFile(filepath.Join(crateRoot, "Cargo.toml"), []byte("[package]\nname=\"demo\"\nversion=\"0.1.0\"\n"), 0o644))
	assert.NoError(t, os.WriteFile(targetFile, []byte("pub fn demo() {}\n"), 0o644))

	tool := &RustWorkspaceDetectTool{BasePath: base}
	res, err := tool.Execute(context.Background(), core.NewContext(), map[string]interface{}{
		"path": targetFile,
	})
	assert.NoError(t, err)
	assert.True(t, res.Success)
	assert.Equal(t, filepath.Join(crateRoot, "Cargo.toml"), res.Data["manifest_path"])
	assert.Equal(t, filepath.Join(base, "Cargo.toml"), res.Data["workspace_manifest"])
}

func TestRustCargoTestToolReturnsStructuredSummary(t *testing.T) {
	base := t.TempDir()
	runner := &stubCommandRunner{
		stdout: "running 1 test\ntest tests::test_add ... FAILED\n\nfailures:\n\n---- tests::test_add stdout ----\n\nthread 'tests::test_add' panicked\n\ntest result: FAILED. 0 passed; 1 failed; 0 ignored; 0 measured; 0 filtered out; finished in 0.00s\n",
		err:    errors.New("exit status 101"),
	}
	tool := NewRustCargoTestTool(base)
	tool.SetCommandRunner(runner)

	res, err := tool.Execute(context.Background(), core.NewContext(), map[string]interface{}{
		"working_directory": ".",
	})
	assert.NoError(t, err)
	assert.False(t, res.Success)
	assert.Contains(t, res.Data["summary"], "tests::test_add")
	assert.Equal(t, 1, res.Data["failed"])
	assert.Equal(t, []string{"tests::test_add"}, res.Data["failed_tests"])
}

func TestRustCargoTestToolUsesCargoCommand(t *testing.T) {
	base := t.TempDir()
	crateDir := filepath.Join(base, "crate")
	assert.NoError(t, os.MkdirAll(crateDir, 0o755))
	assert.NoError(t, os.WriteFile(filepath.Join(crateDir, "Cargo.toml"), []byte("[package]\nname=\"demo\"\nversion=\"0.1.0\"\n"), 0o644))
	runner := &stubCommandRunner{stdout: "test result: ok. 1 passed; 0 failed;", err: nil}
	tool := NewRustCargoTestTool(base)
	tool.SetCommandRunner(runner)

	res, err := tool.Execute(context.Background(), core.NewContext(), map[string]interface{}{
		"working_directory": "crate",
		"extra_args":        []interface{}{"--", "--nocapture"},
	})
	assert.NoError(t, err)
	assert.True(t, res.Success)
	assert.Equal(t, "cargo", runner.lastReq.Args[0])
	assert.Contains(t, runner.lastReq.Args, "test")
	assert.Equal(t, filepath.Join(base, "crate"), runner.lastReq.Workdir)
}

func TestRustCargoCheckToolReturnsStructuredSummary(t *testing.T) {
	base := t.TempDir()
	runner := &stubCommandRunner{
		stderr: "error[E0308]: mismatched types\nwarning: unused variable\n",
		err:    errors.New("exit status 101"),
	}
	tool := NewRustCargoCheckTool(base)
	tool.SetCommandRunner(runner)

	res, err := tool.Execute(context.Background(), core.NewContext(), map[string]interface{}{
		"working_directory": ".",
	})
	assert.NoError(t, err)
	assert.False(t, res.Success)
	assert.Equal(t, 1, res.Data["error_count"])
	assert.Equal(t, 1, res.Data["warning_count"])
	assert.Contains(t, res.Data["summary"], "mismatched types")
}

func TestRustCargoMetadataToolParsesWorkspaceData(t *testing.T) {
	base := t.TempDir()
	runner := &stubCommandRunner{
		stdout: `{"workspace_root":"/tmp/ws","packages":[{"name":"demo","manifest_path":"/tmp/ws/Cargo.toml"},{"name":"helper","manifest_path":"/tmp/ws/helper/Cargo.toml"}]}`,
	}
	tool := NewRustCargoMetadataTool(base)
	tool.SetCommandRunner(runner)

	res, err := tool.Execute(context.Background(), core.NewContext(), map[string]interface{}{
		"working_directory": ".",
	})
	assert.NoError(t, err)
	assert.True(t, res.Success)
	assert.Equal(t, "/tmp/ws", res.Data["workspace_root"])
	assert.Equal(t, 2, res.Data["package_count"])
	assert.Equal(t, []string{"demo", "helper"}, res.Data["package_names"])
	assert.Contains(t, res.Data["summary"], "2 packages")
}
