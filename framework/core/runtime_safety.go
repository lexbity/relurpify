package core

import (
	"fmt"
	"sort"
	"strings"
)

type RuntimeSafetySpec struct {
	MaxCallsPerCapability     int   `yaml:"max_calls_per_capability,omitempty" json:"max_calls_per_capability,omitempty"`
	MaxCallsPerProvider       int   `yaml:"max_calls_per_provider,omitempty" json:"max_calls_per_provider,omitempty"`
	MaxBytesPerSession        int   `yaml:"max_bytes_per_session,omitempty" json:"max_bytes_per_session,omitempty"`
	MaxOutputTokensSession    int   `yaml:"max_output_tokens_per_session,omitempty" json:"max_output_tokens_per_session,omitempty"`
	MaxSubprocessesPerSession int   `yaml:"max_subprocesses_per_session,omitempty" json:"max_subprocesses_per_session,omitempty"`
	MaxNetworkRequestsSession int   `yaml:"max_network_requests_per_session,omitempty" json:"max_network_requests_per_session,omitempty"`
	RedactSensitiveMetadata   *bool `yaml:"redact_sensitive_metadata,omitempty" json:"redact_sensitive_metadata,omitempty"`
}

func (s RuntimeSafetySpec) Validate() error {
	for name, value := range map[string]int{
		"max_calls_per_capability":         s.MaxCallsPerCapability,
		"max_calls_per_provider":           s.MaxCallsPerProvider,
		"max_bytes_per_session":            s.MaxBytesPerSession,
		"max_output_tokens_session":        s.MaxOutputTokensSession,
		"max_subprocesses_per_session":     s.MaxSubprocessesPerSession,
		"max_network_requests_per_session": s.MaxNetworkRequestsSession,
	} {
		if value < 0 {
			return fmt.Errorf("%s must be >= 0", name)
		}
	}
	return nil
}

func (s RuntimeSafetySpec) RedactionEnabled() bool {
	if s.RedactSensitiveMetadata == nil {
		return true
	}
	return *s.RedactSensitiveMetadata
}

func RedactMetadataMap(input map[string]interface{}) map[string]interface{} {
	if input == nil {
		return nil
	}
	out := make(map[string]interface{}, len(input))
	for key, value := range input {
		out[key] = redactValue(key, value)
	}
	return out
}

// RedactAny converts arbitrary structured data into a redacted representation
// suitable for persistence or export.
func RedactAny(input any) any {
	if input == nil {
		return nil
	}
	switch typed := input.(type) {
	case map[string]interface{}:
		return RedactMetadataMap(typed)
	case map[string]string:
		out := make(map[string]interface{}, len(typed))
		for key, value := range typed {
			out[key] = redactValue(key, value)
		}
		return out
	case []interface{}:
		out := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			out = append(out, RedactAny(item))
		}
		return out
	case []string:
		out := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			out = append(out, redactValue("", item))
		}
		return out
	case string:
		return redactValue("", typed)
	default:
		return input
	}
}

func redactValue(key string, value interface{}) interface{} {
	if isSensitiveKey(key) {
		return "[REDACTED]"
	}
	switch typed := value.(type) {
	case map[string]interface{}:
		return RedactMetadataMap(typed)
	case map[string]string:
		out := make(map[string]interface{}, len(typed))
		for k, v := range typed {
			out[k] = redactValue(k, v)
		}
		return out
	case []string:
		out := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			out = append(out, redactValue(key, item))
		}
		return out
	case []interface{}:
		out := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			out = append(out, redactValue(key, item))
		}
		return out
	case string:
		if looksSensitiveValue(typed) {
			return "[REDACTED]"
		}
		return typed
	default:
		return value
	}
}

func isSensitiveKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	for _, needle := range []string{
		"secret", "token", "password", "cookie", "authorization", "auth", "credential", "api_key", "apikey",
	} {
		if strings.Contains(key, needle) {
			return true
		}
	}
	return false
}

func looksSensitiveValue(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	if lower == "" {
		return false
	}
	for _, needle := range []string{"bearer ", "ghp_", "github_pat_", "sk-", "authorization:", "session="} {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}

func EstimatePayloadBytes(values ...interface{}) int {
	total := 0
	for _, value := range values {
		total += len(strings.TrimSpace(fmt.Sprint(value)))
	}
	return total
}

func EstimatePayloadTokens(values ...interface{}) int {
	return EstimatePayloadBytes(values...) / 4
}

type RevocationSnapshot struct {
	Capabilities map[string]string `json:"capabilities,omitempty" yaml:"capabilities,omitempty"`
	Providers    map[string]string `json:"providers,omitempty" yaml:"providers,omitempty"`
	Sessions     map[string]string `json:"sessions,omitempty" yaml:"sessions,omitempty"`
}

func cloneRevocationSnapshot(input RevocationSnapshot) RevocationSnapshot {
	return RevocationSnapshot{
		Capabilities: cloneStringMap(input.Capabilities),
		Providers:    cloneStringMap(input.Providers),
		Sessions:     cloneStringMap(input.Sessions),
	}
}

func cloneStringMap(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	out := make(map[string]string, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}

func SortedKeys(input map[string]string) []string {
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
