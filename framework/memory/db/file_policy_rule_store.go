package db

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/lexcodex/relurpify/framework/core"
	"gopkg.in/yaml.v3"
)

type FilePolicyRuleStore struct {
	path string
}

func NewFilePolicyRuleStore(path string) (*FilePolicyRuleStore, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("policy rule store path required")
	}
	return &FilePolicyRuleStore{path: filepath.Clean(path)}, nil
}

func (s *FilePolicyRuleStore) ListRules(context.Context) ([]core.PolicyRule, error) {
	rules, err := s.load()
	if err != nil {
		return nil, err
	}
	sort.Slice(rules, func(i, j int) bool {
		if rules[i].Priority == rules[j].Priority {
			return rules[i].ID < rules[j].ID
		}
		return rules[i].Priority < rules[j].Priority
	})
	return rules, nil
}

func (s *FilePolicyRuleStore) SetRuleEnabled(_ context.Context, ruleID string, enabled bool) error {
	rules, err := s.load()
	if err != nil {
		return err
	}
	found := false
	for i := range rules {
		if rules[i].ID != ruleID {
			continue
		}
		rules[i].Enabled = enabled
		found = true
		break
	}
	if !found {
		return os.ErrNotExist
	}
	return s.save(rules)
}

func (s *FilePolicyRuleStore) Close() error { return nil }

func (s *FilePolicyRuleStore) load() ([]core.PolicyRule, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return []core.PolicyRule{}, nil
	}
	if err != nil {
		return nil, err
	}
	var rules []core.PolicyRule
	if err := yaml.Unmarshal(data, &rules); err == nil {
		return rules, nil
	}
	var wrapped struct {
		Rules []core.PolicyRule `yaml:"rules"`
	}
	if err := yaml.Unmarshal(data, &wrapped); err != nil {
		return nil, err
	}
	return wrapped.Rules, nil
}

func (s *FilePolicyRuleStore) save(rules []core.PolicyRule) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(rules)
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o644)
}
