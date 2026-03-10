package db

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFilePolicyRuleStoreListAndToggle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "policy_rules.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
- id: rule-a
  name: Rule A
  priority: 10
  enabled: true
  effect:
    action: allow
`), 0o644))

	store, err := NewFilePolicyRuleStore(path)
	require.NoError(t, err)

	rules, err := store.ListRules(context.Background())
	require.NoError(t, err)
	require.Len(t, rules, 1)
	require.True(t, rules[0].Enabled)

	require.NoError(t, store.SetRuleEnabled(context.Background(), "rule-a", false))
	rules, err = store.ListRules(context.Background())
	require.NoError(t, err)
	require.False(t, rules[0].Enabled)
}
