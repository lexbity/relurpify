package configcheck

import (
	"testing"

	"codeburg.org/lexbit/relurpify/capability/toolcapabilities"
)

func egressManifest(name string, networkAccess bool, allowHosts, allowPrivate []string) *toolcapabilities.ToolManifest {
	return &toolcapabilities.ToolManifest{
		Name:   name,
		Family: "network",
		Execution: toolcapabilities.ToolManifestExecution{
			Backend: toolcapabilities.ToolBackendSubprocess,
			Command: &toolcapabilities.ToolManifestCommand{Base: []string{"curl"}},
			Sandbox: &toolcapabilities.ToolManifestSandbox{
				NetworkAccess:     networkAccess,
				AllowHosts:        allowHosts,
				AllowPrivateHosts: allowPrivate,
			},
		},
	}
}

func TestCheckNetworkAllowlists(t *testing.T) {
	manifests := []*toolcapabilities.ToolManifest{
		egressManifest("ok", true, []string{"8.8.8.8"}, nil),
		egressManifest("bad", true, []string{"169.254.169.254"}, nil),
		egressManifest("both", true, []string{"10.0.0.1"}, []string{"10.0.0.1"}),
	}
	results := CheckNetworkAllowlists(manifests)
	if len(results) != 2 {
		t.Fatalf("expected 2 offending manifests, got %d: %v", len(results), results)
	}
	if _, ok := results["ok"]; ok {
		t.Fatal("public allowlist must not be flagged")
	}
	for _, name := range []string{"bad", "both"} {
		if len(results[name]) == 0 {
			t.Fatalf("manifest %q should have blocking problems", name)
		}
	}
}
