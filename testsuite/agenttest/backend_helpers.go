package agenttest

import (
	"strings"

	// The agent test executor composes workspaces with the shipped tool
	// catalog; without this registration the go_native implementations the
	// shipped recipes declare (test runners, file tools) are skipped in
	// strict mode and recipe vocabulary fails to resolve.
	_ "codeburg.org/lexbit/relurpify/platform/tools/native"
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
