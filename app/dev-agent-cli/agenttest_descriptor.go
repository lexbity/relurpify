package main

import (
	"fmt"
	"os"
	"strings"
	"unicode"

	"codeburg.org/lexbit/relurpify/testsuite/agenttest"
)

type preparedRunOverrides struct {
	BackendProvider string
	BackendFamily   string
	BackendEndpoint string
	BackendBinary   string
	BackendService  string
}

func preparedRunCaseKey(caseName, modelName string) string {
	return preparedRunPathName(caseName) + "__" + preparedRunPathName(modelName)
}

func preparedRunPathName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "unnamed"
	}
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
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

func loadPreparedRunDescriptor(path string) (*agenttest.PreparedRunDescriptor, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("descriptor path required")
	}
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	return agenttest.LoadPreparedRunDescriptor(path)
}

func applyPreparedRunOverrides(desc *agenttest.PreparedRunDescriptor, overrides preparedRunOverrides) {
	if desc == nil {
		return
	}
	if strings.TrimSpace(overrides.BackendProvider) != "" {
		desc.BackendProvider = strings.TrimSpace(overrides.BackendProvider)
	}
	if strings.TrimSpace(overrides.BackendFamily) != "" {
		desc.BackendFamily = strings.TrimSpace(overrides.BackendFamily)
	}
	if strings.TrimSpace(overrides.BackendEndpoint) != "" {
		desc.BackendEndpoint = strings.TrimSpace(overrides.BackendEndpoint)
	}
	if strings.TrimSpace(overrides.BackendBinary) != "" {
		desc.BackendBinary = strings.TrimSpace(overrides.BackendBinary)
	}
	if strings.TrimSpace(overrides.BackendService) != "" {
		desc.BackendService = strings.TrimSpace(overrides.BackendService)
	}
}
