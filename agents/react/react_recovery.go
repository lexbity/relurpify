package react

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/lexcodex/relurpify/framework/core"
)

func recoveryProbeArgs(agent *ReActAgent, toolName string, state *core.Context, task *core.Task, lastMap map[string]interface{}) map[string]interface{} {
	if agent == nil || agent.Tools == nil {
		return nil
	}
	tool, ok := agent.Tools.Get(toolName)
	if !ok || tool == nil {
		return nil
	}
	switch toolName {
	case "file_read":
		if path := primaryFailurePath(state, lastMap); path != "" {
			return map[string]interface{}{"path": path}
		}
		return nil
	case "search_grep", "file_search":
		pattern := primaryFailureSearchPattern(lastMap)
		if pattern == "" {
			return nil
		}
		return map[string]interface{}{
			"directory": primaryFailureDirectory(state, lastMap),
			"pattern":   pattern,
		}
	case "query_ast":
		if symbol := inferFailureSymbol(lastMap); symbol != "" {
			return map[string]interface{}{"action": "get_signature", "symbol": symbol}
		}
		return map[string]interface{}{"action": "list_symbols", "category": "function"}
	}

	args := make(map[string]interface{})
	params := tool.Parameters()
	required := map[string]bool{}
	for _, param := range params {
		name := strings.TrimSpace(param.Name)
		if name == "" {
			continue
		}
		required[name] = param.Required
		switch name {
		case "working_directory":
			args[name] = primaryFailureDirectory(state, lastMap)
		case "path":
			path := primaryFailurePath(state, lastMap)
			if path == "" {
				path = "."
			}
			args[name] = path
		case "database_path":
			if db := inferredPathFromObservations(state, "database_path"); db != "" {
				args[name] = db
			} else if path := primaryFailurePath(state, lastMap); isSQLiteFailurePath(path) {
				args[name] = path
			}
		case "query":
			if strings.Contains(strings.ToLower(tool.Name()), "sqlite") {
				args[name] = "SELECT name FROM sqlite_master WHERE type='table' ORDER BY name LIMIT 20;"
			}
		}
	}
	for name, need := range required {
		if !need {
			continue
		}
		if _, ok := args[name]; ok {
			continue
		}
		_ = task
		return nil
	}
	if len(args) == 0 {
		return nil
	}
	return args
}

func failureSignature(lastMap map[string]interface{}) string {
	return strings.TrimSpace(fmt.Sprint(lastMap))
}

func recoveryProbesForSignature(state *core.Context, signature string) map[string]bool {
	out := map[string]bool{}
	if state == nil || signature == "" {
		return out
	}
	raw, ok := state.Get("react.recovery_probes")
	if !ok || raw == nil {
		return out
	}
	store, ok := raw.(map[string][]string)
	if !ok {
		return out
	}
	for _, name := range store[signature] {
		out[name] = true
	}
	return out
}

func recordRecoveryProbeUsage(state *core.Context, signature, toolName string) {
	if state == nil || signature == "" || toolName == "" {
		return
	}
	store := map[string][]string{}
	if raw, ok := state.Get("react.recovery_probes"); ok && raw != nil {
		if current, ok := raw.(map[string][]string); ok {
			for k, v := range current {
				store[k] = append([]string{}, v...)
			}
		}
	}
	store[signature] = append(store[signature], toolName)
	state.Set("react.recovery_probes", store)
}

func primaryFailureDirectory(state *core.Context, lastMap map[string]interface{}) string {
	if task := state.GetString("react.failure_workdir"); task != "" {
		return task
	}
	if path := primaryFailurePath(state, lastMap); path != "" {
		info, err := os.Stat(path)
		if err == nil && info.IsDir() {
			return path
		}
		return filepath.Dir(path)
	}
	return "."
}

func primaryFailurePath(state *core.Context, lastMap map[string]interface{}) string {
	if state != nil {
		if path := strings.TrimSpace(state.GetString("react.failure_path")); path != "" {
			return path
		}
	}
	if path := inferredPathFromObservations(state, "database_path", "manifest_path", "module_path", "workspace_path", "go_mod"); path != "" {
		return path
	}
	_ = lastMap
	return ""
}

func primaryFailureSearchPattern(lastMap map[string]interface{}) string {
	text := strings.TrimSpace(firstMeaningfulLine(fmt.Sprint(lastMap)))
	if text == "" {
		return ""
	}
	return text
}

var rustSymbolPattern = regexp.MustCompile(`([A-Za-z_][A-Za-z0-9_:]*)`)

func inferFailureSymbol(lastMap map[string]interface{}) string {
	text := fmt.Sprint(lastMap)
	matches := rustSymbolPattern.FindAllString(text, -1)
	for _, match := range matches {
		lower := strings.ToLower(match)
		if lower == "error" || lower == "warning" || lower == "failed" || lower == "cargo" {
			continue
		}
		return match
	}
	return ""
}

// --- Manifest / path inference from observations ---

func inferredPathFromObservations(state *core.Context, keys ...string) string {
	observations := getToolObservations(state)
	for i := len(observations) - 1; i >= 0; i-- {
		obs := observations[i]
		for _, key := range keys {
			if value := strings.TrimSpace(fmt.Sprint(obs.Data[key])); value != "" && value != "<nil>" {
				return value
			}
		}
		if obs.Tool == "file_read" {
			path := strings.TrimSpace(fmt.Sprint(obs.Args["path"]))
			if path != "" && path != "<nil>" {
				for _, key := range keys {
					switch key {
					case "database_path":
						if isSQLiteFailurePath(path) {
							return path
						}
					case "manifest_path", "module_path", "workspace_path", "go_mod":
						if strings.HasSuffix(path, ".toml") || strings.HasSuffix(path, ".mod") || strings.HasSuffix(path, ".work") || strings.HasSuffix(path, ".json") || strings.HasSuffix(path, ".cfg") || strings.HasSuffix(path, ".txt") || strings.HasSuffix(path, "Cargo.toml") {
							return path
						}
					}
				}
			}
		}
	}
	return ""
}

func inferredCargoManifest(state *core.Context) string {
	observations := getToolObservations(state)
	for i := len(observations) - 1; i >= 0; i-- {
		obs := observations[i]
		if obs.Tool == "rust_workspace_detect" {
			if manifest := strings.TrimSpace(fmt.Sprint(obs.Data["manifest_path"])); manifest != "" {
				return manifest
			}
		}
		if obs.Tool == "file_read" {
			if path := strings.TrimSpace(fmt.Sprint(obs.Args["path"])); strings.HasSuffix(path, "Cargo.toml") {
				return path
			}
		}
	}
	return ""
}

func inferredPythonManifest(state *core.Context) string {
	observations := getToolObservations(state)
	for i := len(observations) - 1; i >= 0; i-- {
		obs := observations[i]
		switch obs.Tool {
		case "python_workspace_detect", "python_project_metadata":
			if manifest := strings.TrimSpace(fmt.Sprint(obs.Data["manifest_path"])); manifest != "" {
				return manifest
			}
		case "file_read":
			if path := strings.TrimSpace(fmt.Sprint(obs.Args["path"])); strings.HasSuffix(path, "pyproject.toml") || strings.HasSuffix(path, "setup.py") || strings.HasSuffix(path, "setup.cfg") || strings.HasSuffix(path, "requirements.txt") {
				return path
			}
		}
	}
	return ""
}

func inferredNodeManifest(state *core.Context) string {
	observations := getToolObservations(state)
	for i := len(observations) - 1; i >= 0; i-- {
		obs := observations[i]
		switch obs.Tool {
		case "node_workspace_detect", "node_project_metadata":
			if manifest := strings.TrimSpace(fmt.Sprint(obs.Data["manifest_path"])); manifest != "" {
				return manifest
			}
		case "file_read":
			path := strings.TrimSpace(fmt.Sprint(obs.Args["path"]))
			if strings.HasSuffix(path, "package.json") ||
				strings.HasSuffix(path, "package-lock.json") ||
				strings.HasSuffix(path, "pnpm-lock.yaml") ||
				strings.HasSuffix(path, "yarn.lock") ||
				strings.HasSuffix(path, "tsconfig.json") {
				return path
			}
		}
	}
	return ""
}

func inferredGoManifest(state *core.Context) string {
	observations := getToolObservations(state)
	for i := len(observations) - 1; i >= 0; i-- {
		obs := observations[i]
		switch obs.Tool {
		case "go_workspace_detect":
			if manifest := strings.TrimSpace(fmt.Sprint(obs.Data["module_path"])); manifest != "" {
				return manifest
			}
			if manifest := strings.TrimSpace(fmt.Sprint(obs.Data["workspace_path"])); manifest != "" {
				return manifest
			}
		case "go_module_metadata":
			if manifest := strings.TrimSpace(fmt.Sprint(obs.Data["go_mod"])); manifest != "" {
				return manifest
			}
		case "file_read":
			path := strings.TrimSpace(fmt.Sprint(obs.Args["path"]))
			if strings.HasSuffix(path, "go.mod") || strings.HasSuffix(path, "go.work") {
				return path
			}
		}
	}
	return ""
}

func inferredSQLiteDatabase(state *core.Context) string {
	observations := getToolObservations(state)
	for i := len(observations) - 1; i >= 0; i-- {
		obs := observations[i]
		switch obs.Tool {
		case "sqlite_database_detect":
			if db := strings.TrimSpace(fmt.Sprint(obs.Data["database_path"])); db != "" {
				return db
			}
		case "sqlite_query", "sqlite_schema_inspect", "sqlite_integrity_check":
			if db := strings.TrimSpace(fmt.Sprint(obs.Data["database"])); db != "" {
				return db
			}
		case "file_read":
			path := strings.TrimSpace(fmt.Sprint(obs.Args["path"]))
			if isSQLiteFailurePath(path) {
				return path
			}
		}
	}
	return ""
}

func isSQLiteFailurePath(path string) bool {
	lower := strings.ToLower(strings.TrimSpace(path))
	return strings.HasSuffix(lower, ".db") || strings.HasSuffix(lower, ".sqlite") || strings.HasSuffix(lower, ".sqlite3")
}
