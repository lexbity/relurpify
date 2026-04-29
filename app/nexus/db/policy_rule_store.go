package db

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"codeburg.org/lexbit/relurpify/framework/core"
)

type FilePolicyRuleStore struct {
	mu    sync.RWMutex
	path  string
	rules []core.PolicyRule
}

func NewFilePolicyRuleStore(path string) (*FilePolicyRuleStore, error) {
	store := &FilePolicyRuleStore{path: filepath.Clean(path)}
	if err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *FilePolicyRuleStore) ListRules(ctx context.Context) ([]core.PolicyRule, error) {
	if s == nil {
		return nil, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]core.PolicyRule, len(s.rules))
	copy(out, s.rules)
	return out, nil
}

func (s *FilePolicyRuleStore) SetRuleEnabled(ctx context.Context, ruleID string, enabled bool) error {
	if s == nil {
		return errors.New("policy rule store unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.rules {
		if s.rules[i].ID == ruleID {
			s.rules[i].Enabled = enabled
			return s.saveLocked()
		}
	}
	return os.ErrNotExist
}

func (s *FilePolicyRuleStore) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			s.rules = nil
			return nil
		}
		return err
	}
	if len(data) == 0 {
		s.rules = nil
		return nil
	}
	var rules []core.PolicyRule
	if err := json.Unmarshal(data, &rules); err != nil {
		return err
	}
	s.rules = rules
	return nil
}

func (s *FilePolicyRuleStore) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s.rules, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
