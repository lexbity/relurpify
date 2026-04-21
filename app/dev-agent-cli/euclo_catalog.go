package main

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	euclocore "codeburg.org/lexbit/relurpify/named/euclo/core"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
	euclomodes "codeburg.org/lexbit/relurpify/named/euclo/interaction/modes"
	euclorelurpic "codeburg.org/lexbit/relurpify/named/euclo/relurpicabilities"
)

type EucloModeCatalogEntry struct {
	ID                          string   `json:"id" yaml:"id"`
	IntentFamily                string   `json:"intent_family,omitempty" yaml:"intent_family,omitempty"`
	EditPolicy                  string   `json:"edit_policy,omitempty" yaml:"edit_policy,omitempty"`
	EvidencePolicy              string   `json:"evidence_policy,omitempty" yaml:"evidence_policy,omitempty"`
	VerificationPolicy          string   `json:"verification_policy,omitempty" yaml:"verification_policy,omitempty"`
	ReviewPolicy                string   `json:"review_policy,omitempty" yaml:"review_policy,omitempty"`
	DefaultExecutionProfiles    []string `json:"default_execution_profiles,omitempty" yaml:"default_execution_profiles,omitempty"`
	FallbackExecutionProfiles   []string `json:"fallback_execution_profiles,omitempty" yaml:"fallback_execution_profiles,omitempty"`
	PreferredCapabilityFamilies []string `json:"preferred_capability_families,omitempty" yaml:"preferred_capability_families,omitempty"`
	ContextStrategy             string   `json:"context_strategy,omitempty" yaml:"context_strategy,omitempty"`
	RecoveryPolicy              string   `json:"recovery_policy,omitempty" yaml:"recovery_policy,omitempty"`
	ReportingPolicy             string   `json:"reporting_policy,omitempty" yaml:"reporting_policy,omitempty"`
	Phases                      []string `json:"phases,omitempty" yaml:"phases,omitempty"`
}

type EucloCatalog struct {
	capabilities   []CapabilityCatalogEntry
	capabilityByID map[string]CapabilityCatalogEntry
	modeByID       map[string]EucloModeCatalogEntry
}

func newEucloCatalog() *EucloCatalog {
	catalog := &EucloCatalog{
		capabilityByID: map[string]CapabilityCatalogEntry{},
		modeByID:       map[string]EucloModeCatalogEntry{},
	}

	for _, mode := range euclotypes.DefaultModeRegistry().List() {
		entry := eucloModeEntryFromDescriptor(mode)
		catalog.modeByID[entry.ID] = entry
	}

	base := euclocore.DefaultRelurpicRegistry()
	if base != nil {
		for _, id := range base.IDsForMode("") {
			desc, ok := base.Lookup(id)
			if !ok {
				continue
			}
			entry := capabilityEntryFromDescriptor(desc)
			catalog.capabilityByID[entry.ID] = entry
		}
		for _, mode := range []string{"chat", "planning", "debug"} {
			for _, id := range base.IDsForMode(mode) {
				desc, ok := base.Lookup(id)
				if !ok {
					continue
				}
				entry := capabilityEntryFromDescriptor(desc)
				catalog.capabilityByID[entry.ID] = entry
			}
		}
	}

	for _, entry := range eucloSupplementalCapabilityEntries() {
		catalog.capabilityByID[entry.ID] = entry
	}

	catalog.capabilities = make([]CapabilityCatalogEntry, 0, len(catalog.capabilityByID))
	for _, entry := range catalog.capabilityByID {
		catalog.capabilities = append(catalog.capabilities, entry)
	}
	sort.Slice(catalog.capabilities, func(i, j int) bool {
		return catalog.capabilities[i].ID < catalog.capabilities[j].ID
	})
	return catalog
}

func (c *EucloCatalog) Capabilities() []CapabilityCatalogEntry {
	if c == nil {
		return nil
	}
	return append([]CapabilityCatalogEntry(nil), c.capabilities...)
}

func (c *EucloCatalog) BaselineCapabilities() []CapabilityCatalogEntry {
	if c == nil {
		return nil
	}
	out := make([]CapabilityCatalogEntry, 0, len(c.capabilities))
	for _, entry := range c.capabilities {
		if entry.BaselineEligible {
			out = append(out, entry)
		}
	}
	return out
}

func (c *EucloCatalog) CapabilityByID(id string) (*CapabilityCatalogEntry, bool) {
	if c == nil {
		return nil, false
	}
	trimmed := strings.TrimSpace(id)
	entry, ok := c.capabilityByID[trimmed]
	if !ok {
		entry, ok = c.capabilityByID[strings.ToLower(trimmed)]
	}
	if !ok {
		return nil, false
	}
	copy := entry
	return &copy, true
}

func (c *EucloCatalog) ModeByID(id string) (*EucloModeCatalogEntry, bool) {
	if c == nil {
		return nil, false
	}
	entry, ok := c.modeByID[strings.TrimSpace(strings.ToLower(id))]
	if !ok {
		return nil, false
	}
	copy := entry
	return &copy, true
}

func (c *EucloCatalog) SelectCapabilities(selector string) ([]CapabilityCatalogEntry, error) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return nil, errors.New("selector is required")
	}
	if c == nil {
		return nil, nil
	}

	if strings.HasPrefix(strings.ToLower(selector), "trigger:") {
		return c.selectByTriggerPhrase(strings.TrimSpace(selector[len("trigger:"):]))
	}
	if strings.HasPrefix(strings.ToLower(selector), "mode:") {
		return c.selectByMode(strings.TrimSpace(selector[len("mode:"):])), nil
	}

	if exact, ok := c.CapabilityByID(selector); ok {
		return []CapabilityCatalogEntry{*exact}, nil
	}
	if modeMatches := c.selectByMode(selector); len(modeMatches) > 0 {
		return modeMatches, nil
	}
	if strings.HasSuffix(selector, "*") {
		return c.selectByPrefix(strings.TrimSuffix(selector, "*"))
	}
	if prefixMatches, err := c.selectByPrefix(selector); err == nil && len(prefixMatches) > 0 {
		return prefixMatches, nil
	}
	if triggerMatches, err := c.selectByTriggerPhrase(selector); err == nil && len(triggerMatches) > 0 {
		return triggerMatches, nil
	}
	return nil, fmt.Errorf("capability selector %q matched no entries", selector)
}

func (c *EucloCatalog) ShowCapability(selector string) (*CapabilityCatalogEntry, error) {
	matches, err := c.SelectCapabilities(selector)
	if err != nil {
		return nil, err
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("capability %q not found", selector)
	}
	if len(matches) > 1 {
		return nil, fmt.Errorf("capability selector %q matched %d entries", selector, len(matches))
	}
	return &matches[0], nil
}

func (c *EucloCatalog) ListTriggers(mode string) []TriggerCatalogEntry {
	resolver := newEucloTriggerResolver()
	triggers := resolver.TriggersForMode(mode)
	out := make([]TriggerCatalogEntry, 0, len(triggers))
	for _, trigger := range triggers {
		out = append(out, triggerEntryFromMode(mode, trigger))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Mode == out[j].Mode {
			return strings.Join(out[i].Phrases, ",") < strings.Join(out[j].Phrases, ",")
		}
		return out[i].Mode < out[j].Mode
	})
	return out
}

func (c *EucloCatalog) ResolveTrigger(mode, text string) (*TriggerCatalogEntry, bool) {
	resolver := newEucloTriggerResolver()
	trigger, ok := resolver.Resolve(mode, text)
	if !ok || trigger == nil {
		return nil, false
	}
	entry := triggerEntryFromMode(mode, *trigger)
	return &entry, true
}

func (c *EucloCatalog) selectByMode(mode string) []CapabilityCatalogEntry {
	mode = strings.TrimSpace(strings.ToLower(mode))
	if mode == "" {
		return nil
	}
	var out []CapabilityCatalogEntry
	for _, entry := range c.capabilities {
		if strings.EqualFold(entry.ModeFamilies[0], mode) || strings.EqualFold(entry.PrimaryOwner, mode) {
			out = append(out, entry)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return uniqueCapabilityEntries(out)
}

func (c *EucloCatalog) selectByPrefix(prefix string) ([]CapabilityCatalogEntry, error) {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return nil, nil
	}
	var out []CapabilityCatalogEntry
	for _, entry := range c.capabilities {
		if strings.HasPrefix(strings.ToLower(entry.ID), strings.ToLower(prefix)) {
			out = append(out, entry)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return uniqueCapabilityEntries(out), nil
}

func (c *EucloCatalog) selectByTriggerPhrase(phrase string) ([]CapabilityCatalogEntry, error) {
	phrase = strings.TrimSpace(phrase)
	if phrase == "" {
		return nil, nil
	}
	resolver := newEucloTriggerResolver()
	var matches []CapabilityCatalogEntry
	for _, mode := range triggerModes() {
		for _, trigger := range resolver.TriggersForMode(mode) {
			if !triggerPhraseMatches(trigger, phrase) {
				continue
			}
			if strings.TrimSpace(trigger.CapabilityID) == "" {
				continue
			}
			if entry, ok := c.CapabilityByID(trigger.CapabilityID); ok {
				matches = append(matches, *entry)
			}
		}
	}
	for _, trigger := range resolver.TriggersForMode("") {
		if !triggerPhraseMatches(trigger, phrase) {
			continue
		}
		if strings.TrimSpace(trigger.CapabilityID) == "" {
			continue
		}
		if entry, ok := c.CapabilityByID(trigger.CapabilityID); ok {
			matches = append(matches, *entry)
		}
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].ID < matches[j].ID })
	return uniqueCapabilityEntries(matches), nil
}

func triggerPhraseMatches(trigger interaction.AgencyTrigger, phrase string) bool {
	if phrase == "" {
		return false
	}
	normalized := strings.ToLower(strings.TrimSpace(phrase))
	for _, candidate := range trigger.Phrases {
		trimmed := strings.ToLower(strings.TrimSpace(candidate))
		if trimmed == normalized || strings.Contains(normalized, trimmed) || strings.Contains(trimmed, normalized) {
			return true
		}
	}
	return false
}

func uniqueCapabilityEntries(entries []CapabilityCatalogEntry) []CapabilityCatalogEntry {
	if len(entries) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]CapabilityCatalogEntry, 0, len(entries))
	for _, entry := range entries {
		if _, ok := seen[entry.ID]; ok {
			continue
		}
		seen[entry.ID] = struct{}{}
		out = append(out, entry)
	}
	return out
}

func eucloModeEntryFromDescriptor(desc euclotypes.ModeDescriptor) EucloModeCatalogEntry {
	return EucloModeCatalogEntry{
		ID:                          desc.ModeID,
		IntentFamily:                desc.IntentFamily,
		EditPolicy:                  desc.EditPolicy,
		EvidencePolicy:              desc.EvidencePolicy,
		VerificationPolicy:          desc.VerificationPolicy,
		ReviewPolicy:                desc.ReviewPolicy,
		DefaultExecutionProfiles:    append([]string(nil), desc.DefaultExecutionProfiles...),
		FallbackExecutionProfiles:   append([]string(nil), desc.FallbackExecutionProfiles...),
		PreferredCapabilityFamilies: append([]string(nil), desc.PreferredCapabilityFamilies...),
		ContextStrategy:             desc.ContextStrategy,
		RecoveryPolicy:              desc.RecoveryPolicy,
		ReportingPolicy:             desc.ReportingPolicy,
		Phases:                      phaseIDsForMode(desc.ModeID),
	}
}

func phaseIDsForMode(mode string) []string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "chat":
		return euclomodes.ChatPhaseIDs()
	case "code":
		return euclomodes.CodePhaseIDs()
	case "debug":
		return euclomodes.DebugPhaseIDs()
	case "planning":
		return euclomodes.PlanningPhaseIDs()
	case "review":
		return euclomodes.ReviewPhaseIDs()
	case "tdd":
		return euclomodes.TDDPhaseIDs()
	default:
		return nil
	}
}

func triggerModes() []string {
	return []string{"chat", "code", "debug", "planning", "review", "tdd"}
}

func modeIntentFamilyForID(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "chat":
		return "conversational"
	case "code":
		return "implementation"
	case "debug":
		return "debugging"
	case "planning":
		return "planning"
	case "review":
		return "review"
	case "tdd":
		return "test_driven_development"
	default:
		return ""
	}
}

func ownerFromCapabilityID(id string) string {
	trimmed := strings.TrimSpace(id)
	trimmed = strings.TrimPrefix(trimmed, "euclo:")
	if trimmed == "" {
		return ""
	}
	parts := strings.Split(trimmed, ".")
	if len(parts) == 0 {
		return trimmed
	}
	return parts[0]
}

func supportingRoutinesForDescriptor(desc euclorelurpic.Descriptor) []string {
	return nil
}

func capabilityExecutionClass(desc euclorelurpic.Descriptor) string {
	switch strings.TrimSpace(desc.ID) {
	case euclorelurpic.CapabilityArchaeologyCompilePlan,
		euclorelurpic.CapabilityArchaeologyImplement:
		return "journey_only"
	default:
		if desc.SupportingOnly {
			return "baseline_safe"
		}
		return "baseline_safe"
	}
}

func capabilityPreferredLayer(desc euclorelurpic.Descriptor) string {
	if capabilityExecutionClass(desc) == "journey_only" {
		return "journey"
	}
	return "baseline"
}

func capabilityAllowedLayers(desc euclorelurpic.Descriptor) []string {
	if capabilityExecutionClass(desc) == "journey_only" {
		return []string{"journey", "benchmark"}
	}
	return []string{"baseline", "journey", "benchmark"}
}

func expectedArtifactKindsForCapability(id string) []string {
	switch strings.TrimSpace(id) {
	case euclorelurpic.CapabilityChatAsk:
		return []string{string(euclotypes.ArtifactKindAnalyze), string(euclotypes.ArtifactKindReviewFindings), string(euclotypes.ArtifactKindPlanCandidates)}
	case euclorelurpic.CapabilityChatImplement:
		return []string{string(euclotypes.ArtifactKindEditIntent), string(euclotypes.ArtifactKindEditExecution), string(euclotypes.ArtifactKindVerification)}
	case euclorelurpic.CapabilityChatInspect:
		return []string{string(euclotypes.ArtifactKindAnalyze), string(euclotypes.ArtifactKindReviewFindings), string(euclotypes.ArtifactKindCompatibilityAssessment)}
	case euclorelurpic.CapabilityArchaeologyExplore:
		return []string{string(euclotypes.ArtifactKindExplore), string(euclotypes.ArtifactKindReviewFindings), string(euclotypes.ArtifactKindPlanCandidates)}
	case euclorelurpic.CapabilityArchaeologyCompilePlan:
		return []string{string(euclotypes.ArtifactKindPlan), string(euclotypes.ArtifactKindMigrationPlan), string(euclotypes.ArtifactKindPlanCandidates)}
	case euclorelurpic.CapabilityArchaeologyImplement:
		return []string{string(euclotypes.ArtifactKindCompiledExecution), string(euclotypes.ArtifactKindExecutionStatus), string(euclotypes.ArtifactKindFinalReport)}
	case euclorelurpic.CapabilityDebugInvestigateRepair:
		return []string{string(euclotypes.ArtifactKindReproduction), string(euclotypes.ArtifactKindRootCause), string(euclotypes.ArtifactKindVerificationSummary)}
	case euclorelurpic.CapabilityBKCCompile, euclorelurpic.CapabilityBKCStream, euclorelurpic.CapabilityBKCCheckpoint, euclorelurpic.CapabilityBKCInvalidate:
		return []string{string(euclotypes.ArtifactKindSemanticCompile), string(euclotypes.ArtifactKindSemanticContext), string(euclotypes.ArtifactKindTension), string(euclotypes.ArtifactKindContextCompaction)}
	}
	return nil
}

func eucloSupplementalCapabilityEntries() []CapabilityCatalogEntry {
	return []CapabilityCatalogEntry{
		{
			ID:                         "euclo:design.alternatives",
			DisplayName:                "Design Alternatives",
			PrimaryOwner:               "design",
			ModeFamilies:               []string{"planning"},
			PrimaryCapable:             true,
			Mutability:                 string(euclorelurpic.MutabilityNonMutating),
			ParadigmMix:                []string{"planner", "reflection"},
			TransitionCompatible:       []string{"planning", "code"},
			SupportingCapabilities:     []string{"euclo:review.compatibility", "euclo:review.semantic"},
			SupportingRoutines:         []string{"chat.ask.react_inquiry"},
			ExpectedArtifactKinds:      []string{string(euclotypes.ArtifactKindPlanCandidates), string(euclotypes.ArtifactKindPlan)},
			SupportedTransitionTargets: []string{"planning", "code"},
			ExecutionClass:             "baseline_safe",
			PreferredTestLayer:         "baseline",
			AllowedTestLayers:          []string{"baseline", "journey", "benchmark"},
			BaselineEligible:           true,
			BenchmarkEligible:          true,
			Summary:                    "Generate and compare plan alternatives.",
		},
		{
			ID:                         "euclo:trace.analyze",
			DisplayName:                "Trace Analyze",
			PrimaryOwner:               "trace",
			ModeFamilies:               []string{"debug"},
			PrimaryCapable:             true,
			Mutability:                 string(euclorelurpic.MutabilityInspectFirst),
			ParadigmMix:                []string{"blackboard", "reflection"},
			TransitionCompatible:       []string{"debug", "chat"},
			ExpectedArtifactKinds:      []string{string(euclotypes.ArtifactKindTrace), string(euclotypes.ArtifactKindAnalyze)},
			SupportedTransitionTargets: []string{"debug", "chat"},
			ExecutionClass:             "baseline_safe",
			PreferredTestLayer:         "baseline",
			AllowedTestLayers:          []string{"baseline", "journey", "benchmark"},
			BaselineEligible:           true,
			BenchmarkEligible:          true,
			Summary:                    "Collect and analyze execution traces.",
		},
		{
			ID:                         "euclo:review.findings",
			DisplayName:                "Review Findings",
			PrimaryOwner:               "review",
			ModeFamilies:               []string{"review"},
			PrimaryCapable:             true,
			Mutability:                 string(euclorelurpic.MutabilityInspectFirst),
			ParadigmMix:                []string{"reflection"},
			TransitionCompatible:       []string{"review", "planning"},
			ExpectedArtifactKinds:      []string{string(euclotypes.ArtifactKindReviewFindings)},
			SupportedTransitionTargets: []string{"review", "planning"},
			ExecutionClass:             "baseline_safe",
			PreferredTestLayer:         "baseline",
			AllowedTestLayers:          []string{"baseline", "journey", "benchmark"},
			BaselineEligible:           true,
			BenchmarkEligible:          true,
			Summary:                    "Surface review findings from the active workspace.",
		},
		{
			ID:                         "euclo:review.semantic",
			DisplayName:                "Semantic Review",
			PrimaryOwner:               "review",
			ModeFamilies:               []string{"review"},
			PrimaryCapable:             true,
			Mutability:                 string(euclorelurpic.MutabilityInspectFirst),
			ParadigmMix:                []string{"reflection"},
			TransitionCompatible:       []string{"review", "code"},
			ExpectedArtifactKinds:      []string{string(euclotypes.ArtifactKindReviewFindings), string(euclotypes.ArtifactKindCompatibilityAssessment)},
			SupportedTransitionTargets: []string{"review", "code"},
			ExecutionClass:             "baseline_safe",
			PreferredTestLayer:         "baseline",
			AllowedTestLayers:          []string{"baseline", "journey", "benchmark"},
			BaselineEligible:           true,
			BenchmarkEligible:          true,
			Summary:                    "Semantic review with compatibility assessment.",
		},
		{
			ID:                         "euclo:review.compatibility",
			DisplayName:                "Review Compatibility",
			PrimaryOwner:               "review",
			ModeFamilies:               []string{"review"},
			PrimaryCapable:             true,
			Mutability:                 string(euclorelurpic.MutabilityInspectFirst),
			ParadigmMix:                []string{"reflection"},
			TransitionCompatible:       []string{"review"},
			ExpectedArtifactKinds:      []string{string(euclotypes.ArtifactKindCompatibilityAssessment)},
			SupportedTransitionTargets: []string{"review"},
			ExecutionClass:             "baseline_safe",
			PreferredTestLayer:         "baseline",
			AllowedTestLayers:          []string{"baseline", "journey", "benchmark"},
			BaselineEligible:           true,
			BenchmarkEligible:          true,
			Summary:                    "Compatibility-only semantic review.",
		},
		{
			ID:                         "euclo:review.implement_if_safe",
			DisplayName:                "Review Implement If Safe",
			PrimaryOwner:               "review",
			ModeFamilies:               []string{"review"},
			PrimaryCapable:             true,
			Mutability:                 string(euclorelurpic.MutabilityPolicyConstrained),
			ParadigmMix:                []string{"react"},
			TransitionCompatible:       []string{"review", "code"},
			ExpectedArtifactKinds:      []string{string(euclotypes.ArtifactKindReviewFindings), string(euclotypes.ArtifactKindEditIntent), string(euclotypes.ArtifactKindVerification)},
			SupportedTransitionTargets: []string{"review", "code"},
			ExecutionClass:             "baseline_safe",
			PreferredTestLayer:         "baseline",
			AllowedTestLayers:          []string{"baseline", "journey", "benchmark"},
			BaselineEligible:           true,
			BenchmarkEligible:          true,
			Summary:                    "Review findings with bounded automatic implementation.",
		},
		{
			ID:                         "euclo:verification.scope_select",
			DisplayName:                "Verification Scope Select",
			PrimaryOwner:               "verification",
			ModeFamilies:               []string{"code"},
			PrimaryCapable:             true,
			Mutability:                 string(euclorelurpic.MutabilityNonMutating),
			ParadigmMix:                []string{"planner"},
			TransitionCompatible:       []string{"code", "debug", "tdd"},
			ExpectedArtifactKinds:      []string{string(euclotypes.ArtifactKindVerificationPlan)},
			SupportedTransitionTargets: []string{"code", "debug", "tdd"},
			ExecutionClass:             "baseline_safe",
			PreferredTestLayer:         "baseline",
			AllowedTestLayers:          []string{"baseline", "journey", "benchmark"},
			BaselineEligible:           true,
			BenchmarkEligible:          true,
			Summary:                    "Select a verification scope and plan.",
		},
		{
			ID:                         "euclo:verification.execute",
			DisplayName:                "Verification Execute",
			PrimaryOwner:               "verification",
			ModeFamilies:               []string{"code"},
			PrimaryCapable:             true,
			Mutability:                 string(euclorelurpic.MutabilityNonMutating),
			ParadigmMix:                []string{"react"},
			TransitionCompatible:       []string{"code", "debug", "tdd"},
			ExpectedArtifactKinds:      []string{string(euclotypes.ArtifactKindVerification)},
			SupportedTransitionTargets: []string{"code", "debug", "tdd"},
			ExecutionClass:             "baseline_safe",
			PreferredTestLayer:         "baseline",
			AllowedTestLayers:          []string{"baseline", "journey", "benchmark"},
			BaselineEligible:           true,
			BenchmarkEligible:          true,
			Summary:                    "Execute the current verification plan.",
		},
		{
			ID:                         "euclo:repair.failed_verification",
			DisplayName:                "Failed Verification Repair",
			PrimaryOwner:               "repair",
			ModeFamilies:               []string{"code"},
			PrimaryCapable:             true,
			Mutability:                 string(euclorelurpic.MutabilityPolicyConstrained),
			ParadigmMix:                []string{"react"},
			TransitionCompatible:       []string{"code", "debug", "tdd", "review"},
			ExpectedArtifactKinds:      []string{string(euclotypes.ArtifactKindEditIntent), string(euclotypes.ArtifactKindVerification), string(euclotypes.ArtifactKindRecoveryTrace)},
			SupportedTransitionTargets: []string{"code", "debug", "tdd", "review"},
			ExecutionClass:             "journey_only",
			PreferredTestLayer:         "journey",
			AllowedTestLayers:          []string{"journey", "benchmark"},
			BenchmarkEligible:          true,
			Summary:                    "Bounded repair for a failed verification run.",
		},
		{
			ID:                         "euclo:execution_profile.select",
			DisplayName:                "Execution Profile Select",
			PrimaryOwner:               "execution_profile",
			ModeFamilies:               []string{"planning"},
			PrimaryCapable:             true,
			Mutability:                 string(euclorelurpic.MutabilityNonMutating),
			ParadigmMix:                []string{"reflection"},
			TransitionCompatible:       []string{"planning", "code", "debug", "review", "tdd"},
			ExpectedArtifactKinds:      []string{string(euclotypes.ArtifactKindProfileSelection)},
			SupportedTransitionTargets: []string{"planning", "code", "debug", "review", "tdd"},
			ExecutionClass:             "baseline_safe",
			PreferredTestLayer:         "baseline",
			AllowedTestLayers:          []string{"baseline", "journey", "benchmark"},
			BaselineEligible:           true,
			BenchmarkEligible:          true,
			Summary:                    "Select the execution profile for the current task.",
		},
		{
			ID:                         "euclo:migration.execute",
			DisplayName:                "Migration Execute",
			PrimaryOwner:               "migration",
			ModeFamilies:               []string{"planning"},
			PrimaryCapable:             true,
			Mutability:                 string(euclorelurpic.MutabilityPlanBoundExecution),
			ParadigmMix:                []string{"planner"},
			TransitionCompatible:       []string{"planning", "code"},
			ExpectedArtifactKinds:      []string{string(euclotypes.ArtifactKindMigrationPlan), string(euclotypes.ArtifactKindEditIntent), string(euclotypes.ArtifactKindVerification)},
			SupportedTransitionTargets: []string{"planning", "code"},
			ExecutionClass:             "journey_only",
			PreferredTestLayer:         "journey",
			AllowedTestLayers:          []string{"journey", "benchmark"},
			BenchmarkEligible:          true,
			Summary:                    "Execute a bounded migration plan.",
		},
		{
			ID:                         "euclo:artifact.diff_summary",
			DisplayName:                "Artifact Diff Summary",
			PrimaryOwner:               "artifact",
			ModeFamilies:               []string{"chat"},
			PrimaryCapable:             true,
			Mutability:                 string(euclorelurpic.MutabilityInspectFirst),
			ParadigmMix:                []string{"reflection"},
			TransitionCompatible:       []string{"chat", "code", "review"},
			ExpectedArtifactKinds:      []string{string(euclotypes.ArtifactKindDiffSummary)},
			SupportedTransitionTargets: []string{"chat", "code", "review"},
			ExecutionClass:             "baseline_safe",
			PreferredTestLayer:         "baseline",
			AllowedTestLayers:          []string{"baseline", "journey", "benchmark"},
			BaselineEligible:           true,
			BenchmarkEligible:          true,
			Summary:                    "Summarize artifact diffs.",
		},
		{
			ID:                         "euclo:artifact.trace_to_root_cause",
			DisplayName:                "Artifact Trace To Root Cause",
			PrimaryOwner:               "artifact",
			ModeFamilies:               []string{"debug"},
			PrimaryCapable:             true,
			Mutability:                 string(euclorelurpic.MutabilityInspectFirst),
			ParadigmMix:                []string{"reflection"},
			TransitionCompatible:       []string{"debug", "review"},
			ExpectedArtifactKinds:      []string{string(euclotypes.ArtifactKindRootCauseCandidates)},
			SupportedTransitionTargets: []string{"debug", "review"},
			ExecutionClass:             "baseline_safe",
			PreferredTestLayer:         "baseline",
			AllowedTestLayers:          []string{"baseline", "journey", "benchmark"},
			BaselineEligible:           true,
			BenchmarkEligible:          true,
			Summary:                    "Translate trace artifacts into root-cause candidates.",
		},
		{
			ID:                         "euclo:artifact.verification_summary",
			DisplayName:                "Artifact Verification Summary",
			PrimaryOwner:               "artifact",
			ModeFamilies:               []string{"code"},
			PrimaryCapable:             true,
			Mutability:                 string(euclorelurpic.MutabilityInspectFirst),
			ParadigmMix:                []string{"reflection"},
			TransitionCompatible:       []string{"code", "debug", "tdd"},
			ExpectedArtifactKinds:      []string{string(euclotypes.ArtifactKindVerificationSummary)},
			SupportedTransitionTargets: []string{"code", "debug", "tdd"},
			ExecutionClass:             "baseline_safe",
			PreferredTestLayer:         "baseline",
			AllowedTestLayers:          []string{"baseline", "journey", "benchmark"},
			BaselineEligible:           true,
			BenchmarkEligible:          true,
			Summary:                    "Summarize verification output for downstream steps.",
		},
		{
			ID:                         "euclo:tdd.red_green_refactor",
			DisplayName:                "TDD Red Green Refactor",
			PrimaryOwner:               "tdd",
			ModeFamilies:               []string{"tdd"},
			PrimaryCapable:             true,
			Mutability:                 string(euclorelurpic.MutabilityPlanBoundExecution),
			ParadigmMix:                []string{"planner", "react"},
			TransitionCompatible:       []string{"tdd", "code"},
			ExpectedArtifactKinds:      []string{string(euclotypes.ArtifactKindTDDLifecycle), string(euclotypes.ArtifactKindEditIntent), string(euclotypes.ArtifactKindVerification)},
			SupportedTransitionTargets: []string{"tdd", "code"},
			ExecutionClass:             "journey_only",
			PreferredTestLayer:         "journey",
			AllowedTestLayers:          []string{"journey", "benchmark"},
			BenchmarkEligible:          true,
			Summary:                    "Drive the red-green-refactor loop.",
		},
		{
			ID:                         "euclo:test.regression_synthesize",
			DisplayName:                "Regression Synthesize",
			PrimaryOwner:               "test",
			ModeFamilies:               []string{"debug"},
			PrimaryCapable:             true,
			Mutability:                 string(euclorelurpic.MutabilityInspectFirst),
			ParadigmMix:                []string{"reflection"},
			TransitionCompatible:       []string{"debug", "review"},
			ExpectedArtifactKinds:      []string{string(euclotypes.ArtifactKindReproduction), string(euclotypes.ArtifactKindRegressionAnalysis)},
			SupportedTransitionTargets: []string{"debug", "review"},
			ExecutionClass:             "baseline_safe",
			PreferredTestLayer:         "baseline",
			AllowedTestLayers:          []string{"baseline", "journey", "benchmark"},
			BaselineEligible:           true,
			BenchmarkEligible:          true,
			Summary:                    "Synthesize a regression reproduction and analysis.",
		},
	}
}
