// Package testsupport provides shared test doubles for use across the
// Relurpify test suite.
package testsupport

import (
	"context"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/sandbox"
)

// FakeResponse defines a canned response for a command invocation.
// The first response whose MatchArgs matches (or the first with nil MatchArgs
// if no prefix matcher is set) wins. If no response matches, the fake returns
// empty output with no error.
type FakeResponse struct {
	// MatchArgs is an optional arg prefix matcher. When non-nil, this
	// response only applies to requests whose Args begin with these
	// elements. Example: MatchArgs: []string{"echo", "hello"} matches
	// Args: []string{"echo", "hello", "world"} but not Args: []string{"ls"}.
	MatchArgs []string
	Stdout    string
	Stderr    string
	Err       error
}

// FakeCommandRunner is a test double for sandbox.CommandRunner. It records
// every invocation in Calls and returns scripted responses.
type FakeCommandRunner struct {
	responses []FakeResponse
	Calls     []sandbox.CommandRequest
}

// FakeRunner returns a FakeCommandRunner pre-loaded with the given responses.
func FakeRunner(responses ...FakeResponse) *FakeCommandRunner {
	return &FakeCommandRunner{responses: responses}
}

// Run implements sandbox.CommandRunner. It records the request and returns
// the first matching response.
func (f *FakeCommandRunner) Run(_ context.Context, req sandbox.CommandRequest) (string, string, error) {
	if f == nil {
		return "", "", fmt.Errorf("FakeCommandRunner is nil")
	}
	f.Calls = append(f.Calls, req)
	for _, resp := range f.responses {
		if resp.MatchArgs != nil && !argsPrefixMatch(req.Args, resp.MatchArgs) {
			continue
		}
		return resp.Stdout, resp.Stderr, resp.Err
	}
	return "", "", nil
}

// LastCall returns the most recent recorded invocation, or nil if none.
func (f *FakeCommandRunner) LastCall() *sandbox.CommandRequest {
	if f == nil || len(f.Calls) == 0 {
		return nil
	}
	return &f.Calls[len(f.Calls)-1]
}

// CallCount returns the number of recorded invocations.
func (f *FakeCommandRunner) CallCount() int {
	if f == nil {
		return 0
	}
	return len(f.Calls)
}

// Reset clears all recorded invocations and responses.
func (f *FakeCommandRunner) Reset() {
	if f == nil {
		return
	}
	f.Calls = nil
}

// argsPrefixMatch returns true when got starts with want as a prefix.
func argsPrefixMatch(got, want []string) bool {
	if len(got) < len(want) {
		return false
	}
	for i := range want {
		if !strings.EqualFold(got[i], want[i]) {
			return false
		}
	}
	return true
}

var _ sandbox.CommandRunner = (*FakeCommandRunner)(nil)

// NewAuthorizedFakeRunner returns an *sandbox.AuthorizedRunner backed by a
// FakeCommandRunner with the given responses and policy. Tests that need a
// real AuthorizedRunner (e.g. when calling BuildBuiltinCapabilityBundle after
// Phase 2) can use this instead of a real sandbox backend.
func NewAuthorizedFakeRunner(policy sandbox.CommandPolicy, responses ...FakeResponse) (*sandbox.AuthorizedRunner, error) {
	fake := FakeRunner(responses...)
	return sandbox.NewAuthorizedRunner(fake, policy)
}

// PermitAllPolicy returns a CommandPolicy that allows all commands.
func PermitAllPolicy() sandbox.CommandPolicy {
	return sandbox.CommandPolicyFunc(func(_ context.Context, _ sandbox.CommandRequest) error {
		return nil
	})
}
