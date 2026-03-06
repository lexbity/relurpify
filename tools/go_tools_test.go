package tools

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/stretchr/testify/assert"
)

func TestGoWorkspaceDetectToolFindsNearestModule(t *testing.T) {
	base := t.TempDir()
	moduleRoot := filepath.Join(base, "services", "demo")
	targetFile := filepath.Join(moduleRoot, "pkg", "app.go")
	assert.NoError(t, os.MkdirAll(filepath.Dir(targetFile), 0o755))
	assert.NoError(t, os.WriteFile(filepath.Join(base, "go.work"), []byte("go 1.22\nuse ./services/demo\n"), 0o644))
	assert.NoError(t, os.WriteFile(filepath.Join(moduleRoot, "go.mod"), []byte("module example.com/demo\n\ngo 1.22\n"), 0o644))
	assert.NoError(t, os.WriteFile(targetFile, []byte("package pkg\n"), 0o644))

	tool := &GoWorkspaceDetectTool{BasePath: base}
	res, err := tool.Execute(context.Background(), core.NewContext(), map[string]interface{}{"path": targetFile})
	assert.NoError(t, err)
	assert.True(t, res.Success)
	assert.Equal(t, moduleRoot, res.Data["module_root"])
	assert.Equal(t, filepath.Join(moduleRoot, "go.mod"), res.Data["module_path"])
	assert.Equal(t, filepath.Join(base, "go.work"), res.Data["workspace_path"])
}

func TestGoModuleMetadataToolParsesModuleData(t *testing.T) {
	base := t.TempDir()
	runner := &stubCommandRunner{
		stdout: `{"Path":"example.com/demo","Dir":"/tmp/demo","GoMod":"/tmp/demo/go.mod","GoVersion":"1.22","Main":true}`,
	}
	tool := NewGoModuleMetadataTool(base)
	tool.SetCommandRunner(runner)

	res, err := tool.Execute(context.Background(), core.NewContext(), map[string]interface{}{"working_directory": "."})
	assert.NoError(t, err)
	assert.True(t, res.Success)
	assert.Equal(t, "example.com/demo", res.Data["module_name"])
	assert.Equal(t, "1.22", res.Data["go_version"])
	assert.True(t, res.Data["is_main"].(bool))
}

func TestGoTestToolReturnsStructuredSummary(t *testing.T) {
	base := t.TempDir()
	runner := &stubCommandRunner{
		stdout: "--- FAIL: TestAdd (0.00s)\nFAIL\nFAIL\texample.com/demo/math\t0.003s\nok  \texample.com/demo/util\t0.001s\n",
		err:    errors.New("exit status 1"),
	}
	tool := NewGoTestTool(base)
	tool.SetCommandRunner(runner)

	res, err := tool.Execute(context.Background(), core.NewContext(), map[string]interface{}{"working_directory": "."})
	assert.NoError(t, err)
	assert.False(t, res.Success)
	assert.Equal(t, 1, res.Data["failed"])
	assert.Equal(t, []string{"TestAdd"}, res.Data["failed_tests"])
	assert.Contains(t, res.Data["summary"], "TestAdd")
}

func TestGoBuildToolReturnsStructuredSummary(t *testing.T) {
	base := t.TempDir()
	runner := &stubCommandRunner{
		stderr: "./main.go:12:2: undefined: missingSymbol\n",
		err:    errors.New("exit status 1"),
	}
	tool := NewGoBuildTool(base)
	tool.SetCommandRunner(runner)

	res, err := tool.Execute(context.Background(), core.NewContext(), map[string]interface{}{"working_directory": "."})
	assert.NoError(t, err)
	assert.False(t, res.Success)
	assert.Equal(t, 1, res.Data["error_count"])
	assert.Contains(t, res.Data["summary"], "undefined: missingSymbol")
}
