package agents

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/manifest"
	frameworktools "github.com/lexcodex/relurpify/framework/tools"
)

const skillManifestName = "skill.manifest.yaml"

// SkillPaths resolves standard paths for a skill package.
type SkillPaths struct {
	Root      string
	Scripts   []string
	Resources []string
	Templates []string
}

// SkillResolution captures skill loading outcomes.
type SkillResolution struct {
	Name    string
	Applied bool
	Error   string
	Paths   SkillPaths
}

// SkillRoot returns the skill directory for a name.
func SkillRoot(workspace, name string) string {
	return filepath.Join(ConfigDir(workspace), "skills", name)
}

// SkillManifestPath returns the default skill manifest path.
func SkillManifestPath(workspace, name string) string {
	return filepath.Join(SkillRoot(workspace, name), skillManifestName)
}

// ApplySkills merges skill contributions (flat, no inheritance) into baseSpec
// and returns the updated spec plus per-skill resolution results.
func ApplySkills(workspace string, baseSpec *core.AgentRuntimeSpec, skillNames []string,
	registry *capability.Registry, permissions *frameworktools.PermissionManager, agentID string,
) (*core.AgentRuntimeSpec, []SkillResolution) {
	spec := core.MergeAgentSpecs(baseSpec)
	results := make([]SkillResolution, 0, len(skillNames))
	allowedCapabilities := core.EffectiveAllowedCapabilitySelectors(spec)
	toolPolicies := cloneToolPolicies(spec.ToolExecutionPolicy)
	capabilityPolicies := append([]core.CapabilityPolicy{}, spec.CapabilityPolicies...)
	insertionPolicies := append([]core.CapabilityInsertionPolicy{}, spec.InsertionPolicies...)
	globalPolicies := cloneGlobalPolicies(spec.GlobalPolicies)
	providerPolicies := cloneProviderPolicies(spec.ProviderPolicies)
	providers := cloneProviderConfigs(spec.Providers)
	skillConfig := core.AgentSkillConfig{}

	for _, name := range skillNames {
		skillName := strings.TrimSpace(name)
		if skillName == "" {
			continue
		}
		skillManifest, err := manifest.LoadSkill(workspace, skillName)
		if err != nil {
			results = append(results, logSkillError(workspace, skillName, "load_failed", err,
				SkillPaths{Root: SkillRoot(workspace, skillName)}))
			continue
		}

		// Check binary prerequisites.
		if missingBin := findMissingBin(skillManifest.Spec.Requires.Bins); missingBin != "" {
			binErr := fmt.Errorf("required binary %q not found in PATH", missingBin)
			paths := resolveSkillPaths(skillManifest)
			results = append(results, logSkillError(workspace, skillName, "missing_binary", binErr, paths))
			continue
		}

		// Check resource paths.
		paths := resolveSkillPaths(skillManifest)
		if err := validateSkillPaths(paths); err != nil {
			results = append(results, logSkillError(workspace, skillName, "missing_resources", err, paths))
			continue
		}
		if err := registerSkillCapabilities(registry, skillManifest, paths); err != nil {
			results = append(results, logSkillError(workspace, skillName, "capability_registration_failed", err, paths))
			continue
		}

		// Merge contributions.
		allowedCapabilities = mergeCapabilitySelectors(allowedCapabilities, skillAllowedCapabilities(skillManifest.Spec))
		for toolName, policy := range skillManifest.Spec.ToolExecutionPolicy {
			if toolPolicies == nil {
				toolPolicies = make(map[string]core.ToolPolicy)
			}
			toolPolicies[toolName] = policy
		}
		if len(skillManifest.Spec.CapabilityPolicies) > 0 {
			capabilityPolicies = append(capabilityPolicies, cloneCapabilityPolicies(skillManifest.Spec.CapabilityPolicies)...)
		}
		if len(skillManifest.Spec.InsertionPolicies) > 0 {
			insertionPolicies = append(insertionPolicies, cloneInsertionPolicies(skillManifest.Spec.InsertionPolicies)...)
		}
		if len(skillManifest.Spec.GlobalPolicies) > 0 {
			if globalPolicies == nil {
				globalPolicies = make(map[string]core.AgentPermissionLevel)
			}
			for key, value := range skillManifest.Spec.GlobalPolicies {
				globalPolicies[key] = value
			}
		}
		if len(skillManifest.Spec.ProviderPolicies) > 0 {
			if providerPolicies == nil {
				providerPolicies = make(map[string]core.ProviderPolicy)
			}
			for key, value := range skillManifest.Spec.ProviderPolicies {
				providerPolicies[key] = value
			}
		}
		if len(skillManifest.Spec.Providers) > 0 {
			providers = mergeProviderConfigs(providers, skillManifest.Spec.Providers)
		}
		if len(skillManifest.Spec.PromptSnippets) > 0 {
			spec.Prompt = mergePromptSnippets(spec.Prompt, skillManifest.Spec.PromptSnippets)
		}
		skillConfig = mergeSkillConfig(skillConfig, skillManifest.Spec)

		results = append(results, SkillResolution{
			Name:    skillManifest.Metadata.Name,
			Applied: true,
			Paths:   paths,
		})
	}

	spec.AllowedCapabilities = allowedCapabilities
	spec.ToolExecutionPolicy = toolPolicies
	spec.CapabilityPolicies = capabilityPolicies
	spec.InsertionPolicies = insertionPolicies
	spec.GlobalPolicies = globalPolicies
	spec.ProviderPolicies = providerPolicies
	spec.Providers = providers
	spec.SkillConfig = core.MergeAgentSpecs(&core.AgentRuntimeSpec{SkillConfig: spec.SkillConfig}, core.AgentSpecOverlay{SkillConfig: &skillConfig}).SkillConfig
	return spec, results
}

// DeriveGVisorAllowlist returns the binary allowlist for the gVisor sandbox
// by walking the effective (allowed) tool set and collecting each tool's
// declared executable permissions.
func DeriveGVisorAllowlist(allowed []core.CapabilitySelector, registry *capability.Registry) []core.ExecutablePermission {
	if registry == nil {
		return nil
	}
	seen := make(map[string]bool)
	var result []core.ExecutablePermission
	for _, tool := range registry.CallableTools() {
		desc := core.ToolDescriptor(context.Background(), nil, tool)
		if len(allowed) > 0 && !matchesAnyCapabilitySelector(allowed, desc) {
			continue
		}
		perms := tool.Permissions()
		for _, ep := range perms.Permissions.Executables {
			if seen[ep.Binary] {
				continue
			}
			seen[ep.Binary] = true
			result = append(result, ep)
		}
	}
	return result
}

func skillAllowedCapabilities(skillSpec manifest.SkillSpec) []core.CapabilitySelector {
	return append([]core.CapabilitySelector{}, skillSpec.AllowedCapabilities...)
}

func mergeCapabilitySelectors(base, extra []core.CapabilitySelector) []core.CapabilitySelector {
	if len(extra) == 0 {
		return append([]core.CapabilitySelector{}, base...)
	}
	seen := make(map[string]struct{}, len(base)+len(extra))
	out := make([]core.CapabilitySelector, 0, len(base)+len(extra))
	for _, selector := range append(append([]core.CapabilitySelector{}, base...), extra...) {
		key := selector.ID + "|" + selector.Name + "|" + string(selector.Kind) + "|" +
			strings.Join(capabilityRuntimeFamiliesToStrings(selector.RuntimeFamilies), ",") + "|" +
			strings.Join(selector.Tags, ",") + "|" + strings.Join(selector.ExcludeTags, ",") + "|" +
			strings.Join(capabilityScopesToStrings(selector.SourceScopes), ",") + "|" +
			strings.Join(trustClassesToStrings(selector.TrustClasses), ",") + "|" +
			strings.Join(riskClassesToStrings(selector.RiskClasses), ",") + "|" +
			strings.Join(effectClassesToStrings(selector.EffectClasses), ",")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, selector)
	}
	return out
}

func matchesAnyCapabilitySelector(selectors []core.CapabilitySelector, desc core.CapabilityDescriptor) bool {
	if len(selectors) == 0 {
		return true
	}
	for _, selector := range selectors {
		if core.SelectorMatchesDescriptor(selector, desc) {
			return true
		}
	}
	return false
}

func capabilityScopesToStrings(values []core.CapabilityScope) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}

func capabilityRuntimeFamiliesToStrings(values []core.CapabilityRuntimeFamily) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}

func trustClassesToStrings(values []core.TrustClass) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}

func riskClassesToStrings(values []core.RiskClass) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}

func effectClassesToStrings(values []core.EffectClass) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}

// ResolveSkillPaths exposes the resolved resource paths for a skill.
func ResolveSkillPaths(skill *manifest.SkillManifest) SkillPaths {
	return resolveSkillPaths(skill)
}

// ValidateSkillPaths ensures resource entries exist on disk.
func ValidateSkillPaths(paths SkillPaths) error {
	return validateSkillPaths(paths)
}

func resolveSkillPaths(skill *manifest.SkillManifest) SkillPaths {
	root := ""
	if skill != nil && skill.SourcePath != "" {
		root = filepath.Dir(skill.SourcePath)
	}
	paths := SkillPaths{Root: root}
	if skill == nil {
		return paths
	}
	paths.Scripts = resolveSkillList(root, skill.Spec.ResourcePaths.Scripts, filepath.Join(root, "scripts"))
	paths.Resources = resolveSkillList(root, skill.Spec.ResourcePaths.Resources, filepath.Join(root, "resources"))
	paths.Templates = resolveSkillList(root, skill.Spec.ResourcePaths.Templates, filepath.Join(root, "templates"))
	return paths
}

func resolveSkillList(root string, entries []string, fallback string) []string {
	if len(entries) == 0 {
		if fallback == "" {
			return nil
		}
		return []string{fallback}
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if filepath.IsAbs(entry) {
			paths = append(paths, entry)
			continue
		}
		paths = append(paths, filepath.Join(root, entry))
	}
	return paths
}

func validateSkillPaths(paths SkillPaths) error {
	var missing []string
	check := func(label string, entries []string) {
		for _, entry := range entries {
			if entry == "" {
				continue
			}
			if _, err := os.Stat(entry); err != nil {
				missing = append(missing, fmt.Sprintf("%s:%s", label, entry))
			}
		}
	}
	check("scripts", paths.Scripts)
	check("resources", paths.Resources)
	check("templates", paths.Templates)
	if len(missing) > 0 {
		return fmt.Errorf("missing skill resources: %s", strings.Join(missing, ", "))
	}
	return nil
}

func findMissingBin(bins []string) string {
	for _, bin := range bins {
		bin = strings.TrimSpace(bin)
		if bin == "" {
			continue
		}
		if _, err := exec.LookPath(bin); err != nil {
			return bin
		}
	}
	return ""
}

func mergePromptSnippets(base string, snippets []string) string {
	builder := strings.Builder{}
	base = strings.TrimSpace(base)
	if base != "" {
		builder.WriteString(base)
	}
	for _, snippet := range snippets {
		snippet = strings.TrimSpace(snippet)
		if snippet == "" {
			continue
		}
		if builder.Len() > 0 {
			builder.WriteString("\n\n")
		}
		builder.WriteString(snippet)
	}
	return builder.String()
}

func mergeSkillConfig(base core.AgentSkillConfig, skillSpec manifest.SkillSpec) core.AgentSkillConfig {
	overlay := core.AgentSkillConfig{
		Verification: core.AgentVerificationPolicy{
			SuccessTools:               append([]string{}, skillSpec.Verification.SuccessTools...),
			SuccessCapabilitySelectors: append([]core.SkillCapabilitySelector{}, skillSpec.Verification.SuccessCapabilitySelectors...),
			StopOnSuccess:              skillSpec.Verification.StopOnSuccess,
		},
		Recovery: core.AgentRecoveryPolicy{
			FailureProbeTools:               append([]string{}, skillSpec.Recovery.FailureProbeTools...),
			FailureProbeCapabilitySelectors: append([]core.SkillCapabilitySelector{}, skillSpec.Recovery.FailureProbeCapabilitySelectors...),
		},
		Planning: core.AgentPlanningPolicy{
			RequiredBeforeEdit:          append([]core.SkillCapabilitySelector{}, skillSpec.Planning.RequiredBeforeEdit...),
			PreferredEditCapabilities:   append([]core.SkillCapabilitySelector{}, skillSpec.Planning.PreferredEditCapabilities...),
			PreferredVerifyCapabilities: append([]core.SkillCapabilitySelector{}, skillSpec.Planning.PreferredVerifyCapabilities...),
			StepTemplates:               append([]core.SkillStepTemplate{}, skillSpec.Planning.StepTemplates...),
			RequireVerificationStep:     skillSpec.Planning.RequireVerificationStep,
		},
		Review: core.AgentReviewPolicy{
			Criteria:      append([]string{}, skillSpec.Review.Criteria...),
			FocusTags:     append([]string{}, skillSpec.Review.FocusTags...),
			ApprovalRules: skillSpec.Review.ApprovalRules,
		},
		ContextHints: core.AgentSkillContextHints{
			PreferredDetailLevel: skillSpec.ContextHints.PreferredDetailLevel,
			ProtectPatterns:      append([]string{}, skillSpec.ContextHints.ProtectPatterns...),
		},
	}
	if skillSpec.Review.SeverityWeights != nil {
		overlay.Review.SeverityWeights = make(map[string]float64, len(skillSpec.Review.SeverityWeights))
		for k, v := range skillSpec.Review.SeverityWeights {
			overlay.Review.SeverityWeights[k] = v
		}
	}
	if len(skillSpec.PhaseCapabilities) > 0 {
		overlay.PhaseCapabilities = make(map[string][]string, len(skillSpec.PhaseCapabilities))
		for phase, tools := range skillSpec.PhaseCapabilities {
			overlay.PhaseCapabilities[phase] = append([]string{}, tools...)
		}
	}
	if len(skillSpec.PhaseCapabilitySelectors) > 0 {
		overlay.PhaseCapabilitySelectors = make(map[string][]core.SkillCapabilitySelector, len(skillSpec.PhaseCapabilitySelectors))
		for phase, selectors := range skillSpec.PhaseCapabilitySelectors {
			overlay.PhaseCapabilitySelectors[phase] = append([]core.SkillCapabilitySelector{}, selectors...)
		}
	}
	return core.MergeAgentSpecs(&core.AgentRuntimeSpec{SkillConfig: base}, core.AgentSpecOverlay{SkillConfig: &overlay}).SkillConfig
}

func logSkillError(workspace, name, reason string, err error, paths SkillPaths) SkillResolution {
	entry := fmt.Sprintf("[WARNING] %s skill %s (%s): %s", time.Now().UTC().Format(time.RFC3339), name, reason, err.Error())
	logSkillMessage(workspace, entry)
	return SkillResolution{
		Name:    name,
		Applied: false,
		Error:   err.Error(),
		Paths:   paths,
	}
}

func cloneToolPolicies(input map[string]core.ToolPolicy) map[string]core.ToolPolicy {
	if input == nil {
		return nil
	}
	clone := make(map[string]core.ToolPolicy, len(input))
	for name, policy := range input {
		clone[name] = policy
	}
	return clone
}

func mergeStringList(base, extra []string) []string {
	if len(extra) == 0 {
		return base
	}
	seen := make(map[string]struct{}, len(base)+len(extra))
	out := make([]string, 0, len(base)+len(extra))
	for _, entry := range append(base, extra...) {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if _, ok := seen[entry]; ok {
			continue
		}
		seen[entry] = struct{}{}
		out = append(out, entry)
	}
	return out
}

func cloneGlobalPolicies(policies map[string]core.AgentPermissionLevel) map[string]core.AgentPermissionLevel {
	if policies == nil {
		return nil
	}
	out := make(map[string]core.AgentPermissionLevel, len(policies))
	for key, value := range policies {
		out[key] = value
	}
	return out
}

func cloneProviderPolicies(policies map[string]core.ProviderPolicy) map[string]core.ProviderPolicy {
	if policies == nil {
		return nil
	}
	out := make(map[string]core.ProviderPolicy, len(policies))
	for key, value := range policies {
		out[key] = value
	}
	return out
}

func cloneProviderConfigs(values []core.ProviderConfig) []core.ProviderConfig {
	if len(values) == 0 {
		return nil
	}
	out := make([]core.ProviderConfig, len(values))
	copy(out, values)
	for idx := range out {
		if len(values[idx].Config) == 0 {
			continue
		}
		out[idx].Config = make(map[string]any, len(values[idx].Config))
		for key, value := range values[idx].Config {
			out[idx].Config[key] = value
		}
	}
	return out
}

func mergeProviderConfigs(base, extra []core.ProviderConfig) []core.ProviderConfig {
	if len(extra) == 0 {
		return cloneProviderConfigs(base)
	}
	merged := cloneProviderConfigs(base)
	index := make(map[string]int, len(merged))
	for idx, provider := range merged {
		index[provider.ID] = idx
	}
	for _, provider := range extra {
		if idx, ok := index[provider.ID]; ok {
			merged[idx] = provider
			continue
		}
		index[provider.ID] = len(merged)
		merged = append(merged, provider)
	}
	return merged
}

func cloneCapabilityPolicies(policies []core.CapabilityPolicy) []core.CapabilityPolicy {
	if policies == nil {
		return nil
	}
	out := make([]core.CapabilityPolicy, len(policies))
	for i, policy := range policies {
		out[i] = policy
		out[i].Selector.SourceScopes = append([]core.CapabilityScope{}, policy.Selector.SourceScopes...)
		out[i].Selector.TrustClasses = append([]core.TrustClass{}, policy.Selector.TrustClasses...)
		out[i].Selector.RiskClasses = append([]core.RiskClass{}, policy.Selector.RiskClasses...)
		out[i].Selector.EffectClasses = append([]core.EffectClass{}, policy.Selector.EffectClasses...)
	}
	return out
}

func cloneInsertionPolicies(policies []core.CapabilityInsertionPolicy) []core.CapabilityInsertionPolicy {
	if policies == nil {
		return nil
	}
	out := make([]core.CapabilityInsertionPolicy, len(policies))
	for i, policy := range policies {
		out[i] = policy
		out[i].Selector.SourceScopes = append([]core.CapabilityScope{}, policy.Selector.SourceScopes...)
		out[i].Selector.TrustClasses = append([]core.TrustClass{}, policy.Selector.TrustClasses...)
		out[i].Selector.RiskClasses = append([]core.RiskClass{}, policy.Selector.RiskClasses...)
		out[i].Selector.EffectClasses = append([]core.EffectClass{}, policy.Selector.EffectClasses...)
	}
	return out
}

func logSkillMessage(workspace, message string) {
	if workspace == "" {
		return
	}
	logDir := filepath.Join(ConfigDir(workspace), "logs")
	if mkErr := os.MkdirAll(logDir, 0o755); mkErr != nil {
		return
	}
	file := filepath.Join(logDir, "skills.log")
	entry := message + "\n"
	if f, openErr := os.OpenFile(file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); openErr == nil {
		defer f.Close()
		_, _ = f.WriteString(entry)
	}
}
