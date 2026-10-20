package envcomposition

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/context/knowledge/graphdb"
	fauthorization "codeburg.org/lexbit/relurpify/governance/authorization"
	govsandbox "codeburg.org/lexbit/relurpify/governance/sandbox"
	"codeburg.org/lexbit/relurpify/testsuite/testsupport"
	"codeburg.org/lexbit/relurpify/userconfig/config"
	cfgsecurity "codeburg.org/lexbit/relurpify/userconfig/config/security"
	ucperms "codeburg.org/lexbit/relurpify/userconfig/permissions"
	"github.com/stretchr/testify/require"
)

// TestBuildSecurityRuntimeInvalidBashDefault proves an out-of-vocabulary
// bash default is rejected at composition time rather than silently falling
// back to allow.
func TestBuildSecurityRuntimeInvalidBashDefault(t *testing.T) {
	manager, err := fauthorization.NewPermissionManager(t.TempDir(), &ucperms.PermissionSet{}, nil, nil)
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

// TestSandboxPolicyFromConfig proves the decode→runtime bridge maps every
// overlapping field, so nothing authored is silently dropped (FR-8).
func TestSandboxPolicyFromConfig(t *testing.T) {
	cfg := &cfgsecurity.SandboxPolicyConfig{
		ReadOnlyRoot:    true,
		ProtectedPaths:  []string{"a", "b"},
		NoNewPrivileges: true,
		SeccompProfile:  "runtime/default",
		AllowedEnvKeys:  []string{"PATH"},
		DeniedEnvKeys:   []string{"AWS_SECRET"},
		NetworkRules: []cfgsecurity.NetworkRuleConfig{
			{Direction: "egress", Protocol: "tcp", Host: "example.com", Port: 443},
		},
	}
	// Config-only extras are consumed directly at the composition root
	// (reaping, image pinning); pin them so their removal from the config
	// contract is a compile error.
	cfg.ReapOrphans = true
	cfg.OrphanMaxAge = 24 * time.Hour
	cfg.ImageDigest = "sha256:abc"
	require.True(t, cfg.ReapOrphans)
	require.Equal(t, "sha256:abc", cfg.ImageDigest)

	p := sandboxPolicyFromConfig(config.SecuritySpec{ReadOnlyRoot: false, NoNewPrivileges: false}, cfg)
	require.True(t, p.ReadOnlyRoot)
	require.True(t, p.NoNewPrivileges)
	require.Equal(t, []string{"a", "b"}, p.ProtectedPaths)
	require.Equal(t, "runtime/default", p.SeccompProfile)
	require.Equal(t, []string{"PATH"}, p.AllowedEnvKeys)
	require.Equal(t, []string{"AWS_SECRET"}, p.DeniedEnvKeys)
	require.Len(t, p.NetworkRules, 1)
	require.Equal(t, "example.com", p.NetworkRules[0].Host)
	require.Equal(t, 443, p.NetworkRules[0].Port)
	require.NoError(t, p.Validate())

	// Manifest-spec booleans apply even without a bundle policy.
	p2 := sandboxPolicyFromConfig(config.SecuritySpec{ReadOnlyRoot: true, NoNewPrivileges: true}, nil)
	require.True(t, p2.ReadOnlyRoot)
	require.True(t, p2.NoNewPrivileges)
	require.Nil(t, p2.ProtectedPaths)
}

// TestNewSandboxBackendFactory covers the supported-backend and
// unsupported-backend branches without launching any container.
func TestNewSandboxBackendFactory(t *testing.T) {
	factory := NewSandboxBackendFactory()
	require.NotNil(t, factory)

	_, err := factory(context.Background(), "bogus", govsandbox.SandboxConfig{}, "", t.TempDir())
	require.ErrorContains(t, err, "unsupported sandbox backend")

	rt, err := factory(context.Background(), "gvisor", govsandbox.SandboxConfig{}, "", t.TempDir())
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

// TestCapabilityIndexSkipsRuntimeStateDir proves the AST workspace index never
// descends into the runtime state directory (`.relurpify_state`). The index
// stores its own Badger files there, so indexing them is self-referential and
// makes every boot (and every runtime close, which waits for the index pass)
// pay the cost of reading and hashing Badger's value log.
//
// Assertions use the graph store's node index rather than the store's
// ListFiles/GetStats helpers: with a graph-backed store, persist's graph sync
// deletes the file-metadata node it just wrote (see syncGraphResult), so those
// helpers never observe any indexed file. The node index is the durable record
// the graph queries actually use, and it distinguishes a parsed code file from
// an unparsed artifact — so the state-dir file is a real .go file here.
func TestCapabilityIndexSkipsRuntimeStateDir(t *testing.T) {
	runner, err := testsupport.NewAuthorizedFakeRunner(testsupport.PermitAllPolicy())
	require.NoError(t, err)

	ws := t.TempDir()
	src := filepath.Join(ws, "main.go")
	require.NoError(t, os.WriteFile(src, []byte("package main\n\nfunc main() {}\n"), 0o644))
	state := filepath.Join(ws, ".relurpify_state", "hidden.go")
	require.NoError(t, os.MkdirAll(filepath.Dir(state), 0o755))
	require.NoError(t, os.WriteFile(state, []byte("package hidden\n\nfunc Hidden() {}\n"), 0o644))

	cap, err := BuildCapabilityRuntime(context.Background(), ws, runner, CapabilityRuntimeOptions{AgentID: "state-skip"})
	require.NoError(t, err)
	t.Cleanup(func() { _ = cap.IndexManager.Close(context.Background()) })

	require.NoError(t, cap.IndexManager.WaitUntilReady(context.Background()))
	require.NotNil(t, cap.IndexManager.GraphDB)

	require.NotEmpty(t, cap.IndexManager.GraphDB.NodesBySource(src), "source file must be indexed")
	require.Empty(t, cap.IndexManager.GraphDB.NodesBySource(state), "runtime state file must not be indexed")
}
