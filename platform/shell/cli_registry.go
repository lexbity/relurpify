package shell

import (
	"codeburg.org/lexbit/relurpify/platform/contracts"
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
func CommandLineTools(basePath string, runner contracts.CommandRunner, registry contracts.ToolRegistry) []contracts.Tool {
	return CommandLineToolsWithTelemetry(basePath, runner, registry, nil)
}

// CommandLineToolsWithTelemetry exposes the default Unix-style CLI helpers and emits optional telemetry.
func CommandLineToolsWithTelemetry(basePath string, runner contracts.CommandRunner, registry contracts.ToolRegistry, telemetry shelltelemetry.Sink) []contracts.Tool {
	allowed := make(map[string]contracts.ToolManifest)
	if registry != nil {
		tools := registry.ListTools()
		allowed = make(map[string]contracts.ToolManifest, len(tools))
		for _, manifest := range tools {
			name := contracts.NormalizeToolName(manifest.Name)
			if name == "" {
				continue
			}
			allowed[name] = manifest
		}
	}
	sourceGroups := [][]contracts.Tool{
		clitext.Tools(basePath),
		clifileops.Tools(basePath),
		clisystem.Tools(basePath),
		clibuild.Tools(basePath),
		cliarchive.Tools(basePath),
		clinetwork.Tools(basePath),
		clischeduler.Tools(basePath),
	}
	seen := make(map[string]struct{})
	var res []contracts.Tool
	for _, group := range sourceGroups {
		for _, tool := range group {
			name := catalog.NormalizeName(tool.Name())
			if _, ok := allowed[name]; !ok {
				continue
			}
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			res = append(res, tool)
		}
	}
	for _, tool := range []contracts.Tool{
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
		if _, ok := allowed[name]; !ok {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		res = append(res, tool)
	}
	if cat := ToolCatalog(registry); cat != nil {
		for _, tool := range shellquery.ToolsWithTelemetry(cat, telemetry) {
			name := catalog.NormalizeName(tool.Name())
			if _, ok := allowed[name]; !ok {
				continue
			}
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			res = append(res, tool)
		}
	}
	for i, tool := range res {
		if setter, ok := tool.(interface{ SetCommandRunner(contracts.CommandRunner) }); ok {
			setter.SetCommandRunner(runner)
			res[i] = tool
		}
	}
	return res
}

// CatalogEntries returns the current shell family catalog in deterministic order.
func CatalogEntries(registry contracts.ToolRegistry) []catalog.ToolCatalogEntry {
	seen := make(map[string]struct{})
	var entries []catalog.ToolCatalogEntry
	if registry == nil {
		return entries
	}
	for _, manifest := range registry.ListTools() {
		entry := catalog.EntryFromManifest(manifest)
		if entry.Name == "" {
			continue
		}
		if _, ok := seen[entry.Name]; ok {
			continue
		}
		seen[entry.Name] = struct{}{}
		entries = append(entries, entry)
	}
	return entries
}

// ToolCatalog builds a canonical catalog from the current shell registry.
func ToolCatalog(registry contracts.ToolRegistry) *catalog.ToolCatalog {
	cat := catalog.NewToolCatalog()
	for _, entry := range CatalogEntries(registry) {
		_ = cat.Register(entry)
	}
	return cat
}
