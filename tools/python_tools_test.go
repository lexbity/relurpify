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

func TestPythonWorkspaceDetectToolFindsNearestProjectRoot(t *testing.T) {
	base := t.TempDir()
	projectRoot := filepath.Join(base, "services", "demo")
	targetFile := filepath.Join(projectRoot, "pkg", "app.py")
	assert.NoError(t, os.MkdirAll(filepath.Dir(targetFile), 0o755))
	assert.NoError(t, os.WriteFile(filepath.Join(projectRoot, "pyproject.toml"), []byte("[project]\nname = \"demo\"\n"), 0o644))
	assert.NoError(t, os.WriteFile(filepath.Join(projectRoot, "requirements.txt"), []byte("pytest\n"), 0o644))
	assert.NoError(t, os.WriteFile(targetFile, []byte("print('ok')\n"), 0o644))

	tool := &PythonWorkspaceDetectTool{BasePath: base}
	res, err := tool.Execute(context.Background(), core.NewContext(), map[string]interface{}{
		"path": targetFile,
	})
	assert.NoError(t, err)
	assert.True(t, res.Success)
	assert.Equal(t, projectRoot, res.Data["project_root"])
	assert.Equal(t, filepath.Join(projectRoot, "pyproject.toml"), res.Data["manifest_path"])
	assert.Contains(t, res.Data["marker_files"], "requirements.txt")
}

func TestPythonProjectMetadataToolParsesPyprojectAndPrefersPytest(t *testing.T) {
	base := t.TempDir()
	projectRoot := filepath.Join(base, "app")
	assert.NoError(t, os.MkdirAll(projectRoot, 0o755))
	assert.NoError(t, os.WriteFile(filepath.Join(projectRoot, "pyproject.toml"), []byte("[project]\nname = \"sample\"\nrequires-python = \">=3.11\"\n\n[tool.pytest.ini_options]\naddopts = \"-q\"\n"), 0o644))
	assert.NoError(t, os.WriteFile(filepath.Join(projectRoot, "requirements.txt"), []byte("pytest\n"), 0o644))

	tool := &PythonProjectMetadataTool{BasePath: base}
	res, err := tool.Execute(context.Background(), core.NewContext(), map[string]interface{}{
		"path": projectRoot,
	})
	assert.NoError(t, err)
	assert.True(t, res.Success)
	assert.Equal(t, "sample", res.Data["project_name"])
	assert.Equal(t, ">=3.11", res.Data["requires_python"])
	assert.Equal(t, "python_pytest", res.Data["preferred_test_tool"])
	assert.True(t, res.Data["has_pytest_config"].(bool))
}

func TestPythonPytestToolReturnsStructuredSummary(t *testing.T) {
	base := t.TempDir()
	runner := &stubCommandRunner{
		stdout: "FAILED tests/test_math.py::test_add - assert 1 == 2\n=========================== short test summary info ============================\n1 failed, 2 passed in 0.12s\n",
		err:    errors.New("exit status 1"),
	}
	tool := NewPythonPytestTool(base)
	tool.SetCommandRunner(runner)

	res, err := tool.Execute(context.Background(), core.NewContext(), map[string]interface{}{
		"working_directory": ".",
	})
	assert.NoError(t, err)
	assert.False(t, res.Success)
	assert.Equal(t, 2, res.Data["passed"])
	assert.Equal(t, 1, res.Data["failed"])
	assert.Contains(t, res.Data["summary"], "tests/test_math.py::test_add")
	assert.Equal(t, "python3", runner.lastReq.Args[0])
}

func TestPythonUnittestToolReturnsStructuredSummary(t *testing.T) {
	base := t.TempDir()
	runner := &stubCommandRunner{
		stdout: "F.\n======================================================================\nFAIL: test_add (tests.test_math.MathTests.test_add)\n----------------------------------------------------------------------\nRan 2 tests in 0.001s\n\nFAILED (failures=1)\n",
		err:    errors.New("exit status 1"),
	}
	tool := NewPythonUnittestTool(base)
	tool.SetCommandRunner(runner)

	res, err := tool.Execute(context.Background(), core.NewContext(), map[string]interface{}{
		"working_directory": ".",
	})
	assert.NoError(t, err)
	assert.False(t, res.Success)
	assert.Equal(t, 1, res.Data["passed"])
	assert.Equal(t, 1, res.Data["failed"])
	assert.Contains(t, res.Data["summary"], "test_add")
}

func TestPythonCompileCheckToolReturnsStructuredSummary(t *testing.T) {
	base := t.TempDir()
	runner := &stubCommandRunner{
		stderr: "*** Error compiling './app.py'...\n  File \"./app.py\", line 3\n    return )\n           ^\nSyntaxError: unmatched ')'\n",
		err:    errors.New("exit status 1"),
	}
	tool := NewPythonCompileCheckTool(base)
	tool.SetCommandRunner(runner)

	res, err := tool.Execute(context.Background(), core.NewContext(), map[string]interface{}{
		"working_directory": ".",
		"target":            ".",
	})
	assert.NoError(t, err)
	assert.False(t, res.Success)
	assert.Equal(t, 1, res.Data["error_count"])
	assert.Contains(t, res.Data["summary"], "SyntaxError")
}
