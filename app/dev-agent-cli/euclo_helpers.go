package main

import (
	"fmt"
	"strings"
)

func cloneStringMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func appendArtifactString(raw any, artifact string) []string {
	artifact = strings.TrimSpace(artifact)
	if artifact == "" {
		return nil
	}
	var out []string
	switch typed := raw.(type) {
	case []string:
		out = append(out, typed...)
	case []any:
		for _, item := range typed {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
	case nil:
	default:
		if s, ok := typed.(string); ok {
			out = append(out, s)
		}
	}
	out = append(out, artifact)
	return uniqueStrings(out)
}

func assertExpectedArtifact(raw any, expected any) error {
	if expected == nil {
		return nil
	}
	switch typed := expected.(type) {
	case string:
		for _, artifact := range artifactStrings(raw) {
			if artifact == typed {
				return nil
			}
		}
		return errExpectedArtifact(typed)
	case map[string]any:
		if kind, ok := typed["kind"].(string); ok {
			for _, artifact := range artifactStrings(raw) {
				if artifact == kind {
					return nil
				}
			}
			return errExpectedArtifact(kind)
		}
		if value, ok := typed["value"].(string); ok {
			for _, artifact := range artifactStrings(raw) {
				if artifact == value {
					return nil
				}
			}
			return errExpectedArtifact(value)
		}
	}
	return nil
}

func artifactStrings(raw any) []string {
	switch typed := raw.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		if raw == nil {
			return nil
		}
		return []string{strings.TrimSpace(fmt.Sprint(raw))}
	}
}

func equalValue(left, right any) bool {
	switch l := left.(type) {
	case nil:
		return right == nil
	case string:
		if r, ok := right.(string); ok {
			return l == r
		}
	case fmt.Stringer:
		return l.String() == fmt.Sprint(right)
	}
	return fmt.Sprint(left) == fmt.Sprint(right)
}

func errExpectedArtifact(value string) error {
	return fmt.Errorf("expected artifact %q not found", value)
}
