package browser

import (
	"context"
	"errors"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/platform/contracts"
	"github.com/stretchr/testify/require"
)

func TestSessionNavigateChecksNetworkPermissions(t *testing.T) {
	perms := &contracts.PermissionSet{
		Network: []contracts.NetworkPermission{
			{Direction: "egress", Protocol: "tcp", Host: "allowed.example", Port: 443},
		},
	}
	manager, err := authorization.NewPermissionManager("", perms, nil, nil)
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
	perms := &contracts.PermissionSet{
		Network: []contracts.NetworkPermission{
			{Direction: "egress", Protocol: "tcp", Host: "allowed.example", Port: 443},
		},
	}
	manager, err := authorization.NewPermissionManager("", perms, nil, nil)
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
	budget := &stubBudget{remaining: 8}
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

// stubBudget implements contracts.BudgetManager for testing
type stubBudget struct {
	remaining int
}

func (s *stubBudget) Allocate(category string, tokens int, item contracts.BudgetItem) error {
	if tokens > s.remaining {
		return errors.New("budget exhausted")
	}
	s.remaining -= tokens
	return nil
}

func (s *stubBudget) Free(category string, tokens int, itemID string) {}

func (s *stubBudget) GetRemainingBudget(category string) int { return s.remaining }

func (s *stubBudget) ShouldCompress() bool { return s.remaining < 10 }

func (s *stubBudget) CanAddTokens(tokens int) bool { return tokens <= s.remaining }

func (s *stubBudget) SetReservations(system, tools, output int) {}
