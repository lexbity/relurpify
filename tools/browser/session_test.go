package browser

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/runtime"
	"github.com/stretchr/testify/require"
)

func TestSessionNavigateChecksNetworkPermissions(t *testing.T) {
	perms := &core.PermissionSet{
		Network: []core.NetworkPermission{
			{Direction: "egress", Protocol: "tcp", Host: "allowed.example", Port: 443},
		},
	}
	manager, err := runtime.NewPermissionManager("", perms, nil, nil)
	require.NoError(t, err)
	backend := &fakeBackend{}
	session, err := NewSession(SessionConfig{
		Backend:           backend,
		BackendName:       "fake",
		PermissionManager: manager,
		AgentID:           "agent-browser",
	})
	require.NoError(t, err)

	err = session.Navigate(context.Background(), "https://denied.example")

	require.Error(t, err)
	require.True(t, IsErrorCode(err, ErrNavigationBlocked))
	require.Empty(t, backend.currentURL)
}

func TestSessionNavigateAllowsDeclaredDomain(t *testing.T) {
	perms := &core.PermissionSet{
		Network: []core.NetworkPermission{
			{Direction: "egress", Protocol: "tcp", Host: "allowed.example", Port: 443},
		},
	}
	manager, err := runtime.NewPermissionManager("", perms, nil, nil)
	require.NoError(t, err)
	backend := &fakeBackend{}
	session, err := NewSession(SessionConfig{
		Backend:           backend,
		BackendName:       "fake",
		PermissionManager: manager,
		AgentID:           "agent-browser",
	})
	require.NoError(t, err)

	require.NoError(t, session.Navigate(context.Background(), "https://allowed.example/path"))
	require.Equal(t, "https://allowed.example/path", backend.currentURL)
}

func TestSessionNavigateRejectsNonNetworkSchemes(t *testing.T) {
	session, err := NewSession(SessionConfig{
		Backend:     &fakeBackend{},
		BackendName: "fake",
	})
	require.NoError(t, err)

	err = session.Navigate(context.Background(), "file:///etc/passwd")

	require.Error(t, err)
	require.True(t, IsErrorCode(err, ErrNavigationBlocked))
}

func TestSessionExtractionRespectsBudgetAndMarksTruncation(t *testing.T) {
	budget := core.NewContextBudgetWithPolicy(8, &core.AllocationPolicy{
		SystemReserved:     0,
		Allocations:        map[string]float64{"immediate": 1.0},
		AllowBorrowing:     false,
		MinimumPerCategory: 0,
	})
	budget.SetReservations(0, 0, 0)
	session, err := NewSession(SessionConfig{
		Backend: &fakeBackend{
			html: "<html>" + "abcdefghijklmnopqrstuvwxyz" + "</html>",
		},
		BackendName: "fake",
		Budget:      budget,
	})
	require.NoError(t, err)

	extraction, err := session.ExtractHTML(context.Background())

	require.NoError(t, err)
	require.True(t, extraction.Truncated)
	require.Less(t, extraction.FinalTokens, extraction.OriginalTokens)
	require.NotEmpty(t, extraction.Content)
	require.LessOrEqual(t, extraction.FinalTokens, 8)
}

func TestSessionWrapsBackendErrors(t *testing.T) {
	backendErr := &Error{Code: ErrNoSuchElement, Err: errors.New("missing")}
	session, err := NewSession(SessionConfig{
		Backend: &errorBackend{err: backendErr},
	})
	require.NoError(t, err)

	_, err = session.GetText(context.Background(), "#missing")

	require.Error(t, err)
	require.True(t, IsErrorCode(err, ErrNoSuchElement))
}

type errorBackend struct {
	err error
}

func (e *errorBackend) Navigate(context.Context, string) error                      { return e.err }
func (e *errorBackend) Click(context.Context, string) error                         { return e.err }
func (e *errorBackend) Type(context.Context, string, string) error                  { return e.err }
func (e *errorBackend) GetText(context.Context, string) (string, error)             { return "", e.err }
func (e *errorBackend) GetAccessibilityTree(context.Context) (string, error)        { return "", e.err }
func (e *errorBackend) GetHTML(context.Context) (string, error)                     { return "", e.err }
func (e *errorBackend) ExecuteScript(context.Context, string) (any, error)          { return nil, e.err }
func (e *errorBackend) Screenshot(context.Context) ([]byte, error)                  { return nil, e.err }
func (e *errorBackend) WaitFor(context.Context, WaitCondition, time.Duration) error { return e.err }
func (e *errorBackend) CurrentURL(context.Context) (string, error)                  { return "", e.err }
func (e *errorBackend) Close() error                                                { return e.err }
