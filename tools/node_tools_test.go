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

func TestNodeWorkspaceDetectToolFindsNearestProjectRoot(t *testing.T) {
	base := t.TempDir()
	projectRoot := filepath.Join(base, "web", "app")
	targetFile := filepath.Join(projectRoot, "src", "index.js")
	assert.NoError(t, os.MkdirAll(filepath.Dir(targetFile), 0o755))
	assert.NoError(t, os.WriteFile(filepath.Join(projectRoot, "package.json"), []byte(`{"name":"demo"}`), 0o644))
	assert.NoError(t, os.WriteFile(filepath.Join(projectRoot, "package-lock.json"), []byte("{}"), 0o644))
	assert.NoError(t, os.WriteFile(targetFile, []byte("console.log('ok')\n"), 0o644))

	tool := &NodeWorkspaceDetectTool{BasePath: base}
	res, err := tool.Execute(context.Background(), core.NewContext(), map[string]interface{}{"path": targetFile})
	assert.NoError(t, err)
	assert.True(t, res.Success)
	assert.Equal(t, projectRoot, res.Data["project_root"])
	assert.Equal(t, filepath.Join(projectRoot, "package.json"), res.Data["manifest_path"])
	assert.Equal(t, "npm", res.Data["package_manager"])
}

func TestNodeProjectMetadataToolParsesPackageJSON(t *testing.T) {
	base := t.TempDir()
	projectRoot := filepath.Join(base, "frontend")
	assert.NoError(t, os.MkdirAll(projectRoot, 0o755))
	assert.NoError(t, os.WriteFile(filepath.Join(projectRoot, "package.json"), []byte(`{
		"name": "frontend",
		"type": "module",
		"private": true,
		"scripts": {
			"test": "vitest run",
			"build": "vite build",
			"typecheck": "tsc --noEmit"
		}
	}`), 0o644))
	assert.NoError(t, os.WriteFile(filepath.Join(projectRoot, "tsconfig.json"), []byte(`{"compilerOptions":{"strict":true}}`), 0o644))

	tool := &NodeProjectMetadataTool{BasePath: base}
	res, err := tool.Execute(context.Background(), core.NewContext(), map[string]interface{}{"path": projectRoot})
	assert.NoError(t, err)
	assert.True(t, res.Success)
	assert.Equal(t, "frontend", res.Data["project_name"])
	assert.Equal(t, "module", res.Data["package_type"])
	assert.True(t, res.Data["has_test_script"].(bool))
	assert.True(t, res.Data["has_typecheck_script"].(bool))
	assert.True(t, res.Data["is_typescript"].(bool))
}

func TestNodeNPMTestToolReturnsStructuredSummary(t *testing.T) {
	base := t.TempDir()
	runner := &stubCommandRunner{
		stdout: " FAIL  src/math.test.js\n  add\n    expected 2, received 1\n\nTest Suites: 1 failed, 1 total\nTests:       1 failed, 2 passed, 3 total\n",
		err:    errors.New("exit status 1"),
	}
	tool := NewNodeNPMTestTool(base)
	tool.SetCommandRunner(runner)

	res, err := tool.Execute(context.Background(), core.NewContext(), map[string]interface{}{"working_directory": "."})
	assert.NoError(t, err)
	assert.False(t, res.Success)
	assert.Equal(t, 2, res.Data["passed"])
	assert.Equal(t, 1, res.Data["failed"])
	assert.Equal(t, "jest", res.Data["runner"])
	assert.Contains(t, res.Data["summary"], "src/math.test.js")
	assert.Equal(t, "npm", runner.lastReq.Args[0])
}

func TestNodeSyntaxCheckToolReturnsStructuredSummary(t *testing.T) {
	base := t.TempDir()
	runner := &stubCommandRunner{
		stderr: "/tmp/demo/app.js:3\nreturn )\n       ^\n\nSyntaxError: Unexpected token ')'\n",
		err:    errors.New("exit status 1"),
	}
	tool := NewNodeSyntaxCheckTool(base)
	tool.SetCommandRunner(runner)

	res, err := tool.Execute(context.Background(), core.NewContext(), map[string]interface{}{
		"working_directory": ".",
		"path":              "app.js",
	})
	assert.NoError(t, err)
	assert.False(t, res.Success)
	assert.Equal(t, 1, res.Data["error_count"])
	assert.Contains(t, res.Data["summary"], "Unexpected token")
	assert.Equal(t, "node", runner.lastReq.Args[0])
}
