package command

import (
	"context"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/platform/contracts"
)

type recordingRunner struct{ called bool }

func (r *recordingRunner) Run(ctx context.Context, req contracts.CommandRequest) (*contracts.CommandResult, error) {
	r.called = true
	return &contracts.CommandResult{Stdout: "ok", StdoutBytes: 2}, nil
}

func newNetTool(r contracts.CommandRunner, tags ...string) *CommandTool {
	tool := NewCommandTool(".", CommandToolConfig{
		Name:       "cli_curl",
		Command:    "curl",
		AllowFlags: true,
		Tags:       tags,
	})
	tool.SetCommandRunner(r)
	return tool
}

func TestNetworkToolBlocksPrivateAndMetadataHosts(t *testing.T) {
	blocked := []string{
		"http://169.254.169.254/latest/meta-data/", // cloud metadata URL
		"https://127.0.0.1:6443/healthz",           // loopback service
		"http://[::1]/",                            // IPv6 loopback URL
		"10.0.0.5",                                 // bare RFC-1918 IP (nc/ping style)
		"192.168.1.1:8080",                         // host:port
	}
	for _, target := range blocked {
		r := &recordingRunner{}
		tool := newNetTool(r, contracts.TagExecute, contracts.TagNetwork)
		res, err := tool.Execute(context.Background(), map[string]any{"args": []any{target}})
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", target, err)
		}
		if res.Success {
			t.Fatalf("%s: expected egress to be denied", target)
		}
		if !strings.Contains(res.Error, "denied") {
			t.Fatalf("%s: unexpected error message: %s", target, res.Error)
		}
		if r.called {
			t.Fatalf("%s: runner must not execute for a blocked host", target)
		}
	}
}

func TestNetworkToolAllowsPublicHost(t *testing.T) {
	r := &recordingRunner{}
	tool := newNetTool(r, contracts.TagNetwork)
	res, err := tool.Execute(context.Background(), map[string]any{"args": []any{"https://8.8.8.8/"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected public egress to be allowed, got error: %s", res.Error)
	}
	if !r.called {
		t.Fatal("runner should execute for a public host")
	}
}

// A non-network tool must not be screened: an arg that happens to be a private
// IP (e.g. grepping for it in a file) is none of the egress guard's business.
func TestNonNetworkToolNotScreened(t *testing.T) {
	r := &recordingRunner{}
	tool := NewCommandTool(".", CommandToolConfig{
		Name:       "cli_rg",
		Command:    "rg",
		AllowFlags: true,
		Tags:       []string{contracts.TagExecute, contracts.TagReadOnly},
	})
	tool.SetCommandRunner(r)
	res, err := tool.Execute(context.Background(), map[string]any{"args": []any{"10.0.0.1"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success || !r.called {
		t.Fatalf("non-network tool must run unscreened (success=%v called=%v err=%s)", res.Success, r.called, res.Error)
	}
}
