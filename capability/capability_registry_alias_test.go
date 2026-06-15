package capability

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/descriptor"

	"codeburg.org/lexbit/relurpify/capability/registry"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/model"
)

func TestCapabilityRegistry_DefaultAliases(t *testing.T) {
	reg := registry.NewRegistry()
	desc := descriptor.CapabilityDescriptor{
		ID:            "cap:file_edit",
		Name:          "file_edit",
		Kind:          agentspec.CapabilityKindTool,
		RuntimeFamily: agentspec.CapabilityRuntimeFamilyLocalTool,
	}
	if err := reg.RegisterCapability(context.Background(), desc); err != nil {
		t.Fatalf("register capability: %v", err)
	}

	// Verify file_edit resolves as its own canonical capability.
	resolved, ok := reg.GetCapability("file_edit")
	if !ok {
		t.Fatalf("expected to resolve file_edit capability")
	}
	if resolved.ID != desc.ID {
		t.Fatalf("expected resolved capability ID to be %q, got %q", desc.ID, resolved.ID)
	}

	// Register file_list, exec_run_tests, and file_create capabilities to test default aliases
	descList := descriptor.CapabilityDescriptor{
		ID:            "cap:file_list",
		Name:          "file_list",
		Kind:          agentspec.CapabilityKindTool,
		RuntimeFamily: agentspec.CapabilityRuntimeFamilyLocalTool,
	}
	descTests := descriptor.CapabilityDescriptor{
		ID:            "cap:exec_run_tests",
		Name:          "exec_run_tests",
		Kind:          agentspec.CapabilityKindTool,
		RuntimeFamily: agentspec.CapabilityRuntimeFamilyLocalTool,
	}
	descCreate := descriptor.CapabilityDescriptor{
		ID:            "cap:file_create",
		Name:          "file_create",
		Kind:          agentspec.CapabilityKindTool,
		RuntimeFamily: agentspec.CapabilityRuntimeFamilyLocalTool,
	}
	if err := reg.RegisterCapability(context.Background(), descList); err != nil {
		t.Fatalf("register file_list: %v", err)
	}
	if err := reg.RegisterCapability(context.Background(), descTests); err != nil {
		t.Fatalf("register exec_run_tests: %v", err)
	}
	if err := reg.RegisterCapability(context.Background(), descCreate); err != nil {
		t.Fatalf("register file_create: %v", err)
	}

	// list_dir -> file_list
	resolvedList, ok := reg.GetCapability("list_dir")
	if !ok {
		t.Fatalf("expected to resolve list_dir alias")
	}
	if resolvedList.ID != descList.ID {
		t.Fatalf("expected list_dir resolved ID to be %q, got %q", descList.ID, resolvedList.ID)
	}

	// pytest -> exec_run_tests
	resolvedTests, ok := reg.GetCapability("pytest")
	if !ok {
		t.Fatalf("expected to resolve pytest alias")
	}
	if resolvedTests.ID != descTests.ID {
		t.Fatalf("expected pytest resolved ID to be %q, got %q", descTests.ID, resolvedTests.ID)
	}

	// touch -> file_create
	resolvedTouch, ok := reg.GetCapability("touch")
	if !ok {
		t.Fatalf("expected to resolve touch alias")
	}
	if resolvedTouch.ID != descCreate.ID {
		t.Fatalf("expected touch resolved ID to be %q, got %q", descCreate.ID, resolvedTouch.ID)
	}
}

func TestCapabilityRegistry_ModelProfileAliasesOverride(t *testing.T) {
	reg := registry.NewRegistry()
	desc := descriptor.CapabilityDescriptor{
		ID:            "cap:file_write",
		Name:          "file_write",
		Kind:          agentspec.CapabilityKindTool,
		RuntimeFamily: agentspec.CapabilityRuntimeFamilyLocalTool,
	}
	if err := reg.RegisterCapability(context.Background(), desc); err != nil {
		t.Fatalf("register capability: %v", err)
	}

	// Set a custom ModelProfile that overrides file_edit -> another (non-existent) and custom_edit -> file_write
	profile := &model.ModelProfile{}
	profile.ToolCalling.Aliases = map[string]string{
		"file_edit":   "non_existent",
		"custom_edit": "file_write",
	}
	reg.SetModelProfile(profile)

	// file_edit should now normalize to non_existent and NOT match file_write
	_, ok := reg.GetCapability("file_edit")
	if ok {
		t.Fatalf("expected file_edit to not resolve to file_write due to profile override")
	}

	// custom_edit should normalize to file_write and match successfully
	resolvedCustom, ok := reg.GetCapability("custom_edit")
	if !ok {
		t.Fatalf("expected to resolve custom_edit alias to file_write")
	}
	if resolvedCustom.ID != desc.ID {
		t.Fatalf("expected resolved capability ID to be %q, got %q", desc.ID, resolvedCustom.ID)
	}
}

func TestCapabilityRegistry_ComprehensiveDefaultAliases(t *testing.T) {
	reg := registry.NewRegistry()

	// Register some canonical capabilities
	canonicals := []string{
		"file_read",
		"file_write",
		"file_list",
		"file_search",
		"file_create",
		"file_delete",
		"git_diff",
		"git_history",
		"lsp_get_definition",
		"lsp_get_references",
		"query_ast",
		"exec_run_tests",
		"exec_run_code",
		"search_grep",
		"search_semantic",
	}

	for _, name := range canonicals {
		desc := descriptor.CapabilityDescriptor{
			ID:            "cap:" + name,
			Name:          name,
			Kind:          agentspec.CapabilityKindTool,
			RuntimeFamily: agentspec.CapabilityRuntimeFamilyLocalTool,
		}
		if err := reg.RegisterCapability(context.Background(), desc); err != nil {
			t.Fatalf("register capability %q: %v", name, err)
		}
	}

	// Define test cases for various alias categories
	testCases := []struct {
		alias     string
		canonical string
	}{
		// Files
		{"view_file", "file_read"},
		{"file_content", "file_read"},
		{"str_replace_editor", "file_write"},
		{"multi_replace_file_content", "file_write"},
		{"ls", "file_list"},
		{"scan_dir", "file_list"},
		{"ripgrep", "file_search"},
		{"find_in_files", "file_search"},
		{"touch", "file_create"},
		{"newfile", "file_create"},
		{"rm", "file_delete"},
		{"unlink", "file_delete"},

		// Git
		{"diff", "git_diff"},
		{"git_show_changes", "git_diff"},
		{"git_log", "git_history"},
		{"history", "git_history"},

		// LSP
		{"go_to_definition", "lsp_get_definition"},
		{"lsp_definition", "lsp_get_definition"},
		{"find_refs", "lsp_get_references"},

		// AST
		{"ast_analyze", "query_ast"},
		{"query_structure", "query_ast"},

		// Exec
		{"pytest", "exec_run_tests"},
		{"npm_test", "exec_run_tests"},
		{"python", "exec_run_code"},
		{"run_command", "exec_run_code"},

		// Search
		{"ripgrep_search", "search_grep"},
		{"vector_search", "search_semantic"},
	}

	for _, tc := range testCases {
		resolved, ok := reg.GetCapability(tc.alias)
		if !ok {
			t.Errorf("expected alias %q to resolve to %q", tc.alias, tc.canonical)
			continue
		}
		expectedID := "cap:" + tc.canonical
		if resolved.ID != expectedID {
			t.Errorf("expected alias %q to resolve to ID %q, got %q", tc.alias, expectedID, resolved.ID)
		}
	}
}
