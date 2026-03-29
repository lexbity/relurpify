package runtime

import (
	"fmt"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
)

func BuildSecurityRuntimeState(cfg *core.Config, registry *capability.Registry, providers []core.Provider, state *core.Context, work UnitOfWork) SecurityRuntimeState {
	spec := (*core.AgentRuntimeSpec)(nil)
	if cfg != nil {
		spec = cfg.AgentSpec
	}
	allowed := core.EffectiveAllowedCapabilitySelectors(spec)
	security := SecurityRuntimeState{
		ModeID:                     work.ModeID,
		ExecutorFamily:             work.ExecutorDescriptor.Family,
		AllowedSelectorsConfigured: len(allowed) > 0,
		UpdatedAt:                  time.Now().UTC(),
	}
	var executionCatalog *capability.ExecutionCapabilityCatalogSnapshot
	if registry != nil {
		executionCatalog = registry.CaptureExecutionCatalogSnapshot()
		if executionCatalog != nil {
			security.ExecutionCatalogSnapshotID = executionCatalog.ID
			if snapshot := executionCatalog.PolicySnapshot(); snapshot != nil {
				security.PolicySnapshotID = snapshot.ID
			}
			security.AdmittedCallableCaps = capabilityDescriptorIDs(executionCatalog.CallableCapabilities())
			security.AdmittedInspectableCaps = capabilityDescriptorIDs(executionCatalog.InspectableCapabilities())
			security.AdmittedModelTools = modelToolNames(executionCatalog.ModelCallableTools())
		}
	}
	useCatalogAdmission := executionCatalog != nil && len(executionCatalog.AllowedCapabilities()) > 0
	if useCatalogAdmission {
		for _, binding := range work.CapabilityBindings {
			entry, ok := executionCatalog.GetCapability(binding.CapabilityID)
			if ok && entry.Callable {
				continue
			}
			security.DeniedCapabilityUsage = append(security.DeniedCapabilityUsage, binding.CapabilityID)
			summary := fmt.Sprintf("capability %s is not callable in the framework-admitted execution catalog", binding.CapabilityID)
			refs := []string{}
			if ok && entry.Inspectable {
				summary = fmt.Sprintf("capability %s is inspectable but not callable in the framework-admitted execution catalog", binding.CapabilityID)
				refs = append(refs, string(entry.Exposure))
			}
			if security.PolicySnapshotID != "" {
				refs = append(refs, "policy:"+security.PolicySnapshotID)
			}
			security.Diagnostics = append(security.Diagnostics, SecurityDiagnostic{
				Kind:     "framework_catalog_mismatch",
				Subject:  binding.CapabilityID,
				Severity: securitySeverity(binding.Required),
				Summary:  summary,
				Refs:     refs,
			})
		}
	} else if registry != nil && len(allowed) > 0 {
		for _, binding := range work.CapabilityBindings {
			desc, ok := registry.GetCapability(binding.CapabilityID)
			if !ok {
				continue
			}
			if !selectorMatchesAny(allowed, desc) {
				security.DeniedCapabilityUsage = append(security.DeniedCapabilityUsage, binding.CapabilityID)
				security.Diagnostics = append(security.Diagnostics, SecurityDiagnostic{
					Kind:     "capability_policy_mismatch",
					Subject:  binding.CapabilityID,
					Severity: securitySeverity(binding.Required),
					Summary:  fmt.Sprintf("capability %s is outside the manifest/runtime allowlist; framework policy should decide enforcement", binding.CapabilityID),
					Refs:     []string{binding.Family},
				})
			}
		}
	}
	for _, binding := range work.ToolBindings {
		if binding.Allowed || !security.AllowedSelectorsConfigured || !toolBindingMaterialToWork(binding.ToolID, work) {
			continue
		}
		security.DeniedToolUsage = append(security.DeniedToolUsage, binding.ToolID)
		refs := []string{}
		if security.PolicySnapshotID != "" {
			refs = append(refs, "policy:"+security.PolicySnapshotID)
		}
		refs = append(refs, security.AdmittedModelTools...)
		security.Diagnostics = append(security.Diagnostics, SecurityDiagnostic{
			Kind:     "tool_policy_mismatch",
			Subject:  binding.ToolID,
			Severity: "warning",
			Summary:  fmt.Sprintf("tool access %s appears incompatible with current runtime policy; framework should decide enforcement", binding.ToolID),
			Refs:     refs,
		})
	}
	restore, _ := providerRestoreStateFromContext(state)
	for _, provider := range providers {
		desc := provider.Descriptor()
		if desc.TrustBaseline == core.TrustClassProviderLocalUntrusted || desc.TrustBaseline == core.TrustClassRemoteDeclared {
			security.Diagnostics = append(security.Diagnostics, SecurityDiagnostic{
				Kind:     "downgraded_provider_trust",
				Subject:  desc.ID,
				Severity: "warning",
				Summary:  fmt.Sprintf("provider %s has trust baseline %s", desc.ID, desc.TrustBaseline),
				Refs:     []string{string(desc.TrustBaseline)},
			})
		}
		if restore.MateriallyRequired && desc.RecoverabilityMode == core.RecoverabilityPersistedRestore {
			for _, outcome := range restore.Outcomes {
				if strings.TrimSpace(outcome.ProviderID) != desc.ID {
					continue
				}
				if outcome.Reason == "provider_restore_unsupported" || outcome.Reason == "provider_restore_failed" {
					security.Diagnostics = append(security.Diagnostics, SecurityDiagnostic{
						Kind:     "provider_recoverability_mismatch",
						Subject:  desc.ID,
						Severity: "error",
						Summary:  fmt.Sprintf("provider %s declared persisted restore but runtime restore ended with %s", desc.ID, outcome.Reason),
						Refs:     []string{string(desc.RecoverabilityMode)},
					})
				}
			}
		}
	}
	security.DeniedCapabilityUsage = uniqueStrings(security.DeniedCapabilityUsage)
	security.DeniedToolUsage = uniqueStrings(security.DeniedToolUsage)
	security.AdmittedCallableCaps = uniqueStrings(security.AdmittedCallableCaps)
	security.AdmittedInspectableCaps = uniqueStrings(security.AdmittedInspectableCaps)
	security.AdmittedModelTools = uniqueStrings(security.AdmittedModelTools)
	security.Diagnostics = dedupeSecurityDiagnostics(security.Diagnostics)
	return security
}

func BuildSharedContextRuntimeState(shared *core.SharedContext, work UnitOfWork) SharedContextRuntimeState {
	rt := SharedContextRuntimeState{
		Enabled:        shared != nil,
		ExecutorFamily: work.ExecutorDescriptor.Family,
		BehaviorFamily: work.BehaviorFamily,
		UpdatedAt:      time.Now().UTC(),
	}
	participants := []string{
		"executor:" + string(work.ExecutorDescriptor.Family),
		"behavior:" + strings.TrimSpace(work.BehaviorFamily),
	}
	for _, routine := range work.RoutineBindings {
		if id := strings.TrimSpace(routine.Family); id != "" {
			participants = append(participants, "routine:"+id)
		}
	}
	for _, skill := range work.SkillBindings {
		if id := strings.TrimSpace(skill.SkillID); id != "" {
			participants = append(participants, "skill:"+id)
		}
	}
	rt.Participants = uniqueStrings(participants)
	if shared == nil {
		return rt
	}
	for _, ref := range shared.WorkingSetReferences() {
		key := strings.TrimSpace(ref.ID)
		if key == "" {
			key = strings.TrimSpace(ref.URI)
		}
		if key != "" {
			rt.WorkingSetRefs = append(rt.WorkingSetRefs, key)
		}
	}
	mutations := shared.RecentMutations(12)
	rt.RecentMutationCount = len(mutations)
	for _, mutation := range mutations {
		if key := strings.TrimSpace(mutation.Key); key != "" {
			rt.RecentMutationKeys = append(rt.RecentMutationKeys, key)
		}
	}
	rt.WorkingSetRefs = uniqueStrings(rt.WorkingSetRefs)
	rt.RecentMutationKeys = uniqueStrings(rt.RecentMutationKeys)
	return rt
}

func selectorMatchesAny(selectors []core.CapabilitySelector, desc core.CapabilityDescriptor) bool {
	for _, selector := range selectors {
		if core.SelectorMatchesDescriptor(selector, desc) {
			return true
		}
	}
	return false
}

func toolBindingMaterialToWork(toolID string, work UnitOfWork) bool {
	switch strings.TrimSpace(toolID) {
	case "verification":
		return work.VerificationPolicyID != "" || work.ResolvedPolicy.RequireVerificationStep
	case "workspace_write":
		profile := strings.ToLower(strings.TrimSpace(work.ResolvedPolicy.ProfileID))
		return strings.Contains(profile, "edit") ||
			strings.Contains(profile, "patch") ||
			strings.Contains(profile, "repair") ||
			strings.Contains(profile, "implement")
	default:
		return false
	}
}

func securitySeverity(required bool) string {
	if required {
		return "error"
	}
	return "warning"
}

func dedupeSecurityDiagnostics(input []SecurityDiagnostic) []SecurityDiagnostic {
	if len(input) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]SecurityDiagnostic, 0, len(input))
	for _, item := range input {
		key := strings.Join([]string{
			strings.TrimSpace(item.Kind),
			strings.TrimSpace(item.Subject),
			strings.TrimSpace(item.Summary),
		}, "|")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		item.Refs = uniqueStrings(item.Refs)
		out = append(out, item)
	}
	return out
}

func capabilityDescriptorIDs(descs []core.CapabilityDescriptor) []string {
	if len(descs) == 0 {
		return nil
	}
	out := make([]string, 0, len(descs))
	for _, desc := range descs {
		id := strings.TrimSpace(desc.ID)
		if id == "" {
			id = strings.TrimSpace(desc.Name)
		}
		if id != "" {
			out = append(out, id)
		}
	}
	return out
}

func modelToolNames(tools []capability.Tool) []string {
	if len(tools) == 0 {
		return nil
	}
	out := make([]string, 0, len(tools))
	for _, tool := range tools {
		if tool == nil {
			continue
		}
		if name := strings.TrimSpace(tool.Name()); name != "" {
			out = append(out, name)
		}
	}
	return out
}
