package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/lexcodex/relurpify/named/euclo/interaction"
	euclomodes "github.com/lexcodex/relurpify/named/euclo/interaction/modes"
	euclorelurpic "github.com/lexcodex/relurpify/named/euclo/relurpicabilities"
	"gopkg.in/yaml.v3"
)

func (r *eucloCommandRunner) ListCapabilities(ctx context.Context) ([]CapabilityCatalogEntry, error) {
	_ = ctx
	return newEucloCatalog().Capabilities(), nil
}

func (r *eucloCommandRunner) SelectCapabilities(selector string) ([]CapabilityCatalogEntry, error) {
	return newEucloCatalog().SelectCapabilities(selector)
}

func (r *eucloCommandRunner) ShowCapability(ctx context.Context, selector string) (*CapabilityCatalogEntry, error) {
	_ = ctx
	return newEucloCatalog().ShowCapability(selector)
}

func (r *eucloCommandRunner) RunCapability(ctx context.Context, selector string) (*EucloCapabilityRunResult, error) {
	_ = ctx
	entry, err := r.ShowCapability(context.Background(), selector)
	if err != nil {
		return nil, err
	}
	return &EucloCapabilityRunResult{
		RunClass:      "capability",
		Capability:    *entry,
		TestLayer:     entry.PreferredTestLayer,
		AllowedLayers: append([]string(nil), entry.AllowedTestLayers...),
		Workspace:     ensureWorkspace(),
		Success:       true,
		Message:       fmt.Sprintf("selected capability %s", entry.ID),
		Artifacts:     []string{entry.ID},
		TerminalState: map[string]any{
			"selected_capability": entry.ID,
		},
	}, nil
}

func (r *eucloCommandRunner) ListTriggers(ctx context.Context, mode string) ([]TriggerCatalogEntry, error) {
	_ = ctx
	return newEucloCatalog().ListTriggers(mode), nil
}

func (r *eucloCommandRunner) ResolveTrigger(ctx context.Context, mode, text string) (*EucloTriggerResolution, error) {
	_ = ctx
	catalog := newEucloCatalog()
	trigger, ok := catalog.ResolveTrigger(mode, text)
	result := &EucloTriggerResolution{
		RunClass: "trigger",
		Mode:     strings.TrimSpace(mode),
		Text:     text,
		Matched:  ok,
		Message:  "no trigger matched",
	}
	if !ok {
		return result, nil
	}
	result.Trigger = trigger
	result.Message = fmt.Sprintf("matched %s", triggerLabel(trigger))
	return result, nil
}

func (r *eucloCommandRunner) FireTrigger(ctx context.Context, mode, phrase string) (*EucloTriggerFireResult, error) {
	_ = ctx
	resolution, err := r.ResolveTrigger(context.Background(), mode, phrase)
	if err != nil {
		return nil, err
	}
	result := &EucloTriggerFireResult{
		RunClass: "trigger",
		Mode:     resolution.Mode,
		Phrase:   phrase,
		Matched:  resolution.Matched,
		Trigger:  resolution.Trigger,
		Success:  true,
		Message:  "no trigger matched",
	}
	if resolution.Matched && resolution.Trigger != nil {
		result.Message = fmt.Sprintf("fired %s", triggerLabel(resolution.Trigger))
		result.Artifacts = []string{triggerArtifactName(resolution.Trigger)}
	}
	return result, nil
}

func (r *eucloCommandRunner) RunJourney(ctx context.Context, script EucloJourneyScript) (*EucloJourneyReport, error) {
	return executeEucloJourneyScript(ctx, script, newEucloCatalog())
}

func (r *eucloCommandRunner) RunBenchmark(ctx context.Context, matrix EucloBenchmarkMatrix) (*EucloBenchmarkReport, error) {
	_ = ctx
	if strings.TrimSpace(matrix.MatrixVersion) == "" {
		matrix.MatrixVersion = "v1alpha1"
	}
	if strings.TrimSpace(matrix.Name) == "" {
		matrix.Name = "euclo-benchmark"
	}
	if err := matrix.Validate(); err != nil {
		return nil, err
	}
	var cases []EucloBenchmarkCaseReport
	capabilities := matrix.Capabilities
	if len(capabilities) == 0 {
		capabilities = []string{""}
	}
	models := benchmarkMatrixModels(matrix)
	providers := benchmarkMatrixProviders(matrix)
	if len(models) == 0 {
		models = []EucloBenchmarkAxisSpec{{}}
	}
	if len(providers) == 0 {
		providers = []EucloBenchmarkAxisSpec{{}}
	}
	order := strings.TrimSpace(strings.ToLower(matrix.AxisOrder))
	if order == "" {
		order = "provider-first"
	}
	for _, capability := range capabilities {
		if order == "model-first" {
			for _, model := range models {
				for _, provider := range providers {
					entry := benchmarkMatrixCaseRow(capability, model, provider)
					if strings.TrimSpace(matrix.JourneyScript) != "" {
						if err := benchmarkMatrixRunJourney(ctx, r, matrix.JourneyScript, &entry); err != nil {
							entry.Success = false
							entry.Message = err.Error()
						}
					} else {
						entry.Message = "matrix row labeled"
					}
					cases = append(cases, entry)
				}
			}
			continue
		}
		for _, provider := range providers {
			for _, model := range models {
				entry := benchmarkMatrixCaseRow(capability, model, provider)
				if strings.TrimSpace(matrix.JourneyScript) != "" {
					if err := benchmarkMatrixRunJourney(ctx, r, matrix.JourneyScript, &entry); err != nil {
						entry.Success = false
						entry.Message = err.Error()
					}
				} else {
					entry.Message = "matrix row labeled"
				}
				cases = append(cases, entry)
			}
		}
	}
	report := &EucloBenchmarkReport{
		RunClass:  "benchmark",
		Workspace: ensureWorkspace(),
		Matrix:    matrix,
		Summary:   summarizeEucloBenchmarkCases(cases),
		Cases:     cases,
		Success:   true,
	}
	for _, entry := range cases {
		if !entry.Success {
			report.Success = false
			break
		}
	}
	return report, nil
}

func (r *eucloCommandRunner) RunBaseline(ctx context.Context, selectors []string) (*EucloBaselineReport, error) {
	_ = ctx
	catalog := newEucloCatalog()
	if len(selectors) == 0 {
		for _, entry := range catalog.BaselineCapabilities() {
			selectors = append(selectors, entry.ID)
		}
	}
	report := &EucloBaselineReport{
		RunClass:                     "baseline",
		Workspace:                    ensureWorkspace(),
		Layer:                        "baseline",
		Exact:                        true,
		BenchmarkAggregationDisabled: true,
		Success:                      true,
	}
	for _, selector := range selectors {
		matches, err := catalog.SelectCapabilities(selector)
		baselineCase := EucloBaselineCapabilityReport{
			RunClass:                     "baseline",
			Selector:                     selector,
			Exact:                        true,
			BenchmarkAggregationDisabled: true,
			Workspace:                    ensureWorkspace(),
			Success:                      true,
		}
		if err != nil {
			baselineCase.Success = false
			baselineCase.Message = err.Error()
			baselineCase.Failures = append(baselineCase.Failures, err.Error())
			report.Success = false
			report.Failures = append(report.Failures, err.Error())
			report.Capabilities = append(report.Capabilities, baselineCase)
			continue
		}
		if len(matches) == 0 {
			err := fmt.Errorf("baseline selector %q matched no capabilities", selector)
			baselineCase.Success = false
			baselineCase.Message = err.Error()
			baselineCase.Failures = append(baselineCase.Failures, err.Error())
			report.Success = false
			report.Failures = append(report.Failures, err.Error())
			report.Capabilities = append(report.Capabilities, baselineCase)
			continue
		}
		if len(matches) > 1 {
			err := fmt.Errorf("baseline selector %q matched %d capabilities", selector, len(matches))
			baselineCase.Success = false
			baselineCase.Message = err.Error()
			baselineCase.Failures = append(baselineCase.Failures, err.Error())
			report.Success = false
			report.Failures = append(report.Failures, err.Error())
			report.Capabilities = append(report.Capabilities, baselineCase)
			continue
		}
		capability := matches[0]
		baselineCase.Capability = &capability
		baselineCase.Message = fmt.Sprintf("baseline snapshot for %s", capability.ID)
		if !capability.BaselineEligible {
			err := fmt.Errorf("capability %s is not baseline eligible", capability.ID)
			baselineCase.Success = false
			baselineCase.Failures = append(baselineCase.Failures, err.Error())
			baselineCase.Message = err.Error()
			report.Success = false
			report.Failures = append(report.Failures, err.Error())
		}
		if capability.PreferredTestLayer != "baseline" {
			err := fmt.Errorf("capability %s preferred_test_layer=%s", capability.ID, capability.PreferredTestLayer)
			baselineCase.Failures = append(baselineCase.Failures, err.Error())
			baselineCase.Message = err.Error()
			baselineCase.Success = false
			report.Success = false
			report.Failures = append(report.Failures, err.Error())
		}
		report.Capabilities = append(report.Capabilities, baselineCase)
	}
	report.Failures = uniqueStrings(report.Failures)
	return report, nil
}

func newEucloTriggerResolver() *interaction.AgencyResolver {
	resolver := interaction.NewAgencyResolver()
	interaction.RegisterHelpTriggers(resolver)
	euclomodes.RegisterChatTriggers(resolver)
	euclomodes.RegisterDebugTriggers(resolver)
	euclomodes.RegisterPlanningTriggers(resolver)
	euclomodes.RegisterCodeTriggers(resolver)
	euclomodes.RegisterTDDTriggers(resolver)
	euclomodes.RegisterReviewTriggers(resolver)
	return resolver
}

func loadEucloJourneyScript(path string) (EucloJourneyScript, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return EucloJourneyScript{}, err
	}
	var script EucloJourneyScript
	if err := yaml.Unmarshal(data, &script); err != nil {
		return EucloJourneyScript{}, err
	}
	if err := script.Validate(); err != nil {
		return EucloJourneyScript{}, err
	}
	return script, nil
}

func loadEucloBenchmarkMatrix(path string) (EucloBenchmarkMatrix, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return EucloBenchmarkMatrix{}, err
	}
	var matrix EucloBenchmarkMatrix
	if err := yaml.Unmarshal(data, &matrix); err != nil {
		return EucloBenchmarkMatrix{}, err
	}
	if err := matrix.Validate(); err != nil {
		return EucloBenchmarkMatrix{}, err
	}
	return matrix, nil
}

func benchmarkMatrixModels(matrix EucloBenchmarkMatrix) []EucloBenchmarkAxisSpec {
	if len(matrix.ModelSet) > 0 {
		return normalizeBenchmarkAxisSpecs(matrix.ModelSet)
	}
	if len(matrix.Models) == 0 {
		return nil
	}
	rows := make([]EucloBenchmarkAxisSpec, 0, len(matrix.Models))
	for _, model := range matrix.Models {
		rows = append(rows, EucloBenchmarkAxisSpec{Name: strings.TrimSpace(model)})
	}
	return rows
}

func benchmarkMatrixProviders(matrix EucloBenchmarkMatrix) []EucloBenchmarkAxisSpec {
	if len(matrix.ProviderSet) > 0 {
		return normalizeBenchmarkAxisSpecs(matrix.ProviderSet)
	}
	if len(matrix.Providers) == 0 {
		return nil
	}
	rows := make([]EucloBenchmarkAxisSpec, 0, len(matrix.Providers))
	for _, provider := range matrix.Providers {
		rows = append(rows, EucloBenchmarkAxisSpec{Name: strings.TrimSpace(provider)})
	}
	return rows
}

func normalizeBenchmarkAxisSpecs(specs []EucloBenchmarkAxisSpec) []EucloBenchmarkAxisSpec {
	rows := make([]EucloBenchmarkAxisSpec, 0, len(specs))
	for _, spec := range specs {
		trimmed := EucloBenchmarkAxisSpec{
			Name:          strings.TrimSpace(spec.Name),
			Endpoint:      strings.TrimSpace(spec.Endpoint),
			ResetStrategy: strings.TrimSpace(spec.ResetStrategy),
			ResetBetween:  spec.ResetBetween,
		}
		rows = append(rows, trimmed)
	}
	return rows
}

func benchmarkMatrixCaseRow(capability string, model, provider EucloBenchmarkAxisSpec) EucloBenchmarkCaseReport {
	return EucloBenchmarkCaseReport{
		RunClass:      "benchmark",
		Capability:    strings.TrimSpace(capability),
		Model:         model.Name,
		Provider:      provider.Name,
		ModelReset:    model.ResetStrategy,
		ProviderReset: provider.ResetStrategy,
		Success:       true,
	}
}

func benchmarkMatrixRunJourney(ctx context.Context, runner *eucloCommandRunner, scriptPath string, entry *EucloBenchmarkCaseReport) error {
	script, err := loadEucloJourneyScript(scriptPath)
	if err != nil {
		return err
	}
	report, err := runner.RunJourney(ctx, script)
	if err != nil {
		return err
	}
	entry.Journey = report
	entry.Message = fmt.Sprintf("journey run (%d steps)", len(report.Steps))
	entry.Success = report.Success
	return nil
}

func capabilityIDs(reg *euclorelurpic.Registry) []string {
	if reg == nil {
		return nil
	}
	entries := reg.IDsForMode("")
	// IDsForMode only returns exact mode family matches, so use the registry's
	// internal descriptors via a capability list reconstructed from the known modes.
	if len(entries) > 0 {
		return entries
	}
	ids := make([]string, 0)
	for _, mode := range []string{"chat", "planning", "debug"} {
		ids = append(ids, reg.IDsForMode(mode)...)
	}
	if len(ids) == 0 {
		return ids
	}
	sort.Strings(ids)
	ids = uniqueStrings(ids)
	return ids
}

func capabilityEntryFromDescriptor(desc euclorelurpic.Descriptor) CapabilityCatalogEntry {
	allowedLayers := capabilityAllowedLayers(desc)
	return CapabilityCatalogEntry{
		ID:                         desc.ID,
		DisplayName:                desc.DisplayName,
		PrimaryOwner:               ownerFromCapabilityID(desc.ID),
		ModeFamilies:               desc.ModeFamilies,
		PrimaryCapable:             desc.PrimaryCapable,
		SupportingOnly:             desc.SupportingOnly,
		Mutability:                 string(desc.Mutability),
		ArchaeoAssociated:          desc.ArchaeoAssociated,
		LazySemanticAcquisition:    desc.LazySemanticAcquisition,
		LLMDependent:               desc.LLMDependent,
		ArchaeoOperation:           desc.ArchaeoOperation,
		ParadigmMix:                append([]string(nil), desc.ParadigmMix...),
		TransitionCompatible:       append([]string(nil), desc.TransitionCompatible...),
		SupportingCapabilities:     append([]string(nil), desc.SupportingCapabilities...),
		SupportingRoutines:         supportingRoutinesForDescriptor(desc),
		ExpectedArtifactKinds:      expectedArtifactKindsForCapability(desc.ID),
		SupportedTransitionTargets: append([]string(nil), desc.TransitionCompatible...),
		ExecutionClass:             capabilityExecutionClass(desc),
		PreferredTestLayer:         capabilityPreferredLayer(desc),
		AllowedTestLayers:          allowedLayers,
		BaselineEligible:           containsString(allowedLayers, "baseline"),
		BenchmarkEligible:          true,
		Summary:                    desc.Summary,
	}
}

func capabilityMatchesSelector(entry CapabilityCatalogEntry, selector string) bool {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return false
	}
	if entry.ID == selector || strings.EqualFold(entry.ID, selector) {
		return true
	}
	if strings.HasPrefix(entry.ID, selector) {
		return true
	}
	if len(entry.ModeFamilies) > 0 && strings.EqualFold(entry.ModeFamilies[0], selector) {
		return true
	}
	return strings.Contains(strings.ToLower(entry.DisplayName), strings.ToLower(selector))
}

func triggerEntryFromMode(mode string, trigger interaction.AgencyTrigger) TriggerCatalogEntry {
	mode = strings.TrimSpace(strings.ToLower(mode))
	return TriggerCatalogEntry{
		Mode:             mode,
		ModeIntentFamily: modeIntentFamilyForID(mode),
		ModePhases:       phaseIDsForMode(mode),
		Phrases:          append([]string(nil), trigger.Phrases...),
		CapabilityID:     trigger.CapabilityID,
		PhaseJump:        trigger.PhaseJump,
		RequiresMode:     trigger.RequiresMode,
		Description:      trigger.Description,
	}
}

func triggerLabel(entry *TriggerCatalogEntry) string {
	if entry == nil {
		return ""
	}
	if len(entry.Phrases) > 0 {
		return fmt.Sprintf("%q", entry.Phrases[0])
	}
	if entry.CapabilityID != "" {
		return entry.CapabilityID
	}
	if entry.PhaseJump != "" {
		return entry.PhaseJump
	}
	return entry.Description
}

func triggerArtifactName(entry *TriggerCatalogEntry) string {
	if entry == nil {
		return ""
	}
	if entry.CapabilityID != "" {
		return entry.CapabilityID
	}
	return strings.Join(entry.Phrases, ",")
}
