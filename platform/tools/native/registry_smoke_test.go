package tools_native_test

import (
	"sort"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/ports"
	_ "codeburg.org/lexbit/relurpify/platform/tools/native/fs"
	_ "codeburg.org/lexbit/relurpify/platform/tools/native/git"
	_ "codeburg.org/lexbit/relurpify/platform/tools/native/langgo"
	_ "codeburg.org/lexbit/relurpify/platform/tools/native/langjs"
	_ "codeburg.org/lexbit/relurpify/platform/tools/native/langpython"
	_ "codeburg.org/lexbit/relurpify/platform/tools/native/langrust"
	_ "codeburg.org/lexbit/relurpify/platform/tools/native/lsp"
	_ "codeburg.org/lexbit/relurpify/platform/tools/native/search"
	_ "codeburg.org/lexbit/relurpify/platform/tools/native/sqlite"
)

// expectedNativeKeys lists every go_native tool key that should be registered
// at startup. This set must be kept in sync with the go_native tool manifests
// in relurpify_cfg/tools/ and the tool constructors in capability_bundle.go.
var expectedNativeKeys = []string{
	"file_create",
	"file_delete",
	"file_list",
	"file_read",
	"file_search",
	"file_write",
	"git_blame",
	"git_branch",
	"git_commit",
	"git_diff",
	"git_history",
	"go_build",
	"go_module_metadata",
	"go_test",
	"go_workspace_detect",
	"node_npm_test",
	"node_project_metadata",
	"node_syntax_check",
	"node_workspace_detect",
	"python_compile_check",
	"python_project_metadata",
	"python_pytest",
	"python_unittest",
	"python_workspace_detect",
	"rust_cargo_check",
	"rust_cargo_metadata",
	"rust_cargo_test",
	"rust_workspace_detect",
	"search_find_similar",
	"search_grep",
	"search_semantic",
	"sqlite_database_detect",
	"sqlite_integrity_check",
	"sqlite_query",
	"sqlite_schema_inspect",
	"lsp_document_symbols",
	"lsp_format",
	"lsp_get_definition",
	"lsp_get_diagnostics",
	"lsp_get_hover",
	"lsp_get_references",
	"lsp_search_symbols",
}

func TestAllNativeKeysRegistered(t *testing.T) {
	got := ports.NativeKeys()

	gotSet := make(map[string]struct{}, len(got))
	for _, k := range got {
		gotSet[k] = struct{}{}
	}

	expected := make([]string, len(expectedNativeKeys))
	copy(expected, expectedNativeKeys)
	sort.Strings(expected)

	for _, k := range expected {
		if _, ok := gotSet[k]; !ok {
			t.Errorf("expected native key %q not registered", k)
		}
	}

	// Check for unexpected keys
	for _, k := range got {
		found := false
		for _, ek := range expected {
			if k == ek {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("unexpected native key %q registered", k)
		}
	}

	if t.Failed() {
		t.Logf("got keys: %v", got)
		t.Logf("expected: %v", expected)
	}
}

func TestEachConstructorReturnsWorkingTool(t *testing.T) {
	for _, key := range ports.NativeKeys() {
		ctor, ok := ports.LookupNative(key)
		if !ok {
			t.Errorf("NativeKeys() includes %q but LookupNative returns false", key)
			continue
		}
		tool := ctor("/tmp/workspace")
		if tool == nil {
			t.Errorf("constructor for %q returned nil", key)
			continue
		}
		if tool.Name() == "" {
			t.Errorf("tool %q has empty Name()", key)
		}
		if tool.Description() == "" {
			t.Errorf("tool %q has empty Description()", key)
		}
	}
}
