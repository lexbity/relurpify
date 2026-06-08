package arch

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Allowlist holds known violations that are temporarily exempted.
// Each phase removes entries as the corresponding issues are fixed.
type Allowlist struct {
	entries map[string]map[string]bool // category → { description: true }
}

// AllowlistFile is the YAML structure for the allowlist.
type AllowlistFile struct {
	Version           int      `yaml:"version"`
	Cycles            []string `yaml:"cycle_violations"`
	Layers            []string `yaml:"layer_violations"`
	Buckets           []string `yaml:"bucket_violations"`
	Consumers         []string `yaml:"consumer_violations"`
	Globs             []string `yaml:"glob_violations"`
	Stubs             []string `yaml:"stub_violations"`
	InternalConsumers []string `yaml:"internal_consumer_violations"`
	Converters        []string `yaml:"converter_violations"`
}

// LoadAllowlist reads the allowlist from a YAML file.
func LoadAllowlist(path string) (Allowlist, error) {
	a := Allowlist{entries: make(map[string]map[string]bool)}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return a, nil
		}
		return a, fmt.Errorf("read allowlist: %w", err)
	}
	var f AllowlistFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return a, fmt.Errorf("parse allowlist: %w", err)
	}

	categories := map[string][]string{
		"cycle":             f.Cycles,
		"layer":             f.Layers,
		"bucket":            f.Buckets,
		"consumer":          f.Consumers,
		"glob":              f.Globs,
		"stub":              f.Stubs,
		"internal-consumer": f.InternalConsumers,
		"converter":         f.Converters,
	}
	for cat, items := range categories {
		a.entries[cat] = make(map[string]bool)
		for _, item := range items {
			a.entries[cat][item] = true
		}
	}
	return a, nil
}

// Contains reports whether the given violation is in the allowlist.
func (a Allowlist) Contains(category, description string) bool {
	if m, ok := a.entries[category]; ok {
		return m[description]
	}
	return false
}

// ValidateAllowlist checks that every entry in the allowlist corresponds to
// a violation that is no longer present (i.e., can be removed).
func ValidateAllowlist(a Allowlist, violations map[string][]string) []string {
	var stale []string
	for cat, entries := range a.entries {
		current := violations[cat]
		currentSet := make(map[string]bool)
		for _, v := range current {
			currentSet[v] = true
		}
		for entry := range entries {
			if !currentSet[entry] {
				stale = append(stale, fmt.Sprintf("stale allowlist entry [%s]: %s", cat, entry))
			}
		}
	}
	return stale
}
