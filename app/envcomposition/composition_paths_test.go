package envcomposition

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/context/knowledge/graphdb"
	fauthorization "codeburg.org/lexbit/relurpify/governance/authorization"
	gpermissions "codeburg.org/lexbit/relurpify/governance/permissions"
	governanceports "codeburg.org/lexbit/relurpify/governance/ports"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

// TestBuildSecurityRuntimeInvalidBashDefault proves an out-of-vocabulary
// bash default is rejected at composition time rather than silently falling
// back to allow.
func TestBuildSecurityRuntimeInvalidBashDefault(t *testing.T) {
	manager, err := fauthorization.NewPermissionManager(t.TempDir(), &gpermissions.PermissionSet{}, nil, nil)
	require.NoError(t, err)

	_, err = BuildSecurityRuntime(context.Background(), SecurityRuntimeInput{
		ExistingRunner:    fakeRunner{},
		PermissionManager: manager,
		AgentID:           "agent",
		AgentSpec: &agentspec.AgentRuntimeSpec{
			Bash: agentspec.AgentBashPermissions{Default: "bogus"},
		},
	})
	require.ErrorContains(t, err, "agent bash default")
}

// TestBuildKnowledgeRuntime proves the knowledge bundle is assembled with a
// live compiler, event bus, retriever, and stream trigger, and that Close
// shuts the compiler down.
func TestBuildKnowledgeRuntime(t *testing.T) {
	engine, err := graphdb.Open(context.Background(), graphdb.DefaultOptions(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, engine.Close(context.Background())) })

	kr, err := BuildKnowledgeRuntime(KnowledgeRuntimeInput{GraphDB: engine})
	require.NoError(t, err)
	require.NotNil(t, kr)
	require.NotNil(t, kr.Compiler)
	require.NotNil(t, kr.KnowledgeEvents)
	require.NotNil(t, kr.Retriever)
	require.NotNil(t, kr.StreamTrigger)

	// Close is idempotent and safe on a fully-built runtime.
	kr.Close()
	kr.Close()

	_, err = BuildKnowledgeRuntime(KnowledgeRuntimeInput{})
	require.ErrorContains(t, err, "graphdb engine required")
}

// TestSandboxPolicyAdapters round-trips the governance<->sandbox policy bridge
// and the manifest-derived sandbox policy.
func TestSandboxPolicyAdapters(t *testing.T) {
	gp := governanceports.SandboxPolicy{
		ReadOnlyRoot:    true,
		ProtectedPaths:  []string{"a", "b"},
		NoNewPrivileges: true,
		NetworkRules: []governanceports.SandboxNetworkRule{
			{Direction: "egress", Protocol: "tcp", Host: "example.com", Port: 443, Description: "web"},
		},
	}
	sp := toSandboxPolicy(gp)
	require.True(t, sp.ReadOnlyRoot)
	require.True(t, sp.NoNewPrivileges)
	require.Equal(t, []string{"a", "b"}, sp.ProtectedPaths)
	require.Len(t, sp.NetworkRules, 1)
	require.Equal(t, "example.com", sp.NetworkRules[0].Host)

	back := fromSandboxPolicy(sp)
	require.Equal(t, gp.ReadOnlyRoot, back.ReadOnlyRoot)
	require.Equal(t, gp.ProtectedPaths, back.ProtectedPaths)
	require.Len(t, back.NetworkRules, 1)
	require.Equal(t, "example.com", back.NetworkRules[0].Host)

	np := newSandboxPolicy(config.SecuritySpec{ReadOnlyRoot: true, NoNewPrivileges: true}, []string{"p"})
	require.Equal(t, []string{"p"}, np.ProtectedPaths)
	require.True(t, np.ReadOnlyRoot)
	require.True(t, np.NoNewPrivileges)
}

// TestNewSandboxBackendFactory covers the supported-backend and
// unsupported-backend branches without launching any container.
func TestNewSandboxBackendFactory(t *testing.T) {
	factory := NewSandboxBackendFactory()
	require.NotNil(t, factory)

	_, err := factory(context.Background(), "bogus", governanceports.SandboxConfig{}, "", t.TempDir())
	require.ErrorContains(t, err, "unsupported sandbox backend")

	rt, err := factory(context.Background(), "gvisor", governanceports.SandboxConfig{}, "", t.TempDir())
	require.NoError(t, err)
	require.NotNil(t, rt)
	require.NotEmpty(t, rt.Name())
}

// TestJoinErrorsUnwrap pins the multi-error aggregation used by the boot
// invariant validator.
func TestJoinErrorsUnwrap(t *testing.T) {
	require.NoError(t, joinErrors(nil))

	first := errors.New("first")
	second := errors.New("second")
	err := joinErrors([]error{first, second})
	require.Error(t, err)
	require.Contains(t, err.Error(), "first")
	require.Contains(t, err.Error(), "second")
	require.ErrorIs(t, err, first, "multiError must unwrap to its first error")
}
