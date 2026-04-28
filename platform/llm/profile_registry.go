package llm

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"codeburg.org/lexbit/relurpify/platform/contracts"
	"gopkg.in/yaml.v3"
)

// ProfileRegistry loads ModelProfile files from a directory and matches them
// by provider/model identity and model selector.
type ProfileRegistry struct {
	profiles []*profileEntry
}

type profileEntry struct {
	profile    *ModelProfile
	sourcePath string
	isDefault  bool
}

// ProfileResolution captures the selected profile together with match metadata.
type ProfileResolution struct {
	Profile    *ModelProfile
	SourcePath string
	Reason     string
	MatchKind  string
	Provider   string
	Model      string
}

// NewProfileRegistry loads all *.yaml and *.yml files from configDir.
// Missing directory returns an empty registry using built-in defaults.
func NewProfileRegistry(configDir string) (*ProfileRegistry, error) {
	reg := &ProfileRegistry{}
	if strings.TrimSpace(configDir) == "" {
		return reg, nil
	}
	entries, err := os.ReadDir(configDir)
	if err != nil {
		if os.IsNotExist(err) {
			return reg, nil
		}
		return nil, fmt.Errorf("read model profiles dir: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		path := filepath.Join(configDir, name)
		loaded, err := loadProfileFile(path)
		if err != nil {
			return nil, err
		}
		loaded.SourcePath = path
		reg.profiles = append(reg.profiles, &profileEntry{
			profile:    loaded,
			sourcePath: path,
			isDefault:  isDefaultProfileFile(name),
		})
	}
	return reg, nil
}

func loadProfileFile(path string) (*ModelProfile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open profile %s: %w", path, err)
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("read profile %s: %w", path, err)
	}
	var profile ModelProfile
	if err := yaml.Unmarshal(data, &profile); err != nil {
		return nil, fmt.Errorf("parse profile %s: %w", path, err)
	}
	profile.Normalize()
	return &profile, nil
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
func (r *ProfileRegistry) Match(modelName string) *ModelProfile {
	return r.Resolve("", modelName).Profile
}

// ApplyProfile attaches profile metadata to a profile-aware object when
// supported. It returns true if the target accepted the profile.
func ApplyProfile(target any, profile *ModelProfile) bool {
	if target == nil || profile == nil {
		return false
	}
	setter, ok := target.(interface{ SetProfile(*ModelProfile) })
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

func builtinDefaultProfile() *ModelProfile {
	profile := &ModelProfile{Pattern: "*"}
	profile.Normalize()
	return profile
}

func profileScore(profile *ModelProfile, isDefault bool, provider, model string) (int, string) {
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
	return contracts.MatchGlob(pattern, value)
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

func profileReason(kind string, profile *ModelProfile, provider, model string, isDefault bool) string {
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

func isDefaultProfileFile(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	return name == "default.yaml" || name == "default.yml"
}
