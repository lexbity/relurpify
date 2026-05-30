package configcheck

import (
	"testing"

	"codeburg.org/lexbit/relurpify/platform/contracts"
	"github.com/stretchr/testify/require"
)

func TestDeriveBasicSubprocess(t *testing.T) {
	m := contracts.ToolManifest{
		Name: "cli_echo",
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{Base: []string{"echo"}},
		},
	}
	risk, effect := DeriveExpectedCapability(m)
	require.Contains(t, risk, "execute")
	require.Contains(t, effect, "process_spawn")
}

func TestDeriveNetworkTool(t *testing.T) {
	m := contracts.ToolManifest{
		Name: "cli_curl",
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{Base: []string{"curl"}},
			Sandbox: &contracts.ToolManifestSandbox{NetworkAccess: true},
		},
	}
	risk, effect := DeriveExpectedCapability(m)
	require.Contains(t, risk, "execute")
	require.Contains(t, risk, "network")
	require.Contains(t, effect, "process_spawn")
	require.Contains(t, effect, "network_egress")
}

func TestDeriveFileopsTool(t *testing.T) {
	m := contracts.ToolManifest{
		Name:   "cli_rg",
		Family: "fileops",
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{Base: []string{"rg"}},
		},
	}
	_, effect := DeriveExpectedCapability(m)
	require.Contains(t, effect, "filesystem_read")
}

func TestDeriveMkdirTool(t *testing.T) {
	m := contracts.ToolManifest{
		Name:   "cli_mkdir",
		Family: "fileops",
		Execution: contracts.ToolManifestExecution{
			Backend:     contracts.ToolBackendSubprocess,
			Command:     &contracts.ToolManifestCommand{Base: []string{"mkdir"}},
			DefaultArgs: []string{"-p"},
		},
	}
	_, effect := DeriveExpectedCapability(m)
	require.Contains(t, effect, "filesystem_mutation", "mkdir -p must derive filesystem_mutation")
}

func TestCheckManifestPassesWhenDeclared(t *testing.T) {
	m := contracts.ToolManifest{
		Name: "cli_rg",
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{Base: []string{"rg"}},
		},
		Capability: contracts.ToolManifestCapability{
			RiskClass:   []string{"execute"},
			EffectClass: []string{"process_spawn"},
		},
	}
	issues := CheckManifest(m)
	require.Empty(t, issues)
}

func TestCheckManifestFailsOnMissingNetwork(t *testing.T) {
	m := contracts.ToolManifest{
		Name: "cli_curl",
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{Base: []string{"curl"}},
			Sandbox: &contracts.ToolManifestSandbox{NetworkAccess: true},
		},
		Capability: contracts.ToolManifestCapability{
			RiskClass:   []string{"execute"},
			EffectClass: []string{"process_spawn"},
		},
	}
	issues := CheckManifest(m)
	require.NotEmpty(t, issues, "missing network risk/effect must be flagged")
	require.Contains(t, issues[0], "network")
}

func TestCheckManifestFailsOnMissingFilesystemRead(t *testing.T) {
	m := contracts.ToolManifest{
		Name:   "cli_rg",
		Family: "fileops",
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{Base: []string{"rg"}},
		},
		Capability: contracts.ToolManifestCapability{
			RiskClass:   []string{"execute"},
			EffectClass: []string{"process_spawn"},
		},
	}
	issues := CheckManifest(m)
	require.NotEmpty(t, issues, "missing filesystem_read effect must be flagged")
	require.Contains(t, issues[0], "filesystem_read")
}

func TestCheckAllManifests(t *testing.T) {
	manifests := []*contracts.ToolManifest{
		{
			Name:        "good_tool",
			Description: "ok",
			Execution: contracts.ToolManifestExecution{
				Backend: contracts.ToolBackendSubprocess,
				Command: &contracts.ToolManifestCommand{Base: []string{"echo"}},
			},
			Capability: contracts.ToolManifestCapability{
				RiskClass:   []string{"execute"},
				EffectClass: []string{"process_spawn"},
			},
		},
		{
			Name:        "bad_tool",
			Description: "missing effect",
			Execution: contracts.ToolManifestExecution{
				Backend: contracts.ToolBackendSubprocess,
				Command: &contracts.ToolManifestCommand{Base: []string{"curl"}},
				Sandbox: &contracts.ToolManifestSandbox{NetworkAccess: true},
			},
			Capability: contracts.ToolManifestCapability{
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
	m := contracts.ToolManifest{
		Name: "go_test",
		Execution: contracts.ToolManifestExecution{
			Backend:        contracts.ToolBackendGoNative,
			Implementation: "go_test",
		},
		Capability: contracts.ToolManifestCapability{
			RiskClass:   []string{"execute"},
			EffectClass: []string{"process_spawn"},
		},
	}
	issues := CheckManifest(m)
	require.Empty(t, issues, "go_native tools must be skipped")
}
