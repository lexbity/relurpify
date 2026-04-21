package agenttest

import (
	"fmt"
	"path/filepath"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/config"
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

func resolveCaseModelProfile(targetWorkspace string, execution resolvedCaseExecution) (*BackendModelProfileProvenance, *llm.ModelProfile, error) {
	registry, err := llm.NewProfileRegistry(config.New(targetWorkspace).ModelProfilesDir())
	if err != nil {
		return nil, nil, fmt.Errorf("load model profiles: %w", err)
	}
	resolution := registry.Resolve(execution.Provider, execution.Model)
	if resolution.Profile == nil {
		return nil, nil, nil
	}
	profile := resolution.Profile.Clone()
	provenance := &BackendModelProfileProvenance{
		RequestedProvider: strings.TrimSpace(execution.Provider),
		RequestedModel:    strings.TrimSpace(execution.Model),
		ResolvedProvider:  resolution.Provider,
		ResolvedModel:     resolution.Model,
		ProfileSource:     normalizeProfileSource(targetWorkspace, resolution.SourcePath),
		MatchKind:         resolution.MatchKind,
		Reason:            resolution.Reason,
		Profile:           profile,
	}
	return provenance, profile, nil
}

func normalizeProfileSource(targetWorkspace, sourcePath string) string {
	sourcePath = strings.TrimSpace(sourcePath)
	if sourcePath == "" {
		return ""
	}
	if targetWorkspace == "" {
		return filepath.Clean(sourcePath)
	}
	if rel, err := filepath.Rel(targetWorkspace, sourcePath); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return filepath.Clean(sourcePath)
}
