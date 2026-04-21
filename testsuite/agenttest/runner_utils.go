package agenttest

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/config"
)

func resolveAgainstWorkspace(workspace, resolvedBySuite, original string) string {
	// Suites often use workspace-relative manifest and tape paths even though the
	// suite file itself may live elsewhere under the repository tree.
	if resolvedBySuite != "" {
		if _, err := os.Stat(resolvedBySuite); err == nil {
			return resolvedBySuite
		}
	}
	if original == "" || filepath.IsAbs(original) {
		return resolvedBySuite
	}
	candidate := filepath.Clean(filepath.Join(workspace, original))
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	return resolvedBySuite
}

func fallbackManifestPath(manifestPath, workspace string) string {
	if manifestPath != "" {
		if _, err := os.Stat(manifestPath); err == nil {
			return manifestPath
		}
	}
	if workspace == "" {
		return manifestPath
	}
	paths := config.New(workspace)
	candidates := []string{
		paths.ManifestFile(),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return manifestPath
}

func uniqueStrings(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func mapTargetPathToWorkspace(absPath, targetWorkspace, workspace string) string {
	if absPath == "" || targetWorkspace == "" || workspace == "" {
		return absPath
	}
	rel, err := filepath.Rel(targetWorkspace, absPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return absPath
	}
	return filepath.Join(workspace, rel)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func sanitizeName(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "unnamed"
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "unnamed"
	}
	return out
}

func cloneContextMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}
