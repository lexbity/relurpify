//go:build integration

package testutil

import (
	"path/filepath"

	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	memorydb "github.com/lexcodex/relurpify/framework/memory/db"
)

func EnvIntegration(t interface {
	Helper()
	Fatalf(string, ...interface{})
	TempDir() string
	Cleanup(func())
}) agentenv.AgentEnvironment {
	t.Helper()

	store, err := memorydb.NewSQLiteRuntimeMemoryStore(filepath.Join(t.TempDir(), "runtime_memory.db"))
	if err != nil {
		t.Fatalf("failed to create integration memory store: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	return agentenv.AgentEnvironment{
		Model:    nil,
		Registry: capability.NewRegistry(),
		Memory:   store,
		Config:   &core.Config{Name: "test-integration", Model: "integration", MaxIterations: 1},
	}
}
