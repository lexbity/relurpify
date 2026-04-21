package policy

import (
	"fmt"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
	runtimepkg "codeburg.org/lexbit/relurpify/named/euclo/runtime"
	euclorestore "codeburg.org/lexbit/relurpify/named/euclo/runtime/restore"
)

func BuildSecurityRuntimeState(cfg *core.Config, registry *capability.Registry, providers []core.Provider, state *core.Context, work runtimepkg.UnitOfWork) runtimepkg.SecurityRuntimeState {
	spec := (*core.AgentRuntimeSpec)(nil)
	if cfg != nil {
		spec = cfg.AgentSpec
	}
	allowed := core.EffectiveAllowedCapabilitySelectors(spec)
	security := runtimepkg.SecurityRuntimeState{
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
			security.Diagnostics = append(security.Diagnostics, runtimepkg.SecurityDiagnostic{
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
				security.Diagnostics = append(security.Diagnostics, runtimepkg.SecurityDiagnostic{
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
		security.Diagnostics = append(security.Diagnostics, runtimepkg.SecurityDiagnostic{
			Kind:     "tool_policy_mismatch",
			Subject:  binding.ToolID,
			Severity: "warning",
			Summary:  fmt.Sprintf("tool access %s appears incompatible with current runtime policy; framework should decide enforcement", binding.ToolID),
			Refs:     refs,
		})
	}
	restore, _ := euclorestore.ProviderRestoreStateFromContext(state)
	for _, provider := range providers {
		desc := provider.Descriptor()
		if desc.TrustBaseline == core.TrustClassProviderLocalUntrusted || desc.TrustBaseline == core.TrustClassRemoteDeclared {
			security.Diagnostics = append(security.Diagnostics, runtimepkg.SecurityDiagnostic{
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
					security.Diagnostics = append(security.Diagnostics, runtimepkg.SecurityDiagnostic{
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

func selectorMatchesAny(selectors []core.CapabilitySelector, desc core.CapabilityDescriptor) bool {
	for _, selector := range selectors {
		if core.SelectorMatchesDescriptor(selector, desc) {
			return true
		}
	}
	return false
}

func toolBindingMaterialToWork(toolID string, work runtimepkg.UnitOfWork) bool {
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

func dedupeSecurityDiagnostics(input []runtimepkg.SecurityDiagnostic) []runtimepkg.SecurityDiagnostic {
	if len(input) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]runtimepkg.SecurityDiagnostic, 0, len(input))
	for _, item := range input {
		key := strings.Join([]string{item.Kind, item.Subject, item.Severity, item.Summary, strings.Join(item.Refs, ",")}, "|")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	return out
}

func uniqueStrings(input []string) []string {
	if len(input) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(input))
	for _, item := range input {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func capabilityDescriptorIDs(entries []capability.CapabilityDescriptor) []string {
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		if id := strings.TrimSpace(entry.ID); id != "" {
			out = append(out, id)
		}
	}
	return uniqueStrings(out)
}

func modelToolNames(entries []capability.Tool) []string {
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		if name := strings.TrimSpace(entry.Name()); name != "" {
			out = append(out, name)
		}
	}
	return uniqueStrings(out)
}
