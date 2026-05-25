package sandbox

import (
	"reflect"
	"testing"
)

func TestAssembleSubprocessEnvFiltersAndOverrides(t *testing.T) {
	hostEnv := []string{
		"HOME=/home/host",
		"PATH=/usr/bin",
		"RELURPIFY_LLM_API_KEY=llm-secret",
		"RELURPIFY_NEXUS_TOKEN=nexus-secret",
		"IGNORED_ENTRY",
	}
	allowedKeys := []string{"HOME", "PATH", "MISSING"}
	extraEnv := []string{"PATH=/tool/bin", "EXTRA=value", "PATH=/override/bin"}

	got := assembleSubprocessEnv(hostEnv, allowedKeys, extraEnv)
	want := []string{
		"HOME=/home/host",
		"PATH=/override/bin",
		"EXTRA=value",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected env\n got: %#v\nwant: %#v", got, want)
	}
}

func TestAssembleSubprocessEnvEmpty(t *testing.T) {
	if got := assembleSubprocessEnv(nil, nil, nil); got != nil {
		t.Fatalf("expected nil env, got %#v", got)
	}
}
