package thoughtrecipe

import (
	"errors"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/named/euclo/intentcontext"
	"codeburg.org/lexbit/relurpify/named/euclo/surface"
)

// ClarificationStepType identifies a clarification-specific thoughtrecipe step.
type ClarificationStepType string

const (
	ClarificationStepTypeClarify  ClarificationStepType = "intent_clarify"
	ClarificationStepTypeExtract  ClarificationStepType = "intent_extract"
	ClarificationStepTypeGround   ClarificationStepType = "intent_ground"
	ClarificationStepTypeProject  ClarificationStepType = "intent_project"
	ClarificationStepTypeRetrieve ClarificationStepType = "intent_retrieve"
	ClarificationStepTypeHandoff  ClarificationStepType = "intent_handoff"
)

// ClarificationStepConfig is the typed clarification contract carried in step config.
type ClarificationStepConfig struct {
	OutputSchemaID   string
	ValidationMode   intentcontext.ValidationMode
	RequiredFields   []string
	AllowedStatuses  []intentcontext.ClarificationStepStatus
	StateWriteKeys   []string
	ProjectionPolicy string
	RequeryOnSuccess bool
}

// IsClarificationStepType reports whether stepType is one of the clarification-only types.
func IsClarificationStepType(stepType string) bool {
	switch strings.TrimSpace(stepType) {
	case string(ClarificationStepTypeClarify), string(ClarificationStepTypeExtract), string(ClarificationStepTypeGround), string(ClarificationStepTypeProject), string(ClarificationStepTypeRetrieve), string(ClarificationStepTypeHandoff):
		return true
	default:
		return false
	}
}

// DecodeClarificationStepConfig converts a raw step config into the typed clarification config.
func DecodeClarificationStepConfig(step surface.ThoughtRecipeStep) (*ClarificationStepConfig, error) {
	if len(step.Config) == 0 {
		return nil, errors.New("clarification step config is empty")
	}
	cfg := &ClarificationStepConfig{}
	if value, ok := step.Config["output_schema_id"]; ok {
		cfg.OutputSchemaID = strings.TrimSpace(fmt.Sprint(value))
	}
	if value, ok := step.Config["validation_mode"]; ok {
		cfg.ValidationMode = intentcontext.ValidationMode(strings.TrimSpace(fmt.Sprint(value)))
	}
	if cfg.ValidationMode == "" {
		cfg.ValidationMode = intentcontext.ValidationModePartial
	}
	if value, ok := step.Config["required_fields"]; ok {
		cfg.RequiredFields = stringsFromAny(value)
	}
	if value, ok := step.Config["allowed_statuses"]; ok {
		for _, status := range stringsFromAny(value) {
			cfg.AllowedStatuses = append(cfg.AllowedStatuses, intentcontext.ClarificationStepStatus(strings.TrimSpace(status)))
		}
	}
	if value, ok := step.Config["state_write_keys"]; ok {
		cfg.StateWriteKeys = stringsFromAny(value)
	}
	if value, ok := step.Config["projection_policy"]; ok {
		cfg.ProjectionPolicy = strings.TrimSpace(fmt.Sprint(value))
	}
	if value, ok := step.Config["requery_on_success"]; ok {
		cfg.RequeryOnSuccess = truthyValue(value)
	}
	return cfg, validateClarificationStepConfigFields(step.Type, cfg)
}

func validateClarificationStepConfigFields(stepType string, cfg *ClarificationStepConfig) error {
	if cfg == nil {
		return nil
	}
	switch cfg.ValidationMode {
	case "", intentcontext.ValidationModeStrict, intentcontext.ValidationModePartial, intentcontext.ValidationModeRepair:
	default:
		return fmt.Errorf("invalid validation_mode: %s", cfg.ValidationMode)
	}
	if len(cfg.RequiredFields) == 0 {
		cfg.RequiredFields = nil
	}
	if len(cfg.StateWriteKeys) == 0 {
		cfg.StateWriteKeys = nil
	}
	switch ClarificationStepType(strings.TrimSpace(stepType)) {
	case ClarificationStepTypeClarify:
	case ClarificationStepTypeExtract, ClarificationStepTypeGround, ClarificationStepTypeProject, ClarificationStepTypeRetrieve:
		if strings.TrimSpace(cfg.OutputSchemaID) == "" {
			return fmt.Errorf("missing required field: config.output_schema_id")
		}
	case ClarificationStepTypeHandoff:
	}
	return nil
}

func truthyValue(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "", "0", "false", "no", "off", "nil", "null":
			return false
		default:
			return true
		}
	default:
		return strings.TrimSpace(fmt.Sprint(value)) != ""
	}
}

func validateStepParadigm(value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	if !surface.IsSupported(surface.Paradigm(trimmed)) && surface.Paradigm(trimmed) != surface.ParadigmEuclo {
		return fmt.Errorf("invalid paradigm value: %s", value)
	}
	return nil
}

func cloneClarificationStepConfig(cfg *ClarificationStepConfig) *ClarificationStepConfig {
	if cfg == nil {
		return nil
	}
	cp := *cfg
	if len(cfg.RequiredFields) > 0 {
		cp.RequiredFields = append([]string(nil), cfg.RequiredFields...)
	}
	if len(cfg.AllowedStatuses) > 0 {
		cp.AllowedStatuses = append([]intentcontext.ClarificationStepStatus(nil), cfg.AllowedStatuses...)
	}
	if len(cfg.StateWriteKeys) > 0 {
		cp.StateWriteKeys = append([]string(nil), cfg.StateWriteKeys...)
	}
	return &cp
}
