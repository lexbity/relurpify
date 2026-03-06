package pipeline

import (
	"errors"
	"fmt"
	"strings"
)

// ContractDescriptor identifies and validates the schema boundary for a stage.
type ContractDescriptor struct {
	Name        string
	Description string
	Metadata    ContractMetadata
}

// Validate ensures contract metadata is complete enough for the runtime.
func (d ContractDescriptor) Validate() error {
	if strings.TrimSpace(d.Name) == "" {
		return errors.New("contract name required")
	}
	return d.Metadata.Validate()
}

// Validate ensures contract metadata is well formed.
func (m ContractMetadata) Validate() error {
	if strings.TrimSpace(m.InputKey) == "" {
		return errors.New("contract input key required")
	}
	if strings.TrimSpace(m.OutputKey) == "" {
		return errors.New("contract output key required")
	}
	if strings.TrimSpace(m.SchemaVersion) == "" {
		return errors.New("contract schema version required")
	}
	if m.InputKey == m.OutputKey {
		return fmt.Errorf("contract input key %q must differ from output key", m.InputKey)
	}
	if m.RetryPolicy.MaxAttempts < 0 {
		return errors.New("contract retry max attempts cannot be negative")
	}
	return nil
}
