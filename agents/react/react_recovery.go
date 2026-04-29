package react

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
)

func recoveryProbeArgs(agent *ReActAgent, toolName string, env *contextdata.Envelope, task *core.Task, lastMap map[string]interface{}) map[string]interface{} {
	if agent == nil || agent.Tools == nil {
		return nil
	}
	tool, ok := agent.Tools.Get(toolName)
	if !ok || tool == nil {
		return nil
	}
	switch toolName {
	case "file_read":
		if path := primaryFailurePath(env, lastMap); path != "" {
			return map[string]interface{}{"path": path}
		}
		return nil
	case "search_grep", "file_search":
		pattern := primaryFailureSearchPattern(lastMap)
		if pattern == "" {
			return nil
		}
		return map[string]interface{}{
			"directory": primaryFailureDirectory(env, lastMap),
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
			args[name] = primaryFailureDirectory(env, lastMap)
		case "path":
			path := primaryFailurePath(env, lastMap)
			if path == "" {
				path = "."
			}
			args[name] = path
		case "database_path":
			if db := inferredPathFromObservations(env, "database_path"); db != "" {
				args[name] = db
			} else if path := primaryFailurePath(env, lastMap); isSQLiteFailurePath(path) {
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

func recoveryProbesForSignature(env *contextdata.Envelope, signature string) map[string]bool {
	out := map[string]bool{}
	if env == nil || signature == "" {
		return out
	}
	raw, ok := env.GetWorkingValue("react.recovery_probes")
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

func recordRecoveryProbeUsage(env *contextdata.Envelope, signature, toolName string) {
	if env == nil || signature == "" || toolName == "" {
		return
	}
	store := map[string][]string{}
	if raw, ok := env.GetWorkingValue("react.recovery_probes"); ok && raw != nil {
		if current, ok := raw.(map[string][]string); ok {
			for k, v := range current {
				store[k] = append([]string{}, v...)
			}
		}
	}
	store[signature] = append(store[signature], toolName)
	env.SetWorkingValue("react.recovery_probes", store, contextdata.MemoryClassTask)
}

func primaryFailureDirectory(env *contextdata.Envelope, lastMap map[string]interface{}) string {
	if task := envGetString(env, "react.failure_workdir"); task != "" {
		return task
	}
	if path := primaryFailurePath(env, lastMap); path != "" {
		info, err := os.Stat(path)
		if err == nil && info.IsDir() {
			return path
		}
		return filepath.Dir(path)
	}
	return "."
}

func primaryFailurePath(env *contextdata.Envelope, lastMap map[string]interface{}) string {
	if env != nil {
		if path := strings.TrimSpace(envGetString(env, "react.failure_path")); path != "" {
			return path
		}
	}
	if path := inferredPathFromObservations(env, "database_path", "manifest_path", "module_path", "workspace_path", "go_mod"); path != "" {
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

type manifestInferenceRule struct {
	tools      []string
	dataKeys   []string
	pathSuffix []string
}

func inferredManifestFromObservations(env *contextdata.Envelope, rule manifestInferenceRule) string {
	observations := getToolObservations(env)
	for i := len(observations) - 1; i >= 0; i-- {
		obs := observations[i]
		if len(rule.tools) > 0 {
			matched := false
			for _, tool := range rule.tools {
				if strings.EqualFold(strings.TrimSpace(obs.Tool), tool) {
					matched = true
					break
				}
			}
			if matched {
				for _, key := range rule.dataKeys {
					if manifest := strings.TrimSpace(fmt.Sprint(obs.Data[key])); manifest != "" && manifest != "<nil>" {
						return manifest
					}
				}
			}
		}
		if obs.Tool != "file_read" {
			continue
		}
		path := strings.TrimSpace(fmt.Sprint(obs.Args["path"]))
		if path == "" || path == "<nil>" {
			continue
		}
		for _, suffix := range rule.pathSuffix {
			if strings.HasSuffix(path, suffix) {
				return path
			}
		}
	}
	return ""
}

func inferredPathFromObservations(env *contextdata.Envelope, keys ...string) string {
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

func inferredCargoManifest(env *contextdata.Envelope) string {
	return inferredManifestFromObservations(env, manifestInferenceRule{
		tools:      []string{"rust_workspace_detect"},
		dataKeys:   []string{"manifest_path"},
		pathSuffix: []string{"Cargo.toml"},
	})
}

func inferredPythonManifest(env *contextdata.Envelope) string {
	return inferredManifestFromObservations(state, manifestInferenceRule{
		tools:      []string{"python_workspace_detect", "python_project_metadata"},
		dataKeys:   []string{"manifest_path"},
		pathSuffix: []string{"pyproject.toml", "setup.py", "setup.cfg", "requirements.txt"},
	})
}

func inferredNodeManifest(env *contextdata.Envelope) string {
	return inferredManifestFromObservations(env, manifestInferenceRule{
		tools:      []string{"node_workspace_detect", "node_project_metadata"},
		dataKeys:   []string{"manifest_path"},
		pathSuffix: []string{"package.json", "package-lock.json", "pnpm-lock.yaml", "yarn.lock", "tsmanifest.json"},
	})
}

func inferredGoManifest(env *contextdata.Envelope) string {
	return inferredManifestFromObservations(env, manifestInferenceRule{
		tools:      []string{"go_workspace_detect", "go_module_metadata"},
		dataKeys:   []string{"module_path", "workspace_path", "go_mod"},
		pathSuffix: []string{"go.mod", "go.work"},
	})
}

func inferredSQLiteDatabase(env *contextdata.Envelope) string {
	observations := getToolObservations(env)
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
