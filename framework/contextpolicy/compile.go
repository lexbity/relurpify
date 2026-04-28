package contextpolicy

import (
	"codeburg.org/lexbit/relurpify/framework/manifest"
)

// Compile reads the ContextPolicy section from a resolved manifest and compiles
// it into a runtime-facing ContextPolicyBundle with system defaults applied.
func Compile(manifest *manifest.AgentManifest, skills []manifest.ResolvedSkill, defaults *ContextPolicy) (*ContextPolicyBundle, error) {
	if defaults == nil {
		defaults = DefaultContextPolicy()
	}

	// Start with defaults
	bundle := &ContextPolicyBundle{
		Version:               1,
		CompilationMode:       defaults.CompilationMode,
		DefaultTrustClass:     defaults.DefaultTrustClass,
		TrustDemotedPolicy:    defaults.TrustDemotedPolicy,
		DegradedChunkPolicy:   defaults.DegradedChunkPolicy,
		BudgetShortfallPolicy: defaults.BudgetShortfallPolicy,
	}

	// Apply manifest overrides if present
	if manifest != nil && manifest.Spec.Context != nil {
		ctx := manifest.Spec.Context

		if ctx.CompilationMode != "" {
			bundle.CompilationMode = CompilationMode(ctx.CompilationMode)
		}
		if ctx.DefaultTrustClass != "" {
			bundle.DefaultTrustClass = ctx.DefaultTrustClass
		}
		if ctx.TrustDemotedPolicy != "" {
			bundle.TrustDemotedPolicy = TrustDemotedPolicy(ctx.TrustDemotedPolicy)
		}
		if ctx.DegradedChunkPolicy != "" {
			bundle.DegradedChunkPolicy = DegradedChunkPolicy(ctx.DegradedChunkPolicy)
		}
		if ctx.BudgetShortfallPolicy != "" {
			bundle.BudgetShortfallPolicy = BudgetShortfallPolicy(ctx.BudgetShortfallPolicy)
		}

		// Convert and copy rankers
		if len(ctx.Rankers) > 0 {
			bundle.Rankers = make([]RankerRef, len(ctx.Rankers))
			for i, r := range ctx.Rankers {
				bundle.Rankers[i] = RankerRef{
					ID:       r.ID,
					Priority: r.Priority,
					Config:   r.Config,
				}
			}
		}

		// Convert and copy scanners
		if len(ctx.Scanners) > 0 {
			bundle.Scanners = make([]ScannerRef, len(ctx.Scanners))
			for i, s := range ctx.Scanners {
				bundle.Scanners[i] = ScannerRef{
					ID:       s.ID,
					Priority: s.Priority,
					Config:   s.Config,
				}
			}
		}

		// Convert and copy summarizers
		if len(ctx.Summarizers) > 0 {
			bundle.Summarizers = make([]SummarizerRef, len(ctx.Summarizers))
			for i, s := range ctx.Summarizers {
				bundle.Summarizers[i] = SummarizerRef{
					ID:          s.ID,
					ModelRef:    s.ModelRef,
					ProseConfig: s.ProseConfig,
					CodeConfig:  s.CodeConfig,
				}
			}
		}

		// Convert substitution prefs
		if len(ctx.SubstitutionPrefs) > 0 {
			bundle.SubstitutionPrefs = make([]SubstitutionPreference, len(ctx.SubstitutionPrefs))
			for i, sp := range ctx.SubstitutionPrefs {
				bundle.SubstitutionPrefs[i] = SubstitutionPreference{
					SourceContentType: sp.SourceContentType,
					TargetContentType: sp.TargetContentType,
					Strategy:          SubstitutionStrategy(sp.Strategy),
				}
			}
		}

		// Copy quota config
		if ctx.Quota != nil {
			bundle.Quota = convertQuotaSpec(ctx.Quota)
		} else {
			bundle.Quota = *defaults.Quota
		}

		// Copy rate limit config
		if ctx.RateLimit != nil {
			bundle.RateLimit = convertRateLimitSpec(ctx.RateLimit)
		} else {
			bundle.RateLimit = *defaults.RateLimit
		}
	} else {
		// No manifest context policy, use all defaults
		if defaults.Quota != nil {
			bundle.Quota = *defaults.Quota
		}
		if defaults.RateLimit != nil {
			bundle.RateLimit = *defaults.RateLimit
		}
	}

	// Merge skill contributions
	for _, skill := range skills {
		if skill.Spec.Context != nil {
			skillCtx := skill.Spec.Context

			// Merge rankers
			for _, r := range skillCtx.Rankers {
				if !hasRanker(bundle.Rankers, r.ID) {
					bundle.Rankers = append(bundle.Rankers, RankerRef{
						ID:       r.ID,
						Priority: r.Priority,
						Config:   r.Config,
					})
				}
			}

			// Merge scanners
			for _, s := range skillCtx.Scanners {
				if !hasScanner(bundle.Scanners, s.ID) {
					bundle.Scanners = append(bundle.Scanners, ScannerRef{
						ID:       s.ID,
						Priority: s.Priority,
						Config:   s.Config,
					})
				}
			}

			// Merge summarizers
			for _, s := range skillCtx.Summarizers {
				if !hasSummarizer(bundle.Summarizers, s.ID) {
					bundle.Summarizers = append(bundle.Summarizers, SummarizerRef{
						ID:          s.ID,
						ModelRef:    s.ModelRef,
						ProseConfig: s.ProseConfig,
						CodeConfig:  s.CodeConfig,
					})
				}
			}
		}

		// Merge ContextContributions from skills (via Manifest)
		if skill.Manifest != nil && skill.Manifest.Spec.ContextContributions.IngestionSources != nil {
			for _, source := range skill.Manifest.Spec.ContextContributions.IngestionSources {
				bundle.SkillContributions.IngestionSources = append(bundle.SkillContributions.IngestionSources, IngestionSource{
					Path:       source.Path,
					SourceType: source.SourceType,
				})
			}
		}

		// Merge ranker admissions
		if skill.Manifest != nil {
			for _, rankerID := range skill.Manifest.Spec.ContextContributions.RankerAdmission {
				if !hasString(bundle.SkillContributions.AdmittedRankers, rankerID) {
					bundle.SkillContributions.AdmittedRankers = append(bundle.SkillContributions.AdmittedRankers, rankerID)
				}
			}
		}

		// Merge scanner signatures
		if skill.Manifest != nil {
			for _, sig := range skill.Manifest.Spec.ContextContributions.ScannerConfig.AdditionalSignatures {
				bundle.SkillContributions.ScannerSignatures = append(bundle.SkillContributions.ScannerSignatures, ScannerSignature{
					Pattern: sig.Pattern,
					Flag:    sig.Flag,
				})
			}
		}
	}

	return bundle, nil
}

// convertRateLimitSpec converts a manifest.RateLimitSpec to a RateLimitSpec
func convertRateLimitSpec(r *manifest.RateLimitSpec) RateLimitSpec {
	if r == nil {
		return RateLimitSpec{}
	}
	return RateLimitSpec{
		RequestsPerSecond: r.RequestsPerSecond,
		BurstSize:         r.BurstSize,
	}
}

// convertQuotaSpec converts a manifest.QuotaSpec to a QuotaSpec
func convertQuotaSpec(q *manifest.QuotaSpec) QuotaSpec {
	result := QuotaSpec{
		MaxChunksPerWindow: q.MaxChunksPerWindow,
		MaxTokensPerWindow: q.MaxTokensPerWindow,
		PrincipalPattern:   q.PrincipalPattern,
	}
	// Parse window size string (e.g., "1h", "30m")
	if q.WindowSize != "" {
		// Simple parsing - could be enhanced with proper duration parsing
		result.WindowSize = 0 // Will be set from defaults
	}
	return result
}

func hasRanker(rankers []RankerRef, id string) bool {
	for _, r := range rankers {
		if r.ID == id {
			return true
		}
	}
	return false
}

func hasScanner(scanners []ScannerRef, id string) bool {
	for _, s := range scanners {
		if s.ID == id {
			return true
		}
	}
	return false
}

func hasSummarizer(summarizers []SummarizerRef, id string) bool {
	for _, s := range summarizers {
		if s.ID == id {
			return true
		}
	}
	return false
}

func hasString(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}
