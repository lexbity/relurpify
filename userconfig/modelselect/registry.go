package modelselect

import (
	"fmt"
	"path/filepath"
	"strings"

	"codeburg.org/lexbit/relurpify/model"
)

// NewProfileRegistry creates an empty profile registry.
func NewProfileRegistry() *ProfileRegistry {
	return &ProfileRegistry{}
}

// Add adds a model profile to the registry.
func (r *ProfileRegistry) Add(profile *model.ModelProfile) {
	if profile == nil {
		return
	}
	r.profiles = append(r.profiles, &profileEntry{
		profile:    profile,
		sourcePath: profile.SourcePath,
		isDefault:  isDefaultProfileFile(profile.SourcePath),
	})
}

// ProfileRegistry loads model.ModelProfile files from a directory and matches them
// by provider/model identity and model selector.
type ProfileRegistry struct {
	profiles []*profileEntry
}

type profileEntry struct {
	profile    *model.ModelProfile
	sourcePath string
	isDefault  bool
}

// ProfileResolution captures the selected profile together with match metadata.
type ProfileResolution struct {
	Profile    *model.ModelProfile
	SourcePath string
	Reason     string
	MatchKind  string
	Provider   string
	Model      string
}

// NewProfileRegistryFromProfiles builds a registry from already-converted domain
// profiles. The framework/llmconfig adapter is responsible for converting YAML
// config DTOs into these domain profiles, keeping platform/llm free of any
// dependency on framework configuration loaders.
func NewProfileRegistryFromProfiles(profiles []*model.ModelProfile) *ProfileRegistry {
	reg := &ProfileRegistry{}
	for _, profile := range profiles {
		if profile == nil {
			continue
		}
		reg.profiles = append(reg.profiles, &profileEntry{
			profile:    profile,
			sourcePath: profile.SourcePath,
			isDefault:  isDefaultProfileFile(profile.SourcePath),
		})
	}
	return reg
}

// Resolve returns the best-matching profile for provider/model.
// Matching priority:
// 1. exact provider + model match
// 2. exact model match
// 3. longest prefix or glob match
// 4. default.yaml
func (r *ProfileRegistry) Resolve(provider, model string) ProfileResolution {
	provider = strings.ToLower(strings.TrimSpace(provider))
	model = strings.TrimSpace(model)
	if r == nil || len(r.profiles) == 0 {
		return builtinProfileResolution(provider, model)
	}

	var best *profileEntry
	bestScore := -1
	bestKind := ""
	var defaultEntry *profileEntry
	for _, entry := range r.profiles {
		if entry.isDefault {
			defaultEntry = entry
			continue
		}
		score, kind := profileScore(entry.profile, entry.isDefault, provider, model)
		if score > bestScore {
			bestScore = score
			best = entry
			bestKind = kind
		}
	}
	if best == nil || bestScore < 0 {
		if defaultEntry != nil && defaultEntry.profile != nil {
			res := ProfileResolution{
				Profile:    defaultEntry.profile.Clone(),
				SourcePath: defaultEntry.sourcePath,
				Reason:     profileReason("default", defaultEntry.profile, provider, model, true),
				MatchKind:  "default",
				Provider:   provider,
				Model:      model,
			}
			if res.Profile != nil {
				res.Profile.SourcePath = defaultEntry.sourcePath
			}
			return res
		}
		return builtinProfileResolution(provider, model)
	}

	res := ProfileResolution{
		Profile:    best.profile.Clone(),
		SourcePath: best.sourcePath,
		Reason:     profileReason(bestKind, best.profile, provider, model, best.isDefault),
		MatchKind:  bestKind,
		Provider:   provider,
		Model:      model,
	}
	if res.Profile != nil {
		res.Profile.SourcePath = best.sourcePath
	}
	return res
}

// Match preserves the older single-argument API by resolving against a model
// name without provider scoping.
func (r *ProfileRegistry) Match(modelName string) *model.ModelProfile {
	return r.Resolve("", modelName).Profile
}

// ApplyProfile attaches profile metadata to a profile-aware object when
// supported. It returns true if the target accepted the profile.
func ApplyProfile(target any, profile *model.ModelProfile) bool {
	if target == nil || profile == nil {
		return false
	}
	setter, ok := target.(interface{ SetProfile(*model.ModelProfile) })
	if !ok {
		return false
	}
	setter.SetProfile(profile.Clone())
	return true
}

func builtinProfileResolution(provider, model string) ProfileResolution {
	profile := builtinDefaultProfile()
	return ProfileResolution{
		Profile:    profile,
		Reason:     "built-in default profile",
		MatchKind:  "builtin-default",
		Provider:   provider,
		Model:      model,
		SourcePath: "",
	}
}

func builtinDefaultProfile() *model.ModelProfile {
	profile := &model.ModelProfile{Pattern: "*"}
	profile.Normalize()
	return profile
}

func profileScore(profile *model.ModelProfile, isDefault bool, provider, model string) (int, string) {
	if profile == nil {
		return -1, ""
	}
	if model == "" {
		return -1, ""
	}
	if profile.Provider != "" && profile.Provider != provider {
		return -1, ""
	}

	pattern := profile.MatchPattern()
	if pattern == "" {
		if isDefault {
			return 0, "default"
		}
		return -1, ""
	}

	if profile.IsExactModelMatch() {
		if strings.EqualFold(pattern, model) {
			switch {
			case profile.Provider != "" && provider == profile.Provider:
				return 4000 + len(pattern), "provider-model-exact"
			case profile.Provider == "":
				return 3000 + len(pattern), "model-exact"
			}
		}
	}

	if matchPattern(pattern, model) {
		score := 2000 + specificityScore(pattern)
		if profile.Provider != "" {
			score += 250
			return score, "provider-glob"
		}
		return score, "glob"
	}
	return -1, ""
}

func matchPattern(pattern, value string) bool {
	if pattern == "" {
		return false
	}
	if !strings.ContainsAny(pattern, "*?[") {
		return strings.EqualFold(pattern, value)
	}
	return matchGlob(pattern, value)
}

func specificityScore(pattern string) int {
	pattern = filepath.ToSlash(pattern)
	idx := len(pattern)
	for i, r := range pattern {
		switch r {
		case '*', '?', '[':
			idx = i
			return idx
		}
	}
	return idx
}

func profileReason(kind string, profile *model.ModelProfile, provider, model string, isDefault bool) string {
	switch kind {
	case "provider-model-exact":
		return fmt.Sprintf("provider/model exact match for %s/%s", provider, model)
	case "model-exact":
		return fmt.Sprintf("exact model match for %s", model)
	case "provider-glob":
		return fmt.Sprintf("provider-scoped glob match for %s/%s", provider, model)
	case "glob":
		return fmt.Sprintf("glob match for %s", model)
	case "default":
		if isDefault && profile != nil && profile.SourcePath != "" {
			return fmt.Sprintf("default profile from %s", filepath.Base(profile.SourcePath))
		}
		return "default profile"
	case "builtin-default":
		return "built-in default profile"
	default:
		return "no matching profile"
	}
}

func matchGlob(pattern, value string) bool {
	if pattern == "" {
		return false
	}
	if !strings.ContainsAny(pattern, "*?[") {
		return strings.EqualFold(pattern, value)
	}
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return strings.EqualFold(pattern, value)
	}
	idx := 0
	for i, part := range parts {
		if part == "" {
			if i == 0 || i == len(parts)-1 {
				continue
			}
			continue
		}
		lower := strings.ToLower(value)
		partLower := strings.ToLower(part)
		pos := strings.Index(lower[idx:], partLower)
		if pos < 0 {
			return false
		}
		idx += pos + len(part)
	}
	return true
}

type ProviderRegistry struct{}

func NewProviderRegistry(providers any) *ProviderRegistry {
	return &ProviderRegistry{}
}

func isDefaultProfileFile(path string) bool {
	name := strings.ToLower(strings.TrimSpace(filepath.Base(path)))
	return name == "default.llm.yaml"
}
