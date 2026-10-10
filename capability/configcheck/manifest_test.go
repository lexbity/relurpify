package configcheck

import (
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/capability/toolcapabilities"
)

func TestDeriveBasicSubprocess(t *testing.T) {
	m := toolcapabilities.ToolManifest{
		Name: "cli_echo",
		Execution: toolcapabilities.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
			Command: &toolcapabilities.ToolManifestCommand{Base: []string{"echo"}},
		},
	}
	risk, effect := DeriveExpectedCapability(m)
	require.Contains(t, risk, "execute")
	require.Contains(t, effect, "process_spawn")
}

func TestDeriveNetworkTool(t *testing.T) {
	m := toolcapabilities.ToolManifest{
		Name: "cli_curl",
		Execution: toolcapabilities.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
			Command: &toolcapabilities.ToolManifestCommand{Base: []string{"curl"}},
			Sandbox: &toolcapabilities.ToolManifestSandbox{NetworkAccess: true},
		},
	}
	risk, effect := DeriveExpectedCapability(m)
	require.Contains(t, risk, "execute")
	require.Contains(t, risk, "network")
	require.Contains(t, effect, "process_spawn")
	require.Contains(t, effect, "network_egress")
}

func TestDeriveFileopsTool(t *testing.T) {
	m := toolcapabilities.ToolManifest{
		Name:   "cli_rg",
		Family: "fileops",
		Execution: toolcapabilities.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
			Command: &toolcapabilities.ToolManifestCommand{Base: []string{"rg"}},
		},
	}
	_, effect := DeriveExpectedCapability(m)
	require.Contains(t, effect, "filesystem_read")
}

func TestDeriveMkdirTool(t *testing.T) {
	m := toolcapabilities.ToolManifest{
		Name:   "cli_mkdir",
		Family: "fileops",
		Execution: toolcapabilities.ToolManifestExecution{
			Backend:     ports.ToolBackendSubprocess,
			Command:     &toolcapabilities.ToolManifestCommand{Base: []string{"mkdir"}},
			DefaultArgs: []string{"-p"},
		},
	}
	_, effect := DeriveExpectedCapability(m)
	require.Contains(t, effect, "filesystem_mutation", "mkdir -p must derive filesystem_mutation")
}

func TestCheckManifestPassesWhenDeclared(t *testing.T) {
	m := toolcapabilities.ToolManifest{
		Name: "cli_rg",
		Execution: toolcapabilities.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
			Command: &toolcapabilities.ToolManifestCommand{Base: []string{"rg"}},
		},
		Capability: toolcapabilities.ToolManifestCapability{
			RiskClass:   []string{"execute"},
			EffectClass: []string{"process_spawn"},
		},
	}
	issues := CheckManifest(m)
	require.Empty(t, issues)
}

func TestCheckManifestFailsOnMissingNetwork(t *testing.T) {
	m := toolcapabilities.ToolManifest{
		Name: "cli_curl",
		Execution: toolcapabilities.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
			Command: &toolcapabilities.ToolManifestCommand{Base: []string{"curl"}},
			Sandbox: &toolcapabilities.ToolManifestSandbox{NetworkAccess: true},
		},
		Capability: toolcapabilities.ToolManifestCapability{
			RiskClass:   []string{"execute"},
			EffectClass: []string{"process_spawn"},
		},
	}
	issues := CheckManifest(m)
	require.NotEmpty(t, issues, "missing network risk/effect must be flagged")
	require.Contains(t, issues[0], "network")
}

func TestCheckManifestFailsOnMissingFilesystemRead(t *testing.T) {
	m := toolcapabilities.ToolManifest{
		Name:   "cli_rg",
		Family: "fileops",
		Execution: toolcapabilities.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
			Command: &toolcapabilities.ToolManifestCommand{Base: []string{"rg"}},
		},
		Capability: toolcapabilities.ToolManifestCapability{
			RiskClass:   []string{"execute"},
			EffectClass: []string{"process_spawn"},
		},
	}
	issues := CheckManifest(m)
	require.NotEmpty(t, issues, "missing filesystem_read effect must be flagged")
	require.Contains(t, issues[0], "filesystem_read")
}

func TestCheckAllManifests(t *testing.T) {
	manifests := []*toolcapabilities.ToolManifest{
		{
			Name:        "good_tool",
			Description: "ok",
			Execution: toolcapabilities.ToolManifestExecution{
				Backend: ports.ToolBackendSubprocess,
				Command: &toolcapabilities.ToolManifestCommand{Base: []string{"echo"}},
			},
			Capability: toolcapabilities.ToolManifestCapability{
				RiskClass:   []string{"execute"},
				EffectClass: []string{"process_spawn"},
			},
		},
		{
			Name:        "bad_tool",
			Description: "missing effect",
			Execution: toolcapabilities.ToolManifestExecution{
				Backend: ports.ToolBackendSubprocess,
				Command: &toolcapabilities.ToolManifestCommand{Base: []string{"curl"}},
				Sandbox: &toolcapabilities.ToolManifestSandbox{NetworkAccess: true},
			},
			Capability: toolcapabilities.ToolManifestCapability{
				RiskClass:   []string{"execute"},
				EffectClass: []string{"process_spawn"},
			},
		},
	}
	results := CheckAllManifests(manifests)
	require.NotContains(t, results, "good_tool", "good tool must pass")
	require.Contains(t, results, "bad_tool", "bad tool must fail")
	require.Contains(t, results["bad_tool"][0], "network")
}

func TestCheckManifestSkipsGoNative(t *testing.T) {
	m := toolcapabilities.ToolManifest{
		Name: "go_test",
		Execution: toolcapabilities.ToolManifestExecution{
			Backend:        ports.ToolBackendGoNative,
			Implementation: "go_test",
		},
		Capability: toolcapabilities.ToolManifestCapability{
			RiskClass:   []string{"execute"},
			EffectClass: []string{"process_spawn"},
		},
	}
	issues := CheckManifest(m)
	require.Empty(t, issues, "go_native tools must be skipped")
}
