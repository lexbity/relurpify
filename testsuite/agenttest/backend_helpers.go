package agenttest

import (
	"regexp"
	"strings"
)

type BackendModelProvenance struct {
	RequestedModel string         `json:"requested_model"`
	LoadedName     string         `json:"loaded_name,omitempty"`
	LoadedModel    string         `json:"loaded_model,omitempty"`
	Digest         string         `json:"digest,omitempty"`
	Details        map[string]any `json:"details,omitempty"`
}

func shouldResetBackend(err error, patterns []string) bool {
	if err == nil || len(patterns) == 0 {
		return false
	}
	msg := err.Error()
	for _, raw := range patterns {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		re, reErr := regexp.Compile(raw)
		if reErr != nil {
			continue
		}
		if re.MatchString(msg) {
			return true
		}
	}
	return false
}

func shouldPreflightBackend(recordingMode string) bool {
	mode := strings.ToLower(strings.TrimSpace(recordingMode))
	return mode == "" || mode == "off" || mode == "record"
}
