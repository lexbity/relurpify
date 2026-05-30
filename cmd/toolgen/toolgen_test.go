package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenerateSearchGrep(t *testing.T) {
	manifest := filepath.Join(testdataDir(t), "search_grep.tool.yaml")
	ensureTestManifest(t, manifest, `schema: relurpify/tool/v1
name: search_grep
family: search
description: Searches files using substring matching.
parameters:
  - name: pattern
    type: string
    required: true
  - name: directory
    type: string
    required: false
    default: .
execution:
  backend: go_native
  implementation: search_grep
capability:
  trust_class: builtin_trusted
  risk_class: [read_only]
  effect_class: [filesystem_read]
`)

	manifestData, err := loadManifest(manifest)
	require.NoError(t, err)

	src, err := generate(manifestData)
	require.NoError(t, err)
	require.Contains(t, string(src), "type SearchGrepParams struct")
	require.Contains(t, string(src), "Pattern   string")
	require.Contains(t, string(src), "Directory string")
	require.Contains(t, string(src), "ParseSearchGrepParams")
	require.Contains(t, string(src), "SearchGrepParamKeys")
}

func TestGenerateGoTest(t *testing.T) {
	manifest := filepath.Join(testdataDir(t), "go_test.tool.yaml")
	ensureTestManifest(t, manifest, `schema: relurpify/tool/v1
name: go_test
family: build
description: Runs Go tests.
parameters:
  - name: working_directory
    type: string
    required: false
    description: Directory to run tests in.
execution:
  backend: go_native
  implementation: go_test
capability:
  trust_class: builtin_trusted
  risk_class: [execute]
  effect_class: [process_spawn]
`)

	manifestData, err := loadManifest(manifest)
	require.NoError(t, err)

	src, err := generate(manifestData)
	require.NoError(t, err)
	require.Contains(t, string(src), "GoTestParams")
	require.Contains(t, string(src), "WorkingDirectory string")
	require.Contains(t, string(src), `json:"working_directory,omitempty"`)
}

func TestGenerateFileRead(t *testing.T) {
	manifest := filepath.Join(testdataDir(t), "file_read.tool.yaml")
	ensureTestManifest(t, manifest, `schema: relurpify/tool/v1
name: file_read
family: file
description: Reads a file.
parameters:
  - name: path
    type: string
    required: true
execution:
  backend: go_native
  implementation: file_read
capability:
  trust_class: builtin_trusted
  risk_class: [read_only]
  effect_class: [filesystem_read]
`)

	manifestData, err := loadManifest(manifest)
	require.NoError(t, err)

	src, err := generate(manifestData)
	require.NoError(t, err)
	require.Contains(t, string(src), "FileReadParams")
	require.Contains(t, string(src), "Path string")
	require.Contains(t, string(src), `json:"path"`)
}

func testdataDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "toolgentest")
	err := os.MkdirAll(dir, 0o755)
	require.NoError(t, err)
	return dir
}

func ensureTestManifest(t *testing.T, path, content string) {
	t.Helper()
	err := os.WriteFile(path, []byte(content), 0o644)
	require.NoError(t, err)
}
