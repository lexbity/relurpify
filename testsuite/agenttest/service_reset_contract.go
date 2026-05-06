package agenttest

import (
	"fmt"
	"strings"
)

type ServiceResetContract struct {
	Strategy      string
	BetweenCases  bool
	ServiceTarget string
}

func BuildServiceResetContract(desc *PreparedRunDescriptor) (ServiceResetContract, error) {
	if desc == nil {
		return ServiceResetContract{}, fmt.Errorf("descriptor required")
	}
	contract := ServiceResetContract{
		Strategy:      normalizeServiceResetStrategy(desc.ServiceResetStrategy),
		BetweenCases:  desc.ServiceResetBetween,
		ServiceTarget: firstNonEmpty(desc.BackendService, desc.BackendBinary),
	}
	if contract.Strategy == "" {
		contract.Strategy = "none"
	}
	return contract, nil
}

func (c ServiceResetContract) Normalize() ServiceResetContract {
	c.Strategy = normalizeServiceResetStrategy(c.Strategy)
	if c.Strategy == "" {
		c.Strategy = "none"
	}
	c.ServiceTarget = strings.TrimSpace(c.ServiceTarget)
	return c
}

func (c ServiceResetContract) RequiresReset() bool {
	switch normalizeServiceResetStrategy(c.Strategy) {
	case "", "none":
		return false
	default:
		return true
	}
}

func (c ServiceResetContract) ShouldResetBetweenCases() bool {
	return c.BetweenCases && c.RequiresReset()
}

func normalizeServiceResetStrategy(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	switch raw {
	case "", "none", "restart", "stop-start", "clear":
		return raw
	default:
		return raw
	}
}
