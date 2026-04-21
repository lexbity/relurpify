package capabilities

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	"codeburg.org/lexbit/relurpify/named/euclo/execution"
	"codeburg.org/lexbit/relurpify/named/euclo/relurpicabilities"
	bkccaps "codeburg.org/lexbit/relurpify/named/euclo/relurpicabilities/bkc"
	debugcaps "codeburg.org/lexbit/relurpify/named/euclo/relurpicabilities/debug"
	localcaps "codeburg.org/lexbit/relurpify/named/euclo/relurpicabilities/local"
	"codeburg.org/lexbit/relurpify/named/euclo/thoughtrecipes"
)

type EucloCapabilityRegistry struct {
	mu           sync.RWMutex
	capabilities map[string]euclotypes.EucloCodingCapability
}

func NewEucloCapabilityRegistry() *EucloCapabilityRegistry {
	return &EucloCapabilityRegistry{capabilities: map[string]euclotypes.EucloCodingCapability{}}
}

func NewDefaultCapabilityRegistry(env agentenv.AgentEnvironment) *EucloCapabilityRegistry {
	reg := NewEucloCapabilityRegistry()
	_ = reg.Register(bkccaps.NewCompileCapability(env))
	_ = reg.Register(bkccaps.NewStreamCapability(env))
	_ = reg.Register(bkccaps.NewCheckpointCapability(env))
	_ = reg.Register(bkccaps.NewInvalidateCapability(env))
	_ = reg.Register(debugcaps.NewInvestigateRegressionCapability(env))
	_ = reg.Register(localcaps.NewDesignAlternativesCapability(env))
	_ = reg.Register(localcaps.NewExecutionProfileSelectCapability(env))
	_ = reg.Register(localcaps.NewTraceAnalyzeCapability(env))
	_ = reg.Register(localcaps.NewDiffSummaryCapability(env))
	_ = reg.Register(localcaps.NewTraceToRootCauseCapability(env))
	_ = reg.Register(localcaps.NewVerificationSummaryCapability(env))
	_ = reg.Register(localcaps.NewVerificationScopeSelectCapability(env))
	_ = reg.Register(localcaps.NewVerificationExecuteCapability(env))
	_ = reg.Register(localcaps.NewFailedVerificationRepairCapability(env))
	_ = reg.Register(localcaps.NewRegressionSynthesizeCapability(env))
	_ = reg.Register(localcaps.NewTDDRedGreenRefactorCapability(env))
	_ = reg.Register(localcaps.NewMigrationExecuteCapability(env))
	_ = reg.Register(localcaps.NewReviewFindingsCapability(env))
	_ = reg.Register(localcaps.NewReviewSemanticCapability(env))
	_ = reg.Register(localcaps.NewReviewCompatibilityCapability(env))
	_ = reg.Register(localcaps.NewReviewImplementIfSafeCapability(env))
	_ = reg.Register(localcaps.NewRefactorAPICompatibleCapability(env))
	_ = reg.Register(localcaps.NewDeferralsSurfaceCapability(env))
	_ = reg.Register(localcaps.NewDeferralsResolveCapability(env))
	_ = reg.Register(localcaps.NewLearningPromoteCapability(env))
	return reg
}

func (r *EucloCapabilityRegistry) Register(cap euclotypes.EucloCodingCapability) error {
	if r == nil || cap == nil {
		return nil
	}
	id := strings.TrimSpace(cap.Descriptor().ID)
	if id == "" {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.capabilities[id] = cap
	return nil
}

func (r *EucloCapabilityRegistry) Lookup(id string) (euclotypes.EucloCodingCapability, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	cap, ok := r.capabilities[strings.TrimSpace(id)]
	return cap, ok
}

func (r *EucloCapabilityRegistry) ForProfile(profileID string) []euclotypes.EucloCodingCapability {
	if r == nil {
		return nil
	}
	profileID = strings.TrimSpace(profileID)
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]euclotypes.EucloCodingCapability, 0, len(r.capabilities))
	for _, cap := range r.capabilities {
		if profileID == "" || supportsProfile(cap, profileID) {
			out = append(out, cap)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Descriptor().ID < out[j].Descriptor().ID
	})
	return out
}

func supportsProfile(cap euclotypes.EucloCodingCapability, profileID string) bool {
	if cap == nil || profileID == "" {
		return true
	}
	annotations := cap.Descriptor().Annotations
	if annotations == nil {
		return true
	}
	raw, ok := annotations["supported_profiles"]
	if !ok || raw == nil {
		return true
	}
	switch typed := raw.(type) {
	case []string:
		for _, item := range typed {
			if strings.TrimSpace(item) == profileID {
				return true
			}
		}
		return false
	case []any:
		for _, item := range typed {
			if text, ok := item.(string); ok && strings.TrimSpace(text) == profileID {
				return true
			}
		}
		return false
	default:
		return true
	}
}

// RecipeIntegrationResult holds the result of loading and registering thought recipes.
type RecipeIntegrationResult struct {
	Registry   *thoughtrecipes.PlanRegistry
	Executor   *thoughtrecipes.Executor
	Invocables []execution.Invocable
	Warnings   []string
	Errors     []error
}

// LoadAndRegisterRecipes loads thought recipes from the given directory,
// registers them as Descriptors in the relurpic registry, and returns
// the PlanRegistry and Executor for dispatcher integration.
// This implements the Phase 9 startup integration hook.
func LoadAndRegisterRecipes(recipeDir string, relurpicRegistry *relurpicabilities.Registry, env agentenv.AgentEnvironment) *RecipeIntegrationResult {
	result := &RecipeIntegrationResult{
		Registry:   thoughtrecipes.NewPlanRegistry(),
		Executor:   thoughtrecipes.NewExecutor(),
		Invocables: make([]execution.Invocable, 0),
		Warnings:   make([]string, 0),
		Errors:     make([]error, 0),
	}

	// Load all recipes from directory
	loadResult, err := thoughtrecipes.LoadAll(recipeDir)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("failed to load recipes: %w", err))
		return result
	}

	// Register each plan in the PlanRegistry and create a Descriptor
	for _, plan := range loadResult.Plans {
		if plan == nil {
			continue
		}

		// Register in PlanRegistry
		if err := result.Registry.Register(plan); err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("failed to register plan %q: %w", plan.Name, err))
			continue
		}

		// Build capability ID
		capabilityID := "euclo:recipe." + plan.Name

		// Collect keywords from plan metadata if available
		keywords := make([]string, 0)
		if plan.Description != "" {
			// Use description words as keywords (simplified approach)
			words := strings.Fields(strings.ToLower(plan.Description))
			for _, word := range words {
				if len(word) > 3 && !isCommonWord(word) {
					keywords = append(keywords, word)
				}
			}
		}

		// Create Descriptor with IsUserDefined: true
		desc := relurpicabilities.Descriptor{
			ID:                     capabilityID,
			DisplayName:            plan.Name,
			ModeFamilies:           plan.Modes,
			TriggerPriority:        plan.TriggerPriority,
			PrimaryCapable:         true,
			Mutability:             relurpicabilities.MutabilityPolicyConstrained,
			AllowDynamicResolution: true,
			IsUserDefined:          true,
			RecipePath:             plan.Name, // Could be enhanced to store actual path
			Keywords:               keywords,
			Summary:                plan.Description,
		}

		// Register in relurpic registry
		if relurpicRegistry != nil {
			if err := relurpicRegistry.Register(desc); err != nil {
				result.Errors = append(result.Errors, fmt.Errorf("failed to register descriptor for %q: %w", plan.Name, err))
			}
		}

		result.Invocables = append(result.Invocables, &thoughtrecipes.RecipeInvocable{
			Plan:     plan,
			Executor: result.Executor,
		})
	}

	// Collect warnings from load result
	result.Warnings = append(result.Warnings, loadResult.Warnings...)

	// Log summary
	if result.Registry.Count() > 0 {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Loaded %d thought recipes", result.Registry.Count()))
	}

	return result
}

// isCommonWord returns true for common words that shouldn't be used as keywords.
func isCommonWord(word string) bool {
	common := map[string]bool{
		"the": true, "and": true, "for": true, "with": true, "that": true,
		"this": true, "from": true, "have": true, "will": true, "your": true,
		"they": true, "been": true, "were": true, "said": true, "each": true,
		"which": true, "their": true, "time": true, "would": true, "there": true,
		"when": true, "where": true, "what": true, "who": true, "how": true,
		"why": true, "all": true, "any": true, "can": true, "had": true,
		"her": true, "was": true, "one": true, "our": true, "out": true,
		"day": true, "get": true, "has": true, "him": true, "his": true,
		"its": true, "may": true, "new": true, "now": true, "old": true,
		"see": true, "two": true, "use": true, "way": true, "you": true,
	}
	return common[word]
}
