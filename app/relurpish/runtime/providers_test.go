package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/lexcodex/relurpify/framework/ast"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

type testProvider struct {
	initCalls  int
	closeCalls int
	initErr    error
	closeErr   error
	runtime    *Runtime
}

func (p *testProvider) Initialize(_ context.Context, rt *Runtime) error {
	p.initCalls++
	p.runtime = rt
	return p.initErr
}

func (p *testProvider) Close() error {
	p.closeCalls++
	return p.closeErr
}

type testRuntimeCloser struct {
	closeCalls int
	err        error
}

func (c *testRuntimeCloser) Close() error {
	c.closeCalls++
	return c.err
}

type testIndexStore struct {
	closeCalls int
	err        error
}

func (s *testIndexStore) SaveFile(*ast.FileMetadata) error                    { return nil }
func (s *testIndexStore) GetFile(string) (*ast.FileMetadata, error)           { return nil, nil }
func (s *testIndexStore) GetFileByPath(string) (*ast.FileMetadata, error)     { return nil, nil }
func (s *testIndexStore) ListFiles(ast.Category) ([]*ast.FileMetadata, error) { return nil, nil }
func (s *testIndexStore) DeleteFile(string) error                             { return nil }
func (s *testIndexStore) SaveNodes([]*ast.Node) error                         { return nil }
func (s *testIndexStore) GetNode(string) (*ast.Node, error)                   { return nil, nil }
func (s *testIndexStore) GetNodesByFile(string) ([]*ast.Node, error)          { return nil, nil }
func (s *testIndexStore) GetNodesByType(ast.NodeType) ([]*ast.Node, error)    { return nil, nil }
func (s *testIndexStore) GetNodesByName(string) ([]*ast.Node, error)          { return nil, nil }
func (s *testIndexStore) SearchNodes(ast.NodeQuery) ([]*ast.Node, error)      { return nil, nil }
func (s *testIndexStore) DeleteNode(string) error                             { return nil }
func (s *testIndexStore) SaveEdges([]*ast.Edge) error                         { return nil }
func (s *testIndexStore) GetEdge(string) (*ast.Edge, error)                   { return nil, nil }
func (s *testIndexStore) GetEdgesBySource(string) ([]*ast.Edge, error)        { return nil, nil }
func (s *testIndexStore) GetEdgesByTarget(string) ([]*ast.Edge, error)        { return nil, nil }
func (s *testIndexStore) GetEdgesByType(ast.EdgeType) ([]*ast.Edge, error)    { return nil, nil }
func (s *testIndexStore) SearchEdges(ast.EdgeQuery) ([]*ast.Edge, error)      { return nil, nil }
func (s *testIndexStore) DeleteEdge(string) error                             { return nil }
func (s *testIndexStore) GetCallees(string) ([]*ast.Node, error)              { return nil, nil }
func (s *testIndexStore) GetCallers(string) ([]*ast.Node, error)              { return nil, nil }
func (s *testIndexStore) GetImports(string) ([]*ast.Node, error)              { return nil, nil }
func (s *testIndexStore) GetImportedBy(string) ([]*ast.Node, error)           { return nil, nil }
func (s *testIndexStore) GetReferences(string) ([]*ast.Node, error)           { return nil, nil }
func (s *testIndexStore) GetReferencedBy(string) ([]*ast.Node, error)         { return nil, nil }
func (s *testIndexStore) GetDependencies(string) ([]*ast.Node, error)         { return nil, nil }
func (s *testIndexStore) GetDependents(string) ([]*ast.Node, error)           { return nil, nil }
func (s *testIndexStore) BeginTransaction() (ast.Transaction, error)          { return nil, nil }
func (s *testIndexStore) Vacuum() error                                       { return nil }
func (s *testIndexStore) GetStats() (*ast.IndexStats, error)                  { return nil, nil }
func (s *testIndexStore) Close() error {
	s.closeCalls++
	return s.err
}

func TestRuntimeRegisterProviderInitializesAndStoresProvider(t *testing.T) {
	rt := &Runtime{}
	provider := &testProvider{}

	err := rt.RegisterProvider(context.Background(), provider)

	require.NoError(t, err)
	require.Equal(t, 1, provider.initCalls)
	require.Same(t, rt, provider.runtime)
	require.Len(t, rt.registeredProviders(), 1)
}

func TestRuntimeRegisterProviderDoesNotStoreFailedProvider(t *testing.T) {
	rt := &Runtime{}
	provider := &testProvider{initErr: errors.New("init failed")}

	err := rt.RegisterProvider(context.Background(), provider)

	require.ErrorContains(t, err, "init failed")
	require.Equal(t, 1, provider.initCalls)
	require.Empty(t, rt.registeredProviders())
}

func TestRuntimeCloseClosesProvidersRegistryIndexAndLog(t *testing.T) {
	logCloser := &testRuntimeCloser{}
	registryCloser := &testRuntimeCloser{}
	indexStore := &testIndexStore{}
	rt := &Runtime{
		Context:      core.NewContext(),
		IndexManager: ast.NewIndexManager(indexStore, ast.IndexConfig{}),
		logFile:      logCloser,
	}
	rt.Context.SetHandleScoped("browser.session", registryCloser, "task-1")
	first := &testProvider{}
	second := &testProvider{}
	require.NoError(t, rt.RegisterProvider(context.Background(), first))
	require.NoError(t, rt.RegisterProvider(context.Background(), second))

	err := rt.Close()

	require.NoError(t, err)
	require.Equal(t, 1, second.closeCalls)
	require.Equal(t, 1, first.closeCalls)
	require.Equal(t, 1, registryCloser.closeCalls)
	require.Equal(t, 1, indexStore.closeCalls)
	require.Equal(t, 1, logCloser.closeCalls)
}

func TestRuntimeCloseJoinsErrors(t *testing.T) {
	providerErr := errors.New("provider close failed")
	registryErr := errors.New("registry close failed")
	logErr := errors.New("log close failed")
	rt := &Runtime{
		Context: core.NewContext(),
		logFile: &testRuntimeCloser{err: logErr},
	}
	rt.Context.SetHandleScoped("browser.session", &testRuntimeCloser{err: registryErr}, "task-2")
	require.NoError(t, rt.RegisterProvider(context.Background(), &testProvider{closeErr: providerErr}))

	err := rt.Close()

	require.Error(t, err)
	require.ErrorContains(t, err, providerErr.Error())
	require.ErrorContains(t, err, registryErr.Error())
	require.ErrorContains(t, err, logErr.Error())
}
