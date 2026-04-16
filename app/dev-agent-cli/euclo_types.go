package main

import (
	"errors"
	"fmt"
	"strings"
)

// CapabilityCatalogEntry is the CLI-local projection of a relurpic capability.
type CapabilityCatalogEntry struct {
	ID                         string   `json:"id" yaml:"id"`
	DisplayName                string   `json:"display_name,omitempty" yaml:"display_name,omitempty"`
	PrimaryOwner               string   `json:"primary_owner,omitempty" yaml:"primary_owner,omitempty"`
	ModeFamilies               []string `json:"mode_families,omitempty" yaml:"mode_families,omitempty"`
	PrimaryCapable             bool     `json:"primary_capable" yaml:"primary_capable"`
	SupportingOnly             bool     `json:"supporting_only" yaml:"supporting_only"`
	Mutability                 string   `json:"mutability,omitempty" yaml:"mutability,omitempty"`
	ArchaeoAssociated          bool     `json:"archaeo_associated,omitempty" yaml:"archaeo_associated,omitempty"`
	LazySemanticAcquisition    bool     `json:"lazy_semantic_acquisition,omitempty" yaml:"lazy_semantic_acquisition,omitempty"`
	LLMDependent               bool     `json:"llm_dependent,omitempty" yaml:"llm_dependent,omitempty"`
	ArchaeoOperation           string   `json:"archaeo_operation,omitempty" yaml:"archaeo_operation,omitempty"`
	ParadigmMix                []string `json:"paradigm_mix,omitempty" yaml:"paradigm_mix,omitempty"`
	TransitionCompatible       []string `json:"transition_compatible,omitempty" yaml:"transition_compatible,omitempty"`
	SupportingCapabilities     []string `json:"supporting_capabilities,omitempty" yaml:"supporting_capabilities,omitempty"`
	SupportingRoutines         []string `json:"supporting_routines,omitempty" yaml:"supporting_routines,omitempty"`
	ExpectedArtifactKinds      []string `json:"expected_artifact_kinds,omitempty" yaml:"expected_artifact_kinds,omitempty"`
	SupportedTransitionTargets []string `json:"supported_transition_targets,omitempty" yaml:"supported_transition_targets,omitempty"`
	ExecutionClass             string   `json:"execution_class,omitempty" yaml:"execution_class,omitempty"`
	PreferredTestLayer         string   `json:"preferred_test_layer,omitempty" yaml:"preferred_test_layer,omitempty"`
	AllowedTestLayers          []string `json:"allowed_test_layers,omitempty" yaml:"allowed_test_layers,omitempty"`
	BaselineEligible           bool     `json:"baseline_eligible,omitempty" yaml:"baseline_eligible,omitempty"`
	BenchmarkEligible          bool     `json:"benchmark_eligible,omitempty" yaml:"benchmark_eligible,omitempty"`
	Summary                    string   `json:"summary,omitempty" yaml:"summary,omitempty"`
}

// TriggerCatalogEntry is the CLI-local projection of a user-visible trigger.
type TriggerCatalogEntry struct {
	Mode             string   `json:"mode" yaml:"mode"`
	ModeIntentFamily string   `json:"mode_intent_family,omitempty" yaml:"mode_intent_family,omitempty"`
	ModePhases       []string `json:"mode_phases,omitempty" yaml:"mode_phases,omitempty"`
	Phrases          []string `json:"phrases" yaml:"phrases"`
	CapabilityID     string   `json:"capability_id,omitempty" yaml:"capability_id,omitempty"`
	PhaseJump        string   `json:"phase_jump,omitempty" yaml:"phase_jump,omitempty"`
	RequiresMode     string   `json:"requires_mode,omitempty" yaml:"requires_mode,omitempty"`
	Description      string   `json:"description,omitempty" yaml:"description,omitempty"`
}

// EucloJourneyStep represents a single local journey instruction.
type EucloJourneyStep struct {
	Kind                   string   `json:"kind" yaml:"kind"`
	Mode                   string   `json:"mode,omitempty" yaml:"mode,omitempty"`
	Phase                  string   `json:"phase,omitempty" yaml:"phase,omitempty"`
	Trigger                string   `json:"trigger,omitempty" yaml:"trigger,omitempty"`
	Capability             string   `json:"capability,omitempty" yaml:"capability,omitempty"`
	Text                   string   `json:"text,omitempty" yaml:"text,omitempty"`
	Key                    string   `json:"key,omitempty" yaml:"key,omitempty"`
	Value                  any      `json:"value,omitempty" yaml:"value,omitempty"`
	Expected               any      `json:"expected,omitempty" yaml:"expected,omitempty"`
	ExpectedMode           string   `json:"expected_mode,omitempty" yaml:"expected_mode,omitempty"`
	ExpectedPhase          string   `json:"expected_phase,omitempty" yaml:"expected_phase,omitempty"`
	ExpectedFrameKind      string   `json:"expected_frame_kind,omitempty" yaml:"expected_frame_kind,omitempty"`
	ExpectedResponseAction string   `json:"expected_response_action,omitempty" yaml:"expected_response_action,omitempty"`
	ExpectedArtifactKind   string   `json:"expected_artifact_kind,omitempty" yaml:"expected_artifact_kind,omitempty"`
	ExpectedStateKeys      []string `json:"expected_state_keys,omitempty" yaml:"expected_state_keys,omitempty"`
}

// EucloJourneyScript is a versioned local script for ordered semantic flows.
type EucloJourneyScript struct {
	ScriptVersion         string             `json:"script_version" yaml:"script_version"`
	InitialMode           string             `json:"initial_mode" yaml:"initial_mode"`
	InitialContext        map[string]any     `json:"initial_context,omitempty" yaml:"initial_context,omitempty"`
	Steps                 []EucloJourneyStep `json:"steps" yaml:"steps"`
	ExpectedTerminalState map[string]any     `json:"expected_terminal_state,omitempty" yaml:"expected_terminal_state,omitempty"`
	RecordingMode         string             `json:"recording_mode,omitempty" yaml:"recording_mode,omitempty"`
}

// EucloBenchmarkAxisSpec is a row in the model/provider axis set.
type EucloBenchmarkAxisSpec struct {
	Name          string `json:"name" yaml:"name"`
	Endpoint      string `json:"endpoint,omitempty" yaml:"endpoint,omitempty"`
	ResetStrategy string `json:"reset_strategy,omitempty" yaml:"reset_strategy,omitempty"`
	ResetBetween  bool   `json:"reset_between,omitempty" yaml:"reset_between,omitempty"`
}

// EucloBenchmarkMatrix is a local matrix definition for benchmark labeling.
type EucloBenchmarkMatrix struct {
	MatrixVersion string                   `json:"matrix_version" yaml:"matrix_version"`
	Name          string                   `json:"name,omitempty" yaml:"name,omitempty"`
	AxisOrder     string                   `json:"axis_order,omitempty" yaml:"axis_order,omitempty"`
	Capabilities  []string                 `json:"capabilities,omitempty" yaml:"capabilities,omitempty"`
	Models        []string                 `json:"models,omitempty" yaml:"models,omitempty"`
	Providers     []string                 `json:"providers,omitempty" yaml:"providers,omitempty"`
	ModelSet      []EucloBenchmarkAxisSpec `json:"model_set,omitempty" yaml:"model_set,omitempty"`
	ProviderSet   []EucloBenchmarkAxisSpec `json:"provider_set,omitempty" yaml:"provider_set,omitempty"`
	JourneyScript string                   `json:"journey_script,omitempty" yaml:"journey_script,omitempty"`
	Suite         string                   `json:"suite,omitempty" yaml:"suite,omitempty"`
}

var allowedJourneyStepKinds = map[string]struct{}{
	"mode.select":       {},
	"submode.select":    {},
	"trigger.fire":      {},
	"context.add":       {},
	"context.remove":    {},
	"frame.respond":     {},
	"transition.accept": {},
	"transition.reject": {},
	"hitl.approve":      {},
	"hitl.deny":         {},
	"workflow.resume":   {},
	"plan.promote":      {},
	"artifact.expect":   {},
}

func (s EucloJourneyScript) Validate() error {
	switch strings.TrimSpace(s.ScriptVersion) {
	case "":
		return errors.New("script_version is required")
	case "v1alpha1":
	default:
		return fmt.Errorf("unsupported script_version %q", s.ScriptVersion)
	}
	if strings.TrimSpace(s.InitialMode) == "" {
		return errors.New("initial_mode is required")
	}
	if len(s.Steps) == 0 {
		return errors.New("steps must not be empty")
	}
	switch strings.ToLower(strings.TrimSpace(s.RecordingMode)) {
	case "", "off", "live", "replay", "record":
	default:
		return fmt.Errorf("unsupported recording_mode %q", s.RecordingMode)
	}
	for i, step := range s.Steps {
		if strings.TrimSpace(step.Kind) == "" {
			return fmt.Errorf("steps[%d].kind is required", i)
		}
		if _, ok := allowedJourneyStepKinds[step.Kind]; !ok {
			return fmt.Errorf("steps[%d].kind %q is not supported", i, step.Kind)
		}
		switch step.Kind {
		case "mode.select", "submode.select":
			if step.Mode == "" && strings.TrimSpace(stringValue(step.Value)) == "" {
				return fmt.Errorf("steps[%d].mode or value is required for %s", i, step.Kind)
			}
		case "trigger.fire":
			if strings.TrimSpace(step.Text) == "" && strings.TrimSpace(stringValue(step.Value)) == "" && strings.TrimSpace(step.Trigger) == "" {
				return fmt.Errorf("steps[%d].text, trigger, or value is required for trigger.fire", i)
			}
		case "context.add", "context.remove":
			if strings.TrimSpace(step.Key) == "" {
				return fmt.Errorf("steps[%d].key is required for %s", i, step.Kind)
			}
			if step.Kind == "context.add" && step.Value == nil {
				return fmt.Errorf("steps[%d].value is required for context.add", i)
			}
		case "artifact.expect":
			if step.Expected == nil {
				return fmt.Errorf("steps[%d].expected is required for artifact.expect", i)
			}
		}
		for j, key := range step.ExpectedStateKeys {
			if strings.TrimSpace(key) == "" {
				return fmt.Errorf("steps[%d].expected_state_keys[%d] is empty", i, j)
			}
		}
	}
	return nil
}

func (m EucloBenchmarkMatrix) Validate() error {
	switch strings.TrimSpace(m.MatrixVersion) {
	case "":
		return errors.New("matrix_version is required")
	case "v1alpha1":
	default:
		return fmt.Errorf("unsupported matrix_version %q", m.MatrixVersion)
	}
	if strings.TrimSpace(m.Name) == "" {
		return errors.New("name is required")
	}
	for i, capability := range m.Capabilities {
		if strings.TrimSpace(capability) == "" {
			return fmt.Errorf("capabilities[%d] is empty", i)
		}
	}
	switch strings.TrimSpace(m.AxisOrder) {
	case "", "provider-first", "model-first":
	default:
		return fmt.Errorf("axis_order %q unsupported", m.AxisOrder)
	}
	for i, model := range m.Models {
		if strings.TrimSpace(model) == "" {
			return fmt.Errorf("models[%d] is empty", i)
		}
	}
	for i, provider := range m.Providers {
		if strings.TrimSpace(provider) == "" {
			return fmt.Errorf("providers[%d] is empty", i)
		}
	}
	for i, model := range m.ModelSet {
		if strings.TrimSpace(model.Name) == "" {
			return fmt.Errorf("model_set[%d].name is empty", i)
		}
	}
	for i, provider := range m.ProviderSet {
		if strings.TrimSpace(provider.Name) == "" {
			return fmt.Errorf("provider_set[%d].name is empty", i)
		}
		switch strings.TrimSpace(provider.ResetStrategy) {
		case "", "none", "model", "server":
		default:
			return fmt.Errorf("provider_set[%d].reset_strategy %q unsupported", i, provider.ResetStrategy)
		}
	}
	return nil
}

// EucloCapabilityRunResult is a lightweight execution summary for a selected capability.
type EucloCapabilityRunResult struct {
	RunClass      string                 `json:"run_class"`
	Capability    CapabilityCatalogEntry `json:"capability"`
	TestLayer     string                 `json:"test_layer,omitempty"`
	AllowedLayers []string               `json:"allowed_layers,omitempty"`
	Workspace     string                 `json:"workspace,omitempty"`
	Success       bool                   `json:"success"`
	Message       string                 `json:"message,omitempty"`
	Artifacts     []string               `json:"artifacts,omitempty"`
	TerminalState map[string]any         `json:"terminal_state,omitempty"`
}

// EucloTriggerResolution captures trigger lookup results.
type EucloTriggerResolution struct {
	RunClass string               `json:"run_class"`
	Mode     string               `json:"mode"`
	Text     string               `json:"text"`
	Matched  bool                 `json:"matched"`
	Trigger  *TriggerCatalogEntry `json:"trigger,omitempty"`
	Message  string               `json:"message,omitempty"`
}

// EucloTriggerFireResult captures a semantic trigger invocation summary.
type EucloTriggerFireResult struct {
	RunClass  string               `json:"run_class"`
	Mode      string               `json:"mode"`
	Phrase    string               `json:"phrase"`
	Matched   bool                 `json:"matched"`
	Trigger   *TriggerCatalogEntry `json:"trigger,omitempty"`
	Success   bool                 `json:"success"`
	Message   string               `json:"message,omitempty"`
	Artifacts []string             `json:"artifacts,omitempty"`
}

// EucloJourneyStepReport captures the outcome of a script step.
type EucloJourneyStepReport struct {
	Index          int                  `json:"index"`
	Kind           string               `json:"kind"`
	Mode           string               `json:"mode,omitempty"`
	Phase          string               `json:"phase,omitempty"`
	Trigger        string               `json:"trigger,omitempty"`
	Capability     string               `json:"capability,omitempty"`
	FrameKind      string               `json:"frame_kind,omitempty"`
	ResponseAction string               `json:"response_action,omitempty"`
	DurationMillis int64                `json:"duration_millis,omitempty"`
	Message        string               `json:"message,omitempty"`
	Updated        map[string]any       `json:"updated,omitempty"`
	Matched        *TriggerCatalogEntry `json:"matched,omitempty"`
	Artifacts      []string             `json:"artifacts,omitempty"`
	Success        bool                 `json:"success"`
}

// EucloJourneyReport summarizes a local journey execution.
type EucloJourneyReport struct {
	RunClass              string                         `json:"run_class"`
	Workspace             string                         `json:"workspace,omitempty"`
	RunMode               string                         `json:"run_mode,omitempty"`
	ScriptVersion         string                         `json:"script_version,omitempty"`
	InitialMode           string                         `json:"initial_mode,omitempty"`
	FinalMode             string                         `json:"final_mode,omitempty"`
	CurrentPhase          string                         `json:"current_phase,omitempty"`
	RecordingMode         string                         `json:"recording_mode,omitempty"`
	Steps                 []EucloJourneyStepReport       `json:"steps"`
	Transcript            []EucloJourneyTranscriptEntry  `json:"transcript,omitempty"`
	Frames                []EucloJourneyFrameRecord      `json:"frames,omitempty"`
	Responses             []EucloJourneyResponseRecord   `json:"responses,omitempty"`
	Transitions           []EucloJourneyTransitionRecord `json:"transitions,omitempty"`
	TerminalState         map[string]any                 `json:"terminal_state,omitempty"`
	ExpectedTerminalState map[string]any                 `json:"expected_terminal_state,omitempty"`
	Success               bool                           `json:"success"`
	Failures              []string                       `json:"failures,omitempty"`
}

// EucloBenchmarkCaseReport captures one matrix row.
type EucloBenchmarkCaseReport struct {
	RunClass      string              `json:"run_class"`
	Index         int                 `json:"index"`
	Capability    string              `json:"capability,omitempty"`
	Model         string              `json:"model,omitempty"`
	Provider      string              `json:"provider,omitempty"`
	ProviderReset string              `json:"provider_reset,omitempty"`
	ModelReset    string              `json:"model_reset,omitempty"`
	Success       bool                `json:"success"`
	Message       string              `json:"message,omitempty"`
	Journey       *EucloJourneyReport `json:"journey,omitempty"`
}

// EucloBenchmarkSummary captures aggregate benchmark statistics.
type EucloBenchmarkSummary struct {
	TotalCases         int      `json:"total_cases"`
	PassedCases        int      `json:"passed_cases"`
	FailedCases        int      `json:"failed_cases"`
	JourneyCases       int      `json:"journey_cases"`
	UniqueCapabilities []string `json:"unique_capabilities,omitempty"`
	UniqueModels       []string `json:"unique_models,omitempty"`
	UniqueProviders    []string `json:"unique_providers,omitempty"`
}

// EucloBenchmarkComparisonReport captures a baseline comparison request.
type EucloBenchmarkComparisonReport struct {
	RunClass      string `json:"run_class"`
	Baseline      string `json:"baseline"`
	BaselineBytes int    `json:"baseline_bytes"`
	Success       bool   `json:"success"`
	Message       string `json:"message,omitempty"`
}

// EucloBaselineCapabilityReport captures a deterministic capability baseline check.
type EucloBaselineCapabilityReport struct {
	RunClass                     string                  `json:"run_class"`
	Selector                     string                  `json:"selector"`
	Exact                        bool                    `json:"exact"`
	BenchmarkAggregationDisabled bool                    `json:"benchmark_aggregation_disabled"`
	Workspace                    string                  `json:"workspace,omitempty"`
	Capability                   *CapabilityCatalogEntry `json:"capability,omitempty"`
	Success                      bool                    `json:"success"`
	Message                      string                  `json:"message,omitempty"`
	Failures                     []string                `json:"failures,omitempty"`
}

// EucloBaselineReport summarizes baseline-only capability evaluation.
type EucloBaselineReport struct {
	RunClass                     string                          `json:"run_class"`
	Workspace                    string                          `json:"workspace,omitempty"`
	Layer                        string                          `json:"layer,omitempty"`
	Exact                        bool                            `json:"exact"`
	BenchmarkAggregationDisabled bool                            `json:"benchmark_aggregation_disabled"`
	Capabilities                 []EucloBaselineCapabilityReport `json:"capabilities,omitempty"`
	Success                      bool                            `json:"success"`
	Failures                     []string                        `json:"failures,omitempty"`
}

// EucloBenchmarkReport summarizes matrix expansion and execution.
type EucloBenchmarkReport struct {
	RunClass  string                     `json:"run_class"`
	Workspace string                     `json:"workspace,omitempty"`
	Matrix    EucloBenchmarkMatrix       `json:"matrix"`
	Summary   EucloBenchmarkSummary      `json:"summary"`
	Cases     []EucloBenchmarkCaseReport `json:"cases"`
	Success   bool                       `json:"success"`
}
