package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/lexcodex/relurpify/framework/authorization"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/sandbox"
	clinix "github.com/lexcodex/relurpify/platform/shell/command"
)

var sqliteExtensions = []string{".db", ".sqlite", ".sqlite3"}

type SQLiteDatabaseDetectTool struct {
	BasePath string
	manager  *authorization.PermissionManager
	agentID  string
}

func (t *SQLiteDatabaseDetectTool) Name() string { return "sqlite_database_detect" }
func (t *SQLiteDatabaseDetectTool) Description() string {
	return "Detects a SQLite database file for a file or directory path."
}
func (t *SQLiteDatabaseDetectTool) Category() string { return "sqlite" }
func (t *SQLiteDatabaseDetectTool) Parameters() []core.ToolParameter {
	return []core.ToolParameter{{Name: "path", Type: "string", Required: false, Default: "."}}
}
func (t *SQLiteDatabaseDetectTool) SetPermissionManager(manager *authorization.PermissionManager, agentID string) {
	t.manager = manager
	t.agentID = agentID
}
func (t *SQLiteDatabaseDetectTool) Execute(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.ToolResult, error) {
	start := "."
	if raw, ok := args["path"]; ok && raw != nil {
		start = strings.TrimSpace(fmt.Sprint(raw))
		if start == "" || start == "<nil>" {
			start = "."
		}
	}
	resolved := start
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(t.BasePath, resolved)
	}
	resolved = filepath.Clean(resolved)
	if t.manager != nil {
		if err := t.manager.CheckFileAccess(ctx, t.agentID, core.FileSystemRead, resolved); err != nil {
			return nil, err
		}
	}
	dbPath := findSQLiteDatabase(resolved, t.BasePath)
	if dbPath == "" {
		return &core.ToolResult{Success: false, Error: "no SQLite database found"}, nil
	}
	return &core.ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"path":          resolved,
			"database_path": dbPath,
			"summary":       fmt.Sprintf("SQLite database detected at %s", dbPath),
		},
	}, nil
}
func (t *SQLiteDatabaseDetectTool) IsAvailable(ctx context.Context, state *core.Context) bool {
	return true
}
func (t *SQLiteDatabaseDetectTool) Permissions() core.ToolPermissions {
	return core.ToolPermissions{Permissions: core.NewFileSystemPermissionSet(t.BasePath, core.FileSystemRead)}
}
func (t *SQLiteDatabaseDetectTool) Tags() []string {
	return []string{core.TagReadOnly, "lang:sqlite", "workspace-detect", "recovery"}
}

type SQLiteSchemaInspectTool struct {
	BasePath string
	inner    *clinix.CommandTool
}

func NewSQLiteSchemaInspectTool(basePath string) *SQLiteSchemaInspectTool {
	return &SQLiteSchemaInspectTool{
		BasePath: basePath,
		inner: clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
			Name:        "sqlite_schema_inspect",
			Description: "Inspects SQLite schema and returns structured table/index metadata.",
			Command:     "sqlite3",
			Category:    "sqlite",
			Tags:        []string{core.TagExecute},
		}),
	}
}

func (t *SQLiteSchemaInspectTool) Name() string { return "sqlite_schema_inspect" }
func (t *SQLiteSchemaInspectTool) Description() string {
	return "Inspects SQLite schema and returns structured table/index metadata."
}
func (t *SQLiteSchemaInspectTool) Category() string { return "sqlite" }
func (t *SQLiteSchemaInspectTool) Parameters() []core.ToolParameter {
	return []core.ToolParameter{{Name: "database_path", Type: "string", Required: true}}
}
func (t *SQLiteSchemaInspectTool) SetCommandRunner(r sandbox.CommandRunner) {
	t.inner.SetCommandRunner(r)
}
func (t *SQLiteSchemaInspectTool) SetPermissionManager(manager *authorization.PermissionManager, agentID string) {
	t.inner.SetPermissionManager(manager, agentID)
}
func (t *SQLiteSchemaInspectTool) SetAgentSpec(spec *core.AgentRuntimeSpec, agentID string) {
	t.inner.SetAgentSpec(spec, agentID)
}
func (t *SQLiteSchemaInspectTool) Execute(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.ToolResult, error) {
	dbPath := strings.TrimSpace(fmt.Sprint(args["database_path"]))
	if dbPath == "" || dbPath == "<nil>" {
		return &core.ToolResult{Success: false, Error: "database_path is required"}, nil
	}
	result, err := t.inner.Execute(ctx, state, map[string]interface{}{
		"args": []interface{}{
			dbPath,
			"-json",
			"SELECT type, name, tbl_name, sql FROM sqlite_master WHERE type IN ('table','index','view','trigger') AND name NOT LIKE 'sqlite_%' ORDER BY type, name;",
		},
	})
	if err != nil || result == nil {
		return result, err
	}
	stdout := fmt.Sprint(result.Data["stdout"])
	stderr := fmt.Sprint(result.Data["stderr"])
	summary, parsed := parseSQLiteSchema(stdout)
	data := map[string]interface{}{
		"summary": summary,
		"stdout":  stdout,
		"stderr":  stderr,
	}
	for key, value := range parsed {
		data[key] = value
	}
	return &core.ToolResult{Success: result.Success, Error: result.Error, Data: data, Metadata: result.Metadata}, nil
}
func (t *SQLiteSchemaInspectTool) IsAvailable(ctx context.Context, state *core.Context) bool {
	return t.inner.IsAvailable(ctx, state)
}
func (t *SQLiteSchemaInspectTool) Permissions() core.ToolPermissions { return t.inner.Permissions() }
func (t *SQLiteSchemaInspectTool) Tags() []string {
	return []string{core.TagExecute, "lang:sqlite", "schema", "verification", "recovery"}
}

type SQLiteQueryTool struct {
	BasePath string
	inner    *clinix.CommandTool
}

func NewSQLiteQueryTool(basePath string) *SQLiteQueryTool {
	return &SQLiteQueryTool{
		BasePath: basePath,
		inner: clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
			Name:        "sqlite_query",
			Description: "Executes a SQLite query and returns structured row output.",
			Command:     "sqlite3",
			Category:    "sqlite",
			Tags:        []string{core.TagExecute},
		}),
	}
}

func (t *SQLiteQueryTool) Name() string { return "sqlite_query" }
func (t *SQLiteQueryTool) Description() string {
	return "Executes a SQLite query and returns structured row output."
}
func (t *SQLiteQueryTool) Category() string { return "sqlite" }
func (t *SQLiteQueryTool) Parameters() []core.ToolParameter {
	return []core.ToolParameter{
		{Name: "database_path", Type: "string", Required: true},
		{Name: "query", Type: "string", Required: true},
	}
}
func (t *SQLiteQueryTool) SetCommandRunner(r sandbox.CommandRunner) { t.inner.SetCommandRunner(r) }
func (t *SQLiteQueryTool) SetPermissionManager(manager *authorization.PermissionManager, agentID string) {
	t.inner.SetPermissionManager(manager, agentID)
}
func (t *SQLiteQueryTool) SetAgentSpec(spec *core.AgentRuntimeSpec, agentID string) {
	t.inner.SetAgentSpec(spec, agentID)
}
func (t *SQLiteQueryTool) Execute(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.ToolResult, error) {
	dbPath := strings.TrimSpace(fmt.Sprint(args["database_path"]))
	query := strings.TrimSpace(fmt.Sprint(args["query"]))
	if dbPath == "" || dbPath == "<nil>" || query == "" || query == "<nil>" {
		return &core.ToolResult{Success: false, Error: "database_path and query are required"}, nil
	}
	result, err := t.inner.Execute(ctx, state, map[string]interface{}{
		"args": []interface{}{dbPath, "-json", query},
	})
	if err != nil || result == nil {
		return result, err
	}
	stdout := fmt.Sprint(result.Data["stdout"])
	stderr := fmt.Sprint(result.Data["stderr"])
	rows := parseJSONArray(stdout)
	return &core.ToolResult{
		Success: result.Success,
		Error:   result.Error,
		Data: map[string]interface{}{
			"summary":   fmt.Sprintf("sqlite query returned %d rows", len(rows)),
			"row_count": len(rows),
			"rows":      rows,
			"stdout":    stdout,
			"stderr":    stderr,
			"query":     query,
			"database":  dbPath,
		},
		Metadata: result.Metadata,
	}, nil
}
func (t *SQLiteQueryTool) IsAvailable(ctx context.Context, state *core.Context) bool {
	return t.inner.IsAvailable(ctx, state)
}
func (t *SQLiteQueryTool) Permissions() core.ToolPermissions { return t.inner.Permissions() }
func (t *SQLiteQueryTool) Tags() []string {
	return []string{core.TagExecute, "lang:sqlite", "query", "verification"}
}

type SQLiteIntegrityCheckTool struct {
	BasePath string
	inner    *clinix.CommandTool
}

func NewSQLiteIntegrityCheckTool(basePath string) *SQLiteIntegrityCheckTool {
	return &SQLiteIntegrityCheckTool{
		BasePath: basePath,
		inner: clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
			Name:        "sqlite_integrity_check",
			Description: "Runs PRAGMA integrity_check on a SQLite database.",
			Command:     "sqlite3",
			Category:    "sqlite",
			Tags:        []string{core.TagExecute},
		}),
	}
}

func (t *SQLiteIntegrityCheckTool) Name() string { return "sqlite_integrity_check" }
func (t *SQLiteIntegrityCheckTool) Description() string {
	return "Runs PRAGMA integrity_check on a SQLite database."
}
func (t *SQLiteIntegrityCheckTool) Category() string { return "sqlite" }
func (t *SQLiteIntegrityCheckTool) Parameters() []core.ToolParameter {
	return []core.ToolParameter{{Name: "database_path", Type: "string", Required: true}}
}
func (t *SQLiteIntegrityCheckTool) SetCommandRunner(r sandbox.CommandRunner) {
	t.inner.SetCommandRunner(r)
}
func (t *SQLiteIntegrityCheckTool) SetPermissionManager(manager *authorization.PermissionManager, agentID string) {
	t.inner.SetPermissionManager(manager, agentID)
}
func (t *SQLiteIntegrityCheckTool) SetAgentSpec(spec *core.AgentRuntimeSpec, agentID string) {
	t.inner.SetAgentSpec(spec, agentID)
}
func (t *SQLiteIntegrityCheckTool) Execute(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.ToolResult, error) {
	dbPath := strings.TrimSpace(fmt.Sprint(args["database_path"]))
	if dbPath == "" || dbPath == "<nil>" {
		return &core.ToolResult{Success: false, Error: "database_path is required"}, nil
	}
	result, err := t.inner.Execute(ctx, state, map[string]interface{}{
		"args": []interface{}{dbPath, "PRAGMA integrity_check;"},
	})
	if err != nil || result == nil {
		return result, err
	}
	stdout := strings.TrimSpace(fmt.Sprint(result.Data["stdout"]))
	stderr := fmt.Sprint(result.Data["stderr"])
	ok := strings.EqualFold(stdout, "ok")
	summary := "sqlite integrity check completed"
	if ok {
		summary = "sqlite integrity check passed"
	} else if stdout != "" {
		summary = "sqlite integrity check failed: " + firstNonEmptyLine(stdout)
	}
	return &core.ToolResult{
		Success: result.Success && ok,
		Error:   result.Error,
		Data: map[string]interface{}{
			"summary":  summary,
			"ok":       ok,
			"stdout":   stdout,
			"stderr":   stderr,
			"database": dbPath,
		},
		Metadata: result.Metadata,
	}, nil
}
func (t *SQLiteIntegrityCheckTool) IsAvailable(ctx context.Context, state *core.Context) bool {
	return t.inner.IsAvailable(ctx, state)
}
func (t *SQLiteIntegrityCheckTool) Permissions() core.ToolPermissions { return t.inner.Permissions() }
func (t *SQLiteIntegrityCheckTool) Tags() []string {
	return []string{core.TagExecute, "lang:sqlite", "integrity-check", "verification", "diagnostics"}
}

func findSQLiteDatabase(start, basePath string) string {
	info, err := os.Stat(start)
	if err == nil && !info.IsDir() && isSQLitePath(start) {
		return start
	}
	searchDir := start
	if err == nil && !info.IsDir() {
		searchDir = filepath.Dir(start)
	}
	basePath = filepath.Clean(basePath)
	current := filepath.Clean(searchDir)
	for {
		entries, readErr := os.ReadDir(current)
		if readErr == nil {
			candidates := make([]string, 0)
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				name := entry.Name()
				if isSQLitePath(name) {
					candidates = append(candidates, filepath.Join(current, name))
				}
			}
			sort.Strings(candidates)
			if len(candidates) > 0 {
				return candidates[0]
			}
		}
		if current == basePath {
			break
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return ""
}

func isSQLitePath(path string) bool {
	lower := strings.ToLower(path)
	for _, ext := range sqliteExtensions {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

func parseSQLiteSchema(stdout string) (string, map[string]interface{}) {
	rows := parseJSONArray(stdout)
	tables := make([]string, 0)
	indexes := make([]string, 0)
	views := make([]string, 0)
	triggers := make([]string, 0)
	for _, row := range rows {
		typ := strings.TrimSpace(fmt.Sprint(row["type"]))
		name := strings.TrimSpace(fmt.Sprint(row["name"]))
		switch typ {
		case "table":
			tables = append(tables, name)
		case "index":
			indexes = append(indexes, name)
		case "view":
			views = append(views, name)
		case "trigger":
			triggers = append(triggers, name)
		}
	}
	summary := fmt.Sprintf("sqlite schema: %d tables, %d indexes, %d views, %d triggers", len(tables), len(indexes), len(views), len(triggers))
	return summary, map[string]interface{}{
		"table_names":   tables,
		"index_names":   indexes,
		"view_names":    views,
		"trigger_names": triggers,
		"object_count":  len(rows),
		"objects":       rows,
	}
}

func parseJSONArray(stdout string) []map[string]interface{} {
	text := strings.TrimSpace(stdout)
	if text == "" {
		return nil
	}
	var rows []map[string]interface{}
	if err := json.Unmarshal([]byte(text), &rows); err == nil {
		return rows
	}
	return nil
}
