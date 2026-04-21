package shell

import (
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/sandbox"
	platformsqlite "codeburg.org/lexbit/relurpify/platform/db/sqlite"
	platformgo "codeburg.org/lexbit/relurpify/platform/lang/go"
	platformjs "codeburg.org/lexbit/relurpify/platform/lang/js"
	platformpython "codeburg.org/lexbit/relurpify/platform/lang/python"
	platformrust "codeburg.org/lexbit/relurpify/platform/lang/rust"
	cliarchive "codeburg.org/lexbit/relurpify/platform/shell/archive"
	clibuild "codeburg.org/lexbit/relurpify/platform/shell/build"
	"codeburg.org/lexbit/relurpify/platform/shell/catalog"
	clifileops "codeburg.org/lexbit/relurpify/platform/shell/fileops"
	clinetwork "codeburg.org/lexbit/relurpify/platform/shell/network"
	shellquery "codeburg.org/lexbit/relurpify/platform/shell/query"
	clischeduler "codeburg.org/lexbit/relurpify/platform/shell/scheduler"
	clisystem "codeburg.org/lexbit/relurpify/platform/shell/system"
	shelltelemetry "codeburg.org/lexbit/relurpify/platform/shell/telemetry"
	clitext "codeburg.org/lexbit/relurpify/platform/shell/text"
)

// CommandLineTools exposes the default Unix-style CLI helpers.
func CommandLineTools(basePath string, runner sandbox.CommandRunner) []core.Tool {
	return CommandLineToolsWithTelemetry(basePath, runner, nil)
}

// CommandLineToolsWithTelemetry exposes the default Unix-style CLI helpers and emits optional telemetry.
func CommandLineToolsWithTelemetry(basePath string, runner sandbox.CommandRunner, telemetry shelltelemetry.Sink) []core.Tool {
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
			name := catalog.NormalizeName(tool.Name())
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
		name := catalog.NormalizeName(tool.Name())
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		res = append(res, tool)
	}
	if cat := ToolCatalog(); cat != nil {
		for _, tool := range shellquery.ToolsWithTelemetry(cat, telemetry) {
			name := catalog.NormalizeName(tool.Name())
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			res = append(res, tool)
		}
	}
	for i, tool := range res {
		if setter, ok := tool.(interface{ SetCommandRunner(sandbox.CommandRunner) }); ok {
			setter.SetCommandRunner(runner)
			res[i] = tool
		}
	}
	return res
}

// CatalogEntries returns the current shell family catalog in deterministic order.
func CatalogEntries() []catalog.ToolCatalogEntry {
	families := [][]catalog.ToolCatalogEntry{
		clitext.CatalogEntries(),
		clifileops.CatalogEntries(),
		clisystem.CatalogEntries(),
		clibuild.CatalogEntries(),
		cliarchive.CatalogEntries(),
		clinetwork.CatalogEntries(),
		clischeduler.CatalogEntries(),
	}
	seen := make(map[string]struct{})
	var entries []catalog.ToolCatalogEntry
	for _, family := range families {
		for _, entry := range family {
			if _, ok := seen[entry.Name]; ok {
				continue
			}
			seen[entry.Name] = struct{}{}
			entries = append(entries, entry)
		}
	}
	return entries
}

// ToolCatalog builds a canonical catalog from the current shell registry.
func ToolCatalog() *catalog.ToolCatalog {
	cat := catalog.NewToolCatalog()
	for _, entry := range CatalogEntries() {
		_ = cat.Register(entry)
	}
	return cat
}
