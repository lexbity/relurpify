package tools

import (
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/runtime"
	cliarchive "github.com/lexcodex/relurpify/tools/cli_nix/archive"
	clibuild "github.com/lexcodex/relurpify/tools/cli_nix/build"
	clifileops "github.com/lexcodex/relurpify/tools/cli_nix/fileops"
	clinetwork "github.com/lexcodex/relurpify/tools/cli_nix/network"
	clischeduler "github.com/lexcodex/relurpify/tools/cli_nix/scheduler"
	clisystem "github.com/lexcodex/relurpify/tools/cli_nix/system"
	clitext "github.com/lexcodex/relurpify/tools/cli_nix/text"
)

// CommandLineTools exposes the default Unix-style CLI helpers.
func CommandLineTools(basePath string, runner runtime.CommandRunner) []core.Tool {
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
		&RustWorkspaceDetectTool{BasePath: basePath},
		NewRustCargoMetadataTool(basePath),
		NewRustCargoCheckTool(basePath),
		NewRustCargoTestTool(basePath),
		&PythonWorkspaceDetectTool{BasePath: basePath},
		&PythonProjectMetadataTool{BasePath: basePath},
		NewPythonCompileCheckTool(basePath),
		NewPythonPytestTool(basePath),
		NewPythonUnittestTool(basePath),
		&NodeWorkspaceDetectTool{BasePath: basePath},
		&NodeProjectMetadataTool{BasePath: basePath},
		NewNodeNPMTestTool(basePath),
		NewNodeSyntaxCheckTool(basePath),
		&GoWorkspaceDetectTool{BasePath: basePath},
		NewGoModuleMetadataTool(basePath),
		NewGoTestTool(basePath),
		NewGoBuildTool(basePath),
		&SQLiteDatabaseDetectTool{BasePath: basePath},
		NewSQLiteSchemaInspectTool(basePath),
		NewSQLiteQueryTool(basePath),
		NewSQLiteIntegrityCheckTool(basePath),
	} {
		name := tool.Name()
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		res = append(res, tool)
	}
	for i, tool := range res {
		if setter, ok := tool.(interface{ SetCommandRunner(runtime.CommandRunner) }); ok {
			setter.SetCommandRunner(runner)
			res[i] = tool
		}
	}
	return res
}
