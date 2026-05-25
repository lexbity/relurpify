package sandbox

import (
	"strings"
)

// assembleSubprocessEnv builds the child process environment from a filtered
// host slice plus explicit overrides. Only keys in allowedKeys are inherited
// from hostEnv.
//
// The returned slice is stable and contains each key at most once, with later
// explicit values overriding inherited values.
func assembleSubprocessEnv(hostEnv, allowedKeys, extraEnv []string) []string {
	if len(hostEnv) == 0 && len(allowedKeys) == 0 && len(extraEnv) == 0 {
		return nil
	}

	hostValues := make(map[string]string, len(hostEnv))
	for _, entry := range hostEnv {
		key, value, ok := splitEnvEntry(entry)
		if !ok {
			continue
		}
		hostValues[key] = value
	}

	merged := make(map[string]string, len(allowedKeys)+len(extraEnv))
	order := make([]string, 0, len(allowedKeys)+len(extraEnv))

	add := func(key, value string) {
		if key == "" {
			return
		}
		if _, seen := merged[key]; !seen {
			order = append(order, key)
		}
		merged[key] = value
	}

	for _, key := range allowedKeys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if value, ok := hostValues[key]; ok {
			add(key, value)
		}
	}

	for _, entry := range extraEnv {
		key, value, ok := splitEnvEntry(entry)
		if !ok {
			continue
		}
		add(key, value)
	}

	if len(order) == 0 {
		return nil
	}

	env := make([]string, 0, len(order))
	for _, key := range order {
		env = append(env, key+"="+merged[key])
	}
	return env
}

func splitEnvEntry(entry string) (string, string, bool) {
	if entry == "" {
		return "", "", false
	}
	key, value, ok := strings.Cut(entry, "=")
	if !ok {
		return "", "", false
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return "", "", false
	}
	return key, value, true
}
