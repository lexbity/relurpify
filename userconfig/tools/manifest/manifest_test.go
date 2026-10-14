package manifest

import (
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestNormalizeToolNameCanonicalizesIdentifiers(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", ""},
		{"blank", "   ", ""},
		{"already canonical", "file_read", "file_read"},
		{"lowercases", "File_Read", "file_read"},
		{"trims surrounding space", "  bash  ", "bash"},
		{"dashes become underscores", "git-blame", "git_blame"},
		{"dots become underscores", "go.test", "go_test"},
		{"slashes become underscores", "lsp/get_diagnostics", "lsp_get_diagnostics"},
		{"separator runs collapse", "a--b__c  d", "a_b_c_d"},
		{"leading and trailing separators trim", ".gitignore", "gitignore"},
		{"trailing dash trims", "rg-", "rg"},
		{"unknown runes drop", "cargo+nightly(build)", "cargonightlybuild"},
		{"inner whitespace collapses", "Bash Tool", "bash_tool"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, NormalizeToolName(tc.input))
		})
	}
}

// FuzzNormalizeToolName pins the canonical-identifier contract for arbitrary
// tool names: lowercase output, no whitespace, no path separators, and no
// empty or double underscores (the builder collapses separator runs).
func FuzzNormalizeToolName(f *testing.F) {
	f.Add("File_Create")
	f.Add("lsp/get_diagnostics")
	f.Add("  bash  tool ")
	f.Add("--a--")
	f.Add("___")
	f.Add("a///b")

	f.Fuzz(func(t *testing.T, name string) {
		got := NormalizeToolName(name)
		require.False(t, strings.ContainsAny(got, "ABCDEFGHIJKLMNOPQRSTUVWXYZ/\\"),
			"normalized name %q must be lowercase and path-free", got)
		require.NotContains(t, got, "__", "normalized name %q must not contain double underscores", got)
		require.False(t, strings.HasPrefix(got, "_") || strings.HasSuffix(got, "_"),
			"normalized name %q must not start or end with an underscore", got)
		for _, r := range got {
			require.False(t, unicode.IsSpace(r), "normalized name %q must not contain whitespace", got)
		}
	})
}

// TestToolManifestFullDecode exercises the full declarative surface of a tool
// manifest: parameters, execution backends, platform variants, flags,
// sandbox, returns chunking, rate limit, guidance, and composition steps.
func TestToolManifestFullDecode(t *testing.T) {
	doc := `
name: search_grep
version: "2"
family: search
intent: [find, inspect]
description: grep the workspace
guidance:
  use_when: [exact strings]
  avoid_when: [semantic queries]
parameters:
  - name: pattern
    type: string
    description: regex to find
    required: true
  - name: max
    type: integer
    default: 100
execution:
  backend: subprocess
  implementation: shell
  command:
    base: [rg]
    args: ["-n", "{pattern}"]
    flags:
      ignore-case:
        when_true: ["-i"]
        when_false: []
      count:
        param: mode
        style: equals
        type: string
        repeat: true
  platform_variants:
    windows:
      base: [rg.exe]
  sandbox:
    allowed_root: workspace
    timeout_seconds: 30
    network_access: false
    allow_flags: true
    allow_hosts: [internal.example]
    allow_private_hosts: [10.0.0.5]
  stdin: none
  default_args: ["--json"]
  allow_stdin: false
  supports_workdir: true
returns:
  type: object
  chunking:
    mode: per_item
    item_path: matches
    ref_fields: [file, line]
errors:
  NO_MATCH: returned when nothing matches
capability:
  trust_class: sandboxed
  risk_class: [read-only]
  effect_class: [read]
rate_limit:
  per_second: 5.5
  burst: 10
composition:
  steps:
    - tool: file_read
      args: {path: "{file}"}
      alias: source
`
	var m ToolManifest
	require.NoError(t, yaml.Unmarshal([]byte(doc), &m))

	require.Equal(t, "search_grep", m.Name)
	require.Equal(t, "2", m.Version)
	require.Equal(t, "search", m.Family)
	require.Equal(t, []string{"find", "inspect"}, m.Intent)
	require.Equal(t, []string{"exact strings"}, m.Guidance.UseWhen)
	require.Equal(t, []string{"semantic queries"}, m.Guidance.AvoidWhen)
	require.Len(t, m.Parameters, 2)
	require.Equal(t, ToolParamString, m.Parameters[0].Type)
	require.True(t, m.Parameters[0].Required)
	require.Equal(t, ToolParamInteger, m.Parameters[1].Type)
	require.Equal(t, 100, m.Parameters[1].Default)

	exec := m.Execution
	require.Equal(t, ToolBackendSubprocess, exec.Backend)
	require.Equal(t, "shell", exec.Implementation)
	require.Equal(t, []string{"rg"}, exec.Command.Base)
	require.Equal(t, []string{"-n", "{pattern}"}, exec.Command.Args)
	require.Equal(t, []string{"-i"}, exec.Command.Flags["ignore-case"].WhenTrue)
	require.Equal(t, "mode", exec.Command.Flags["count"].Param)
	require.Equal(t, FlagStyleEquals, exec.Command.Flags["count"].Style)
	require.True(t, exec.Command.Flags["count"].Repeat)
	require.Equal(t, []string{"rg.exe"}, exec.PlatformVariants["windows"].Base)
	require.Equal(t, "workspace", exec.Sandbox.AllowedRoot)
	require.Equal(t, 30, exec.Sandbox.TimeoutSeconds)
	require.False(t, exec.Sandbox.NetworkAccess)
	require.True(t, exec.Sandbox.AllowFlags)
	require.Equal(t, []string{"internal.example"}, exec.Sandbox.AllowHosts)
	require.Equal(t, []string{"10.0.0.5"}, exec.Sandbox.AllowPrivateHosts)
	require.Equal(t, "none", exec.Stdin)
	require.Equal(t, []string{"--json"}, exec.DefaultArgs)
	require.True(t, exec.SupportsWorkdir)

	require.Equal(t, "object", m.Returns.Type)
	require.Equal(t, ChunkingModePerItem, m.Returns.Chunking.Mode)
	require.Equal(t, "matches", m.Returns.Chunking.ItemPath)
	require.Equal(t, []string{"file", "line"}, m.Returns.Chunking.RefFields)
	require.Equal(t, map[string]string{"NO_MATCH": "returned when nothing matches"}, m.Errors)
	require.Equal(t, "sandboxed", m.Capability.TrustClass)
	require.Equal(t, []string{"read-only"}, m.Capability.RiskClass)
	require.Equal(t, []string{"read"}, m.Capability.EffectClass)
	require.InDelta(t, 5.5, m.RateLimit.PerSecond, 1e-9)
	require.Equal(t, 10, m.RateLimit.Burst)
	require.Equal(t, "file_read", m.Composition.Steps[0].Tool)
	require.Equal(t, map[string]any{"path": "{file}"}, m.Composition.Steps[0].Args)
	require.Equal(t, "source", m.Composition.Steps[0].Alias)
}

// TestToolManifestCompositeBackendDecodes pins the composite backend and the
// remaining parameter type vocabulary round-trip through YAML.
func TestToolManifestCompositeBackendDecodes(t *testing.T) {
	doc := `
name: shell_tool_discover
execution:
  backend: composite
  command:
    base: [discover]
parameters:
  - name: verbose
    type: boolean
  - name: tags
    type: array
  - name: meta
    type: object
  - name: score
    type: number
`
	var m ToolManifest
	require.NoError(t, yaml.Unmarshal([]byte(doc), &m))
	require.Equal(t, ToolBackendComposite, m.Execution.Backend)
	require.Equal(t, ToolParamBoolean, m.Parameters[0].Type)
	require.Equal(t, ToolParamArray, m.Parameters[1].Type)
	require.Equal(t, ToolParamObject, m.Parameters[2].Type)
	require.Equal(t, ToolParamNumber, m.Parameters[3].Type)
}

// TestToolManifestSandboxGracePeriodType keeps the duration decode pinned
// alongside the sandbox test's numeric limits.
func TestToolManifestSandboxGracePeriodType(t *testing.T) {
	var m ToolManifest
	require.NoError(t, yaml.Unmarshal([]byte("execution:\n  sandbox:\n    grace_period: 500ms\n"), &m))
	require.Equal(t, 500*time.Millisecond, m.Execution.Sandbox.GracePeriod)
}
