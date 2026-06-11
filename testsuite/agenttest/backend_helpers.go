package agenttest

import (
	"strings"
)

type BackendModelProvenance struct {
	RequestedModel string         `json:"requested_model"`
	LoadedName     string         `json:"loaded_name,omitempty"`
	LoadedModel    string         `json:"loaded_model,omitempty"`
	Digest         string         `json:"digest,omitempty"`
	Details        map[string]any `json:"details,omitempty"`
}

func shouldPreflightBackend(recordingMode string) bool {
	mode := strings.ToLower(strings.TrimSpace(recordingMode))
	return mode == "" || mode == "off" || mode == "record"
}
