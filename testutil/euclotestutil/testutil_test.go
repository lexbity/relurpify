package testutil

import "testing"

func TestEnvMinimalProvidesRegistryAndConfig(t *testing.T) {
	env := EnvMinimal()
	if env.Registry == nil {
		t.Fatal("EnvMinimal returned nil registry")
	}
	if env.Config == nil {
		t.Fatal("EnvMinimal returned nil config")
	}
}

func TestEnvProvidesMemoryRegistryAndConfig(t *testing.T) {
	env := Env(t)
	if env.Model == nil {
		t.Fatal("Env returned nil model")
	}
	if env.Registry == nil {
		t.Fatal("Env returned nil registry")
	}
	if env.Memory == nil {
		t.Fatal("Env returned nil memory")
	}
	if env.Config == nil {
		t.Fatal("Env returned nil config")
	}
}
