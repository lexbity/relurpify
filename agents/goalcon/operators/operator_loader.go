package operators

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"codeburg.org/lexbit/relurpify/agents/goalcon/types"
	"gopkg.in/yaml.v3"
)

// OperatorConfig represents the YAML/JSON configuration for a single operator.
type OperatorConfig struct {
	Name          string         `json:"name" yaml:"name"`
	Description   string         `json:"description" yaml:"description"`
	Preconditions []string       `json:"preconditions" yaml:"preconditions"`
	Effects       []string       `json:"effects" yaml:"effects"`
	DefaultParams map[string]any `json:"default_params" yaml:"default_params"`
	Tags          []string       `json:"tags,omitempty" yaml:"tags,omitempty"`
}

// OperatorsConfigSection represents the top-level operators configuration section.
type OperatorsConfigSection struct {
	Operators []OperatorConfig `json:"operators" yaml:"operators"`
}

// NewOperatorRegistryFromConfig builds an operator registry from raw configuration data.
// Falls back to DefaultOperatorRegistry if config is missing or invalid.
// Accepts: OperatorsConfigSection, YAML string, JSON string, or map[string]any.
func NewOperatorRegistryFromConfig(raw any) *types.OperatorRegistry {
	if raw == nil {
		return DefaultOperatorRegistry()
	}

	// Try to parse as OperatorsConfigSection
	section, err := ParseOperatorConfig(raw)
	if err != nil || section == nil || len(section.Operators) == 0 {
		return DefaultOperatorRegistry()
	}

	// Build registry from config
	registry := &types.OperatorRegistry{}
	for _, opConfig := range section.Operators {
		if err := addOperatorFromConfig(registry, opConfig); err != nil {
			// Log error, continue with other operators
			continue
		}
	}

	if len(registry.All()) == 0 {
		return DefaultOperatorRegistry()
	}

	return registry
}

// LoadOperatorsFromFile reads and parses operators from a YAML or JSON file.
func LoadOperatorsFromFile(filePath string) *types.OperatorRegistry {
	if filePath == "" {
		return DefaultOperatorRegistry()
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		// Log would go here; fall back to defaults
		return DefaultOperatorRegistry()
	}

	// Determine format from file extension
	ext := filepath.Ext(filePath)
	var section OperatorsConfigSection

	switch ext {
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, &section); err != nil {
			return DefaultOperatorRegistry()
		}
	case ".json":
		if err := json.Unmarshal(data, &section); err != nil {
			return DefaultOperatorRegistry()
		}
	default:
		// Try YAML first, then JSON
		if err := yaml.Unmarshal(data, &section); err != nil {
			if err := json.Unmarshal(data, &section); err != nil {
				return DefaultOperatorRegistry()
			}
		}
	}

	if len(section.Operators) == 0 {
		return DefaultOperatorRegistry()
	}

	// Build registry from config
	registry := &types.OperatorRegistry{}
	for _, opConfig := range section.Operators {
		if err := addOperatorFromConfig(registry, opConfig); err != nil {
			continue
		}
	}

	if len(registry.All()) == 0 {
		return DefaultOperatorRegistry()
	}

	return registry
}

// ParseOperatorConfig converts raw config data to OperatorsConfigSection.
// Handles multiple input formats: YAML string, JSON string, map[string]any, OperatorsConfigSection.
func ParseOperatorConfig(raw any) (*OperatorsConfigSection, error) {
	if raw == nil {
		return nil, fmt.Errorf("config is nil")
	}

	// If already parsed, return directly
	if section, ok := raw.(*OperatorsConfigSection); ok {
		return section, nil
	}
	if section, ok := raw.(OperatorsConfigSection); ok {
		return &section, nil
	}

	// Try YAML string
	if yamlStr, ok := raw.(string); ok {
		var section OperatorsConfigSection
		if err := yaml.Unmarshal([]byte(yamlStr), &section); err == nil {
			return &section, nil
		}
		// Fall through to other formats
	}

	// Try map[string]any
	if mapData, ok := raw.(map[string]any); ok {
		jsonBytes, err := json.Marshal(mapData)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal map: %w", err)
		}
		var section OperatorsConfigSection
		if err := json.Unmarshal(jsonBytes, &section); err != nil {
			return nil, fmt.Errorf("failed to unmarshal JSON: %w", err)
		}
		return &section, nil
	}

	// Try direct JSON unmarshal
	jsonBytes, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal to JSON: %w", err)
	}
	var section OperatorsConfigSection
	if err := json.Unmarshal(jsonBytes, &section); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %w", err)
	}
	return &section, nil
}

// addOperatorFromConfig converts an OperatorConfig to an types.Operator and adds it to the registry.
func addOperatorFromConfig(registry *types.OperatorRegistry, opConfig OperatorConfig) error {
	if registry == nil {
		return fmt.Errorf("registry is nil")
	}

	if opConfig.Name == "" {
		return fmt.Errorf("operator name is required")
	}

	// Convert string arrays to types.Predicate arrays
	preconditions := make([]types.Predicate, len(opConfig.Preconditions))
	for i, p := range opConfig.Preconditions {
		preconditions[i] = types.Predicate(p)
	}

	effects := make([]types.Predicate, len(opConfig.Effects))
	for i, e := range opConfig.Effects {
		effects[i] = types.Predicate(e)
	}

	// Create and register operator
	op := types.Operator{
		Name:          opConfig.Name,
		Description:   opConfig.Description,
		Preconditions: preconditions,
		Effects:       effects,
		DefaultParams: opConfig.DefaultParams,
	}

	registry.Register(op)
	return nil
}

// OperatorVariant allows specialization of operators per domain/context.
type OperatorVariant struct {
	Name         string // e.g., "ReadFile:large_files" or "SearchCode:fast"
	BaseOperator string // Reference to base operator
	Description  string
	// Override specific aspects
	Preconditions *[]types.Predicate
	Effects       *[]types.Predicate
	DefaultParams map[string]any
}

// OperatorLibrary manages operator collections with versioning support.
type OperatorLibrary struct {
	Name      string
	Version   string
	Operators []*types.Operator
}

// LoadOperatorLibraryFromConfig loads an operator library from raw configuration.
// Returns nil if config is invalid or empty.
func LoadOperatorLibraryFromConfig(raw any, name string) *OperatorLibrary {
	if raw == nil {
		return nil
	}

	section, err := ParseOperatorConfig(raw)
	if err != nil || section == nil || len(section.Operators) == 0 {
		return nil
	}

	lib := &OperatorLibrary{
		Name:      name,
		Version:   "1.0", // Default version
		Operators: make([]*types.Operator, 0, len(section.Operators)),
	}

	for _, opConfig := range section.Operators {
		preconditions := make([]types.Predicate, len(opConfig.Preconditions))
		for i, p := range opConfig.Preconditions {
			preconditions[i] = types.Predicate(p)
		}

		effects := make([]types.Predicate, len(opConfig.Effects))
		for i, e := range opConfig.Effects {
			effects[i] = types.Predicate(e)
		}

		op := &types.Operator{
			Name:          opConfig.Name,
			Description:   opConfig.Description,
			Preconditions: preconditions,
			Effects:       effects,
			DefaultParams: opConfig.DefaultParams,
		}
		lib.Operators = append(lib.Operators, op)
	}

	return lib
}

// LoadOperatorLibraryFromFile loads an operator library from a file.
func LoadOperatorLibraryFromFile(filePath string, name string) *OperatorLibrary {
	if filePath == "" {
		return nil
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil
	}

	var section OperatorsConfigSection
	if err := yaml.Unmarshal(data, &section); err != nil {
		if err := json.Unmarshal(data, &section); err != nil {
			return nil
		}
	}

	return LoadOperatorLibraryFromConfig(&section, name)
}

// ToRegistry converts a library to an types.OperatorRegistry.
func (lib *OperatorLibrary) ToRegistry() *types.OperatorRegistry {
	if lib == nil {
		return &types.OperatorRegistry{}
	}

	registry := &types.OperatorRegistry{}
	for _, op := range lib.Operators {
		if op != nil {
			registry.Register(*op)
		}
	}
	return registry
}

// MergeRegistries combines multiple registries, with later ones taking precedence.
func MergeRegistries(registries ...*types.OperatorRegistry) *types.OperatorRegistry {
	merged := &types.OperatorRegistry{}

	for _, reg := range registries {
		if reg == nil {
			continue
		}
		for _, op := range reg.All() {
			merged.Register(*op)
		}
	}

	return merged
}

// ValidateOperatorConfig checks if operator configuration is valid.
func ValidateOperatorConfig(opConfig OperatorConfig) error {
	if opConfig.Name == "" {
		return fmt.Errorf("operator name is required")
	}

	if len(opConfig.Effects) == 0 {
		return fmt.Errorf("operator must have at least one effect")
	}

	// Check for duplicate preconditions/effects
	seen := make(map[string]bool)
	for _, p := range opConfig.Preconditions {
		if seen[p] {
			return fmt.Errorf("duplicate precondition: %s", p)
		}
		seen[p] = true
	}

	seen = make(map[string]bool)
	for _, e := range opConfig.Effects {
		if seen[e] {
			return fmt.Errorf("duplicate effect: %s", e)
		}
		seen[e] = true
	}

	return nil
}

// OperatorConfigValidator provides validation for operator sets.
type OperatorConfigValidator struct {
	Strict bool // If true, fail on any issues; if false, warn and continue
}

// ValidateRegistry checks entire registry for issues.
func (v *OperatorConfigValidator) ValidateRegistry(registry *types.OperatorRegistry) []error {
	if registry == nil {
		return nil
	}

	var errors []error
	seen := make(map[string]bool)

	for _, op := range registry.All() {
		if op == nil {
			continue
		}

		// Check for duplicate operators
		if seen[op.Name] {
			errors = append(errors, fmt.Errorf("duplicate operator: %s", op.Name))
		}
		seen[op.Name] = true

		// Check for effects
		if len(op.Effects) == 0 {
			errors = append(errors, fmt.Errorf("operator %s has no effects", op.Name))
		}

		// Check for self-referential preconditions
		for _, pre := range op.Preconditions {
			for _, eff := range op.Effects {
				if pre == eff {
					errors = append(errors, fmt.Errorf("operator %s has precondition matching effect: %s", op.Name, pre))
				}
			}
		}
	}

	return errors
}
