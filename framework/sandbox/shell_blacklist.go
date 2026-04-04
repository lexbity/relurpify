package sandbox

import (
	"fmt"
	"io"
	"os"
	"regexp"

	"gopkg.in/yaml.v3"
)

// BlacklistAction defines the action to take when a rule matches.
type BlacklistAction string

const (
	// BlacklistActionBlock prevents command execution entirely.
	BlacklistActionBlock BlacklistAction = "block"
	// BlacklistActionHITL requires human-in-the-loop approval.
	BlacklistActionHITL BlacklistAction = "hitl"
)

// BlacklistRule represents a single compiled rule.
type BlacklistRule struct {
	ID      string          `yaml:"id"`
	Pattern *regexp.Regexp  `yaml:"-"`
	Raw     string          `yaml:"pattern"`
	Reason  string          `yaml:"reason"`
	Action  BlacklistAction `yaml:"action"`
}

// shellBlacklistYAML is the raw YAML structure.
type shellBlacklistYAML struct {
	Version string          `yaml:"version"`
	Rules   []BlacklistRule `yaml:"rules"`
}

// ShellBlacklist holds compiled rules loaded from shell_blacklist.yaml.
// A nil ShellBlacklist is valid and passes all commands through.
type ShellBlacklist struct {
	rules []BlacklistRule
}

// Load reads and compiles rules from path. Missing file returns empty blacklist, no error.
func Load(path string) (*ShellBlacklist, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &ShellBlacklist{}, nil
		}
		return nil, fmt.Errorf("open blacklist file: %w", err)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("read blacklist file: %w", err)
	}

	var raw shellBlacklistYAML
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse blacklist YAML: %w", err)
	}

	bl := &ShellBlacklist{}
	for _, r := range raw.Rules {
		pat, err := regexp.Compile(r.Raw)
		if err != nil {
			return nil, fmt.Errorf("compile pattern %q for rule %q: %w", r.Raw, r.ID, err)
		}
		bl.rules = append(bl.rules, BlacklistRule{
			ID:      r.ID,
			Pattern: pat,
			Raw:     r.Raw,
			Reason:  r.Reason,
			Action:  r.Action,
		})
	}
	return bl, nil
}

// Check returns the first matching rule for the command string, or nil.
func (b *ShellBlacklist) Check(commandString string) *BlacklistRule {
	if b == nil || len(b.rules) == 0 {
		return nil
	}
	for _, rule := range b.rules {
		if rule.Pattern.MatchString(commandString) {
			return &rule
		}
	}
	return nil
}
