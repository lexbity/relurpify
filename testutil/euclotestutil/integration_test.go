//go:build integration

package testutil

import "testing"

func TestEnvIntegrationProvidesSQLiteBackedMemory(t *testing.T) {
	env := EnvIntegration(t)
	if env.Model != nil {
		t.Fatal("EnvIntegration should not provide a model")
	}
	if env.Registry == nil {
		t.Fatal("EnvIntegration returned nil registry")
	}
	if env.Memory == nil {
		t.Fatal("EnvIntegration returned nil memory")
	}
	if env.Config == nil {
		t.Fatal("EnvIntegration returned nil config")
	}
}
