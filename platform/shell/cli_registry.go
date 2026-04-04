package shell

import (
	"path/filepath"

	"github.com/lexcodex/relurpify/framework/config"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/sandbox"
	platformsqlite "github.com/lexcodex/relurpify/platform/db/sqlite"
	platformgo "github.com/lexcodex/relurpify/platform/lang/go"
	platformjs "github.com/lexcodex/relurpify/platform/lang/js"
	platformpython "github.com/lexcodex/relurpify/platform/lang/python"
	platformrust "github.com/lexcodex/relurpify/platform/lang/rust"
	cliarchive "github.com/lexcodex/relurpify/platform/shell/archive"
	clibuild "github.com/lexcodex/relurpify/platform/shell/build"
	clifileops "github.com/lexcodex/relurpify/platform/shell/fileops"
	clinetwork "github.com/lexcodex/relurpify/platform/shell/network"
	clischeduler "github.com/lexcodex/relurpify/platform/shell/scheduler"
	clisystem "github.com/lexcodex/relurpify/platform/shell/system"
	clitext "github.com/lexcodex/relurpify/platform/shell/text"
)

// CommandLineTools exposes the default Unix-style CLI helpers.
func CommandLineTools(basePath string, runner sandbox.CommandRunner) []core.Tool {
	sourceGroups := [][]core.Tool{
		clitext.Tools(basePath),
		clifileops.Tools(basePath),
		clisystem.Tools(basePath),
		clibuild.Tools(basePath),
		cliarchive.Tools(basePath),
		clinetwork.Tools(basePath),
		clischeduler.Tools(basePath),
	}
	seen := make(map[string]struct{})
	var res []core.Tool
	for _, group := range sourceGroups {
		for _, tool := range group {
			name := tool.Name()
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			res = append(res, tool)
		}
	}
	for _, tool := range []core.Tool{
		&platformrust.RustWorkspaceDetectTool{BasePath: basePath},
		platformrust.NewRustCargoMetadataTool(basePath),
		platformrust.NewRustCargoCheckTool(basePath),
		platformrust.NewRustCargoTestTool(basePath),
		&platformpython.PythonWorkspaceDetectTool{BasePath: basePath},
		&platformpython.PythonProjectMetadataTool{BasePath: basePath},
		platformpython.NewPythonCompileCheckTool(basePath),
		platformpython.NewPythonPytestTool(basePath),
		platformpython.NewPythonUnittestTool(basePath),
		&platformjs.NodeWorkspaceDetectTool{BasePath: basePath},
		&platformjs.NodeProjectMetadataTool{BasePath: basePath},
		platformjs.NewNodeNPMTestTool(basePath),
		platformjs.NewNodeSyntaxCheckTool(basePath),
		&platformgo.GoWorkspaceDetectTool{BasePath: basePath},
		platformgo.NewGoModuleMetadataTool(basePath),
		platformgo.NewGoTestTool(basePath),
		platformgo.NewGoBuildTool(basePath),
		&platformsqlite.SQLiteDatabaseDetectTool{BasePath: basePath},
		platformsqlite.NewSQLiteSchemaInspectTool(basePath),
		platformsqlite.NewSQLiteQueryTool(basePath),
		platformsqlite.NewSQLiteIntegrityCheckTool(basePath),
	} {
		name := tool.Name()
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		res = append(res, tool)
	}
	// Load shell bindings and create tools for them
	bindingsPath := filepath.Join(config.New(basePath).SkillsDir(), "shell_bindings.yaml")
	bindings, err := LoadShellBindings(bindingsPath)
	if err != nil {
		// Log error? For now, just ignore
	}
	// Determine allowed binaries from runner's permission set
	// For simplicity, we'll assume all binaries are allowed for now
	// In a real implementation, we would query the capability registry
	allowedBinaries := []string{} // Placeholder
	query := NewCommandQuery(allowedBinaries, bindings)
	for _, binding := range bindings {
		name := binding.Name
		if _, ok := seen[name]; ok {
			// Skip if name conflicts with built-in tool
			continue
		}
		// Create a tool from the binding
		tool := NewShellBindingTool(binding, query, runner)
		res = append(res, tool)
		seen[name] = struct{}{}
	}
	for i, tool := range res {
		if setter, ok := tool.(interface{ SetCommandRunner(sandbox.CommandRunner) }); ok {
			setter.SetCommandRunner(runner)
			res[i] = tool
		}
	}
	return res
}
