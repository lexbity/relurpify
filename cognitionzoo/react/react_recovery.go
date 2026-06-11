package react

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
)

func recoveryProbeArgs(agent *ReActAgent, toolName string, env *contextdata.Envelope, task *execution.Task, lastMap map[string]any) map[string]any {
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
			return map[string]any{"path": path}
		}
		return nil
	case "search_grep", "file_search":
		pattern := primaryFailureSearchPattern(lastMap)
		if pattern == "" {
			return nil
		}
		return map[string]any{
			"directory": primaryFailureDirectory(env, lastMap),
			"pattern":   pattern,
		}
	case "query_ast":
		if symbol := inferFailureSymbol(lastMap); symbol != "" {
			return map[string]any{"action": "get_signature", "symbol": symbol}
		}
		return map[string]any{"action": "list_symbols", "category": "function"}
	}

	args := make(map[string]any)
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

func failureSignature(lastMap map[string]any) string {
	return strings.TrimSpace(fmt.Sprint(lastMap))
}

func recoveryProbesForSignature(env *contextdata.Envelope, signature string) map[string]bool {
	out := map[string]bool{}
	if env == nil || signature == "" {
		return out
	}
	raw, ok := contextdata.GetTyped[any](env, "react.recovery_probes")
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
	if current, ok := contextdata.GetTyped[map[string][]string](env, "react.recovery_probes"); ok {
		for k, v := range current {
			store[k] = append([]string{}, v...)
		}
	}
	store[signature] = append(store[signature], toolName)
	env.SetWorkingValueWithClass("react.recovery_probes", store, contextdata.MemoryClassTask)
}

func primaryFailureDirectory(env *contextdata.Envelope, lastMap map[string]any) string {
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

func primaryFailurePath(env *contextdata.Envelope, lastMap map[string]any) string {
	if env != nil {
		if path := strings.TrimSpace(envGetString(env, "react.failure_path")); path != "" {
			return path
		}
	}
	if path := inferredPathFromObservations(env, "database_path", "module_path", "workspace_path", "go_mod"); path != "" {
		return path
	}
	_ = lastMap
	return ""
}

func primaryFailureSearchPattern(lastMap map[string]any) string {
	text := strings.TrimSpace(firstMeaningfulLine(fmt.Sprint(lastMap)))
	if text == "" {
		return ""
	}
	return text
}

var rustSymbolPattern = regexp.MustCompile(`([A-Za-z_][A-Za-z0-9_:]*)`)

func inferFailureSymbol(lastMap map[string]any) string {
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

func inferredPathFromObservations(env *contextdata.Envelope, keys ...string) string {
	observations := getToolObservations(env)
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
					case "module_path", "workspace_path", "go_mod":
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
