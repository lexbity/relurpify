package agenttest

import (
	"codeburg.org/lexbit/relurpify/platform/llm"
)

type BackendModelProfileProvenance struct {
	RequestedProvider string            `json:"requested_provider,omitempty"`
	RequestedModel    string            `json:"requested_model,omitempty"`
	ResolvedProvider  string            `json:"resolved_provider,omitempty"`
	ResolvedModel     string            `json:"resolved_model,omitempty"`
	ProfileSource     string            `json:"profile_source,omitempty"`
	MatchKind         string            `json:"match_kind,omitempty"`
	Reason            string            `json:"reason,omitempty"`
	Profile           *llm.ModelProfile `json:"profile,omitempty"`
}
