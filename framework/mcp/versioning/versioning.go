package versioning

import (
	"fmt"
	"strings"

	"github.com/lexcodex/relurpify/framework/mcp/protocol"
)

type RevisionSupport struct {
	Revision             string
	FeatureFamilies      []string
	TransportKinds       []string
	DeprecatedMethods    []string
	UnsupportedMethods   []string
	ContentShapeVariants []string
	TestFixtureCoverage  string
}

type SupportMatrix struct {
	Primary   string
	Supported map[string]RevisionSupport
}

type NegotiationResult struct {
	Requested  string
	Negotiated string
	IsPrimary  bool
}

func DefaultSupportMatrix() SupportMatrix {
	supported := map[string]RevisionSupport{
		protocol.Revision20250618: {
			Revision:             protocol.Revision20250618,
			FeatureFamilies:      []string{"base-protocol", "tools", "prompts", "resources"},
			TransportKinds:       []string{"stdio", "streamable-http"},
			ContentShapeVariants: []string{"single-content"},
			TestFixtureCoverage:  "phase1",
		},
	}
	return SupportMatrix{
		Primary:   protocol.Revision20250618,
		Supported: supported,
	}
}

func (m SupportMatrix) ChooseRevision(preferred []string) (string, error) {
	if len(m.Supported) == 0 {
		return "", fmt.Errorf("support matrix has no supported revisions")
	}
	for _, candidate := range preferred {
		candidate = protocol.NormalizeRevision(candidate)
		if candidate == "" {
			continue
		}
		if _, ok := m.Supported[candidate]; ok {
			return candidate, nil
		}
	}
	if _, ok := m.Supported[m.Primary]; ok && strings.TrimSpace(m.Primary) != "" {
		return m.Primary, nil
	}
	return "", fmt.Errorf("support matrix primary revision %q unsupported", m.Primary)
}

func (m SupportMatrix) Negotiate(peerRevision string, preferred []string) (NegotiationResult, error) {
	requested, err := m.ChooseRevision(preferred)
	if err != nil {
		return NegotiationResult{}, err
	}
	peerRevision = protocol.NormalizeRevision(peerRevision)
	if peerRevision == "" {
		return NegotiationResult{}, fmt.Errorf("peer protocol version required")
	}
	if _, ok := m.Supported[peerRevision]; !ok {
		return NegotiationResult{}, fmt.Errorf("unsupported peer protocol version %q", peerRevision)
	}
	if peerRevision != requested {
		return NegotiationResult{}, fmt.Errorf("peer protocol version %q does not match requested version %q", peerRevision, requested)
	}
	return NegotiationResult{
		Requested:  requested,
		Negotiated: peerRevision,
		IsPrimary:  peerRevision == m.Primary,
	}, nil
}

func (m SupportMatrix) Supports(revision string) bool {
	_, ok := m.Supported[protocol.NormalizeRevision(revision)]
	return ok
}
